// Package msoauth runs the desktop-app OAuth flow against Microsoft identity
// (consumers + work/school via the common tenant) and exposes /me/messages
// via Microsoft Graph.
//
// Flow (per https://learn.microsoft.com/azure/active-directory/develop/v2-oauth2-auth-code-flow):
//  1. Read client_id from ~/.miniclaw/ms_oauth_client.json
//  2. Spin loopback HTTP server on a free port
//  3. Open https://login.microsoftonline.com/common/oauth2/v2.0/authorize
//     with response_type=code and PKCE
//  4. Exchange code for access + refresh tokens at the v2.0/token endpoint
//  5. Refresh token stored in keychain under "ms_oauth:<email>"
package msoauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/oauth2"

	"github.com/mandloideep/miniclaw/internal/services/keychain"
)

// clientConfig is what the user drops at ~/.miniclaw/ms_oauth_client.json.
// Only client_id is required for a public/native app (no secret).
type clientConfig struct {
	ClientID string `json:"client_id"`
	// Tenant defaults to "common" (work + personal). Override to a tenant id
	// for single-tenant apps.
	Tenant string `json:"tenant,omitempty"`
}

// Scopes asked at consent. offline_access is what unlocks refresh tokens.
var Scopes = []string{
	"offline_access",
	"https://graph.microsoft.com/Mail.Read",
	"https://graph.microsoft.com/User.Read",
}

// LoadClientConfig reads ~/.miniclaw/ms_oauth_client.json.
func LoadClientConfig() (*oauth2.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, ".miniclaw", "ms_oauth_client.json")
	b, err := os.ReadFile(path) //nolint:gosec // fixed location under user home
	if err != nil {
		return nil, fmt.Errorf("read %s: %w (see SETUP.md §3)", path, err)
	}
	var cc clientConfig
	if err := json.Unmarshal(b, &cc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cc.ClientID == "" {
		return nil, fmt.Errorf("%s missing client_id", path)
	}
	tenant := cc.Tenant
	if tenant == "" {
		tenant = "common"
	}
	return &oauth2.Config{
		ClientID: cc.ClientID,
		Scopes:   Scopes,
		Endpoint: oauth2.Endpoint{
			AuthURL:  fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", tenant),
			TokenURL: fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenant),
		},
	}, nil
}

// AuthorizeResult is what Authorize returns.
type AuthorizeResult struct {
	EmailAddress string
	RefreshToken string
}

// Authorize runs the loopback PKCE flow.
func Authorize(ctx context.Context) (AuthorizeResult, error) {
	cfg, err := LoadClientConfig()
	if err != nil {
		return AuthorizeResult{}, err
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("loopback listen: %w", err)
	}
	defer func() { _ = lis.Close() }()
	port := lis.Addr().(*net.TCPAddr).Port
	cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	state := random32()
	verifier := random64()
	challenge := s256Challenge(verifier)

	authURL := cfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		oauth2.SetAuthURLParam("response_mode", "query"),
		oauth2.SetAuthURLParam("prompt", "consent"),
	)
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Open this URL to grant access: %s\n", authURL)
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- errors.New("oauth: state mismatch")
			http.Error(w, "state mismatch", http.StatusBadRequest)
			return
		}
		if e := r.URL.Query().Get("error"); e != "" {
			errCh <- fmt.Errorf("oauth provider error: %s", e)
			http.Error(w, e, http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- errors.New("oauth: empty code")
			http.Error(w, "missing code", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte("miniclaw connected to Microsoft. You can close this tab."))
		codeCh <- code
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = srv.Serve(lis) }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	var code string
	select {
	case <-ctx.Done():
		return AuthorizeResult{}, ctx.Err()
	case e := <-errCh:
		return AuthorizeResult{}, e
	case code = <-codeCh:
	}

	tok, err := cfg.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", verifier),
	)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("token exchange: %w", err)
	}
	if tok.RefreshToken == "" {
		return AuthorizeResult{}, errors.New("oauth: no refresh token (need offline_access scope)")
	}
	email, err := fetchUserEmail(ctx, cfg, tok)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("fetch userinfo: %w", err)
	}
	return AuthorizeResult{EmailAddress: email, RefreshToken: tok.RefreshToken}, nil
}

// TokenSource returns an auto-refreshing oauth2.TokenSource using the stored
// refresh token under secretRef.
func TokenSource(ctx context.Context, secretRef string) (oauth2.TokenSource, error) {
	cfg, err := LoadClientConfig()
	if err != nil {
		return nil, err
	}
	rt, err := keychain.Get(secretRef)
	if err != nil {
		return nil, fmt.Errorf("read refresh token: %w", err)
	}
	return cfg.TokenSource(ctx, &oauth2.Token{RefreshToken: rt}), nil
}

func fetchUserEmail(ctx context.Context, cfg *oauth2.Config, tok *oauth2.Token) (string, error) {
	client := cfg.Client(ctx, tok)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://graph.microsoft.com/v1.0/me?$select=mail,userPrincipalName", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Mail              string `json:"mail"`
		UserPrincipalName string `json:"userPrincipalName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.Mail != "" {
		return body.Mail, nil
	}
	return body.UserPrincipalName, nil
}

// Service is the Wails-registered facade.
type Service struct{}

// New returns a ready-to-register Service.
func New() *Service { return &Service{} }

// StartAuthorize kicks the consent flow. Returns the user's email + refresh
// token for the frontend onboarding to hand to AccountService.AddMSOAuth.
func (s *Service) StartAuthorize(ctx context.Context) (AuthorizeResult, error) {
	return Authorize(ctx)
}

// ClientConfigPath is where the user should drop their ms_oauth_client.json.
func (s *Service) ClientConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".miniclaw", "ms_oauth_client.json")
}

// HasClientConfig reports whether the JSON file is present and parseable.
func (s *Service) HasClientConfig() bool {
	_, err := LoadClientConfig()
	return err == nil
}

func random32() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func random64() string {
	b := make([]byte, 48)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func s256Challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	return exec.Command(cmd, args...).Start() //nolint:gosec // url is from our own oauth.Config
}

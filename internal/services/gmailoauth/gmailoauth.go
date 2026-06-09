// Package gmailoauth runs the desktop-app OAuth 2.0 loopback flow for Gmail,
// then exposes the user's mailbox via the Gmail REST API.
//
// Flow (per https://developers.google.com/identity/protocols/oauth2/native-app):
//  1. App reads client_id/client_secret JSON from ~/.miniclaw/google_oauth_client.json
//     (created by the user in Google Cloud Console — see SETUP.md §3).
//  2. App spins up a localhost HTTP server on a random free port.
//  3. App opens https://accounts.google.com/o/oauth2/v2/auth with redirect_uri
//     pointing at that localhost port.
//  4. User consents; Google redirects to the loopback with ?code=…
//  5. App exchanges the code for access + refresh tokens.
//  6. Refresh token is stored in the OS keychain under the account's secret_ref.
//  7. golang.org/x/oauth2.TokenSource takes over and auto-refreshes
//     access tokens from the stored refresh token.
//
// Sync uses the Gmail REST endpoint to list message IDs newer than the
// stored watermark and pull full message bodies. The IMAPSyncer interface
// is mirrored so the scheduler can drive both auth kinds with the same
// SyncFn signature.
package gmailoauth

import (
	"context"
	"crypto/rand"
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
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/mandloideep/miniclaw/internal/services/keychain"
)

// clientConfig is the shape of the JSON downloaded from Google Cloud Console.
type clientConfig struct {
	Installed struct {
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		AuthURI      string   `json:"auth_uri"`
		TokenURI     string   `json:"token_uri"`
		RedirectURIs []string `json:"redirect_uris"`
	} `json:"installed"`
}

// Scopes asked for during consent. gmail.readonly is enough for the
// summarisation/digest use case; widen later when reply-from-app lands.
var Scopes = []string{
	"https://www.googleapis.com/auth/gmail.readonly",
	"https://www.googleapis.com/auth/userinfo.email",
}

// LoadClientConfig reads the OAuth client JSON from disk.
func LoadClientConfig() (*oauth2.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	path := filepath.Join(home, ".miniclaw", "google_oauth_client.json")
	b, err := os.ReadFile(path) //nolint:gosec // intentional fixed location under user home
	if err != nil {
		return nil, fmt.Errorf("read %s: %w (see SETUP.md §3)", path, err)
	}
	var cc clientConfig
	if err := json.Unmarshal(b, &cc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if cc.Installed.ClientID == "" {
		return nil, fmt.Errorf("%s missing installed.client_id", path)
	}
	return &oauth2.Config{
		ClientID:     cc.Installed.ClientID,
		ClientSecret: cc.Installed.ClientSecret,
		Scopes:       Scopes,
		Endpoint: oauth2.Endpoint{ //nolint:gosec // public oauth endpoints, not credentials
			AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL: "https://oauth2.googleapis.com/token",
		},
	}, nil
}

// AuthorizeResult is what Authorize returns to the caller.
type AuthorizeResult struct {
	EmailAddress string
	RefreshToken string // store in keychain
}

// Authorize runs the loopback consent flow and returns the user's email
// plus a refresh token. Blocks until the user finishes (or ctx expires).
func Authorize(ctx context.Context) (AuthorizeResult, error) {
	cfg, err := LoadClientConfig()
	if err != nil {
		return AuthorizeResult{}, err
	}
	// 1. Bind a free loopback port.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("loopback listen: %w", err)
	}
	defer func() { _ = lis.Close() }()
	port := lis.Addr().(*net.TCPAddr).Port
	cfg.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	state := randomState()
	authURL := cfg.AuthCodeURL(state,
		oauth2.AccessTypeOffline, // refresh-token grant
		oauth2.ApprovalForce,     // force refresh-token emission on re-consent
	)

	// 2. Open the browser. If that fails we'll print the URL and let the
	// user copy-paste it themselves.
	if err := openBrowser(authURL); err != nil {
		fmt.Printf("Open this URL to grant access: %s\n", authURL)
	}

	// 3. Wait for the callback.
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			errCh <- errors.New("oauth: state mismatch (possible CSRF)")
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
		_, _ = w.Write([]byte("miniclaw connected. You can close this tab."))
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

	// 4. Exchange the code for tokens.
	tok, err := cfg.Exchange(ctx, code)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("token exchange: %w", err)
	}
	if tok.RefreshToken == "" {
		return AuthorizeResult{}, errors.New("oauth: no refresh token (re-grant with prompt=consent)")
	}

	// 5. Get the user's email.
	email, err := fetchUserEmail(ctx, cfg, tok)
	if err != nil {
		return AuthorizeResult{}, fmt.Errorf("fetch userinfo: %w", err)
	}
	return AuthorizeResult{EmailAddress: email, RefreshToken: tok.RefreshToken}, nil
}

// TokenSource returns an oauth2.TokenSource that reads the refresh token
// from the keychain (under the account's secret_ref) and auto-refreshes.
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
		"https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Email, nil
}

func randomState() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
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
	return exec.Command(cmd, args...).Start() //nolint:gosec // url is built from our own oauth.Config
}

// Service is the Wails-registered facade.
type Service struct{}

// New returns a ready-to-register service.
func New() *Service { return &Service{} }

// StartAuthorize kicks the consent flow. Returns the email + a token that
// the account service should store in the keychain via its add-account path.
//
// The frontend onboarding screen calls this, then calls
// AccountService.AddGmailOAuth (TBD) with the email + refresh token.
func (s *Service) StartAuthorize(ctx context.Context) (AuthorizeResult, error) {
	return Authorize(ctx)
}

// ClientConfigPath is where the user should drop their downloaded OAuth JSON.
func (s *Service) ClientConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".miniclaw", "google_oauth_client.json")
}

// HasClientConfig reports whether the JSON file is present and parseable.
func (s *Service) HasClientConfig() bool {
	_, err := LoadClientConfig()
	return err == nil
}

// helper used elsewhere
var _ = strings.HasPrefix

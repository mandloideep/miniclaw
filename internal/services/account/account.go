// Package account exposes account CRUD to the frontend. An account is a
// single email mailbox the user has connected — IMAP/SMTP or Gmail OAuth.
//
// Secrets (IMAP password, OAuth refresh token) never sit in the DB. They go
// through internal/services/keychain, and the DB row only stores a stable
// secret_ref that the keychain knows by.
package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
	"github.com/mandloideep/miniclaw/internal/services/keychain"
)

// AuthKind enumerates supported authentication strategies.
type AuthKind string

// Supported authentication strategies. Values match the schema's CHECK
// constraint on accounts.auth_kind.
const (
	AuthIMAP       AuthKind = "imap"
	AuthGmailOAuth AuthKind = "gmail_oauth"
)

// Account is the frontend-facing shape. We never expose secret material here.
type Account struct {
	ID              int64    `json:"id"`
	WorkspaceID     int64    `json:"workspaceId"`
	DisplayName     string   `json:"displayName"`
	EmailAddress    string   `json:"emailAddress"`
	AuthKind        AuthKind `json:"authKind"`
	IMAPHost        string   `json:"imapHost,omitempty"`
	IMAPPort        int64    `json:"imapPort,omitempty"`
	SMTPHost        string   `json:"smtpHost,omitempty"`
	SMTPPort        int64    `json:"smtpPort,omitempty"`
	FetchSince      string   `json:"fetchSince,omitempty"` // ISO date, "" means all
	SyncCadenceSecs int64    `json:"syncCadenceSecs"`
	LastSyncedAt    string   `json:"lastSyncedAt,omitempty"`
	OllamaModel     string   `json:"ollamaModel"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

// IMAPCreds is what the frontend submits when adding an IMAP account.
type IMAPCreds struct {
	WorkspaceID  int64  `json:"workspaceId"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	IMAPHost     string `json:"imapHost"`
	IMAPPort     int64  `json:"imapPort"`
	SMTPHost     string `json:"smtpHost"`
	SMTPPort     int64  `json:"smtpPort"`
	Password     string `json:"password"` // never persisted; routed to keychain
	FetchSince   string `json:"fetchSince,omitempty"`
	OllamaModel  string `json:"ollamaModel,omitempty"`
}

// Service is the Wails-registered service for accounts.
type Service struct {
	q *sqlcgen.Queries
}

// New takes the shared DB pool and returns a service.
func New(pool *sql.DB) *Service { return &Service{q: sqlcgen.New(pool)} }

// List returns every account in the store.
func (s *Service) List(ctx context.Context) ([]Account, error) {
	rows, err := s.q.ListAccounts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	out := make([]Account, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAccount(r))
	}
	return out, nil
}

// ListByWorkspace returns accounts in a single workspace.
func (s *Service) ListByWorkspace(ctx context.Context, workspaceID int64) ([]Account, error) {
	rows, err := s.q.ListAccountsByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list accounts for workspace %d: %w", workspaceID, err)
	}
	out := make([]Account, 0, len(rows))
	for _, r := range rows {
		out = append(out, toAccount(r))
	}
	return out, nil
}

// Get returns one account by ID.
func (s *Service) Get(ctx context.Context, id int64) (Account, error) {
	r, err := s.q.GetAccount(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, fmt.Errorf("account %d: not found", id)
	}
	if err != nil {
		return Account{}, fmt.Errorf("get account %d: %w", id, err)
	}
	return toAccount(r), nil
}

// AddIMAP creates an IMAP/SMTP account. The password is sent to the keychain
// under a stable ref ("imap:<email>") and the DB row records only the ref.
func (s *Service) AddIMAP(ctx context.Context, c IMAPCreds) (Account, error) {
	if err := validateIMAP(c); err != nil {
		return Account{}, err
	}
	ref := secretRef(AuthIMAP, c.EmailAddress)
	if err := keychain.Set(ref, c.Password); err != nil {
		return Account{}, fmt.Errorf("store password in keychain: %w", err)
	}

	r, err := s.q.CreateIMAPAccount(ctx, sqlcgen.CreateIMAPAccountParams{
		WorkspaceID:     c.WorkspaceID,
		DisplayName:     c.DisplayName,
		EmailAddress:    c.EmailAddress,
		ImapHost:        sql.NullString{String: c.IMAPHost, Valid: c.IMAPHost != ""},
		ImapPort:        sql.NullInt64{Int64: c.IMAPPort, Valid: c.IMAPPort != 0},
		SmtpHost:        sql.NullString{String: c.SMTPHost, Valid: c.SMTPHost != ""},
		SmtpPort:        sql.NullInt64{Int64: c.SMTPPort, Valid: c.SMTPPort != 0},
		SecretRef:       ref,
		FetchSince:      sql.NullString{String: c.FetchSince, Valid: c.FetchSince != ""},
		SyncCadenceSecs: 300, // 5 min default; user can change later
		OllamaModel:     c.OllamaModel,
	})
	if err != nil {
		// Roll back the keychain write so we don't leak a secret with no DB row.
		_ = keychain.Delete(ref)
		return Account{}, fmt.Errorf("create account: %w", err)
	}
	return toAccount(r), nil
}

// OAuthCreds is what the onboarding hands over after a successful Gmail
// OAuth consent flow.
type OAuthCreds struct {
	WorkspaceID  int64  `json:"workspaceId"`
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	RefreshToken string `json:"refreshToken"`
	FetchSince   string `json:"fetchSince,omitempty"`
	OllamaModel  string `json:"ollamaModel,omitempty"`
}

// AddGmailOAuth persists a Gmail-OAuth account. The refresh token goes to
// the keychain under ref "gmail_oauth:<email>" and only the ref hits the DB.
func (s *Service) AddGmailOAuth(ctx context.Context, c OAuthCreds) (Account, error) {
	switch {
	case c.WorkspaceID == 0:
		return Account{}, errors.New("workspace is required")
	case c.EmailAddress == "":
		return Account{}, errors.New("email address is required")
	case c.RefreshToken == "":
		return Account{}, errors.New("refresh token is required")
	}
	ref := secretRef(AuthGmailOAuth, c.EmailAddress)
	if err := keychain.Set(ref, c.RefreshToken); err != nil {
		return Account{}, fmt.Errorf("store refresh token: %w", err)
	}
	r, err := s.q.CreateOAuthAccount(ctx, sqlcgen.CreateOAuthAccountParams{
		WorkspaceID:     c.WorkspaceID,
		DisplayName:     c.DisplayName,
		EmailAddress:    c.EmailAddress,
		SecretRef:       ref,
		FetchSince:      sql.NullString{String: c.FetchSince, Valid: c.FetchSince != ""},
		SyncCadenceSecs: 300,
		OllamaModel:     c.OllamaModel,
	})
	if err != nil {
		_ = keychain.Delete(ref)
		return Account{}, fmt.Errorf("create account: %w", err)
	}
	return toAccount(r), nil
}

// Delete removes the account row and its associated keychain secret.
func (s *Service) Delete(ctx context.Context, id int64) error {
	row, err := s.q.GetAccount(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // already gone
	}
	if err != nil {
		return fmt.Errorf("locate account %d: %w", id, err)
	}
	if err := s.q.DeleteAccount(ctx, id); err != nil {
		return fmt.Errorf("delete account %d: %w", id, err)
	}
	// Best-effort: orphaned keychain entries aren't dangerous but they're
	// noise. Don't fail the whole call if cleanup misses.
	_ = keychain.Delete(row.SecretRef)
	return nil
}

// Password returns the plaintext IMAP password for use by the ingest service.
// Intentionally not exposed as a Wails-bound method — internal use only.
func (s *Service) Password(ctx context.Context, id int64) (string, error) {
	row, err := s.q.GetAccount(ctx, id)
	if err != nil {
		return "", fmt.Errorf("get account %d: %w", id, err)
	}
	return keychain.Get(row.SecretRef)
}

// SetCadence updates how often the scheduler kicks ingest for this account.
func (s *Service) SetCadence(ctx context.Context, id, secs int64) error {
	if secs < 30 {
		return errors.New("cadence must be at least 30s")
	}
	return s.q.UpdateAccountCadence(ctx, sqlcgen.UpdateAccountCadenceParams{
		ID: id, SyncCadenceSecs: secs,
	})
}

// SetModel changes the per-account Ollama model used for summaries.
func (s *Service) SetModel(ctx context.Context, id int64, model string) error {
	return s.q.UpdateAccountModel(ctx, sqlcgen.UpdateAccountModelParams{
		ID: id, OllamaModel: model,
	})
}

// MarkSynced stamps last_synced_at = now. Called by the scheduler after a
// successful pass.
func (s *Service) MarkSynced(ctx context.Context, id int64) error {
	return s.q.UpdateAccountSync(ctx, id)
}

func validateIMAP(c IMAPCreds) error {
	switch {
	case c.WorkspaceID == 0:
		return errors.New("workspace is required")
	case c.EmailAddress == "":
		return errors.New("email address is required")
	case c.IMAPHost == "" || c.IMAPPort == 0:
		return errors.New("IMAP host and port are required")
	case c.Password == "":
		return errors.New("password is required")
	}
	return nil
}

// secretRef builds the stable keychain key for an account's secret.
// Format: "<auth-kind>:<email>". Stable across renames because we key on
// email, not display name.
func secretRef(kind AuthKind, email string) string {
	return string(kind) + ":" + email
}

func toAccount(r sqlcgen.Account) Account {
	return Account{
		ID:              r.ID,
		WorkspaceID:     r.WorkspaceID,
		DisplayName:     r.DisplayName,
		EmailAddress:    r.EmailAddress,
		AuthKind:        AuthKind(r.AuthKind),
		IMAPHost:        r.ImapHost.String,
		IMAPPort:        r.ImapPort.Int64,
		SMTPHost:        r.SmtpHost.String,
		SMTPPort:        r.SmtpPort.Int64,
		FetchSince:      r.FetchSince.String,
		SyncCadenceSecs: r.SyncCadenceSecs,
		LastSyncedAt:    r.LastSyncedAt.String,
		OllamaModel:     r.OllamaModel,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
	}
}

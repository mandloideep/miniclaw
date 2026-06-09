// Package telegram wraps the Telegram Bot API for outbound notifications.
//
// Only sendMessage is used today. The bot token lives in telegram_settings
// (single-row config). Recipients are stored in telegram_recipients and
// assigned to workspaces and/or accounts per docs/decisions.md §5.
//
// Docs: https://core.telegram.org/bots/api#sendmessage
package telegram

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
)

// Recipient is the frontend-facing shape.
type Recipient struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	ChatID    string `json:"chatId"`
	CreatedAt string `json:"createdAt,omitempty"`
}

// Settings is the frontend-facing snapshot of telegram_settings.
type Settings struct {
	BotToken   string `json:"botToken"`
	DigestTime string `json:"digestTime"` // HH:MM local
}

// Service is the Wails-registered telegram service.
type Service struct {
	q      *sqlcgen.Queries
	client *http.Client
	// baseURL is "https://api.telegram.org" in prod; tests inject a fake.
	baseURL string
}

// New wires the service to the shared pool.
func New(pool *sql.DB) *Service {
	return &Service{
		q:       sqlcgen.New(pool),
		client:  &http.Client{Timeout: 15 * time.Second},
		baseURL: "https://api.telegram.org",
	}
}

// GetSettings returns the saved bot token and digest time.
func (s *Service) GetSettings(ctx context.Context) (Settings, error) {
	r, err := s.q.GetTelegramSettings(ctx)
	if err != nil {
		return Settings{}, fmt.Errorf("get telegram settings: %w", err)
	}
	return Settings{BotToken: r.BotToken, DigestTime: r.DigestTime}, nil
}

// SetBotToken updates the bot token used for outbound messages.
func (s *Service) SetBotToken(ctx context.Context, token string) error {
	return s.q.SetTelegramBotToken(ctx, token)
}

// SetDigestTime updates the HH:MM time the daily digest job fires.
func (s *Service) SetDigestTime(ctx context.Context, hhmm string) error {
	if !validHHMM(hhmm) {
		return errors.New("digest time must be HH:MM")
	}
	return s.q.SetDigestTime(ctx, hhmm)
}

// ListRecipients returns every recipient.
func (s *Service) ListRecipients(ctx context.Context) ([]Recipient, error) {
	rows, err := s.q.ListRecipients(ctx)
	if err != nil {
		return nil, fmt.Errorf("list recipients: %w", err)
	}
	out := make([]Recipient, 0, len(rows))
	for _, r := range rows {
		out = append(out, Recipient{ID: r.ID, Name: r.Name, ChatID: r.ChatID, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// AddRecipient creates a new recipient.
func (s *Service) AddRecipient(ctx context.Context, name, chatID string) (Recipient, error) {
	if name == "" || chatID == "" {
		return Recipient{}, errors.New("name and chatID are required")
	}
	r, err := s.q.CreateRecipient(ctx, sqlcgen.CreateRecipientParams{Name: name, ChatID: chatID})
	if err != nil {
		return Recipient{}, fmt.Errorf("create recipient: %w", err)
	}
	return Recipient{ID: r.ID, Name: r.Name, ChatID: r.ChatID, CreatedAt: r.CreatedAt}, nil
}

// DeleteRecipient removes a recipient. Workspace/account assignments cascade.
func (s *Service) DeleteRecipient(ctx context.Context, id int64) error {
	return s.q.DeleteRecipient(ctx, id)
}

// AssignToWorkspace links a recipient to a workspace (idempotent).
func (s *Service) AssignToWorkspace(ctx context.Context, workspaceID, recipientID int64) error {
	return s.q.AssignRecipientToWorkspace(ctx, sqlcgen.AssignRecipientToWorkspaceParams{
		WorkspaceID: workspaceID, RecipientID: recipientID,
	})
}

// UnassignFromWorkspace removes a workspace link.
func (s *Service) UnassignFromWorkspace(ctx context.Context, workspaceID, recipientID int64) error {
	return s.q.UnassignRecipientFromWorkspace(ctx, sqlcgen.UnassignRecipientFromWorkspaceParams{
		WorkspaceID: workspaceID, RecipientID: recipientID,
	})
}

// AssignToAccount links a recipient as a per-account override.
func (s *Service) AssignToAccount(ctx context.Context, accountID, recipientID int64) error {
	return s.q.AssignRecipientToAccount(ctx, sqlcgen.AssignRecipientToAccountParams{
		AccountID: accountID, RecipientID: recipientID,
	})
}

// UnassignFromAccount removes the per-account link.
func (s *Service) UnassignFromAccount(ctx context.Context, accountID, recipientID int64) error {
	return s.q.UnassignRecipientFromAccount(ctx, sqlcgen.UnassignRecipientFromAccountParams{
		AccountID: accountID, RecipientID: recipientID,
	})
}

// RecipientsForAccount returns the fan-out set (workspace + account union).
// Used by daily-digest and ping flows.
func (s *Service) RecipientsForAccount(ctx context.Context, accountID int64) ([]Recipient, error) {
	rows, err := s.q.ListRecipientsForFanout(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("fanout for account %d: %w", accountID, err)
	}
	out := make([]Recipient, 0, len(rows))
	for _, r := range rows {
		out = append(out, Recipient{ID: r.ID, Name: r.Name, ChatID: r.ChatID})
	}
	return out, nil
}

// SendTo posts text to a single chat. Returns a non-nil error if Telegram
// responds with ok:false or transport fails.
func (s *Service) SendTo(ctx context.Context, chatID, text string) error {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return err
	}
	if settings.BotToken == "" {
		return errors.New("telegram bot token not configured")
	}
	body, _ := json.Marshal(map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	})
	url := fmt.Sprintf("%s/bot%s/sendMessage", s.baseURL, settings.BotToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build sendMessage: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram sendMessage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("decode sendMessage: %w", err)
	}
	if !out.OK {
		return fmt.Errorf("telegram: %s", out.Description)
	}
	return nil
}

// NotifySnoozeWake fires a short ping to every recipient assigned to the
// workspace, telling them a snoozed message is back. The snooze ticker
// calls this only for needs_attention rows, so cold newsletters don't
// generate noise on wake.
func (s *Service) NotifySnoozeWake(ctx context.Context, workspaceID, emailID int64, subject string) error {
	if subject == "" {
		subject = "(no subject)"
	}
	rows, err := s.q.ListRecipientsForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("workspace recipients: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	text := fmt.Sprintf("⏰ Back from snooze: *%s* (id %d)", escapeMarkdown(subject), emailID)
	var firstErr error
	for _, r := range rows {
		if err := s.SendTo(ctx, r.ChatID, text); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// escapeMarkdown escapes the small subset of characters Telegram's legacy
// Markdown mode treats as syntax. Conservative — overescaping looks fine.
func escapeMarkdown(s string) string {
	r := strings.NewReplacer("_", "\\_", "*", "\\*", "[", "\\[", "`", "\\`")
	return r.Replace(s)
}

// Broadcast sends text to every chat in chatIDs, returning the first error
// (if any) but always attempting every recipient — one bad chat ID shouldn't
// silently swallow notifications to the others.
func (s *Service) Broadcast(ctx context.Context, chatIDs []string, text string) error {
	var firstErr error
	for _, id := range chatIDs {
		if err := s.SendTo(ctx, id, text); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func validHHMM(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	hh, mm := s[:2], s[3:]
	for _, r := range hh + mm {
		if r < '0' || r > '9' {
			return false
		}
	}
	h := (int(hh[0]-'0'))*10 + int(hh[1]-'0')
	m := (int(mm[0]-'0'))*10 + int(mm[1]-'0')
	return h < 24 && m < 60
}

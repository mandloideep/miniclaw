package summary

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/mandloideep/miniclaw/internal/db"
	"github.com/mandloideep/miniclaw/internal/services/account"
	"github.com/mandloideep/miniclaw/internal/services/ollama"
)

// fakeLLM lets us drive Summarize end-to-end without real Ollama.
type fakeLLM struct {
	resp string
	err  error
}

func (f fakeLLM) Generate(_ context.Context, _ ollama.GenerateRequest) (string, error) {
	return f.resp, f.err
}

func TestSummarize_WritesSummaryAndAttention(t *testing.T) {
	keyring.MockInit()
	pool, err := db.Open(context.Background(), db.Config{Path: filepath.Join(t.TempDir(), "s.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	accSvc := account.New(pool)
	acc, err := accSvc.AddIMAP(context.Background(), account.IMAPCreds{
		WorkspaceID: 1, EmailAddress: "a@x.test",
		IMAPHost: "h", IMAPPort: 993, SMTPHost: "h", SMTPPort: 465,
		Password: "p", OllamaModel: "llama3.2:3b",
	})
	if err != nil {
		t.Fatalf("AddIMAP: %v", err)
	}

	// Seed one email directly.
	_, err = pool.Exec(`INSERT INTO emails
		(account_id, message_id, folder, from_address, subject, received_at, body_plain)
		VALUES (?, ?, 'INBOX', 'b@x.test', 'do this thing', '2026-01-01T00:00:00Z', 'please reply')`,
		acc.ID, "msg-1")
	if err != nil {
		t.Fatalf("seed email: %v", err)
	}

	s := New(pool, fakeLLM{
		resp: `{"summary":"asks for reply","needs_attention":true,"reason":"asks for reply"}`,
	})
	n, err := s.Summarize(context.Background(), acc.ID)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 summary written, got %d", n)
	}

	// Re-running is a no-op (no unsummarized rows left).
	n, err = s.Summarize(context.Background(), acc.ID)
	if err != nil {
		t.Fatalf("Summarize idempotent: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 on second pass, got %d", n)
	}
}

func TestSummarize_BadJSONIsSkipped(t *testing.T) {
	keyring.MockInit()
	pool, err := db.Open(context.Background(), db.Config{Path: filepath.Join(t.TempDir(), "s.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	accSvc := account.New(pool)
	acc, _ := accSvc.AddIMAP(context.Background(), account.IMAPCreds{
		WorkspaceID: 1, EmailAddress: "a@x.test",
		IMAPHost: "h", IMAPPort: 993, SMTPHost: "h", SMTPPort: 465,
		Password: "p", OllamaModel: "m",
	})
	_, _ = pool.Exec(`INSERT INTO emails
		(account_id, message_id, folder, from_address, subject, received_at, body_plain)
		VALUES (?, 'm-1', 'INBOX', 'x@x.test', 'x', '2026-01-01T00:00:00Z', 'x')`, acc.ID)

	s := New(pool, fakeLLM{resp: "not json"})
	n, _ := s.Summarize(context.Background(), acc.ID)
	if n != 0 {
		t.Fatalf("want 0 written on bad json, got %d", n)
	}
}

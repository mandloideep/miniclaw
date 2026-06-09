package triage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/mandloideep/miniclaw/internal/db"
	"github.com/mandloideep/miniclaw/internal/services/account"
)

func seed(t *testing.T) (*Service, *sql.DB, int64) {
	t.Helper()
	keyring.MockInit()
	pool, err := db.Open(context.Background(), db.Config{Path: filepath.Join(t.TempDir(), "tri.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	accSvc := account.New(pool)
	acc, err := accSvc.AddIMAP(context.Background(), account.IMAPCreds{
		WorkspaceID: 1, EmailAddress: "me@x.test",
		IMAPHost: "h", IMAPPort: 993, SMTPHost: "h", SMTPPort: 465, Password: "p",
	})
	if err != nil {
		t.Fatalf("AddIMAP: %v", err)
	}
	return New(pool), pool, acc.ID
}

func TestFilterRulesAndBlockMatch(t *testing.T) {
	s, _, accID := seed(t)
	ctx := context.Background()

	_, err := s.AddFilterRule(ctx, accID, "block", "spam.example.com", "marketing")
	if err != nil {
		t.Fatalf("AddFilterRule: %v", err)
	}

	hit, err := s.MatchesBlock(ctx, accID, "no-reply@spam.example.com")
	if err != nil {
		t.Fatalf("MatchesBlock: %v", err)
	}
	if !hit {
		t.Fatal("expected domain block match")
	}

	hit, _ = s.MatchesBlock(ctx, accID, "ok@other.test")
	if hit {
		t.Fatal("unexpected match for unrelated domain")
	}
}

func TestScreenerLifecycle(t *testing.T) {
	s, pool, accID := seed(t)
	ctx := context.Background()

	if err := s.RegisterSender(ctx, accID, "stranger@new.test"); err != nil {
		t.Fatalf("RegisterSender: %v", err)
	}
	// Idempotent on repeat.
	_ = s.RegisterSender(ctx, accID, "stranger@new.test")

	// Need at least one email row to satisfy the screener listing's JOIN.
	_, _ = pool.Exec(`INSERT INTO emails (account_id, message_id, folder, from_address, subject, received_at)
		VALUES (?, 'm-1', 'INBOX', 'stranger@new.test', 's', '2026-01-01T00:00:00Z')`, accID)

	rows, err := s.ListUnscreened(ctx, accID)
	if err != nil {
		t.Fatalf("ListUnscreened: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("want 1 entry, got %d", len(rows))
	}
	if err := s.ApproveSender(ctx, rows[0].SenderID); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListUnscreened(ctx, accID)
	if len(rows) != 0 {
		t.Fatalf("want 0 after approve, got %d", len(rows))
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		addr, pat string
		want      bool
	}{
		{"a@b.test", "a@b.test", true},
		{"a@b.test", "b.test", true},
		{"a@b.test", "@b.test", true},
		{"a@b.test", "c.test", false},
	}
	for _, tc := range tests {
		if got := matchesPattern(tc.addr, tc.pat); got != tc.want {
			t.Errorf("matchesPattern(%q, %q)=%v, want %v", tc.addr, tc.pat, got, tc.want)
		}
	}
}

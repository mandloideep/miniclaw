package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/mandloideep/miniclaw/internal/db"
	"github.com/mandloideep/miniclaw/internal/services/account"
)

func newService(t *testing.T) (*Service, *account.Service) {
	t.Helper()
	keyring.MockInit()
	pool, err := db.Open(context.Background(), db.Config{Path: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return New(pool), account.New(pool)
}

func TestRecipientsAndFanout(t *testing.T) {
	s, acc := newService(t)
	ctx := context.Background()

	a, err := acc.AddIMAP(ctx, account.IMAPCreds{
		WorkspaceID: 1, EmailAddress: "x@x.test",
		IMAPHost: "h", IMAPPort: 993, SMTPHost: "h", SMTPPort: 465, Password: "p",
	})
	if err != nil {
		t.Fatalf("AddIMAP: %v", err)
	}

	r1, err := s.AddRecipient(ctx, "Mom", "111")
	if err != nil {
		t.Fatalf("AddRecipient: %v", err)
	}
	r2, _ := s.AddRecipient(ctx, "Dad", "222")
	r3, _ := s.AddRecipient(ctx, "Sis", "333")

	// Workspace fan-out (Family / id 1)
	if err := s.AssignToWorkspace(ctx, 1, r1.ID); err != nil {
		t.Fatalf("AssignToWorkspace: %v", err)
	}
	if err := s.AssignToWorkspace(ctx, 1, r2.ID); err != nil {
		t.Fatal(err)
	}
	// Per-account override (sister gets this one account too)
	if err := s.AssignToAccount(ctx, a.ID, r3.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.RecipientsForAccount(ctx, a.ID)
	if err != nil {
		t.Fatalf("RecipientsForAccount: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 recipients in union, got %d (%+v)", len(got), got)
	}
}

func TestSendTo_HitsTelegramAPI(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	if err := s.SetBotToken(ctx, "TESTTOKEN"); err != nil {
		t.Fatalf("SetBotToken: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/botTESTTOKEN/") {
			t.Fatalf("bad path: %s", r.URL.Path)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["chat_id"] != "42" || body["text"] != "hi" {
			t.Fatalf("bad body: %+v", body)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	s.baseURL = srv.URL

	if err := s.SendTo(ctx, "42", "hi"); err != nil {
		t.Fatalf("SendTo: %v", err)
	}
}

func TestValidHHMM(t *testing.T) {
	good := []string{"00:00", "23:59", "08:00"}
	bad := []string{"24:00", "00:60", "8:00", "08-00", ""}
	for _, g := range good {
		if !validHHMM(g) {
			t.Fatalf("want valid: %q", g)
		}
	}
	for _, b := range bad {
		if validHHMM(b) {
			t.Fatalf("want invalid: %q", b)
		}
	}
}

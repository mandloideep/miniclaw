package account

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/mandloideep/miniclaw/internal/db"
)

func TestAddIMAPAndReadBack(t *testing.T) {
	keyring.MockInit()
	pool, err := db.Open(context.Background(), db.Config{Path: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	s := New(pool)
	got, err := s.AddIMAP(context.Background(), IMAPCreds{
		WorkspaceID:  1, // Family seed
		DisplayName:  "Me",
		EmailAddress: "me@example.test",
		IMAPHost:     "imap.example.test",
		IMAPPort:     993,
		SMTPHost:     "smtp.example.test",
		SMTPPort:     465,
		Password:     "hunter2",
		OllamaModel:  "llama3.2:3b",
	})
	if err != nil {
		t.Fatalf("AddIMAP: %v", err)
	}
	if got.ID == 0 || got.EmailAddress != "me@example.test" {
		t.Fatalf("unexpected account: %+v", got)
	}

	// Password landed in the keychain under the derived ref.
	pwd, err := s.Password(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("Password: %v", err)
	}
	if pwd != "hunter2" {
		t.Fatalf("password roundtrip: got %q", pwd)
	}

	// Delete cleans up keychain too.
	if err := s.Delete(context.Background(), got.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Password(context.Background(), got.ID); err == nil {
		t.Fatal("expected error reading password for deleted account")
	}
}

func TestAddIMAP_Validation(t *testing.T) {
	keyring.MockInit()
	pool, err := db.Open(context.Background(), db.Config{Path: filepath.Join(t.TempDir(), "t.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	s := New(pool)
	if _, err := s.AddIMAP(context.Background(), IMAPCreds{}); err == nil {
		t.Fatal("expected validation error on empty IMAPCreds")
	}
}

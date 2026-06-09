package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestOpen_AppliesMigrationsAndSeedsWorkspaces(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	pool, err := Open(context.Background(), Config{Path: dbPath})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	// Migration applied — workspaces table exists with the 4 seeded rows.
	var count int
	if err := pool.QueryRow("SELECT COUNT(*) FROM workspaces").Scan(&count); err != nil {
		t.Fatalf("count workspaces: %v", err)
	}
	if count != 4 {
		t.Fatalf("want 4 seeded workspaces, got %d", count)
	}

	// FTS5 virtual table is reachable.
	if _, err := pool.Exec("INSERT INTO emails_fts(emails_fts) VALUES ('integrity-check')"); err != nil {
		t.Fatalf("fts integrity-check: %v", err)
	}

	// Re-opening is idempotent (migrations are no-op on a fully-applied DB).
	pool2, err := Open(context.Background(), Config{Path: dbPath})
	if err != nil {
		t.Fatalf("re-open: %v", err)
	}
	_ = pool2.Close()
}

func TestOpen_ForeignKeysEnforced(t *testing.T) {
	pool, err := Open(context.Background(), Config{Path: filepath.Join(t.TempDir(), "fk.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	// Inserting an account with a bogus workspace_id should fail under FK enforcement.
	_, err = pool.Exec(`INSERT INTO accounts
		(workspace_id, display_name, email_address, auth_kind, secret_ref)
		VALUES (999, 'x', 'a@b.test', 'imap', 'k')`)
	if err == nil {
		t.Fatal("expected FK violation, got nil")
	}
}

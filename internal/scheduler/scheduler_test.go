package scheduler

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/mandloideep/miniclaw/internal/db"
	"github.com/mandloideep/miniclaw/internal/services/account"
)

func TestScheduler_RunsForEachAccount(t *testing.T) {
	keyring.MockInit()
	pool, err := db.Open(context.Background(), db.Config{Path: filepath.Join(t.TempDir(), "s.db")})
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	accSvc := account.New(pool)
	_, err = accSvc.AddIMAP(context.Background(), account.IMAPCreds{
		WorkspaceID: 1, EmailAddress: "a@x.test", IMAPHost: "h", IMAPPort: 993,
		SMTPHost: "h", SMTPPort: 465, Password: "p",
	})
	if err != nil {
		t.Fatalf("AddIMAP: %v", err)
	}

	var calls atomic.Int64
	s := New(accSvc, func(_ context.Context, _ int64) error {
		calls.Add(1)
		return nil
	})
	s.supervisorPeriod = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	go s.Start(ctx)

	// Wait long enough for the initial immediate-run on each account loop.
	time.Sleep(200 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond)

	if calls.Load() == 0 {
		t.Fatal("sync was never called")
	}
}

// Package scheduler runs per-account ingest + summarization on a ticker.
//
// One goroutine per account. The set of accounts is rebuilt from the DB on
// every tick of the supervisor — that way add/delete/cadence changes are
// picked up without restart and without needing eventbus plumbing yet.
//
// Cancellation is via context: cancel the supervisor's context and every
// per-account goroutine drains and exits.
package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/mandloideep/miniclaw/internal/services/account"
)

// SyncFn runs a single ingest+summarize pass for one account.
type SyncFn func(ctx context.Context, accountID int64) error

// Scheduler watches the accounts table and runs SyncFn for each account on
// its configured cadence.
type Scheduler struct {
	accounts *account.Service
	sync     SyncFn

	// supervisorPeriod is how often the supervisor re-reads the account list
	// to start/stop per-account loops. Default 30s; new accounts kick within
	// that window.
	supervisorPeriod time.Duration

	mu     sync.Mutex
	active map[int64]context.CancelFunc
}

// New returns a Scheduler. sync is called with the per-account context; it
// MUST honour ctx cancellation or shutdown will hang.
func New(accounts *account.Service, sync SyncFn) *Scheduler {
	return &Scheduler{
		accounts:         accounts,
		sync:             sync,
		supervisorPeriod: 30 * time.Second,
		active:           map[int64]context.CancelFunc{},
	}
}

// Start blocks until ctx is cancelled. Spawn it in a goroutine.
func (s *Scheduler) Start(ctx context.Context) {
	t := time.NewTicker(s.supervisorPeriod)
	defer t.Stop()
	s.reconcile(ctx) // first pass immediate
	for {
		select {
		case <-ctx.Done():
			s.stopAll()
			return
		case <-t.C:
			s.reconcile(ctx)
		}
	}
}

// reconcile diffs the current accounts list against active goroutines.
// Starts loops for new accounts, leaves existing ones alone, cancels
// loops for accounts that have been deleted.
func (s *Scheduler) reconcile(ctx context.Context) {
	accs, err := s.accounts.List(ctx)
	if err != nil {
		log.Printf("scheduler: list accounts: %v", err)
		return
	}
	want := map[int64]bool{}
	for _, a := range accs {
		want[a.ID] = true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	// cancel deleted
	for id, cancel := range s.active {
		if !want[id] {
			cancel()
			delete(s.active, id)
		}
	}
	// start new
	for _, a := range accs {
		if _, ok := s.active[a.ID]; ok {
			continue
		}
		cadence := time.Duration(a.SyncCadenceSecs) * time.Second
		if cadence <= 0 {
			cadence = 5 * time.Minute
		}
		loopCtx, cancel := context.WithCancel(ctx)
		s.active[a.ID] = cancel
		go func(id int64, cd time.Duration) {
			defer cancel()
			s.loop(loopCtx, id, cd)
		}(a.ID, cadence)
	}
}

func (s *Scheduler) loop(ctx context.Context, accountID int64, cadence time.Duration) {
	// Run once immediately so a freshly-added account doesn't wait a full
	// cadence for first ingest.
	s.runOnce(ctx, accountID)
	t := time.NewTicker(cadence)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runOnce(ctx, accountID)
		}
	}
}

func (s *Scheduler) runOnce(ctx context.Context, accountID int64) {
	if err := ctx.Err(); err != nil {
		return
	}
	// Bound a single sync at 5 minutes so a hung connection doesn't wedge
	// the per-account loop forever.
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := s.sync(runCtx, accountID); err != nil {
		log.Printf("scheduler: sync account %d: %v", accountID, err)
	}
}

func (s *Scheduler) stopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.active {
		cancel()
		delete(s.active, id)
	}
}

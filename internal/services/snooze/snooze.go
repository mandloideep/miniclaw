// Package snooze hides an email until a future timestamp, then surfaces it
// back to the inbox. Snooze is independent of the put-aside flag: an email
// can be either, neither, or (rarely) both. The wake ticker runs every
// minute and clears any rows whose snoozed_until <= now.
//
// Telegram nudges fire on wake only when the email already has a
// needs_attention summary — otherwise the wake is silent. That heuristic
// keeps the noise down on background newsletters the user snoozed away.
package snooze

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
)

// Notifier delivers a wake-up ping. Implemented by *telegram.Service so the
// snooze ticker can fire a "this came back" nudge for needs_attention rows.
// nil-safe inside Service for tests.
type Notifier interface {
	NotifySnoozeWake(ctx context.Context, workspaceID, emailID int64, subject string) error
}

// Service is the Wails-registered snooze surface plus the wake-loop driver.
type Service struct {
	q        *sqlcgen.Queries
	notifier Notifier
	// tickEvery controls the ticker cadence. Override in tests to drive
	// runOnce deterministically; production uses 60s.
	tickEvery time.Duration
}

// New wires the service to the shared DB pool.
func New(pool *sql.DB) *Service {
	return &Service{q: sqlcgen.New(pool), tickEvery: time.Minute}
}

// AttachNotifier lets main wire a Telegram notifier in place. Mirrors
// the AttachInlineStore pattern used by the syncers.
func (s *Service) AttachNotifier(n Notifier) { s.notifier = n }

// SnoozedEmail is the row shape returned by ListSnoozed.
type SnoozedEmail struct {
	EmailID      int64  `json:"emailId"`
	AccountID    int64  `json:"accountId"`
	FromAddress  string `json:"fromAddress"`
	FromName     string `json:"fromName"`
	Subject      string `json:"subject"`
	ReceivedAt   string `json:"receivedAt"`
	SnoozedUntil string `json:"snoozedUntil"`
}

// Snooze hides an email until the given RFC3339 timestamp. Strings instead
// of time.Time because Wails JSON marshalling round-trips cleaner.
func (s *Service) Snooze(ctx context.Context, emailID int64, untilRFC3339 string) error {
	t, err := time.Parse(time.RFC3339, untilRFC3339)
	if err != nil {
		return fmt.Errorf("snooze until %q: %w", untilRFC3339, err)
	}
	if t.Before(time.Now().UTC()) {
		return fmt.Errorf("snooze until must be in the future")
	}
	return s.q.SetSnooze(ctx, sqlcgen.SetSnoozeParams{
		SnoozedUntil: sql.NullString{String: t.UTC().Format(time.RFC3339), Valid: true},
		ID:           emailID,
	})
}

// Unsnooze clears the snooze flag immediately.
func (s *Service) Unsnooze(ctx context.Context, emailID int64) error {
	return s.q.ClearSnooze(ctx, emailID)
}

// ListSnoozed returns every snoozed email in a workspace, soonest-wake first.
func (s *Service) ListSnoozed(ctx context.Context, workspaceID int64) ([]SnoozedEmail, error) {
	rows, err := s.q.ListSnoozedByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list snoozed: %w", err)
	}
	out := make([]SnoozedEmail, 0, len(rows))
	for _, r := range rows {
		out = append(out, SnoozedEmail{
			EmailID: r.ID, AccountID: r.AccountID,
			FromAddress: r.FromAddress, FromName: r.FromName,
			Subject: r.Subject, ReceivedAt: r.ReceivedAt,
			SnoozedUntil: r.SnoozedUntil.String,
		})
	}
	return out, nil
}

// Start runs the wake-loop until ctx is cancelled. Safe to call once at
// app start; the ticker fires at the configured cadence (default 1 min).
func (s *Service) Start(ctx context.Context) {
	if s.tickEvery <= 0 {
		s.tickEvery = time.Minute
	}
	// First pass immediately so a freshly-booted app catches anything
	// that woke while it was closed.
	s.runOnce(ctx)
	t := time.NewTicker(s.tickEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.runOnce(ctx)
		}
	}
}

// runOnce walks every due snooze, clears it, and optionally pings.
func (s *Service) runOnce(ctx context.Context) {
	now := time.Now().UTC().Format(time.RFC3339)
	rows, err := s.q.ListDueSnoozed(ctx, sql.NullString{String: now, Valid: true})
	if err != nil {
		log.Printf("snooze: list due: %v", err)
		return
	}
	for _, r := range rows {
		if ctx.Err() != nil {
			return
		}
		if err := s.q.ClearSnooze(ctx, r.ID); err != nil {
			log.Printf("snooze: clear %d: %v", r.ID, err)
			continue
		}
		// Only ping when there's a needs_attention summary attached; the
		// wake itself surfacing in the inbox is enough for cold rows.
		if s.notifier == nil || !r.NeedsAttention.Valid || r.NeedsAttention.Int64 != 1 {
			continue
		}
		if err := s.notifier.NotifySnoozeWake(ctx, r.WorkspaceID, r.ID, r.Subject); err != nil {
			log.Printf("snooze: notify %d: %v", r.ID, err)
		}
	}
}

// Package triage powers the hey.com-style controls: put-aside, the
// new-sender screener, and the per-account block/allow filter list.
//
// Put-aside is a per-email flag toggled from the inbox.
// The screener is a per-sender approve/block state populated by ingest.
// Filter rules are exact email or domain-suffix matches the user adds
// to drop senders before they reach the inbox.
package triage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
)

// Service is the Wails-registered triage service.
type Service struct {
	q *sqlcgen.Queries
}

// New wires the service to the shared pool.
func New(pool *sql.DB) *Service { return &Service{q: sqlcgen.New(pool)} }

// PutAside is the snapshot returned for the put-aside tab.
type PutAside struct {
	EmailID     int64  `json:"emailId"`
	AccountID   int64  `json:"accountId"`
	FromAddress string `json:"fromAddress"`
	FromName    string `json:"fromName"`
	Subject     string `json:"subject"`
	ReceivedAt  string `json:"receivedAt"`
}

// ListPutAside returns put-aside emails inside a workspace.
func (s *Service) ListPutAside(ctx context.Context, workspaceID int64) ([]PutAside, error) {
	rows, err := s.q.ListPutAsideByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list put-aside: %w", err)
	}
	out := make([]PutAside, 0, len(rows))
	for _, r := range rows {
		out = append(out, PutAside{
			EmailID: r.ID, AccountID: r.AccountID,
			FromAddress: r.FromAddress, FromName: r.FromName,
			Subject: r.Subject, ReceivedAt: r.ReceivedAt,
		})
	}
	return out, nil
}

// TogglePutAside flips the put-aside bit on an email.
func (s *Service) TogglePutAside(ctx context.Context, emailID int64) error {
	return s.q.TogglePutAside(ctx, emailID)
}

// ScreenerEntry is a sender awaiting an approve/block decision, returned
// alongside the email that surfaced them.
type ScreenerEntry struct {
	SenderID    int64  `json:"senderId"`
	Address     string `json:"address"`
	FirstSeenAt string `json:"firstSeenAt"`
	EmailID     int64  `json:"emailId"`
	Subject     string `json:"subject"`
	ReceivedAt  string `json:"receivedAt"`
}

// ListUnscreened returns the sender entries pending a screener decision.
func (s *Service) ListUnscreened(ctx context.Context, accountID int64) ([]ScreenerEntry, error) {
	rows, err := s.q.ListUnscreenedByAccount(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list unscreened: %w", err)
	}
	out := make([]ScreenerEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, ScreenerEntry{
			SenderID: r.SenderID, Address: r.Address,
			FirstSeenAt: r.FirstSeenAt,
			EmailID:     r.EmailID, Subject: r.Subject, ReceivedAt: r.ReceivedAt,
		})
	}
	return out, nil
}

// ApproveSender marks a sender as allowed.
func (s *Service) ApproveSender(ctx context.Context, senderID int64) error {
	return s.q.ApproveSender(ctx, senderID)
}

// BlockSender marks a sender as blocked. Future ingest can drop them.
func (s *Service) BlockSender(ctx context.Context, senderID int64) error {
	return s.q.BlockSender(ctx, senderID)
}

// RegisterSender is called by the ingest pipeline as it sees each message.
// Idempotent — first appearance of a from-address creates the sender row in
// the unscreened state.
func (s *Service) RegisterSender(ctx context.Context, accountID int64, address string) error {
	if address == "" {
		return nil
	}
	return s.q.UpsertSender(ctx, sqlcgen.UpsertSenderParams{
		AccountID: accountID, Address: strings.ToLower(address),
	})
}

// FilterRule is the frontend-facing shape.
type FilterRule struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"accountId"`
	Kind      string `json:"kind"` // block | allow
	Pattern   string `json:"pattern"`
	Reason    string `json:"reason,omitempty"`
	CreatedAt string `json:"createdAt"`
}

// ListFilterRules returns rules for an account.
func (s *Service) ListFilterRules(ctx context.Context, accountID int64) ([]FilterRule, error) {
	rows, err := s.q.ListFilterRules(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("list rules: %w", err)
	}
	out := make([]FilterRule, 0, len(rows))
	for _, r := range rows {
		out = append(out, FilterRule{
			ID: r.ID, AccountID: r.AccountID, Kind: r.Kind, Pattern: r.Pattern,
			Reason: r.Reason, CreatedAt: r.CreatedAt,
		})
	}
	return out, nil
}

// AddFilterRule creates a block or allow rule for an account.
func (s *Service) AddFilterRule(ctx context.Context, accountID int64, kind, pattern, reason string) (FilterRule, error) {
	if kind != "block" && kind != "allow" {
		return FilterRule{}, errors.New("kind must be 'block' or 'allow'")
	}
	if pattern == "" {
		return FilterRule{}, errors.New("pattern is required")
	}
	r, err := s.q.AddFilterRule(ctx, sqlcgen.AddFilterRuleParams{
		AccountID: accountID, Kind: kind, Pattern: strings.ToLower(pattern), Reason: reason,
	})
	if err != nil {
		return FilterRule{}, fmt.Errorf("add rule: %w", err)
	}
	return FilterRule{
		ID: r.ID, AccountID: r.AccountID, Kind: r.Kind, Pattern: r.Pattern,
		Reason: r.Reason, CreatedAt: r.CreatedAt,
	}, nil
}

// DeleteFilterRule removes a rule.
func (s *Service) DeleteFilterRule(ctx context.Context, id int64) error {
	return s.q.DeleteFilterRule(ctx, id)
}

// MatchesBlock returns true if the given from-address matches any block
// rule for the account. Used by ingest to drop messages before storage.
// Pattern semantics: exact "user@host.tld" match, or domain suffix match
// when pattern is "@host.tld" or "host.tld".
func (s *Service) MatchesBlock(ctx context.Context, accountID int64, fromAddress string) (bool, error) {
	addr := strings.ToLower(fromAddress)
	rows, err := s.q.ListFilterRules(ctx, accountID)
	if err != nil {
		return false, err
	}
	for _, r := range rows {
		if r.Kind != "block" {
			continue
		}
		if matchesPattern(addr, r.Pattern) {
			return true, nil
		}
	}
	return false, nil
}

func matchesPattern(addr, pattern string) bool {
	if addr == pattern {
		return true
	}
	// Domain match: "@host.tld" or "host.tld" against the domain part.
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return false
	}
	domain := addr[at+1:]
	bare := strings.TrimPrefix(pattern, "@")
	return domain == bare
}

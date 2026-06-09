// Package digest fires a daily Telegram rundown of needs-attention email.
//
// Schedule: one tick per minute. When the wall clock crosses the configured
// HH:MM, run a single digest pass for every account, fanning out to that
// account's recipient union per docs/decisions.md §5. A simple
// last-fired-on date guards against double-firing within the same day if
// the app is suspended and resumed across the trigger minute.
package digest

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
	"github.com/mandloideep/miniclaw/internal/services/account"
	"github.com/mandloideep/miniclaw/internal/services/telegram"
	"github.com/mandloideep/miniclaw/internal/services/workspace"
)

// Service runs the daily digest scheduler.
type Service struct {
	q          *sqlcgen.Queries
	accounts   *account.Service
	workspaces *workspace.Service
	telegram   *telegram.Service

	mu        sync.Mutex
	lastFired time.Time // YYYY-MM-DD precision (local)
	clock     func() time.Time
}

// New wires the digest scheduler.
func New(pool *sql.DB, accounts *account.Service, ws *workspace.Service, tg *telegram.Service) *Service {
	return &Service{
		q:          sqlcgen.New(pool),
		accounts:   accounts,
		workspaces: ws,
		telegram:   tg,
		clock:      time.Now,
	}
}

// Start blocks until ctx is cancelled. Ticks every minute, fires the digest
// when wall-clock minute matches telegram_settings.digest_time and we
// haven't already fired today.
func (s *Service) Start(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.maybeFire(ctx)
		}
	}
}

// RunNow forces a digest pass irrespective of time. Used for a "send test
// digest" button in the UI and the digest_time changes.
func (s *Service) RunNow(ctx context.Context) error {
	return s.fire(ctx)
}

func (s *Service) maybeFire(ctx context.Context) {
	settings, err := s.telegram.GetSettings(ctx)
	if err != nil || settings.BotToken == "" || !isHHMM(settings.DigestTime) {
		return
	}
	now := s.clock()
	if now.Format("15:04") != settings.DigestTime {
		return
	}
	s.mu.Lock()
	if sameDate(s.lastFired, now) {
		s.mu.Unlock()
		return
	}
	s.lastFired = now
	s.mu.Unlock()

	if err := s.fire(ctx); err != nil {
		log.Printf("digest: %v", err)
	}
}

func (s *Service) fire(ctx context.Context) error {
	since := s.clock().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	rows, err := s.q.ListNeedsAttentionSince(ctx, since)
	if err != nil {
		return fmt.Errorf("list needs-attention: %w", err)
	}
	if len(rows) == 0 {
		// Nothing to say. Don't ping recipients with an empty message.
		return nil
	}

	// Group by account so we can fan out via that account's recipient union.
	byAccount := map[int64][]sqlcgen.ListNeedsAttentionSinceRow{}
	for _, r := range rows {
		byAccount[r.AccountID] = append(byAccount[r.AccountID], r)
	}

	for accountID, accountRows := range byAccount {
		recipients, rerr := s.telegram.RecipientsForAccount(ctx, accountID)
		if rerr != nil {
			log.Printf("digest: recipients for account %d: %v", accountID, rerr)
			continue
		}
		if len(recipients) == 0 {
			continue
		}
		acc, gerr := s.accounts.Get(ctx, accountID)
		if gerr != nil {
			log.Printf("digest: get account %d: %v", accountID, gerr)
			continue
		}
		ws, werr := s.workspaces.Get(ctx, acc.WorkspaceID)
		if werr != nil {
			log.Printf("digest: get workspace %d: %v", acc.WorkspaceID, werr)
			continue
		}
		text := buildMessage(ws, acc, accountRows)
		chatIDs := make([]string, 0, len(recipients))
		for _, r := range recipients {
			chatIDs = append(chatIDs, r.ChatID)
		}
		if err := s.telegram.Broadcast(ctx, chatIDs, text); err != nil {
			log.Printf("digest: broadcast account %d: %v", accountID, err)
		}
	}
	return nil
}

// buildMessage renders the per-account digest. Markdown-light so it renders
// well in Telegram clients without escaping landmines.
func buildMessage(ws workspace.Workspace, acc account.Account, rows []sqlcgen.ListNeedsAttentionSinceRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "*%s %s — %s*\n", ws.Emoji, ws.Name, acc.EmailAddress)
	fmt.Fprintf(&b, "_%d need attention in the last 24h_\n\n", len(rows))
	for _, r := range rows {
		who := r.FromName
		if who == "" {
			who = r.FromAddress
		}
		fmt.Fprintf(&b, "• *%s* — %s\n", escape(who), escape(r.Subject))
		if r.AttentionReason != "" {
			fmt.Fprintf(&b, "   _%s_\n", escape(r.AttentionReason))
		}
		if s := strings.TrimSpace(r.Summary); s != "" {
			fmt.Fprintf(&b, "   %s\n", escape(s))
		}
	}
	return b.String()
}

// escape neutralises the underscore/asterisk that Telegram parses as markdown.
func escape(s string) string {
	r := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"`", "\\`",
	)
	return r.Replace(s)
}

func isHHMM(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	for i, r := range s {
		if i == 2 {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	return h < 24 && m < 60
}

func sameDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

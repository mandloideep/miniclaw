package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/mandloideep/miniclaw/internal/services/account"
)

// IDLE keeps one long-lived IMAP IDLE connection per IMAP account. When the
// server reports new EXISTS/EXPUNGE it triggers OnChange, which the caller
// wires to an immediate sync pass.
//
// IDLE goes well beyond the cadence-based scheduler: instead of waiting up
// to sync_cadence_secs to notice a new message, we get a push within a few
// hundred milliseconds. Falls back to polling if IDLE isn't supported.
//
// Per RFC 2177, the connection must be re-issued at least every 29 minutes
// to avoid intermediate NAT timeouts. We rotate every 25.
type IDLE struct {
	accounts *account.Service
	onChange func(ctx context.Context, accountID int64)

	mu     sync.Mutex
	active map[int64]context.CancelFunc
}

// NewIDLE wires the supervisor. onChange fires whenever an IDLE session
// sees a mailbox update; usually you'd hand it a closure that calls the
// IMAP syncer's Sync.
func NewIDLE(accounts *account.Service, onChange func(ctx context.Context, accountID int64)) *IDLE {
	return &IDLE{
		accounts: accounts,
		onChange: onChange,
		active:   map[int64]context.CancelFunc{},
	}
}

// Start blocks until ctx is cancelled. Spawn it in a goroutine.
// Reconciles the active IDLE loops against the live account list every
// supervisorPeriod. New IMAP accounts get a loop; deleted ones get cancelled.
func (i *IDLE) Start(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	i.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			i.stopAll()
			return
		case <-t.C:
			i.reconcile(ctx)
		}
	}
}

func (i *IDLE) reconcile(ctx context.Context) {
	accs, err := i.accounts.List(ctx)
	if err != nil {
		log.Printf("idle: list accounts: %v", err)
		return
	}
	want := map[int64]bool{}
	for _, a := range accs {
		if a.AuthKind == account.AuthIMAP {
			want[a.ID] = true
		}
	}

	i.mu.Lock()
	defer i.mu.Unlock()
	for id, cancel := range i.active {
		if !want[id] {
			cancel()
			delete(i.active, id)
		}
	}
	for _, a := range accs {
		if a.AuthKind != account.AuthIMAP {
			continue
		}
		if _, ok := i.active[a.ID]; ok {
			continue
		}
		loopCtx, cancel := context.WithCancel(ctx)
		i.active[a.ID] = cancel
		go func(id int64) {
			defer cancel()
			i.runLoop(loopCtx, id)
		}(a.ID)
	}
}

// runLoop runs an IDLE session for one account, reconnecting on failures
// with capped exponential backoff so a flaky provider doesn't hot-loop.
func (i *IDLE) runLoop(ctx context.Context, accountID int64) {
	backoff := 5 * time.Second
	const maxBackoff = 5 * time.Minute
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		if err := i.oneSession(ctx, accountID); err != nil {
			log.Printf("idle: account %d: %v (retry in %v)", accountID, err, backoff)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < maxBackoff {
				backoff *= 2
			}
			continue
		}
		// Clean rotation — reset backoff.
		backoff = 5 * time.Second
	}
}

// oneSession dials, logs in, selects INBOX, then IDLEs for up to 25 minutes
// before politely returning to let the loop reconnect. EXISTS/EXPUNGE
// notifications fire onChange asynchronously.
func (i *IDLE) oneSession(ctx context.Context, accountID int64) error {
	acc, err := i.accounts.Get(ctx, accountID)
	if err != nil {
		return err
	}
	pwd, err := i.accounts.Password(ctx, accountID)
	if err != nil {
		return fmt.Errorf("load password: %w", err)
	}
	addr := fmt.Sprintf("%s:%d", acc.IMAPHost, acc.IMAPPort)

	// Buffered so the unilateral handler never blocks the IMAP reader.
	dirty := make(chan struct{}, 1)
	signal := func() {
		select {
		case dirty <- struct{}{}:
		default:
		}
	}

	opts := &imapclient.Options{
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: acc.IMAPHost},
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if data.NumMessages != nil {
					signal()
				}
			},
			Expunge: func(uint32) { signal() },
		},
	}
	c, err := imapclient.DialTLS(addr, opts)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = c.Close() }()

	if err := c.Login(acc.EmailAddress, pwd).Wait(); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	defer func() { _ = c.Logout().Wait() }()

	if _, err := c.Select("INBOX", nil).Wait(); err != nil {
		return fmt.Errorf("select INBOX: %w", err)
	}

	idle, err := c.Idle()
	if err != nil {
		return fmt.Errorf("start IDLE (server may not support it): %w", err)
	}

	// Bound the session at 25 min per RFC 2177 advice.
	timer := time.NewTimer(25 * time.Minute)
	defer timer.Stop()
	// Listen for triggers; coalesce bursts via a debounce.
	pending := false
	debounce := 500 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			_ = idle.Close()
			return ctx.Err()
		case <-timer.C:
			return idle.Close() // rotate session
		case <-dirty:
			pending = true
			// Reset a debounce window — coalesce a burst of EXISTS/EXPUNGE
			// (e.g. when a server delivers many messages quickly) into one
			// sync pass instead of N.
			deb := time.NewTimer(debounce)
			for pending {
				select {
				case <-ctx.Done():
					deb.Stop()
					_ = idle.Close()
					return ctx.Err()
				case <-deb.C:
					pending = false
					i.onChange(ctx, accountID)
				case <-dirty:
					// Burst continues; reset the window.
					if !deb.Stop() {
						<-deb.C
					}
					deb.Reset(debounce)
				}
			}
		}
	}
}

func (i *IDLE) stopAll() {
	i.mu.Lock()
	defer i.mu.Unlock()
	for id, cancel := range i.active {
		cancel()
		delete(i.active, id)
	}
}

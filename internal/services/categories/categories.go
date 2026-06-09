// Package categories assigns a "Promotions / Updates / Social / Newsletter"
// label to each email.
//
// Per docs/decisions.md §4, OAuth accounts will eventually use the provider's
// own labels (Gmail labels, MS Graph categories). Until OAuth lands, every
// account — IMAP or OAuth — falls through to the local filter pack.
//
// The starter rule pack matches a curated set of header and from-domain
// patterns. It's intentionally simple: a few exact-domain matches plus a
// header-presence check (List-Unsubscribe ⇒ newsletter unless something
// stronger fires).
package categories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mandloideep/miniclaw/internal/db/sqlcgen"
)

// Category enumerates the labels the engine emits.
type Category string

// Known category values. Empty string means "unclassified".
const (
	CatPromotions Category = "promotions"
	CatUpdates    Category = "updates"
	CatSocial     Category = "social"
	CatNewsletter Category = "newsletter"
)

// Engine scans stored emails and stamps the category column.
type Engine struct {
	q *sqlcgen.Queries
}

// New wires the engine to the shared pool.
func New(pool *sql.DB) *Engine { return &Engine{q: sqlcgen.New(pool)} }

// ClassifyAccount walks recently-ingested emails for an account and assigns
// categories using the starter pack. Returns count classified.
//
// Cheap to call; runs against the local store, no network.
func (e *Engine) ClassifyAccount(ctx context.Context, accountID int64) (int, error) {
	rows, err := e.q.ListEmailsByAccount(ctx, sqlcgen.ListEmailsByAccountParams{
		AccountID: accountID,
		Limit:     500,
	})
	if err != nil {
		return 0, fmt.Errorf("list emails: %w", err)
	}
	written := 0
	for _, r := range rows {
		if r.Category != "" {
			continue // already labelled
		}
		// ListEmailsByAccount doesn't currently return headers_json — for the
		// re-classify pass we only have the from-address. ingest-time
		// classification (in IMAPSyncer) sees the full headers and is the
		// preferred path.
		cat := classify(r.FromAddress, "")
		if cat == "" {
			continue
		}
		if err := e.q.SetCategory(ctx, sqlcgen.SetCategoryParams{
			Category: string(cat), ID: r.ID,
		}); err != nil {
			return written, fmt.Errorf("set category for email %d: %w", r.ID, err)
		}
		written++
	}
	return written, nil
}

// Classify is exposed so ingest (or tests) can ask for a single email's
// category without persisting.
func Classify(fromAddress, listUnsubscribeHeader string) Category {
	return classify(fromAddress, listUnsubscribeHeader)
}

func classify(fromAddress, listUnsubscribe string) Category {
	addr := strings.ToLower(fromAddress)
	domain := domainOf(addr)

	// Promotions: marketing platforms and known retailers' marketing reply-to.
	if matchesAny(domain, promotionsDomains) {
		return CatPromotions
	}
	// Social: notifications from social platforms.
	if matchesAny(domain, socialDomains) {
		return CatSocial
	}
	// Updates: status/transactional senders from common platforms.
	if matchesAny(domain, updatesDomains) {
		return CatUpdates
	}
	// Fallback: a List-Unsubscribe header on its own is a strong newsletter
	// signal across most providers.
	if listUnsubscribe != "" {
		return CatNewsletter
	}
	return ""
}

func domainOf(addr string) string {
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return ""
	}
	return addr[at+1:]
}

func matchesAny(domain string, patterns []string) bool {
	for _, p := range patterns {
		if domain == p || strings.HasSuffix(domain, "."+p) {
			return true
		}
	}
	return false
}

// Starter pack. Trim to the obvious offenders so we have signal without
// false positives. Users can override via the filter-rules pipeline.
var (
	promotionsDomains = []string{
		"mailchimp.com",
		"mailgun.com",
		"sendgrid.net",
		"mktomail.com",
		"e.shop.target.com",
		"deals.amazon.com",
	}
	socialDomains = []string{
		"facebookmail.com",
		"linkedin.com",
		"e.x.com",
		"notifications.twitter.com",
		"instagram.com",
		"reddit.com",
	}
	updatesDomains = []string{
		"stripe.com",
		"shopify.com",
		"github.com",
		"gitlab.com",
		"notion.so",
		"intercom-mail.com",
		"atlassian.net",
	}
)

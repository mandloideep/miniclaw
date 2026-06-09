// Package attachments stores and serves inline images and file attachments
// for cached emails. We persist the bytes inside SQLite — everything is
// local-only, the blob sizes are small (most inline mail images are
// 5-200kb), and a single email DELETE cascades the attachment row away
// via the schema's FK constraint.
package attachments

import (
	"context"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"
)

// Inline is the API-shape returned by GetInline. The data URL form lets
// the inbox reader splice the bytes straight into an iframe srcdoc
// without needing a separate Wails asset handler — same-origin policy
// inside a sandboxed srcdoc treats cross-document requests as the empty
// origin, which makes plain HTTP fetches inside it fragile.
type Inline struct {
	ContentID string `json:"contentId"`
	Filename  string `json:"filename"`
	MIME      string `json:"mime"`
	Size      int64  `json:"size"`
	DataURL   string `json:"dataUrl"` // base64-encoded data: URL the reader inlines
}

// Service is the Wails-registered facade.
type Service struct {
	pool *sql.DB
}

// New wires the service to the shared DB pool.
func New(pool *sql.DB) *Service { return &Service{pool: pool} }

// Store upserts one attachment row. Idempotent on (email_id, content_id, filename).
// Used by ingest paths (gmail/imap) when they decode a multipart message.
func (s *Service) Store(ctx context.Context, emailID int64, contentID, filename, mime string, isInline bool, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	inline := int64(0)
	if isInline {
		inline = 1
	}
	const q = `
INSERT INTO email_attachments
    (email_id, content_id, filename, mime_type, size_bytes, data, is_inline)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(email_id, content_id, filename) DO UPDATE SET
    mime_type  = excluded.mime_type,
    size_bytes = excluded.size_bytes,
    data       = excluded.data,
    is_inline  = excluded.is_inline`
	if _, err := s.pool.ExecContext(ctx, q,
		emailID, normalizeCID(contentID), filename, mime, int64(len(data)), data, inline,
	); err != nil {
		return fmt.Errorf("store attachment: %w", err)
	}
	return nil
}

// GetInline returns every inline attachment for an email, keyed for the
// reader pane to splice into the iframe. The reader matches on contentId.
func (s *Service) GetInline(ctx context.Context, emailID int64) ([]Inline, error) {
	const q = `
SELECT content_id, filename, mime_type, size_bytes, data
FROM email_attachments
WHERE email_id = ? AND (is_inline = 1 OR content_id != '')
ORDER BY id`
	rows, err := s.pool.QueryContext(ctx, q, emailID)
	if err != nil {
		return nil, fmt.Errorf("list inline: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]Inline, 0, 4)
	for rows.Next() {
		var (
			a    Inline
			data []byte
		)
		if err := rows.Scan(&a.ContentID, &a.Filename, &a.MIME, &a.Size, &data); err != nil {
			return nil, fmt.Errorf("scan attachment: %w", err)
		}
		mime := a.MIME
		if mime == "" {
			mime = "application/octet-stream"
		}
		a.DataURL = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
		out = append(out, a)
	}
	return out, rows.Err()
}

// normalizeCID strips RFC 2392 angle brackets so the stored cid matches
// what the HTML `cid:` URL references — Gmail returns "<abc@host>" but
// the HTML body writes `<img src="cid:abc@host">`.
func normalizeCID(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "<")
	raw = strings.TrimSuffix(raw, ">")
	return raw
}

package email

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/mandloideep/miniclaw/internal/services/account"
)

// SMTPSender sends replies/new mail from an account's SMTP host.
type SMTPSender struct {
	accounts *account.Service
}

// NewSMTPSender wires the sender to the account service for credential lookup.
func NewSMTPSender(acc *account.Service) *SMTPSender {
	return &SMTPSender{accounts: acc}
}

// OutgoingMessage is what callers (UI handlers, reply flows) hand to Send.
type OutgoingMessage struct {
	To          []string             `json:"to"`
	Cc          []string             `json:"cc"`
	Subject     string               `json:"subject"`
	Body        string               `json:"body"`
	Attachments []OutgoingAttachment `json:"attachments,omitempty"`
}

// OutgoingAttachment is one file attached to an outbound message. Data is
// base64-encoded so it round-trips through the Wails JSON bridge without
// running into byte-array marshalling quirks.
type OutgoingAttachment struct {
	Filename   string `json:"filename"`
	MIME       string `json:"mime"`
	DataBase64 string `json:"dataBase64"`
}

// Send dispatches msg from accountID via STARTTLS-on-submission. Works for
// the common provider config (Gmail :587, Yahoo :587, Outlook :587).
// For implicit TLS (:465) callers can use SendImplicitTLS.
func (s *SMTPSender) Send(ctx context.Context, accountID int64, msg OutgoingMessage) error {
	acc, err := s.accounts.Get(ctx, accountID)
	if err != nil {
		return err
	}
	pwd, err := s.accounts.Password(ctx, accountID)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", acc.SMTPHost, acc.SMTPPort)
	auth := smtp.PlainAuth("", acc.EmailAddress, pwd, acc.SMTPHost)
	raw := buildRFC822(acc.EmailAddress, msg)
	all := append([]string{}, msg.To...)
	all = append(all, msg.Cc...)
	return smtp.SendMail(addr, auth, acc.EmailAddress, all, []byte(raw))
}

// SendImplicitTLS dials directly with TLS on the submission port (465).
func (s *SMTPSender) SendImplicitTLS(ctx context.Context, accountID int64, msg OutgoingMessage) error {
	acc, err := s.accounts.Get(ctx, accountID)
	if err != nil {
		return err
	}
	pwd, err := s.accounts.Password(ctx, accountID)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("%s:%d", acc.SMTPHost, acc.SMTPPort)
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: acc.SMTPHost, MinVersion: tls.VersionTLS12})
	if err != nil {
		return fmt.Errorf("tls dial %s: %w", addr, err)
	}
	client, err := smtp.NewClient(conn, acc.SMTPHost)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp client: %w", err)
	}
	defer func() { _ = client.Quit() }()
	if err := client.Auth(smtp.PlainAuth("", acc.EmailAddress, pwd, acc.SMTPHost)); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if err := client.Mail(acc.EmailAddress); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	for _, rcpt := range append(msg.To, msg.Cc...) {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("rcpt %s: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write([]byte(buildRFC822(acc.EmailAddress, msg))); err != nil {
		return fmt.Errorf("write data: %w", err)
	}
	return w.Close()
}

func buildRFC822(from string, m OutgoingMessage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(m.To, ", "))
	if len(m.Cc) > 0 {
		fmt.Fprintf(&b, "Cc: %s\r\n", strings.Join(m.Cc, ", "))
	}
	fmt.Fprintf(&b, "Subject: %s\r\n", m.Subject)
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "MIME-Version: 1.0\r\n")

	if len(m.Attachments) == 0 {
		fmt.Fprintf(&b, "Content-Type: text/plain; charset=utf-8\r\n")
		b.WriteString("\r\n")
		b.WriteString(m.Body)
		return b.String()
	}

	boundary := randomBoundary()
	fmt.Fprintf(&b, "Content-Type: multipart/mixed; boundary=%q\r\n", boundary)
	b.WriteString("\r\n")

	// Plain-text body part.
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(m.Body)
	b.WriteString("\r\n")

	for _, att := range m.Attachments {
		mime := att.MIME
		if mime == "" {
			mime = "application/octet-stream"
		}
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		fmt.Fprintf(&b, "Content-Type: %s; name=%q\r\n", mime, att.Filename)
		b.WriteString("Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&b, "Content-Disposition: attachment; filename=%q\r\n", att.Filename)
		b.WriteString("\r\n")
		b.WriteString(encodeBase64Lines(att.DataBase64))
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.String()
}

// randomBoundary returns a multipart boundary unlikely to collide with body
// content. RFC 2046 puts no minimum, but ~24 random hex chars is plenty.
func randomBoundary() string {
	var buf [12]byte
	_, _ = rand.Read(buf[:])
	return "miniclaw-" + hex.EncodeToString(buf[:])
}

// encodeBase64Lines wraps the supplied base64 payload to 76-character lines
// per RFC 2045. Accepts pre-encoded base64 strings (what the Wails bridge
// hands us); decodes only to re-encode with the right line breaks.
func encodeBase64Lines(b64 string) string {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		// Fall back to the input as-is. Better to send an oddly-wrapped
		// attachment than to drop it silently.
		return b64
	}
	enc := base64.StdEncoding.EncodeToString(raw)
	var out strings.Builder
	for i := 0; i < len(enc); i += 76 {
		end := min(i+76, len(enc))
		out.WriteString(enc[i:end])
		out.WriteString("\r\n")
	}
	return out.String()
}

package email

import (
	"context"
	"crypto/tls"
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
	To      []string
	Cc      []string
	Subject string
	Body    string
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
	fmt.Fprintf(&b, "Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(m.Body)
	return b.String()
}

package email

import (
	"testing"
	"time"
)

func TestParseFetchSince(t *testing.T) {
	tests := []struct {
		name string
		in   string
		ok   bool
	}{
		{"iso date", "2026-01-15", true},
		{"rfc3339", "2026-01-15T10:00:00Z", true},
		{"junk", "yesterday", false},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseFetchSince(tc.in)
			if (err == nil) != tc.ok {
				t.Fatalf("ok=%v, err=%v", tc.ok, err)
			}
		})
	}
}

func TestParseMessage_PlainAndHTML(t *testing.T) {
	raw := []byte("From: a@b.test\r\n" +
		"To: c@d.test\r\n" +
		"Subject: hi\r\n" +
		"Date: " + time.Now().UTC().Format(time.RFC1123Z) + "\r\n" +
		"Message-ID: <m@1>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=BOUNDARY\r\n" +
		"\r\n" +
		"--BOUNDARY\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\n" +
		"plain body here\r\n" +
		"--BOUNDARY\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n\r\n" +
		"<p>html body</p>\r\n" +
		"--BOUNDARY--\r\n")

	parsed := parseMessage(raw)
	if parsed.plain == "" {
		t.Fatal("plain body missing")
	}
	if parsed.htmlBody == "" {
		t.Fatal("html body missing")
	}
	if parsed.headers["Subject"] != "hi" {
		t.Fatalf("subject header: %q", parsed.headers["Subject"])
	}
}

func TestDeriveThreadID(t *testing.T) {
	if got := DeriveThreadID("<a>", "", ""); got != "<a>" {
		t.Fatalf("self fallback: %q", got)
	}
	if got := DeriveThreadID("<c>", "<b>", "<a> <b>"); got != "<a>" {
		t.Fatalf("first ref wins: %q", got)
	}
	if got := DeriveThreadID("<c>", "<b>", ""); got != "<b>" {
		t.Fatalf("in-reply-to fallback: %q", got)
	}
}

func TestParseMessage_ThreadingHeaders(t *testing.T) {
	raw := []byte("From: a@b.test\r\n" +
		"Subject: re\r\n" +
		"Message-ID: <m@2>\r\n" +
		"In-Reply-To: <m@1>\r\n" +
		"References: <root@x> <m@1>\r\n" +
		"Content-Type: text/plain\r\n\r\nbody\r\n")
	p := parseMessage(raw)
	if p.inReplyTo != "<m@1>" {
		t.Fatalf("in-reply-to: %q", p.inReplyTo)
	}
	if p.references != "<root@x> <m@1>" {
		t.Fatalf("references: %q", p.references)
	}
}

func TestStripHTML(t *testing.T) {
	got := stripHTML("<p>hello <b>there</b></p>")
	if got != "hello there" {
		t.Fatalf("got %q", got)
	}
}

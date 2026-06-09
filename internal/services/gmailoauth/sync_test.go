package gmailoauth

import (
	"testing"
)

func TestSplitAddress(t *testing.T) {
	tests := []struct {
		raw, wantName, wantAddr string
	}{
		{"Jane Doe <jane@example.test>", "Jane Doe", "jane@example.test"},
		{"\"Quoted, Name\" <q@x.test>", "Quoted, Name", "q@x.test"},
		{"plain@x.test", "", "plain@x.test"},
	}
	for _, tc := range tests {
		n, a := splitAddress(tc.raw)
		if n != tc.wantName || a != tc.wantAddr {
			t.Errorf("splitAddress(%q) = (%q,%q), want (%q,%q)", tc.raw, n, a, tc.wantName, tc.wantAddr)
		}
	}
}

func TestGmailWatermarkQuery(t *testing.T) {
	got := gmailWatermarkQuery("", "")
	if got != "in:inbox" {
		t.Fatalf("got %q", got)
	}
	got = gmailWatermarkQuery("2026-01-15", "2026-01-10T00:00:00Z")
	if got != "in:inbox after:2026/01/15" {
		t.Fatalf("got %q", got)
	}
}

func TestInternalDateToRFC3339(t *testing.T) {
	if got := internalDateToRFC3339("1735689600000"); got != "2025-01-01T00:00:00Z" {
		t.Fatalf("got %q", got)
	}
	if got := internalDateToRFC3339(""); got != "" {
		t.Fatalf("empty: got %q", got)
	}
}

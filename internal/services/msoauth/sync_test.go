package msoauth

import "testing"

func TestWatermark(t *testing.T) {
	if got := watermark("", ""); got != "" {
		t.Fatalf("empty case: got %q", got)
	}
	if got := watermark("2026-01-15", "2026-01-10T00:00:00Z"); got == "" {
		t.Fatal("expected non-empty")
	}
}

func TestFromRaw(t *testing.T) {
	raw := rawMessage{
		ID: "abc", InternetMessageID: "msg-1",
		Subject: "hi", ReceivedDateTime: "2026-01-01T00:00:00Z",
	}
	raw.From.EmailAddress.Name = "Jane"
	raw.From.EmailAddress.Address = "j@x.test"
	raw.Body.ContentType = "html"
	raw.Body.Content = "<p>hi</p>"

	got := fromRaw(raw)
	if got.MessageID != "msg-1" || got.FromName != "Jane" {
		t.Fatalf("got %+v", got)
	}
	if got.BodyHTML == "" {
		t.Fatal("expected html body")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", "a", "b") != "a" {
		t.Fatal("first non-empty")
	}
	if firstNonEmpty("", "", "") != "" {
		t.Fatal("all empty")
	}
}

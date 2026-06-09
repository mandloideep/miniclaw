package keychain

import (
	"errors"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestRoundTrip(t *testing.T) {
	// Use the in-memory mock so tests don't pollute the real OS keychain.
	keyring.MockInit()

	if err := Set("ref-1", "hunter2"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	v, err := Get("ref-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if v != "hunter2" {
		t.Fatalf("got %q want %q", v, "hunter2")
	}
	if err := Delete("ref-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := Get("ref-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("post-delete Get: want ErrNotFound, got %v", err)
	}
}

func TestSet_RejectsEmptyRef(t *testing.T) {
	keyring.MockInit()
	if err := Set("", "x"); err == nil {
		t.Fatal("want error for empty ref")
	}
}

func TestDelete_MissingIsNoop(t *testing.T) {
	keyring.MockInit()
	if err := Delete("never-existed"); err != nil {
		t.Fatalf("Delete on missing: %v", err)
	}
}

// Package keychain stores per-account secrets (IMAP passwords, OAuth refresh
// tokens) in the OS keychain so they never sit on disk in plaintext.
//
// Backed by go-keyring:
//   - macOS: Keychain
//   - Windows: Credential Manager
//   - Linux: Secret Service via D-Bus (gnome-keyring, kwallet, etc.)
package keychain

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// service is the OS-keychain service name; per-account secrets all live under
// this single service with the account's secret_ref as the username/key.
const service = "miniclaw"

// ErrNotFound is returned by Get when no secret exists for the given ref.
var ErrNotFound = errors.New("keychain: not found")

// Service is registered with Wails so the frontend can ask "is the keychain
// usable?" without exposing any read/write surface.
type Service struct{}

// New returns a ready-to-register Service.
func New() *Service { return &Service{} }

// Available probes the keychain with a write+read+delete on a sentinel key.
// Surfaces to the frontend so onboarding can warn the user if the keychain
// is locked (Linux Secret Service not running, etc.).
func (s *Service) Available() bool {
	const probe = "__miniclaw_probe"
	if err := keyring.Set(service, probe, "ok"); err != nil {
		return false
	}
	defer func() { _ = keyring.Delete(service, probe) }()
	v, err := keyring.Get(service, probe)
	return err == nil && v == "ok"
}

// Set stores secret under ref. Overwrites any existing value for ref.
func Set(ref, secret string) error {
	if ref == "" {
		return errors.New("keychain: ref must be non-empty")
	}
	if err := keyring.Set(service, ref, secret); err != nil {
		return fmt.Errorf("keychain set %s: %w", ref, err)
	}
	return nil
}

// Get returns the secret stored under ref. Returns ErrNotFound if no such ref.
func Get(ref string) (string, error) {
	v, err := keyring.Get(service, ref)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("keychain get %s: %w", ref, err)
	}
	return v, nil
}

// Delete removes the secret under ref. No-op (returns nil) if not present.
func Delete(ref string) error {
	err := keyring.Delete(service, ref)
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return fmt.Errorf("keychain delete %s: %w", ref, err)
}

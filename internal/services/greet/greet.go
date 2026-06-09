// Package greet is the Wails-scaffolded sample service. Kept as a
// wiring sanity-check until the real services (email, ollama, telegram)
// replace it.
package greet

// Service exposes greeting methods to the frontend over the Wails bridge.
type Service struct{}

// New returns a ready-to-register Service.
func New() *Service { return &Service{} }

// Greet returns a personalised hello for name.
func (s *Service) Greet(name string) string {
	return "Hello " + name + "!"
}

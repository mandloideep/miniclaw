// Package ollama is a thin client over Ollama's local HTTP API.
//
// Endpoints used:
//
//	GET  /api/version    health check
//	GET  /api/tags       list installed models
//	POST /api/generate   one-shot non-streaming completion
//
// Docs: https://github.com/ollama/ollama/blob/main/docs/api.md
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// DefaultBaseURL is where Ollama listens out of the box.
const DefaultBaseURL = "http://localhost:11434"

// Model is one entry from /api/tags.
type Model struct {
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	Family        string `json:"family,omitempty"`
	ParameterSize string `json:"parameterSize,omitempty"`
	ModifiedAt    string `json:"modifiedAt,omitempty"`
}

// Status is the frontend-facing snapshot of Ollama's reachability.
//
// LoadedModels lists models currently held in memory (via /api/ps). The
// summariser warms the configured model proactively, but the UI uses
// this to tell the user "summaries will be slow until X loads" rather
// than silently surfacing a timeout error after a few minutes.
type Status struct {
	Running      bool     `json:"running"`
	Version      string   `json:"version,omitempty"`
	Error        string   `json:"error,omitempty"`
	LastError    string   `json:"lastError,omitempty"` // most recent generate failure, if any
	LoadedModels []string `json:"loadedModels,omitempty"`
}

// Service is the Wails-registered Ollama client.
//
// Two HTTP clients live here on purpose:
//
//   - short  — 15s, for /api/version / /api/tags / /api/ps. Anything
//     longer than that is a hung Ollama process, not a slow response.
//   - long   — 8min, for /api/generate. Cold-loading a 7B+ model on a
//     fresh process routinely takes 30-120s before the first token; the
//     old 60s ceiling was the root cause of the "context deadline
//     exceeded" wave the user hit on the first sync after launch.
type Service struct {
	baseURL string
	short   *http.Client
	long    *http.Client

	errMu   sync.RWMutex
	lastErr string
}

// New returns a Service pointed at DefaultBaseURL with sensible default
// timeouts for both fast metadata calls and slow generate calls.
func New() *Service {
	return NewWithURL(DefaultBaseURL)
}

// NewWithURL lets callers (tests, custom installs) override the host.
func NewWithURL(baseURL string) *Service {
	return &Service{
		baseURL: baseURL,
		short:   &http.Client{Timeout: 15 * time.Second},
		long:    &http.Client{Timeout: 8 * time.Minute},
	}
}

// Status hits /api/version + /api/ps. Returns Running=false (with Error
// populated) if the server is down — non-fatal, the frontend uses it for
// the status banner.
func (s *Service) Status(ctx context.Context) Status {
	resp, err := s.do(ctx, s.short, http.MethodGet, "/api/version", nil)
	if err != nil {
		return Status{Running: false, Error: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Status{Running: false, Error: fmt.Sprintf("decode version: %v", err)}
	}
	return Status{
		Running:      true,
		Version:      body.Version,
		LastError:    s.LastError(),
		LoadedModels: s.loadedModels(ctx),
	}
}

// loadedModels hits /api/ps. Best-effort; returns nil on any error so a
// missing endpoint (older Ollama builds) doesn't break the status pill.
func (s *Service) loadedModels(ctx context.Context) []string {
	resp, err := s.do(ctx, s.short, http.MethodGet, "/api/ps", nil)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil
	}
	out := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		if m.Name != "" {
			out = append(out, m.Name)
		}
	}
	return out
}

// ListModels returns installed models (`ollama list` equivalent).
func (s *Service) ListModels(ctx context.Context) ([]Model, error) {
	resp, err := s.do(ctx, s.short, http.MethodGet, "/api/tags", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	var body struct {
		Models []struct {
			Name       string `json:"name"`
			Size       int64  `json:"size"`
			ModifiedAt string `json:"modified_at"`
			Details    struct {
				Family        string `json:"family"`
				ParameterSize string `json:"parameter_size"`
			} `json:"details"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode tags: %w", err)
	}
	out := make([]Model, 0, len(body.Models))
	for _, m := range body.Models {
		out = append(out, Model{
			Name:          m.Name,
			Size:          m.Size,
			Family:        m.Details.Family,
			ParameterSize: m.Details.ParameterSize,
			ModifiedAt:    m.ModifiedAt,
		})
	}
	return out, nil
}

// GenerateRequest is the input shape for Generate.
type GenerateRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	System      string  `json:"system,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
	// JSONMode forces the model to emit valid JSON (Ollama format="json").
	JSONMode bool `json:"jsonMode,omitempty"`
}

// Generate runs a non-streaming completion. stream=false keeps the response a
// single JSON body, which is what the summarisation pipeline wants.
func (s *Service) Generate(ctx context.Context, req GenerateRequest) (string, error) {
	if req.Model == "" {
		return "", errors.New("ollama: model is required")
	}
	payload := map[string]any{
		"model":  req.Model,
		"prompt": req.Prompt,
		"stream": false,
	}
	if req.System != "" {
		payload["system"] = req.System
	}
	if req.Temperature != 0 {
		payload["options"] = map[string]any{"temperature": req.Temperature}
	}
	if req.JSONMode {
		payload["format"] = "json"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal generate: %w", err)
	}
	resp, err := s.do(ctx, s.long, http.MethodPost, "/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Response   string `json:"response"`
		Done       bool   `json:"done"`
		DoneReason string `json:"done_reason"`
		Error      string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode generate: %w", err)
	}
	if out.Error != "" {
		return "", fmt.Errorf("ollama: %s", out.Error)
	}
	if !out.Done {
		return "", errors.New("ollama: generate did not complete")
	}
	// An empty response with done=true means the model loaded (or hit a
	// content guard) without producing anything. Surface a real error so
	// the caller knows to skip / retry / fall back, instead of handing back
	// "" and forcing every downstream JSON decoder to fail.
	if out.Response == "" {
		reason := out.DoneReason
		if reason == "" {
			reason = "no response"
		}
		return "", fmt.Errorf("ollama: empty response (done_reason=%s)", reason)
	}
	return out.Response, nil
}

// LastError holds the most recent generate error the service saw, for
// surfacing in the status banner. Empty when the last call succeeded.
func (s *Service) LastError() string {
	s.errMu.RLock()
	defer s.errMu.RUnlock()
	return s.lastErr
}

// SetLastError stores an error string for retrieval by Status. Internal
// use — Generate doesn't write here itself, the caller decides what's
// worth surfacing (since transient retries shouldn't pollute the UI).
func (s *Service) SetLastError(msg string) {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	s.lastErr = msg
}

func (s *Service) do(ctx context.Context, client *http.Client, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama %s %s: %w", method, path, err)
	}
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("ollama %s %s: %s: %s", method, path, resp.Status, string(msg))
	}
	return resp, nil
}

// Warm asks Ollama to load the given model into memory without producing
// any output (prompt=" "). Frontend onboarding can call it after picking
// a model so the very first user-driven Generate is warm. Best-effort:
// returns nil on success, the wrapped Generate error otherwise.
func (s *Service) Warm(ctx context.Context, model string) error {
	if model == "" {
		return errors.New("ollama: model is required")
	}
	// Use a fresh-bodied request so the empty-response branch in Generate
	// doesn't fire — we genuinely don't care about the reply, only that
	// the model loaded.
	payload, _ := json.Marshal(map[string]any{
		"model":      model,
		"prompt":     " ",
		"stream":     false,
		"keep_alive": "30m",
	})
	resp, err := s.do(ctx, s.long, http.MethodPost, "/api/generate", bytes.NewReader(payload))
	if err != nil {
		s.SetLastError(fmt.Sprintf("warm %s: %v", model, err))
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	s.SetLastError("")
	return nil
}

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
type Status struct {
	Running bool   `json:"running"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Service is the Wails-registered Ollama client.
type Service struct {
	baseURL string
	client  *http.Client
}

// New returns a Service pointed at DefaultBaseURL with a 60s default timeout
// suitable for non-streaming generates against small local models.
func New() *Service {
	return NewWithURL(DefaultBaseURL)
}

// NewWithURL lets callers (tests, custom installs) override the host.
func NewWithURL(baseURL string) *Service {
	return &Service{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// Status hits /api/version. Returns Running=false (with Error populated) if
// the server is down — non-fatal, the frontend uses it for the status banner.
func (s *Service) Status(ctx context.Context) Status {
	resp, err := s.do(ctx, http.MethodGet, "/api/version", nil)
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
	return Status{Running: true, Version: body.Version}
}

// ListModels returns installed models (`ollama list` equivalent).
func (s *Service) ListModels(ctx context.Context) ([]Model, error) {
	resp, err := s.do(ctx, http.MethodGet, "/api/tags", nil)
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
	resp, err := s.do(ctx, http.MethodPost, "/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	var out struct {
		Response string `json:"response"`
		Done     bool   `json:"done"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode generate: %w", err)
	}
	if !out.Done {
		return "", errors.New("ollama: generate did not complete")
	}
	return out.Response, nil
}

func (s *Service) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.client.Do(req)
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

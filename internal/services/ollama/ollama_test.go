package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatus_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_ = json.NewEncoder(w).Encode(map[string]string{"version": "0.5.1"})
		case "/api/ps":
			_, _ = w.Write([]byte(`{"models":[{"name":"llama3.2:3b"}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)

	s := NewWithURL(srv.URL)
	got := s.Status(context.Background())
	if !got.Running || got.Version != "0.5.1" {
		t.Fatalf("unexpected: %+v", got)
	}
	if len(got.LoadedModels) != 1 || got.LoadedModels[0] != "llama3.2:3b" {
		t.Fatalf("loaded models: %+v", got.LoadedModels)
	}
}

func TestStatus_Down(t *testing.T) {
	s := NewWithURL("http://127.0.0.1:1") // closed port
	got := s.Status(context.Background())
	if got.Running {
		t.Fatalf("expected Running=false, got: %+v", got)
	}
}

func TestListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3.2:3b","size":42,"modified_at":"t","details":{"family":"llama","parameter_size":"3.2B"}}]}`))
	}))
	t.Cleanup(srv.Close)

	got, err := NewWithURL(srv.URL).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(got) != 1 || got[0].Name != "llama3.2:3b" || got[0].ParameterSize != "3.2B" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestGenerate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["model"] != "llama3.2:3b" || body["stream"] != false {
			t.Fatalf("unexpected body: %+v", body)
		}
		_, _ = w.Write([]byte(`{"response":"hello","done":true}`))
	}))
	t.Cleanup(srv.Close)

	out, err := NewWithURL(srv.URL).Generate(context.Background(), GenerateRequest{
		Model: "llama3.2:3b", Prompt: "hi",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if out != "hello" {
		t.Fatalf("got %q", out)
	}
}

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/johnuopini/searchmux/internal/formatter"
	"github.com/johnuopini/searchmux/internal/provider"
	"github.com/johnuopini/searchmux/internal/router"
)

// testProvider is a mock that implements provider.Provider for handler tests.
type testProvider struct {
	name      string
	available bool
	results   []provider.Result
	err       error
}

func (p *testProvider) Name() string { return p.name }
func (p *testProvider) Available() bool { return p.available }
func (p *testProvider) Search(_ context.Context, _ string, _ provider.SearchOpts) (*provider.Response, error) {
	if p.err != nil {
		return nil, p.err
	}
	return &provider.Response{Results: p.results}, nil
}

// newTestHandler creates a Handler with the given providers registered.
func newTestHandler(providers ...*testProvider) *Handler {
	r := router.New(router.Config{
		CBThreshold: 5,
		CBTimeout:   time.Minute,
	})
	for _, p := range providers {
		r.AddProvider(p, 1)
	}

	fmts := formatter.NewRegistry()
	fmts.Register(formatter.NewSearXNG())

	return NewHandler(r, fmts, "searxng")
}

func TestSearchHandler(t *testing.T) {
	p := &testProvider{
		name:      "mock",
		available: true,
		results: []provider.Result{
			{Title: "Test Result", URL: "https://example.com", Snippet: "A test result", Source: "mock"},
		},
	}
	h := newTestHandler(p)

	req := httptest.NewRequest(http.MethodGet, "/search?q=hello", nil)
	rec := httptest.NewRecorder()

	h.HandleSearch(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body["query"] != "hello" {
		t.Errorf("query = %v, want %q", body["query"], "hello")
	}

	results, ok := body["results"].([]interface{})
	if !ok || len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}

	first := results[0].(map[string]interface{})
	if first["title"] != "Test Result" {
		t.Errorf("first result title = %v, want %q", first["title"], "Test Result")
	}
}

func TestSearchHandlerMissingQuery(t *testing.T) {
	h := newTestHandler(&testProvider{name: "mock", available: true})

	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	rec := httptest.NewRecorder()

	h.HandleSearch(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestHealthHandler(t *testing.T) {
	p := &testProvider{name: "mock", available: true}
	h := newTestHandler(p)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	h.HandleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body["status"] != "ok" {
		t.Errorf("status = %v, want %q", body["status"], "ok")
	}

	providers, ok := body["providers"].(map[string]interface{})
	if !ok {
		t.Fatal("providers field missing or wrong type")
	}

	if _, ok := providers["mock"]; !ok {
		t.Error("providers should contain 'mock'")
	}
}

func TestSearchHandlerAllProvidersDown(t *testing.T) {
	p := &testProvider{
		name:      "dead",
		available: false,
	}
	h := newTestHandler(p)

	req := httptest.NewRequest(http.MethodGet, "/search?q=test", nil)
	rec := httptest.NewRecorder()

	h.HandleSearch(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body["error"] == "" {
		t.Error("expected error message for 503")
	}
}

func TestSearchHandlerRootAlias(t *testing.T) {
	p := &testProvider{
		name:      "mock",
		available: true,
		results: []provider.Result{
			{Title: "Root Result", URL: "https://example.com", Snippet: "Root alias test", Source: "mock"},
		},
	}
	h := newTestHandler(p)
	mux := h.Mux()

	req := httptest.NewRequest(http.MethodGet, "/?q=roottest", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if body["query"] != "roottest" {
		t.Errorf("query = %v, want %q", body["query"], "roottest")
	}

	results, ok := body["results"].([]interface{})
	if !ok || len(results) != 1 {
		t.Fatalf("results count = %d, want 1", len(results))
	}
}

package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTavilyName(t *testing.T) {
	tv := NewTavily("test-key", 5*time.Second)
	if tv.Name() != "tavily" {
		t.Errorf("Name() = %q, want %q", tv.Name(), "tavily")
	}
}

func TestTavilyAvailable(t *testing.T) {
	tv := NewTavily("test-key", 5*time.Second)
	if !tv.Available() {
		t.Error("Available() = false, want true")
	}
}

func TestTavilySearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		// Verify Bearer auth
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer test-api-key")
		}

		// Verify Content-Type
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		// Verify request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("reading body: %v", err)
		}
		var req struct {
			Query      string `json:"query"`
			MaxResults int    `json:"max_results"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshalling body: %v", err)
		}
		if req.Query != "test query" {
			t.Errorf("query = %q, want %q", req.Query, "test query")
		}
		if req.MaxResults != 5 {
			t.Errorf("max_results = %d, want %d", req.MaxResults, 5)
		}

		// Return mock response
		resp := map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"title":   "Tavily Result 1",
					"url":     "https://example.com/1",
					"content": "First tavily result content",
					"score":   0.95,
				},
				{
					"title":   "Tavily Result 2",
					"url":     "https://example.com/2",
					"content": "Second tavily result content",
					"score":   0.88,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tv := NewTavily("test-api-key", 5*time.Second)
	tv.BaseURL = srv.URL

	resp, err := tv.Search(context.Background(), "test query", SearchOpts{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(resp.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(resp.Results))
	}

	r := resp.Results[0]
	if r.Title != "Tavily Result 1" {
		t.Errorf("Title = %q, want %q", r.Title, "Tavily Result 1")
	}
	if r.URL != "https://example.com/1" {
		t.Errorf("URL = %q, want %q", r.URL, "https://example.com/1")
	}
	if r.Snippet != "First tavily result content" {
		t.Errorf("Snippet = %q, want %q", r.Snippet, "First tavily result content")
	}
	if r.Source != "tavily" {
		t.Errorf("Source = %q, want %q", r.Source, "tavily")
	}
}

func TestTavilySearchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": "rate limited"}`))
	}))
	defer srv.Close()

	tv := NewTavily("test-api-key", 5*time.Second)
	tv.BaseURL = srv.URL

	_, err := tv.Search(context.Background(), "test query", SearchOpts{MaxResults: 5})
	if err == nil {
		t.Fatal("Search() expected error for 429 response, got nil")
	}
}

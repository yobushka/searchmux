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

func TestSerperName(t *testing.T) {
	s := NewSerper("test-key", 5*time.Second)
	if s.Name() != "serper" {
		t.Errorf("Name() = %q, want %q", s.Name(), "serper")
	}
}

func TestSerperSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		// Verify X-API-KEY header
		apiKey := r.Header.Get("X-API-KEY")
		if apiKey != "serper-api-key" {
			t.Errorf("X-API-KEY = %q, want %q", apiKey, "serper-api-key")
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
			Q   string `json:"q"`
			Num int    `json:"num"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("unmarshalling body: %v", err)
		}
		if req.Q != "test query" {
			t.Errorf("q = %q, want %q", req.Q, "test query")
		}
		if req.Num != 5 {
			t.Errorf("num = %d, want %d", req.Num, 5)
		}

		// Return mock response
		resp := map[string]interface{}{
			"organic": []map[string]interface{}{
				{
					"title":   "Serper Result 1",
					"link":    "https://serper.example.com/1",
					"snippet": "First serper result snippet",
				},
				{
					"title":   "Serper Result 2",
					"link":    "https://serper.example.com/2",
					"snippet": "Second serper result snippet",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewSerper("serper-api-key", 5*time.Second)
	s.BaseURL = srv.URL

	resp, err := s.Search(context.Background(), "test query", SearchOpts{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(resp.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(resp.Results))
	}

	r := resp.Results[0]
	if r.Title != "Serper Result 1" {
		t.Errorf("Title = %q, want %q", r.Title, "Serper Result 1")
	}
	if r.URL != "https://serper.example.com/1" {
		t.Errorf("URL = %q, want %q", r.URL, "https://serper.example.com/1")
	}
	if r.Snippet != "First serper result snippet" {
		t.Errorf("Snippet = %q, want %q", r.Snippet, "First serper result snippet")
	}
	if r.Source != "serper" {
		t.Errorf("Source = %q, want %q", r.Source, "serper")
	}
}

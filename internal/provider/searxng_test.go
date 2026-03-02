package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSearXNGName(t *testing.T) {
	s := NewSearXNG("http://localhost:8888", 5*time.Second)
	if s.Name() != "searxng" {
		t.Errorf("Name() = %q, want %q", s.Name(), "searxng")
	}
}

func TestSearXNGAvailable(t *testing.T) {
	s := NewSearXNG("http://localhost:8888", 5*time.Second)
	if !s.Available() {
		t.Error("Available() = false, want true")
	}
}

func TestSearXNGSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}

		// Verify query params
		q := r.URL.Query()
		if q.Get("q") != "test query" {
			t.Errorf("q = %q, want %q", q.Get("q"), "test query")
		}
		if q.Get("format") != "json" {
			t.Errorf("format = %q, want %q", q.Get("format"), "json")
		}
		if q.Get("language") != "en" {
			t.Errorf("language = %q, want %q", q.Get("language"), "en")
		}

		// Return more results than MaxResults to test limiting
		resp := map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"title":   "SearXNG Result 1",
					"url":     "https://searxng.example.com/1",
					"content": "First searxng result content",
					"engine":  "google",
				},
				{
					"title":   "SearXNG Result 2",
					"url":     "https://searxng.example.com/2",
					"content": "Second searxng result content",
					"engine":  "bing",
				},
				{
					"title":   "SearXNG Result 3",
					"url":     "https://searxng.example.com/3",
					"content": "Third searxng result content",
					"engine":  "duckduckgo",
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	s := NewSearXNG(srv.URL, 5*time.Second)

	resp, err := s.Search(context.Background(), "test query", SearchOpts{
		MaxResults: 2,
		Language:   "en",
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	// Should be limited to MaxResults=2
	if len(resp.Results) != 2 {
		t.Fatalf("got %d results, want 2 (limited by MaxResults)", len(resp.Results))
	}

	r := resp.Results[0]
	if r.Title != "SearXNG Result 1" {
		t.Errorf("Title = %q, want %q", r.Title, "SearXNG Result 1")
	}
	if r.URL != "https://searxng.example.com/1" {
		t.Errorf("URL = %q, want %q", r.URL, "https://searxng.example.com/1")
	}
	if r.Snippet != "First searxng result content" {
		t.Errorf("Snippet = %q, want %q", r.Snippet, "First searxng result content")
	}
	// Source should always be "searxng", NOT the sub-engine
	if r.Source != "searxng" {
		t.Errorf("Source = %q, want %q (should not use sub-engine name)", r.Source, "searxng")
	}
}

package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBraveName(t *testing.T) {
	b := NewBrave(5 * time.Second)
	if b.Name() != "brave" {
		t.Errorf("Name() = %q, want %q", b.Name(), "brave")
	}
}

func TestBraveSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify method
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}

		// Verify X-Subscription-Token header
		token := r.Header.Get("X-Subscription-Token")
		if token != "brave-api-key" {
			t.Errorf("X-Subscription-Token = %q, want %q", token, "brave-api-key")
		}

		// Verify query params
		q := r.URL.Query()
		if q.Get("q") != "test query" {
			t.Errorf("q = %q, want %q", q.Get("q"), "test query")
		}
		if q.Get("count") != "3" {
			t.Errorf("count = %q, want %q", q.Get("count"), "3")
		}
		if q.Get("safesearch") != "moderate" {
			t.Errorf("safesearch = %q, want %q", q.Get("safesearch"), "moderate")
		}

		resp := map[string]interface{}{
			"web": map[string]interface{}{
				"results": []map[string]interface{}{
					{
						"title":       "Brave Result 1",
						"url":         "https://brave.example.com/1",
						"description": "First brave result description",
					},
					{
						"title":       "Brave Result 2",
						"url":         "https://brave.example.com/2",
						"description": "Second brave result description",
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	b := NewBrave(5 * time.Second)
	b.BaseURL = srv.URL

	resp, err := b.Search(context.Background(), "test query", SearchOpts{APIKey: "brave-api-key",
		MaxResults: 3,
		SafeSearch: 1, // moderate
	})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(resp.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(resp.Results))
	}

	r := resp.Results[0]
	if r.Title != "Brave Result 1" {
		t.Errorf("Title = %q, want %q", r.Title, "Brave Result 1")
	}
	if r.URL != "https://brave.example.com/1" {
		t.Errorf("URL = %q, want %q", r.URL, "https://brave.example.com/1")
	}
	if r.Snippet != "First brave result description" {
		t.Errorf("Snippet = %q, want %q", r.Snippet, "First brave result description")
	}
	if r.Source != "brave" {
		t.Errorf("Source = %q, want %q", r.Source, "brave")
	}
}

func TestBraveNoWebResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return response with no web results
		resp := map[string]interface{}{}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	b := NewBrave(5 * time.Second)
	b.BaseURL = srv.URL

	resp, err := b.Search(context.Background(), "no results", SearchOpts{APIKey: "brave-api-key", MaxResults: 5})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}

	if len(resp.Results) != 0 {
		t.Errorf("got %d results, want 0", len(resp.Results))
	}
}

package formatter

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/frenzy/searchmux/internal/provider"
)

func TestSearXNGFormatterName(t *testing.T) {
	f := NewSearXNG()
	if got := f.Name(); got != "searxng" {
		t.Errorf("Name() = %q, want %q", got, "searxng")
	}
}

func TestSearXNGContentType(t *testing.T) {
	f := NewSearXNG()
	if got := f.ContentType(); got != "application/json" {
		t.Errorf("ContentType() = %q, want %q", got, "application/json")
	}
}

func TestSearXNGFormat(t *testing.T) {
	f := NewSearXNG()
	results := []provider.Result{
		{Title: "Go Programming", URL: "https://go.dev", Snippet: "The Go programming language", Source: "google"},
		{Title: "Go Tour", URL: "https://go.dev/tour", Snippet: "A tour of Go", Source: "bing"},
	}
	meta := Metadata{
		ProviderUsed: "searxng-primary",
		TierUsed:     1,
		Fallback:     false,
		Latency:      150 * time.Millisecond,
	}

	data, err := f.Format("golang", results, meta)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}

	// Check top-level fields
	if resp["query"] != "golang" {
		t.Errorf("query = %v, want %q", resp["query"], "golang")
	}
	if resp["provider_used"] != "searxng-primary" {
		t.Errorf("provider_used = %v, want %q", resp["provider_used"], "searxng-primary")
	}
	if int(resp["tier_used"].(float64)) != 1 {
		t.Errorf("tier_used = %v, want 1", resp["tier_used"])
	}
	if resp["fallback"] != false {
		t.Errorf("fallback = %v, want false", resp["fallback"])
	}

	// Check results array
	rawResults, ok := resp["results"].([]interface{})
	if !ok {
		t.Fatalf("results is not an array")
	}
	if len(rawResults) != 2 {
		t.Fatalf("results length = %d, want 2", len(rawResults))
	}

	// Verify first result has correct field names (SearXNG format: content, engine)
	first := rawResults[0].(map[string]interface{})
	if first["title"] != "Go Programming" {
		t.Errorf("results[0].title = %v, want %q", first["title"], "Go Programming")
	}
	if first["url"] != "https://go.dev" {
		t.Errorf("results[0].url = %v, want %q", first["url"], "https://go.dev")
	}
	if first["content"] != "The Go programming language" {
		t.Errorf("results[0].content = %v, want %q", first["content"], "The Go programming language")
	}
	if first["engine"] != "google" {
		t.Errorf("results[0].engine = %v, want %q", first["engine"], "google")
	}
}

func TestSearXNGFormatEmptyResults(t *testing.T) {
	f := NewSearXNG()
	meta := Metadata{ProviderUsed: "test", TierUsed: 1}

	data, err := f.Format("empty", nil, meta)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshal error = %v", err)
	}
	if resp["query"] != "empty" {
		t.Errorf("query = %v, want %q", resp["query"], "empty")
	}
	// null results is acceptable for nil input
}

func TestGetFormatter(t *testing.T) {
	reg := NewRegistry()
	reg.Register(NewSearXNG())

	// Should find registered formatter
	f, err := reg.Get("searxng")
	if err != nil {
		t.Fatalf("Get(searxng) error = %v", err)
	}
	if f.Name() != "searxng" {
		t.Errorf("Name() = %q, want %q", f.Name(), "searxng")
	}

	// Should error for unknown formatter
	_, err = reg.Get("nonexistent")
	if err == nil {
		t.Error("Get(nonexistent) should return error")
	}
}

func TestRegistryMultipleFormatters(t *testing.T) {
	reg := NewRegistry()

	// Register SearXNG and a mock formatter
	reg.Register(NewSearXNG())
	reg.Register(&mockFormatter{name: "raw"})

	// Get each one
	f1, err := reg.Get("searxng")
	if err != nil {
		t.Fatalf("Get(searxng) error = %v", err)
	}
	if f1.Name() != "searxng" {
		t.Errorf("Name() = %q, want %q", f1.Name(), "searxng")
	}

	f2, err := reg.Get("raw")
	if err != nil {
		t.Fatalf("Get(raw) error = %v", err)
	}
	if f2.Name() != "raw" {
		t.Errorf("Name() = %q, want %q", f2.Name(), "raw")
	}

	// Unknown still errors
	_, err = reg.Get("xml")
	if err == nil {
		t.Error("Get(xml) should return error")
	}
}

// mockFormatter is a simple formatter for testing the registry with multiple entries.
type mockFormatter struct {
	name string
}

func (m *mockFormatter) Name() string       { return m.name }
func (m *mockFormatter) ContentType() string { return "text/plain" }
func (m *mockFormatter) Format(query string, results []provider.Result, meta Metadata) ([]byte, error) {
	return []byte(query), nil
}

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SearXNG implements Provider for a self-hosted SearXNG instance.
type SearXNG struct {
	apiBase string
	client  *http.Client
}

// NewSearXNG creates a new SearXNG provider.
// apiBase is the base URL of the SearXNG instance (e.g., "http://searxng:8888").
func NewSearXNG(apiBase string, timeout time.Duration) *SearXNG {
	return &SearXNG{
		apiBase: apiBase,
		client:  &http.Client{Timeout: timeout},
	}
}

func (s *SearXNG) Name() string    { return "searxng" }
func (s *SearXNG) Available() bool { return s.apiBase != "" }

// searxngResponse is the JSON response from the SearXNG API.
type searxngResponse struct {
	Results []searxngResult `json:"results"`
}

type searxngResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
	Engine  string `json:"engine"`
}

func (s *SearXNG) Search(ctx context.Context, query string, opts SearchOpts) (*Response, error) {
	url := s.apiBase + "/search"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("searxng: create request")
	}

	q := req.URL.Query()
	q.Set("q", query)
	q.Set("format", "json")
	if opts.Language != "" {
		q.Set("language", opts.Language)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searxng: do request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("searxng: unexpected status %d", resp.StatusCode)
	}

	var searxResp searxngResponse
	if err := json.NewDecoder(resp.Body).Decode(&searxResp); err != nil {
		return nil, fmt.Errorf("searxng: decode response")
	}

	// Limit results to MaxResults if specified
	items := searxResp.Results
	if opts.MaxResults > 0 && len(items) > opts.MaxResults {
		items = items[:opts.MaxResults]
	}

	results := make([]Result, len(items))
	for i, r := range items {
		results[i] = Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
			Source:  "searxng", // Always "searxng", not the sub-engine
		}
	}

	return &Response{Results: results}, nil
}

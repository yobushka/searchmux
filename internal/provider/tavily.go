package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Tavily implements Provider for the Tavily search API.
type Tavily struct {
	client  *http.Client
	BaseURL string
}

// NewTavily creates a new Tavily provider.
func NewTavily(timeout time.Duration) *Tavily {
	return &Tavily{
		client:  newHTTPClient(timeout),
		BaseURL: "https://api.tavily.com",
	}
}

func (t *Tavily) Name() string    { return "tavily" }
func (t *Tavily) Available() bool { return t.BaseURL != "" }

// tavilyRequest is the JSON body sent to the Tavily API.
type tavilyRequest struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// tavilyResponse is the JSON response from the Tavily API.
type tavilyResponse struct {
	Results []tavilyResult `json:"results"`
}

type tavilyResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

func (t *Tavily) RequiresAPIKey() bool { return true }

func (t *Tavily) Search(ctx context.Context, query string, opts SearchOpts) (*Response, error) {
	if opts.APIKey == "" {
		return nil, ErrMissingAPIKey
	}
	reqBody := tavilyRequest{
		Query:      query,
		MaxResults: opts.MaxResults,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("tavily: marshal request")
	}

	url := t.BaseURL + "/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tavily: create request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily: do request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode}
	}

	var tavilyResp tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&tavilyResp); err != nil {
		return nil, fmt.Errorf("tavily: decode response")
	}

	results := make([]Result, len(tavilyResp.Results))
	for i, r := range tavilyResp.Results {
		results[i] = Result{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
			Source:  "tavily",
		}
	}

	return &Response{Results: results}, nil
}

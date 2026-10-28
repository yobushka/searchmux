package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Serper implements Provider for the Serper (Google) search API.
type Serper struct {
	client  *http.Client
	BaseURL string
}

// NewSerper creates a new Serper provider.
func NewSerper(timeout time.Duration) *Serper {
	return &Serper{
		client:  newHTTPClient(timeout),
		BaseURL: "https://google.serper.dev",
	}
}

func (s *Serper) Name() string    { return "serper" }
func (s *Serper) Available() bool { return s.BaseURL != "" }

// serperRequest is the JSON body sent to the Serper API.
type serperRequest struct {
	Q   string `json:"q"`
	Num int    `json:"num"`
}

// serperResponse is the JSON response from the Serper API.
type serperResponse struct {
	Organic []serperResult `json:"organic"`
}

type serperResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

func (s *Serper) RequiresAPIKey() bool { return true }

func (s *Serper) Search(ctx context.Context, query string, opts SearchOpts) (*Response, error) {
	if opts.APIKey == "" {
		return nil, ErrMissingAPIKey
	}
	reqBody := serperRequest{
		Q:   query,
		Num: opts.MaxResults,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("serper: marshal request")
	}

	url := s.BaseURL + "/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("serper: create request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", opts.APIKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("serper: do request")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode}
	}

	var serperResp serperResponse
	if err := json.NewDecoder(resp.Body).Decode(&serperResp); err != nil {
		return nil, fmt.Errorf("serper: decode response")
	}

	results := make([]Result, len(serperResp.Organic))
	for i, r := range serperResp.Organic {
		results[i] = Result{
			Title:   r.Title,
			URL:     r.Link,
			Snippet: r.Snippet,
			Source:  "serper",
		}
	}

	return &Response{Results: results}, nil
}

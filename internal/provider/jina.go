package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Jina implements Jina AI Search with request-scoped authentication.
type Jina struct {
	client  *http.Client
	BaseURL string
}

func NewJina(timeout time.Duration) *Jina {
	return &Jina{client: newHTTPClient(timeout), BaseURL: "https://s.jina.ai"}
}
func (j *Jina) Name() string         { return "jina" }
func (j *Jina) Available() bool      { return j.BaseURL != "" }
func (j *Jina) RequiresAPIKey() bool { return true }

func (j *Jina) Search(ctx context.Context, query string, opts SearchOpts) (*Response, error) {
	if opts.APIKey == "" {
		return nil, ErrMissingAPIKey
	}
	body, err := json.Marshal(struct {
		Q string `json:"q"`
	}{Q: query})
	if err != nil {
		return nil, fmt.Errorf("jina: encode request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.BaseURL+"/", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("jina: create request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+opts.APIKey)
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jina: request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{StatusCode: resp.StatusCode}
	}
	var data struct {
		Data []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			Content     string `json:"content"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("jina: invalid response")
	}
	results := make([]Result, 0, len(data.Data))
	for _, item := range data.Data {
		text := item.Description
		if text == "" {
			text = item.Content
		}
		results = append(results, Result{Title: item.Title, URL: item.URL, Snippet: text, Source: j.Name()})
		if opts.MaxResults > 0 && len(results) == opts.MaxResults {
			break
		}
	}
	return &Response{Results: results}, nil
}

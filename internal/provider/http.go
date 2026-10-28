package provider

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

var ErrMissingAPIKey = errors.New("provider requires request credentials")

// HTTPError exposes only the status, never an upstream response body.
type HTTPError struct{ StatusCode int }

func (e *HTTPError) Error() string { return fmt.Sprintf("provider returned HTTP %d", e.StatusCode) }

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

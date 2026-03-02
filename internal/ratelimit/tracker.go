package ratelimit

import (
	"sync"
	"time"
)

type window struct {
	count     int
	limit     int
	duration  time.Duration
	resetAt   time.Time
	hardLimit bool
}

// Tracker tracks per-provider rate limits using sliding time windows.
type Tracker struct {
	mu      sync.Mutex
	windows map[string]*window
}

// New creates a new rate limit Tracker.
func New() *Tracker {
	return &Tracker{windows: make(map[string]*window)}
}

// Configure sets the rate limit for a provider.
func (t *Tracker) Configure(provider string, limit int, duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.windows[provider] = &window{
		limit:    limit,
		duration: duration,
		resetAt:  time.Now().Add(duration),
	}
}

// Allow returns true if the provider has remaining capacity.
// Unconfigured providers are always allowed.
func (t *Tracker) Allow(provider string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, ok := t.windows[provider]
	if !ok {
		return true
	}
	t.resetIfExpired(w)
	if w.hardLimit {
		return false
	}
	return w.count < w.limit
}

// Record increments the request count for a provider.
func (t *Tracker) Record(provider string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, ok := t.windows[provider]
	if !ok {
		return
	}
	t.resetIfExpired(w)
	w.count++
}

// RecordLimit marks a provider as hard-limited (e.g. after receiving a 429).
// The hard limit clears when the current window expires.
func (t *Tracker) RecordLimit(provider string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, ok := t.windows[provider]
	if !ok {
		t.windows[provider] = &window{
			hardLimit: true,
			duration:  5 * time.Minute,
			resetAt:   time.Now().Add(5 * time.Minute),
		}
		return
	}
	w.hardLimit = true
}

// Remaining returns the number of requests remaining for a provider.
// Returns -1 for unconfigured providers.
func (t *Tracker) Remaining(provider string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	w, ok := t.windows[provider]
	if !ok {
		return -1
	}
	t.resetIfExpired(w)
	r := w.limit - w.count
	if r < 0 {
		return 0
	}
	return r
}

func (t *Tracker) resetIfExpired(w *window) {
	if time.Now().After(w.resetAt) {
		w.count = 0
		w.hardLimit = false
		w.resetAt = time.Now().Add(w.duration)
	}
}

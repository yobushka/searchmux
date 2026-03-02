package router

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/johnuopini/searchmux/internal/provider"
	"github.com/johnuopini/searchmux/internal/ratelimit"
)

// Config holds configuration for the Router.
type Config struct {
	CBThreshold int
	CBTimeout   time.Duration
	RateLimiter *ratelimit.Tracker
}

// SearchResult holds the result of a routed search including routing metadata.
type SearchResult struct {
	Results      []provider.Result
	ProviderUsed string
	TierUsed     int
	Fallback     bool
	Latency      time.Duration
}

// ProviderHealth reports the health status of a single provider.
type ProviderHealth struct {
	Available          bool   `json:"available"`
	Circuit            string `json:"circuit"`
	RateLimitRemaining int    `json:"rate_limit_remaining"`
}

type entry struct {
	provider provider.Provider
	tier     int
	cb       *CircuitBreaker
}

// Router manages tiered search providers with round-robin selection,
// circuit breaking, and rate limit awareness.
type Router struct {
	mu           sync.RWMutex
	entries      []entry
	config       Config
	tierCounters map[int]*atomic.Uint64
}

// New creates a new Router with the given configuration.
func New(cfg Config) *Router {
	return &Router{
		config:       cfg,
		tierCounters: make(map[int]*atomic.Uint64),
	}
}

// AddProvider registers a provider at the given tier level.
// Lower tier numbers have higher priority.
func (r *Router) AddProvider(p provider.Provider, tier int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	cb := NewCircuitBreaker(r.config.CBThreshold, r.config.CBTimeout)
	r.entries = append(r.entries, entry{provider: p, tier: tier, cb: cb})

	if _, ok := r.tierCounters[tier]; !ok {
		r.tierCounters[tier] = &atomic.Uint64{}
	}
}

// Search executes a search query using tiered routing with fallback.
//
// Algorithm:
//  1. Group providers by tier, sorted ascending (lowest tier = highest priority).
//  2. Within each tier, filter to available providers (Available(), circuit breaker, rate limit).
//  3. Round-robin select among available providers in the tier.
//  4. On success, record to rate limiter and circuit breaker, return result.
//  5. On failure, record failure, try next provider in same tier.
//  6. If all providers in a tier fail, move to next tier (mark as fallback).
//  7. All tiers exhausted: return error.
func (r *Router) Search(ctx context.Context, query string, opts provider.SearchOpts) (*SearchResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if len(r.entries) == 0 {
		return nil, fmt.Errorf("no providers registered")
	}

	// Group entries by tier.
	tiers := r.groupByTier()

	// Sort tier keys ascending.
	tierKeys := make([]int, 0, len(tiers))
	for k := range tiers {
		tierKeys = append(tierKeys, k)
	}
	sort.Ints(tierKeys)

	fallback := false
	for _, tier := range tierKeys {
		entries := tiers[tier]

		// Filter to available providers.
		available := r.filterAvailable(entries)
		if len(available) == 0 {
			fallback = true
			continue
		}

		// Round-robin within tier.
		counter := r.tierCounters[tier]
		startIdx := int(counter.Add(1) - 1)

		for i := 0; i < len(available); i++ {
			idx := (startIdx + i) % len(available)
			e := available[idx]

			start := time.Now()
			resp, err := e.provider.Search(ctx, query, opts)
			latency := time.Since(start)

			if err != nil {
				e.cb.RecordFailure()
				// Check for rate limit error (429-like).
				if r.config.RateLimiter != nil && isRateLimitError(err) {
					r.config.RateLimiter.RecordLimit(e.provider.Name())
				}
				continue
			}

			// Success.
			e.cb.RecordSuccess()
			if r.config.RateLimiter != nil {
				r.config.RateLimiter.Record(e.provider.Name())
			}

			return &SearchResult{
				Results:      resp.Results,
				ProviderUsed: e.provider.Name(),
				TierUsed:     tier,
				Fallback:     fallback,
				Latency:      latency,
			}, nil
		}

		// All providers in this tier failed; next tier is a fallback.
		fallback = true
	}

	return nil, fmt.Errorf("all providers exhausted")
}

// Health returns the health status of all registered providers.
func (r *Router) Health() map[string]ProviderHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()

	health := make(map[string]ProviderHealth, len(r.entries))
	for _, e := range r.entries {
		remaining := -1
		if r.config.RateLimiter != nil {
			remaining = r.config.RateLimiter.Remaining(e.provider.Name())
		}
		health[e.provider.Name()] = ProviderHealth{
			Available:          e.provider.Available(),
			Circuit:            e.cb.State().String(),
			RateLimitRemaining: remaining,
		}
	}
	return health
}

// groupByTier groups entries by their tier number.
func (r *Router) groupByTier() map[int][]entry {
	tiers := make(map[int][]entry)
	for _, e := range r.entries {
		tiers[e.tier] = append(tiers[e.tier], e)
	}
	return tiers
}

// filterAvailable returns entries whose provider is available,
// circuit breaker allows, and rate limiter allows.
func (r *Router) filterAvailable(entries []entry) []entry {
	var available []entry
	for _, e := range entries {
		if !e.provider.Available() {
			continue
		}
		if !e.cb.Allow() {
			continue
		}
		if r.config.RateLimiter != nil && !r.config.RateLimiter.Allow(e.provider.Name()) {
			continue
		}
		available = append(available, e)
	}
	return available
}

// isRateLimitError checks if the error indicates a rate limit (HTTP 429).
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "429")
}

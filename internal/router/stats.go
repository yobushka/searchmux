package router

import (
	"sync"
	"time"
)

// ProviderStats contains process-lifetime aggregates, never request data.
type ProviderStats struct {
	Available             bool    `json:"available"`
	Circuit               string  `json:"circuit"`
	Requests              uint64  `json:"requests"`
	Successes             uint64  `json:"successes"`
	Failures              uint64  `json:"failures"`
	SuccessRate           float64 `json:"success_rate"`
	FailureRate           float64 `json:"failure_rate"`
	AverageLatencySeconds float64 `json:"average_latency_seconds"`
	LastLatencySeconds    float64 `json:"last_latency_seconds"`
}

type providerCounters struct {
	mu                 sync.Mutex
	requests, failures uint64
	total, last        time.Duration
}

func (s *providerCounters) record(latency time.Duration, failed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests++
	if failed {
		s.failures++
	}
	s.total += latency
	s.last = latency
}

func (r *Router) Stats() map[string]ProviderStats {
	r.mu.RLock()
	defer r.mu.RUnlock()
	stats := make(map[string]ProviderStats, len(r.entries))
	for _, e := range r.entries {
		state := e.cb.State()
		available := e.provider.Available() && state != StateOpen
		if !requiresKey(e.provider) && r.config.RateLimiter != nil {
			available = available && r.config.RateLimiter.Allow(e.provider.Name())
		}
		e.stats.mu.Lock()
		s := ProviderStats{Available: available, Circuit: state.String(), Requests: e.stats.requests, Failures: e.stats.failures, Successes: e.stats.requests - e.stats.failures, LastLatencySeconds: e.stats.last.Seconds()}
		if s.Requests > 0 {
			s.SuccessRate = float64(s.Successes) / float64(s.Requests)
			s.FailureRate = float64(s.Failures) / float64(s.Requests)
			s.AverageLatencySeconds = e.stats.total.Seconds() / float64(s.Requests)
		}
		e.stats.mu.Unlock()
		stats[e.provider.Name()] = s
	}
	return stats
}

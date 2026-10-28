package router

import (
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	// StateClosed allows all requests through.
	StateClosed CircuitState = iota
	// StateOpen blocks all requests.
	StateOpen
	// StateHalfOpen allows a single probe request through.
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern for provider failover.
type CircuitBreaker struct {
	mu         sync.Mutex
	failures   []time.Time
	threshold  int
	timeout    time.Duration
	window     time.Duration
	now        func() time.Time
	probe      bool
	generation uint64
	state      CircuitState
	openedAt   time.Time
}

// NewCircuitBreaker creates a circuit breaker that opens after threshold
// failures within the window and transitions to half-open after timeout.
func NewCircuitBreaker(threshold int, timeout time.Duration, windows ...time.Duration) *CircuitBreaker {
	window := time.Minute
	if len(windows) > 0 && windows[0] > 0 {
		window = windows[0]
	}
	if threshold <= 0 {
		threshold = 5
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &CircuitBreaker{
		window:    window,
		now:       time.Now,
		threshold: threshold,
		timeout:   timeout,
		state:     StateClosed,
	}
}

// Allow returns true if the circuit breaker permits a request.
// In the closed state, all requests are allowed.
// In the open state, requests are blocked until the timeout expires.
// In the half-open state, a single probe request is allowed.
func (cb *CircuitBreaker) Allow() bool {
	_, allowed := cb.acquire()
	return allowed
}

// acquire associates completion with the breaker generation at dispatch.
func (cb *CircuitBreaker) acquire() (uint64, bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.advance()
	switch cb.state {
	case StateClosed:
		return cb.generation, true
	case StateOpen:
		return cb.generation, false
	case StateHalfOpen:
		if cb.probe {
			return cb.generation, false
		}
		cb.probe = true
		return cb.generation, true
	default:
		return cb.generation, false
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.advance()
	return cb.state
}

func (cb *CircuitBreaker) advance() {
	if cb.state == StateOpen && !cb.now().Before(cb.openedAt.Add(cb.timeout)) {
		cb.state = StateHalfOpen
		cb.probe = false
	}
}

// RecordSuccess resets the failure counter and closes the circuit.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.success()
}

func (cb *CircuitBreaker) success() {
	if cb.state == StateHalfOpen && cb.probe {
		cb.failures = nil
		cb.state = StateClosed
		cb.probe = false
		cb.generation++
	}
}

// RecordFailure increments the failure counter and opens the circuit
// if the threshold is reached.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failure()
}

func (cb *CircuitBreaker) failure() {
	if cb.state == StateOpen {
		return
	}
	now := cb.now()
	cutoff := now.Add(-cb.window)
	i := 0
	for i < len(cb.failures) && !cb.failures[i].After(cutoff) {
		i++
	}
	cb.failures = append(cb.failures[i:], now)
	if len(cb.failures) >= cb.threshold || cb.state == StateHalfOpen {
		cb.state = StateOpen
		cb.openedAt = now
		cb.probe = false
		cb.generation++
	}
}

func (cb *CircuitBreaker) complete(generation uint64, failed, ignored bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if generation != cb.generation {
		return
	}
	if ignored {
		cb.probe = false
		return
	}
	if failed {
		cb.failure()
	} else {
		cb.success()
	}
}

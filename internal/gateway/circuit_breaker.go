package gateway

import (
	"sync"
	"time"
)

// CircuitState represents the current state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Healthy, allowing requests
	CircuitOpen                         // Tripped, rejecting requests
	CircuitHalfOpen                     // Allowing a single probe request
)

// CircuitBreakerConfig holds per-server circuit breaker settings.
type CircuitBreakerConfig struct {
	FailThreshold int           // Failures before opening
	OpenDuration  time.Duration // How long to stay open before half-open probe
}

type circuitEntry struct {
	state         CircuitState
	failures      int
	openedAt      time.Time
	probeInFlight bool
}

// CircuitBreaker tracks per-label circuit state.
type CircuitBreaker struct {
	mu      sync.Mutex
	entries map[string]*circuitEntry
}

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		entries: make(map[string]*circuitEntry),
	}
}

// Allow returns true if the label's circuit permits a request.
func (cb *CircuitBreaker) Allow(label string, cfg CircuitBreakerConfig) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.getOrCreate(label)

	switch e.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(e.openedAt) >= cfg.OpenDuration {
			e.state = CircuitHalfOpen
			e.probeInFlight = true
			return true
		}
		return false
	case CircuitHalfOpen:
		return false
	}
	return false
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess(label string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.getOrCreate(label)
	e.failures = 0
	e.state = CircuitClosed
	e.probeInFlight = false
}

// RecordFailure records a failed request and may open the circuit.
func (cb *CircuitBreaker) RecordFailure(label string, cfg CircuitBreakerConfig) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e := cb.getOrCreate(label)
	e.failures++

	if e.state == CircuitHalfOpen || e.failures >= cfg.FailThreshold {
		e.state = CircuitOpen
		e.openedAt = time.Now()
		e.probeInFlight = false
	}
}

// State returns the current circuit state for a label.
func (cb *CircuitBreaker) State(label string) CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	e, ok := cb.entries[label]
	if !ok {
		return CircuitClosed
	}
	return e.state
}

// Reset clears all circuit state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.entries = make(map[string]*circuitEntry)
}

func (cb *CircuitBreaker) getOrCreate(label string) *circuitEntry {
	e, ok := cb.entries[label]
	if !ok {
		e = &circuitEntry{state: CircuitClosed}
		cb.entries[label] = e
	}
	return e
}

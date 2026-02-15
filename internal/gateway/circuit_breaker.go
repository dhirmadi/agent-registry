package gateway

import (
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Allow all requests
	CircuitOpen                         // Reject all requests
	CircuitHalfOpen                     // Allow one probe request
)

// CircuitBreakerConfig from MCP server circuit_breaker JSON.
type CircuitBreakerConfig struct {
	FailThreshold int           // Failures before opening (1-100)
	OpenDuration  time.Duration // How long to stay open (1-3600s)
}

// circuitState tracks per-label circuit state.
type circuitState struct {
	state        CircuitState
	failures     int
	lastFailTime time.Time
	openedAt     time.Time
}

// CircuitBreaker manages circuit breaker state for multiple labels.
type CircuitBreaker struct {
	mu       sync.RWMutex
	circuits map[string]*circuitState
}

// NewCircuitBreaker creates a new circuit breaker manager.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		circuits: make(map[string]*circuitState),
	}
}

// Allow checks if a request should be allowed for the given label.
func (cb *CircuitBreaker) Allow(label string, config CircuitBreakerConfig) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	circuit, exists := cb.circuits[label]
	if !exists {
		circuit = &circuitState{state: CircuitClosed}
		cb.circuits[label] = circuit
	}

	now := time.Now()

	switch circuit.state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		if now.Sub(circuit.openedAt) >= config.OpenDuration {
			circuit.state = CircuitHalfOpen
			return true // Allow probe
		}
		return false

	case CircuitHalfOpen:
		return false

	default:
		return false
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess(label string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	circuit, exists := cb.circuits[label]
	if !exists {
		return
	}

	circuit.failures = 0
	circuit.state = CircuitClosed
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure(label string, config CircuitBreakerConfig) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	circuit, exists := cb.circuits[label]
	if !exists {
		circuit = &circuitState{state: CircuitClosed}
		cb.circuits[label] = circuit
	}

	now := time.Now()
	circuit.failures++
	circuit.lastFailTime = now

	if circuit.failures >= config.FailThreshold {
		circuit.state = CircuitOpen
		circuit.openedAt = now
	} else if circuit.state == CircuitHalfOpen {
		circuit.state = CircuitOpen
		circuit.openedAt = now
	}
}

// State returns the current state for a label.
func (cb *CircuitBreaker) State(label string) CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	circuit, exists := cb.circuits[label]
	if !exists {
		return CircuitClosed
	}

	return circuit.state
}

// Reset clears all circuit state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.circuits = make(map[string]*circuitState)
}

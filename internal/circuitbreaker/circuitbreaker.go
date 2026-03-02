package circuitbreaker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	StateClosed   State = iota // Normal operation
	StateOpen                  // Tripped — rejecting calls
	StateHalfOpen              // Testing recovery
)

func (s State) String() string {
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

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	name         string
	maxFailures  int
	resetTimeout time.Duration
	state        State
	failures     int
	lastFailure  time.Time
	mu           sync.RWMutex
	logger       *slog.Logger
}

// New creates a new circuit breaker.
func New(name string, maxFailures int, resetTimeout time.Duration, logger *slog.Logger) *CircuitBreaker {
	return &CircuitBreaker{
		name:         name,
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        StateClosed,
		logger:       logger,
	}
}

// Execute runs the given function if the circuit breaker allows it.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	if !cb.canExecute() {
		return fmt.Errorf("circuit breaker %s is open", cb.name)
	}

	err := fn(ctx)
	if err != nil {
		cb.recordFailure()
		return err
	}

	cb.recordSuccess()
	return nil
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

func (cb *CircuitBreaker) canExecute() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailure) >= cb.resetTimeout {
			cb.state = StateHalfOpen
			cb.logger.Info("circuit breaker half-open", "name", cb.name)
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.maxFailures {
		cb.state = StateOpen
		cb.logger.Warn("circuit breaker opened",
			"name", cb.name,
			"failures", cb.failures,
			"threshold", cb.maxFailures,
		)
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.logger.Info("circuit breaker closed (recovered)", "name", cb.name)
	}

	cb.state = StateClosed
	cb.failures = 0
}

// KillSwitch provides a global emergency stop mechanism.
type KillSwitch struct {
	engaged atomic.Bool
	logger  *slog.Logger
}

// NewKillSwitch creates a new kill switch.
func NewKillSwitch(logger *slog.Logger) *KillSwitch {
	return &KillSwitch{logger: logger}
}

// Engage activates the kill switch — all operations should stop.
func (ks *KillSwitch) Engage() {
	ks.engaged.Store(true)
	ks.logger.Error("KILL SWITCH ENGAGED — all operations stopping")
}

// Disengage deactivates the kill switch.
func (ks *KillSwitch) Disengage() {
	ks.engaged.Store(false)
	ks.logger.Warn("kill switch disengaged — operations may resume")
}

// IsEngaged returns whether the kill switch is active.
func (ks *KillSwitch) IsEngaged() bool {
	return ks.engaged.Load()
}

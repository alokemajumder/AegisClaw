package circuitbreaker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestExecute_SucceedsInClosedState(t *testing.T) {
	cb := New("test", 3, 5*time.Second, newTestLogger())

	called := false
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called, "function should have been called")
	assert.Equal(t, StateClosed, cb.GetState())
}

func TestExecute_PropagatesErrorFromFunction(t *testing.T) {
	cb := New("test", 3, 5*time.Second, newTestLogger())
	expected := errors.New("something broke")

	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return expected
	})

	require.ErrorIs(t, err, expected)
	assert.Equal(t, StateClosed, cb.GetState(), "should still be closed after one failure")
}

func TestCircuitOpensAfterMaxFailures(t *testing.T) {
	maxFailures := 3
	cb := New("test", maxFailures, 5*time.Second, newTestLogger())
	fail := errors.New("fail")

	for i := 0; i < maxFailures; i++ {
		_ = cb.Execute(context.Background(), func(ctx context.Context) error {
			return fail
		})
	}

	assert.Equal(t, StateOpen, cb.GetState(), "circuit should be open after %d consecutive failures", maxFailures)
}

func TestOpenCircuitRejectsCallsImmediately(t *testing.T) {
	maxFailures := 2
	cb := New("test", maxFailures, 10*time.Second, newTestLogger())
	fail := errors.New("fail")

	// Trip the breaker.
	for i := 0; i < maxFailures; i++ {
		_ = cb.Execute(context.Background(), func(ctx context.Context) error {
			return fail
		})
	}
	require.Equal(t, StateOpen, cb.GetState())

	// Subsequent call should be rejected without invoking the function.
	called := false
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		called = true
		return nil
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "circuit breaker")
	assert.Contains(t, err.Error(), "is open")
	assert.False(t, called, "function must not be called when circuit is open")
}

func TestHalfOpenStateAllowsOneProbe(t *testing.T) {
	maxFailures := 1
	// Use a very short reset timeout so we can trigger half-open quickly.
	cb := New("test", maxFailures, 1*time.Millisecond, newTestLogger())

	// Trip the breaker.
	_ = cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})
	require.Equal(t, StateOpen, cb.GetState())

	// Wait for the reset timeout to expire.
	time.Sleep(5 * time.Millisecond)

	// Next call should be allowed (half-open probe).
	called := false
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		called = true
		return errors.New("still broken")
	})

	require.Error(t, err)
	assert.True(t, called, "probe function should be called in half-open state")
	// After the probe fails, the breaker should go back to open.
	assert.Equal(t, StateOpen, cb.GetState())
}

func TestSuccessInHalfOpenClosesCircuit(t *testing.T) {
	maxFailures := 1
	cb := New("test", maxFailures, 1*time.Millisecond, newTestLogger())

	// Trip the breaker.
	_ = cb.Execute(context.Background(), func(ctx context.Context) error {
		return errors.New("fail")
	})
	require.Equal(t, StateOpen, cb.GetState())

	// Wait for reset timeout.
	time.Sleep(5 * time.Millisecond)

	// Successful probe should close the circuit.
	err := cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})

	require.NoError(t, err)
	assert.Equal(t, StateClosed, cb.GetState(), "circuit should be closed after successful half-open probe")

	// Subsequent calls should work normally.
	err = cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	require.NoError(t, err)
}

func TestFailureCountResetsAfterSuccess(t *testing.T) {
	maxFailures := 3
	cb := New("test", maxFailures, 5*time.Second, newTestLogger())
	fail := errors.New("fail")

	// Accumulate failures just below the threshold.
	for i := 0; i < maxFailures-1; i++ {
		_ = cb.Execute(context.Background(), func(ctx context.Context) error {
			return fail
		})
	}
	assert.Equal(t, StateClosed, cb.GetState())

	// One success should reset the counter.
	_ = cb.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})

	// Now another round of (maxFailures - 1) failures should NOT open the circuit.
	for i := 0; i < maxFailures-1; i++ {
		_ = cb.Execute(context.Background(), func(ctx context.Context) error {
			return fail
		})
	}
	assert.Equal(t, StateClosed, cb.GetState(), "failure count should have been reset by the success")
}

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

// --- KillSwitch Tests ---

func TestKillSwitch_InitiallyDisengaged(t *testing.T) {
	ks := NewKillSwitch(newTestLogger())
	assert.False(t, ks.IsEngaged(), "kill switch should start disengaged")
}

func TestKillSwitch_Engage(t *testing.T) {
	ks := NewKillSwitch(newTestLogger())

	ks.Engage()
	assert.True(t, ks.IsEngaged(), "kill switch should be engaged after Engage()")
}

func TestKillSwitch_Disengage(t *testing.T) {
	ks := NewKillSwitch(newTestLogger())

	ks.Engage()
	require.True(t, ks.IsEngaged())

	ks.Disengage()
	assert.False(t, ks.IsEngaged(), "kill switch should be disengaged after Disengage()")
}

func TestKillSwitch_EngageDisengageCycle(t *testing.T) {
	ks := NewKillSwitch(newTestLogger())

	assert.False(t, ks.IsEngaged())

	ks.Engage()
	assert.True(t, ks.IsEngaged())

	ks.Disengage()
	assert.False(t, ks.IsEngaged())

	ks.Engage()
	assert.True(t, ks.IsEngaged())

	ks.Engage() // Double engage should be idempotent.
	assert.True(t, ks.IsEngaged())

	ks.Disengage()
	assert.False(t, ks.IsEngaged())

	ks.Disengage() // Double disengage should be idempotent.
	assert.False(t, ks.IsEngaged())
}

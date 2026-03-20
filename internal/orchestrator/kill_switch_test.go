package orchestrator

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKillSwitch_InitialState(t *testing.T) {
	ks := NewKillSwitch()
	assert.False(t, ks.IsEngaged(), "kill switch should be disengaged initially")
}

func TestKillSwitch_Engage(t *testing.T) {
	ks := NewKillSwitch()
	ks.Engage()
	assert.True(t, ks.IsEngaged(), "kill switch should be engaged after Engage()")
}

func TestKillSwitch_Disengage(t *testing.T) {
	ks := NewKillSwitch()
	ks.Engage()
	ks.Disengage()
	assert.False(t, ks.IsEngaged(), "kill switch should be disengaged after Disengage()")
}

func TestKillSwitch_DoubleEngage_Idempotent(t *testing.T) {
	ks := NewKillSwitch()
	ks.Engage()
	ks.Engage()
	assert.True(t, ks.IsEngaged(), "double engage should still be engaged")

	ks.Disengage()
	assert.False(t, ks.IsEngaged(), "single disengage should disengage after double engage")
}

func TestKillSwitch_DoubleDisengage_Idempotent(t *testing.T) {
	ks := NewKillSwitch()
	ks.Disengage()
	ks.Disengage()
	assert.False(t, ks.IsEngaged(), "double disengage on already-disengaged should stay disengaged")
}

func TestKillSwitch_ConcurrentAccess(t *testing.T) {
	ks := NewKillSwitch()
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 3) // engage + disengage + read goroutines

	// Concurrent engages
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ks.Engage()
		}()
	}

	// Concurrent disengages
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ks.Disengage()
		}()
	}

	// Concurrent reads (should not panic or race)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_ = ks.IsEngaged()
		}()
	}

	wg.Wait()
	// No assertion on final state — the point is no data race or panic.
}

func TestKillSwitch_EngageDisengageToggle(t *testing.T) {
	ks := NewKillSwitch()
	for i := 0; i < 50; i++ {
		ks.Engage()
		assert.True(t, ks.IsEngaged())
		ks.Disengage()
		assert.False(t, ks.IsEngaged())
	}
}

package scheduler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// weekdayPtr returns a pointer to a time.Weekday value.
func weekdayPtr(d time.Weekday) *time.Weekday {
	return &d
}

func TestIsInBlackout(t *testing.T) {
	// Save original defaultBlackouts and restore after tests.
	origBlackouts := defaultBlackouts
	t.Cleanup(func() { defaultBlackouts = origBlackouts })

	t.Run("time within same-day window returns true", func(t *testing.T) {
		defaultBlackouts = []BlackoutWindow{
			{StartHour: 2, EndHour: 6}, // 02:00-06:00 UTC every day
		}
		// 03:00 UTC should be in blackout
		ts := time.Date(2026, 3, 4, 3, 30, 0, 0, time.UTC)
		assert.True(t, IsInBlackout(ts))
	})

	t.Run("time outside same-day window returns false", func(t *testing.T) {
		defaultBlackouts = []BlackoutWindow{
			{StartHour: 2, EndHour: 6},
		}
		// 07:00 UTC should not be in blackout
		ts := time.Date(2026, 3, 4, 7, 0, 0, 0, time.UTC)
		assert.False(t, IsInBlackout(ts))
	})

	t.Run("time at start boundary is in blackout", func(t *testing.T) {
		defaultBlackouts = []BlackoutWindow{
			{StartHour: 2, EndHour: 6},
		}
		// Exactly 02:00 should be in blackout (>= StartHour)
		ts := time.Date(2026, 3, 4, 2, 0, 0, 0, time.UTC)
		assert.True(t, IsInBlackout(ts))
	})

	t.Run("time at end boundary is not in blackout", func(t *testing.T) {
		defaultBlackouts = []BlackoutWindow{
			{StartHour: 2, EndHour: 6},
		}
		// Exactly 06:00 should NOT be in blackout (< EndHour, not <=)
		ts := time.Date(2026, 3, 4, 6, 0, 0, 0, time.UTC)
		assert.False(t, IsInBlackout(ts))
	})

	t.Run("overnight window wraps past midnight", func(t *testing.T) {
		defaultBlackouts = []BlackoutWindow{
			{StartHour: 22, EndHour: 6}, // 22:00-06:00 UTC
		}

		tests := []struct {
			name     string
			hour     int
			expected bool
		}{
			{"23:00 is in blackout", 23, true},
			{"00:00 is in blackout", 0, true},
			{"03:00 is in blackout", 3, true},
			{"05:59 is in blackout", 5, true},
			{"06:00 is not in blackout", 6, false},
			{"12:00 is not in blackout", 12, false},
			{"21:00 is not in blackout", 21, false},
			{"22:00 is in blackout", 22, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ts := time.Date(2026, 3, 4, tt.hour, 0, 0, 0, time.UTC)
				assert.Equal(t, tt.expected, IsInBlackout(ts))
			})
		}
	})

	t.Run("empty blackout list returns false", func(t *testing.T) {
		defaultBlackouts = []BlackoutWindow{}
		ts := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
		assert.False(t, IsInBlackout(ts))
	})

	t.Run("nil blackout list returns false", func(t *testing.T) {
		defaultBlackouts = nil
		ts := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
		assert.False(t, IsInBlackout(ts))
	})

	t.Run("weekday-specific blackout matches correct day", func(t *testing.T) {
		defaultBlackouts = []BlackoutWindow{
			{Weekday: weekdayPtr(time.Tuesday), StartHour: 9, EndHour: 17},
		}
		// 2026-03-03 is a Tuesday
		tuesday := time.Date(2026, 3, 3, 12, 0, 0, 0, time.UTC)
		assert.True(t, IsInBlackout(tuesday))
	})

	t.Run("weekday-specific blackout skips wrong day", func(t *testing.T) {
		defaultBlackouts = []BlackoutWindow{
			{Weekday: weekdayPtr(time.Tuesday), StartHour: 9, EndHour: 17},
		}
		// 2026-03-04 is a Wednesday
		wednesday := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
		assert.False(t, IsInBlackout(wednesday))
	})

	t.Run("multiple blackout windows any match returns true", func(t *testing.T) {
		defaultBlackouts = []BlackoutWindow{
			{StartHour: 2, EndHour: 4},
			{StartHour: 14, EndHour: 16},
		}
		// 03:00 matches first window
		ts1 := time.Date(2026, 3, 4, 3, 0, 0, 0, time.UTC)
		assert.True(t, IsInBlackout(ts1))

		// 15:00 matches second window
		ts2 := time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)
		assert.True(t, IsInBlackout(ts2))

		// 10:00 matches neither
		ts3 := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
		assert.False(t, IsInBlackout(ts3))
	})

	t.Run("non-UTC time is converted to UTC", func(t *testing.T) {
		defaultBlackouts = []BlackoutWindow{
			{StartHour: 2, EndHour: 6},
		}
		// 10:00 in UTC+8 = 02:00 UTC, should be in blackout
		loc := time.FixedZone("UTC+8", 8*60*60)
		ts := time.Date(2026, 3, 4, 10, 0, 0, 0, loc)
		assert.True(t, IsInBlackout(ts))
	})
}

package scheduler

import "time"

// BlackoutWindow defines a recurring time window during which runs should not be triggered.
type BlackoutWindow struct {
	Weekday   *time.Weekday
	StartHour int // 0-23, UTC
	EndHour   int // 0-23, UTC
}

// defaultBlackouts defines the default blackout windows.
// For MVP, no default blackouts are configured. These can be loaded from
// engagement-level or org-level configuration in a future phase.
var defaultBlackouts []BlackoutWindow

// IsInBlackout checks whether the given time falls within any blackout window.
func IsInBlackout(t time.Time) bool {
	t = t.UTC()
	for _, bw := range defaultBlackouts {
		if bw.Weekday != nil && t.Weekday() != *bw.Weekday {
			continue
		}
		hour := t.Hour()
		if bw.StartHour <= bw.EndHour {
			// Same-day window (e.g., 02:00 to 06:00)
			if hour >= bw.StartHour && hour < bw.EndHour {
				return true
			}
		} else {
			// Overnight window (e.g., 22:00 to 06:00)
			if hour >= bw.StartHour || hour < bw.EndHour {
				return true
			}
		}
	}
	return false
}

package policy

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Engine enforces tier policies, allowlists, rate limits, and blackout windows.
type Engine struct {
	mu          sync.RWMutex
	rateCounts  map[string]*rateBucket
	globalRate  int
	globalConcurrencyCap int
}

type rateBucket struct {
	count     int
	resetAt   time.Time
}

// ValidationRequest describes a proposed action to validate against policy.
type ValidationRequest struct {
	EngagementID uuid.UUID
	RunID        uuid.UUID
	Tier         int
	TargetAssetID uuid.UUID
	TechniqueID  string
	AllowedTiers []int
	Allowlist    []uuid.UUID
	Exclusions   []uuid.UUID
	RateLimit    int
	RunWindowStart *time.Time
	RunWindowEnd   *time.Time
	BlackoutPeriods []BlackoutPeriod
}

// BlackoutPeriod defines a time window during which no actions are allowed.
type BlackoutPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ValidationResult is the outcome of a policy check.
type ValidationResult struct {
	Allowed bool
	Reason  string
}

// NewEngine creates a new policy engine.
func NewEngine(globalRate, globalConcurrencyCap int) *Engine {
	return &Engine{
		rateCounts:           make(map[string]*rateBucket),
		globalRate:           globalRate,
		globalConcurrencyCap: globalConcurrencyCap,
	}
}

// Validate checks a proposed action against all policy rules.
func (e *Engine) Validate(_ context.Context, req ValidationRequest) ValidationResult {
	// Check tier is allowed
	if !e.tierAllowed(req.Tier, req.AllowedTiers) {
		return ValidationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("tier %d is not in allowed tiers %v", req.Tier, req.AllowedTiers),
		}
	}

	// Tier 3 is always blocked
	if req.Tier >= 3 {
		return ValidationResult{
			Allowed: false,
			Reason:  "tier 3 actions are prohibited by default",
		}
	}

	// Check target is in allowlist (if allowlist is set)
	if len(req.Allowlist) > 0 && !e.inUUIDList(req.TargetAssetID, req.Allowlist) {
		return ValidationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("asset %s is not in target allowlist", req.TargetAssetID),
		}
	}

	// Check target is not in exclusions
	if e.inUUIDList(req.TargetAssetID, req.Exclusions) {
		return ValidationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("asset %s is in exclusion list", req.TargetAssetID),
		}
	}

	// Check blackout windows
	now := time.Now()
	for _, bp := range req.BlackoutPeriods {
		if now.After(bp.Start) && now.Before(bp.End) {
			return ValidationResult{
				Allowed: false,
				Reason:  fmt.Sprintf("currently in blackout period: %s to %s", bp.Start, bp.End),
			}
		}
	}

	// Check run window
	if req.RunWindowStart != nil && req.RunWindowEnd != nil {
		currentTime := now.Format("15:04:05")
		startTime := req.RunWindowStart.Format("15:04:05")
		endTime := req.RunWindowEnd.Format("15:04:05")
		if currentTime < startTime || currentTime > endTime {
			return ValidationResult{
				Allowed: false,
				Reason:  fmt.Sprintf("outside run window: %s - %s (current: %s)", startTime, endTime, currentTime),
			}
		}
	}

	// Check rate limits
	if !e.checkRate(req.EngagementID.String(), req.RateLimit) {
		return ValidationResult{
			Allowed: false,
			Reason:  fmt.Sprintf("rate limit exceeded for engagement %s (limit: %d/min)", req.EngagementID, req.RateLimit),
		}
	}

	return ValidationResult{Allowed: true}
}

// RequiresApproval returns true if the tier requires human approval.
func RequiresApproval(tier int) bool {
	return tier >= 2
}

func (e *Engine) tierAllowed(tier int, allowed []int) bool {
	for _, t := range allowed {
		if t == tier {
			return true
		}
	}
	return false
}

func (e *Engine) inUUIDList(id uuid.UUID, list []uuid.UUID) bool {
	for _, item := range list {
		if item == id {
			return true
		}
	}
	return false
}

func (e *Engine) checkRate(key string, limit int) bool {
	if limit <= 0 {
		return true
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	bucket, exists := e.rateCounts[key]
	if !exists || now.After(bucket.resetAt) {
		e.rateCounts[key] = &rateBucket{
			count:   1,
			resetAt: now.Add(time.Minute),
		}
		return true
	}

	if bucket.count >= limit {
		return false
	}

	bucket.count++
	return true
}

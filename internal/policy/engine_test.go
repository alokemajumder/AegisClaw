package policy

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_TierAllowedVsDenied(t *testing.T) {
	engine := NewEngine(100, 10)
	ctx := context.Background()

	tests := []struct {
		name         string
		tier         int
		allowedTiers []int
		wantAllowed  bool
	}{
		{
			name:         "tier 1 allowed when in list",
			tier:         1,
			allowedTiers: []int{0, 1, 2},
			wantAllowed:  true,
		},
		{
			name:         "tier 0 allowed when in list",
			tier:         0,
			allowedTiers: []int{0, 1},
			wantAllowed:  true,
		},
		{
			name:         "tier 2 denied when not in list",
			tier:         2,
			allowedTiers: []int{0, 1},
			wantAllowed:  false,
		},
		{
			name:         "tier 1 denied when list is empty",
			tier:         1,
			allowedTiers: []int{},
			wantAllowed:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ValidationRequest{
				EngagementID: uuid.New(),
				RunID:        uuid.New(),
				Tier:         tt.tier,
				TargetAssetID: uuid.New(),
				AllowedTiers: tt.allowedTiers,
				RateLimit:    0, // no rate limit
			}
			result := engine.Validate(ctx, req)
			assert.Equal(t, tt.wantAllowed, result.Allowed, "reason: %s", result.Reason)
		})
	}
}

func TestValidate_Tier3AlwaysBlocked(t *testing.T) {
	engine := NewEngine(100, 10)
	ctx := context.Background()

	tests := []struct {
		name string
		tier int
	}{
		{name: "tier 3 blocked", tier: 3},
		{name: "tier 4 blocked", tier: 4},
		{name: "tier 5 blocked", tier: 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ValidationRequest{
				EngagementID:  uuid.New(),
				RunID:         uuid.New(),
				Tier:          tt.tier,
				TargetAssetID: uuid.New(),
				AllowedTiers:  []int{0, 1, 2, 3, 4, 5}, // even if tier is in allowed list
				RateLimit:     0,
			}
			result := engine.Validate(ctx, req)
			assert.False(t, result.Allowed, "tier %d should always be blocked", tt.tier)
			assert.Contains(t, result.Reason, "tier 3 actions are prohibited")
		})
	}
}

func TestValidate_TargetInExclusionListBlocked(t *testing.T) {
	engine := NewEngine(100, 10)
	ctx := context.Background()

	excludedAsset := uuid.New()
	otherAsset := uuid.New()

	tests := []struct {
		name        string
		target      uuid.UUID
		exclusions  []uuid.UUID
		wantAllowed bool
	}{
		{
			name:        "target in exclusion list is blocked",
			target:      excludedAsset,
			exclusions:  []uuid.UUID{excludedAsset},
			wantAllowed: false,
		},
		{
			name:        "target not in exclusion list is allowed",
			target:      otherAsset,
			exclusions:  []uuid.UUID{excludedAsset},
			wantAllowed: true,
		},
		{
			name:        "empty exclusion list allows all",
			target:      excludedAsset,
			exclusions:  []uuid.UUID{},
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ValidationRequest{
				EngagementID:  uuid.New(),
				RunID:         uuid.New(),
				Tier:          1,
				TargetAssetID: tt.target,
				AllowedTiers:  []int{0, 1, 2},
				Exclusions:    tt.exclusions,
				RateLimit:     0,
			}
			result := engine.Validate(ctx, req)
			assert.Equal(t, tt.wantAllowed, result.Allowed, "reason: %s", result.Reason)
			if !tt.wantAllowed {
				assert.Contains(t, result.Reason, "exclusion list")
			}
		})
	}
}

func TestValidate_AllowlistEnforcement(t *testing.T) {
	engine := NewEngine(100, 10)
	ctx := context.Background()

	allowedAsset := uuid.New()
	otherAsset := uuid.New()

	tests := []struct {
		name        string
		target      uuid.UUID
		allowlist   []uuid.UUID
		wantAllowed bool
	}{
		{
			name:        "target in allowlist passes",
			target:      allowedAsset,
			allowlist:   []uuid.UUID{allowedAsset},
			wantAllowed: true,
		},
		{
			name:        "target not in allowlist blocked",
			target:      otherAsset,
			allowlist:   []uuid.UUID{allowedAsset},
			wantAllowed: false,
		},
		{
			name:        "empty allowlist allows all targets",
			target:      otherAsset,
			allowlist:   []uuid.UUID{},
			wantAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := ValidationRequest{
				EngagementID:  uuid.New(),
				RunID:         uuid.New(),
				Tier:          1,
				TargetAssetID: tt.target,
				AllowedTiers:  []int{0, 1, 2},
				Allowlist:     tt.allowlist,
				RateLimit:     0,
			}
			result := engine.Validate(ctx, req)
			assert.Equal(t, tt.wantAllowed, result.Allowed, "reason: %s", result.Reason)
		})
	}
}

func TestRequiresApproval(t *testing.T) {
	tests := []struct {
		name     string
		tier     int
		expected bool
	}{
		{name: "tier 0 does not require approval", tier: 0, expected: false},
		{name: "tier 1 does not require approval", tier: 1, expected: false},
		{name: "tier 2 requires approval", tier: 2, expected: true},
		{name: "tier 3 requires approval", tier: 3, expected: true},
		{name: "tier 4 requires approval", tier: 4, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RequiresApproval(tt.tier)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidate_RateLimit_UnderLimitPasses(t *testing.T) {
	engine := NewEngine(100, 10)
	ctx := context.Background()

	engagementID := uuid.New()
	req := ValidationRequest{
		EngagementID:  engagementID,
		RunID:         uuid.New(),
		Tier:          1,
		TargetAssetID: uuid.New(),
		AllowedTiers:  []int{0, 1, 2},
		RateLimit:     5,
	}

	// First 5 calls should all pass (rate limit is 5 per minute)
	for i := 0; i < 5; i++ {
		result := engine.Validate(ctx, req)
		require.True(t, result.Allowed, "call %d should be allowed, reason: %s", i+1, result.Reason)
	}
}

func TestValidate_RateLimit_OverLimitBlocked(t *testing.T) {
	engine := NewEngine(100, 10)
	ctx := context.Background()

	engagementID := uuid.New()
	req := ValidationRequest{
		EngagementID:  engagementID,
		RunID:         uuid.New(),
		Tier:          1,
		TargetAssetID: uuid.New(),
		AllowedTiers:  []int{0, 1, 2},
		RateLimit:     3,
	}

	// Exhaust the rate limit
	for i := 0; i < 3; i++ {
		result := engine.Validate(ctx, req)
		require.True(t, result.Allowed, "call %d should be allowed", i+1)
	}

	// The 4th call should be blocked
	result := engine.Validate(ctx, req)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "rate limit exceeded")
}

func TestValidate_RateLimit_ZeroMeansUnlimited(t *testing.T) {
	engine := NewEngine(100, 10)
	ctx := context.Background()

	req := ValidationRequest{
		EngagementID:  uuid.New(),
		RunID:         uuid.New(),
		Tier:          1,
		TargetAssetID: uuid.New(),
		AllowedTiers:  []int{0, 1, 2},
		RateLimit:     0, // no limit
	}

	// Should pass many times without hitting a limit
	for i := 0; i < 100; i++ {
		result := engine.Validate(ctx, req)
		require.True(t, result.Allowed, "call %d should be allowed with zero rate limit", i+1)
	}
}

func TestValidate_BlackoutPeriod_CurrentlyInBlackout(t *testing.T) {
	engine := NewEngine(100, 10)
	ctx := context.Background()

	now := time.Now()
	req := ValidationRequest{
		EngagementID:  uuid.New(),
		RunID:         uuid.New(),
		Tier:          1,
		TargetAssetID: uuid.New(),
		AllowedTiers:  []int{0, 1, 2},
		RateLimit:     0,
		BlackoutPeriods: []BlackoutPeriod{
			{
				Start: now.Add(-1 * time.Hour),
				End:   now.Add(1 * time.Hour),
			},
		},
	}

	result := engine.Validate(ctx, req)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "blackout period")
}

func TestValidate_BlackoutPeriod_OutsideBlackout(t *testing.T) {
	engine := NewEngine(100, 10)
	ctx := context.Background()

	now := time.Now()
	req := ValidationRequest{
		EngagementID:  uuid.New(),
		RunID:         uuid.New(),
		Tier:          1,
		TargetAssetID: uuid.New(),
		AllowedTiers:  []int{0, 1, 2},
		RateLimit:     0,
		BlackoutPeriods: []BlackoutPeriod{
			{
				Start: now.Add(-3 * time.Hour),
				End:   now.Add(-1 * time.Hour),
			},
		},
	}

	result := engine.Validate(ctx, req)
	assert.True(t, result.Allowed)
}

func TestValidate_BlackoutPeriod_MultipleWindows(t *testing.T) {
	engine := NewEngine(100, 10)
	ctx := context.Background()

	now := time.Now()
	req := ValidationRequest{
		EngagementID:  uuid.New(),
		RunID:         uuid.New(),
		Tier:          1,
		TargetAssetID: uuid.New(),
		AllowedTiers:  []int{0, 1, 2},
		RateLimit:     0,
		BlackoutPeriods: []BlackoutPeriod{
			{
				Start: now.Add(-5 * time.Hour),
				End:   now.Add(-3 * time.Hour), // past
			},
			{
				Start: now.Add(-30 * time.Minute),
				End:   now.Add(30 * time.Minute), // current
			},
		},
	}

	result := engine.Validate(ctx, req)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.Reason, "blackout period")
}

func TestValidate_FullyValidRequest(t *testing.T) {
	engine := NewEngine(100, 10)
	ctx := context.Background()

	targetAsset := uuid.New()
	req := ValidationRequest{
		EngagementID:  uuid.New(),
		RunID:         uuid.New(),
		Tier:          1,
		TargetAssetID: targetAsset,
		AllowedTiers:  []int{0, 1},
		Allowlist:     []uuid.UUID{targetAsset},
		Exclusions:    []uuid.UUID{uuid.New()},
		RateLimit:     10,
	}

	result := engine.Validate(ctx, req)
	assert.True(t, result.Allowed)
	assert.Empty(t, result.Reason)
}

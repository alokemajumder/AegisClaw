package receipt

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestReceipt() *RunReceipt {
	return &RunReceipt{
		RunID:        uuid.New(),
		EngagementID: uuid.New(),
		OrgID:        uuid.New(),
		Tier:         1,
		ScopeSnapshot: ScopeSnapshot{
			TargetAllowlist:   []uuid.UUID{uuid.New()},
			TargetExclusions:  []uuid.UUID{},
			AllowedTiers:      []int{0, 1},
			AllowedTechniques: []string{"T1059.001"},
			RateLimit:         10,
			ConcurrencyCap:    2,
		},
		Steps: []StepRecord{
			{
				StepNumber:  1,
				AgentType:   "red",
				Action:      "execute_technique",
				Tier:        1,
				Status:      "completed",
				StartedAt:   time.Now().Add(-5 * time.Minute).UTC(),
				CompletedAt: time.Now().Add(-4 * time.Minute).UTC(),
				EvidenceIDs: []string{"ev_001"},
				CleanupDone: true,
			},
		},
		StartedAt:   time.Now().Add(-10 * time.Minute).UTC(),
		CompletedAt: time.Now().UTC(),
		Outcome:     "completed",
		EvidenceManifest: []string{"ev_001"},
		ToolVersions: map[string]string{
			"runner": "0.1.0",
		},
	}
}

func TestGenerate_CreatesReceiptWithIDAndSignature(t *testing.T) {
	gen := NewGenerator([]byte("test-hmac-key-32-bytes-long!!!!!"))
	receipt := newTestReceipt()

	err := gen.Generate(receipt)
	require.NoError(t, err)

	assert.NotEmpty(t, receipt.ReceiptID, "ReceiptID should be populated")
	assert.True(t, len(receipt.ReceiptID) > 5, "ReceiptID should have meaningful length")
	assert.Contains(t, receipt.ReceiptID, "rcpt_", "ReceiptID should have rcpt_ prefix")

	assert.NotEmpty(t, receipt.Signature, "Signature should be populated")
	assert.Len(t, receipt.Signature, 64, "HMAC-SHA256 hex signature should be 64 chars")

	assert.False(t, receipt.GeneratedAt.IsZero(), "GeneratedAt should be set")
}

func TestVerify_SucceedsWithValidReceipt(t *testing.T) {
	gen := NewGenerator([]byte("test-hmac-key-32-bytes-long!!!!!"))
	receipt := newTestReceipt()

	err := gen.Generate(receipt)
	require.NoError(t, err)

	valid, err := gen.Verify(receipt)
	require.NoError(t, err)
	assert.True(t, valid, "valid receipt should verify successfully")
}

func TestVerify_FailsWithTamperedReceipt(t *testing.T) {
	gen := NewGenerator([]byte("test-hmac-key-32-bytes-long!!!!!"))

	tests := []struct {
		name   string
		tamper func(r *RunReceipt)
	}{
		{
			name: "tampered outcome",
			tamper: func(r *RunReceipt) {
				r.Outcome = "failed"
			},
		},
		{
			name: "tampered tier",
			tamper: func(r *RunReceipt) {
				r.Tier = 3
			},
		},
		{
			name: "tampered receipt ID",
			tamper: func(r *RunReceipt) {
				r.ReceiptID = "rcpt_tampered123"
			},
		},
		{
			name: "tampered step status",
			tamper: func(r *RunReceipt) {
				r.Steps[0].Status = "skipped"
			},
		},
		{
			name: "tampered evidence manifest",
			tamper: func(r *RunReceipt) {
				r.EvidenceManifest = append(r.EvidenceManifest, "ev_injected")
			},
		},
		{
			name: "tampered signature directly",
			tamper: func(r *RunReceipt) {
				r.Signature = "0000000000000000000000000000000000000000000000000000000000000000"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receipt := newTestReceipt()
			err := gen.Generate(receipt)
			require.NoError(t, err)

			tt.tamper(receipt)

			valid, err := gen.Verify(receipt)
			require.NoError(t, err)
			assert.False(t, valid, "tampered receipt should fail verification")
		})
	}
}

func TestRoundTrip_GenerateThenVerify(t *testing.T) {
	keys := [][]byte{
		[]byte("short-key"),
		[]byte("a-much-longer-key-that-exceeds-32-bytes-for-hmac"),
		[]byte("exact-32-bytes-key-for-hmac!!!!"),
	}

	for _, key := range keys {
		t.Run(string(key), func(t *testing.T) {
			gen := NewGenerator(key)
			receipt := newTestReceipt()

			err := gen.Generate(receipt)
			require.NoError(t, err)

			// Verify immediately after generation
			valid, err := gen.Verify(receipt)
			require.NoError(t, err)
			assert.True(t, valid, "round-trip should succeed")

			// Verify a second time (ensure Verify doesn't corrupt state)
			valid2, err := gen.Verify(receipt)
			require.NoError(t, err)
			assert.True(t, valid2, "second verification should also succeed")
		})
	}
}

func TestVerify_DifferentKeyFails(t *testing.T) {
	gen1 := NewGenerator([]byte("key-one-for-signing"))
	gen2 := NewGenerator([]byte("key-two-for-verification"))

	receipt := newTestReceipt()
	err := gen1.Generate(receipt)
	require.NoError(t, err)

	valid, err := gen2.Verify(receipt)
	require.NoError(t, err)
	assert.False(t, valid, "verification with different key should fail")
}

func TestGenerate_UniqueReceiptIDs(t *testing.T) {
	gen := NewGenerator([]byte("test-key"))

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		receipt := newTestReceipt()
		err := gen.Generate(receipt)
		require.NoError(t, err)
		assert.False(t, ids[receipt.ReceiptID], "receipt ID should be unique, got duplicate: %s", receipt.ReceiptID)
		ids[receipt.ReceiptID] = true
	}
}

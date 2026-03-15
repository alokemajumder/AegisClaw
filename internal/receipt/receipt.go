package receipt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// RunReceipt is an immutable, audit-grade record of a run.
type RunReceipt struct {
	ReceiptID    string            `json:"receipt_id"`
	RunID        uuid.UUID         `json:"run_id"`
	EngagementID uuid.UUID         `json:"engagement_id"`
	OrgID        uuid.UUID         `json:"org_id"`
	Tier         int               `json:"tier"`
	ScopeSnapshot ScopeSnapshot    `json:"scope_snapshot"`
	Steps        []StepRecord      `json:"steps"`
	StartedAt    time.Time         `json:"started_at"`
	CompletedAt  time.Time         `json:"completed_at"`
	Outcome      string            `json:"outcome"` // completed, failed, killed
	EvidenceManifest []string      `json:"evidence_manifest"`
	ToolVersions map[string]string `json:"tool_versions"`
	Signature    string            `json:"signature"` // HMAC-SHA256
	GeneratedAt  time.Time         `json:"generated_at"`
}

// ScopeSnapshot captures the exact scope at run time.
type ScopeSnapshot struct {
	TargetAllowlist  []uuid.UUID `json:"target_allowlist"`
	TargetExclusions []uuid.UUID `json:"target_exclusions"`
	AllowedTiers     []int       `json:"allowed_tiers"`
	AllowedTechniques []string   `json:"allowed_techniques"`
	RateLimit        int         `json:"rate_limit"`
	ConcurrencyCap   int         `json:"concurrency_cap"`
}

// StepRecord captures a single step's execution.
type StepRecord struct {
	StepNumber   int             `json:"step_number"`
	AgentType    string          `json:"agent_type"`
	Action       string          `json:"action"`
	Tier         int             `json:"tier"`
	Status       string          `json:"status"`
	StartedAt    time.Time       `json:"started_at"`
	CompletedAt  time.Time       `json:"completed_at"`
	EvidenceIDs  []string        `json:"evidence_ids"`
	CleanupDone  bool            `json:"cleanup_done"`
	ErrorMessage string          `json:"error_message,omitempty"`
}

// Generator creates and signs run receipts.
type Generator struct {
	hmacKey []byte
}

// NewGenerator creates a new receipt generator with the given signing key.
func NewGenerator(hmacKey []byte) *Generator {
	return &Generator{hmacKey: hmacKey}
}

// Generate creates a new signed receipt for a completed run.
func (g *Generator) Generate(receipt *RunReceipt) error {
	receipt.ReceiptID = fmt.Sprintf("rcpt_%s", uuid.New().String()[:12])
	receipt.GeneratedAt = time.Now().UTC()

	sig, err := g.sign(receipt)
	if err != nil {
		return fmt.Errorf("signing receipt: %w", err)
	}
	receipt.Signature = sig

	return nil
}

// Verify checks the HMAC signature of a receipt.
func (g *Generator) Verify(receipt *RunReceipt) (bool, error) {
	savedSig := receipt.Signature
	receipt.Signature = ""

	expected, err := g.sign(receipt)
	if err != nil {
		return false, fmt.Errorf("computing signature: %w", err)
	}

	receipt.Signature = savedSig
	return hmac.Equal([]byte(savedSig), []byte(expected)), nil
}

func (g *Generator) sign(receipt *RunReceipt) (string, error) {
	// Serialize without the signature field for signing.
	// Go's json.Marshal produces deterministic key ordering (alphabetical by field
	// name in structs), so this is safe for HMAC comparison across runs.
	savedSig := receipt.Signature
	receipt.Signature = ""
	data, err := json.Marshal(receipt)
	receipt.Signature = savedSig
	if err != nil {
		return "", fmt.Errorf("marshaling receipt for signing: %w", err)
	}

	mac := hmac.New(sha256.New, g.hmacKey)
	if _, err := mac.Write(data); err != nil {
		return "", fmt.Errorf("writing to HMAC: %w", err)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

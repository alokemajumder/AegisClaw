package models

import (
	"encoding/json"
	"net"
	"time"

	"github.com/google/uuid"
)

// Organization represents a tenant organization.
type Organization struct {
	ID        uuid.UUID       `json:"id" db:"id"`
	Name      string          `json:"name" db:"name"`
	Settings  json.RawMessage `json:"settings" db:"settings"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}

// UserRole defines the RBAC roles.
type UserRole string

const (
	RoleAdmin    UserRole = "admin"
	RoleOperator UserRole = "operator"
	RoleViewer   UserRole = "viewer"
	RoleApprover UserRole = "approver"
)

// User represents a platform user.
type User struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	OrgID        uuid.UUID       `json:"org_id" db:"org_id"`
	Email        string          `json:"email" db:"email"`
	Name         string          `json:"name" db:"name"`
	PasswordHash *string         `json:"-" db:"password_hash"`
	Role         UserRole        `json:"role" db:"role"`
	SSOSubject   *string         `json:"sso_subject,omitempty" db:"sso_subject"`
	Settings     json.RawMessage `json:"settings" db:"settings"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}

// AssetType defines the kinds of assets that can be tracked.
type AssetType string

const (
	AssetEndpoint     AssetType = "endpoint"
	AssetServer       AssetType = "server"
	AssetApplication  AssetType = "application"
	AssetIdentity     AssetType = "identity"
	AssetCloudAccount AssetType = "cloud_account"
	AssetK8sCluster   AssetType = "k8s_cluster"
)

// Criticality levels for assets.
type Criticality string

const (
	CriticalityCritical Criticality = "critical"
	CriticalityHigh     Criticality = "high"
	CriticalityMedium   Criticality = "medium"
	CriticalityLow      Criticality = "low"
)

// Environment tags for assets.
type Environment string

const (
	EnvProduction  Environment = "production"
	EnvStaging     Environment = "staging"
	EnvLab         Environment = "lab"
	EnvDevelopment Environment = "development"
)

// Asset represents a target asset in the inventory.
type Asset struct {
	ID              uuid.UUID       `json:"id" db:"id"`
	OrgID           uuid.UUID       `json:"org_id" db:"org_id"`
	Name            string          `json:"name" db:"name"`
	AssetType       AssetType       `json:"asset_type" db:"asset_type"`
	Metadata        json.RawMessage `json:"metadata" db:"metadata"`
	Owner           *string         `json:"owner,omitempty" db:"owner"`
	Criticality     *Criticality    `json:"criticality,omitempty" db:"criticality"`
	Environment     *Environment    `json:"environment,omitempty" db:"environment"`
	BusinessService *string         `json:"business_service,omitempty" db:"business_service"`
	Tags            []string        `json:"tags" db:"tags"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at" db:"updated_at"`
}

// ConnectorCategory for grouping connector types.
type ConnectorCategory string

const (
	ConnectorCategorySIEM         ConnectorCategory = "siem"
	ConnectorCategoryEDR          ConnectorCategory = "edr"
	ConnectorCategoryITSM         ConnectorCategory = "itsm"
	ConnectorCategoryIdentity     ConnectorCategory = "identity"
	ConnectorCategoryNotification ConnectorCategory = "notification"
	ConnectorCategoryCloud        ConnectorCategory = "cloud"
)

// ConnectorRegistryEntry defines an available connector type.
type ConnectorRegistryEntry struct {
	ConnectorType string            `json:"connector_type" db:"connector_type"`
	Category      ConnectorCategory `json:"category" db:"category"`
	DisplayName   string            `json:"display_name" db:"display_name"`
	Description   *string           `json:"description,omitempty" db:"description"`
	Version       string            `json:"version" db:"version"`
	ConfigSchema  json.RawMessage   `json:"config_schema" db:"config_schema"`
	Capabilities  []string          `json:"capabilities" db:"capabilities"`
	Status        string            `json:"status" db:"status"`
	CreatedAt     time.Time         `json:"created_at" db:"created_at"`
}

// ConnectorInstance represents a configured connector integration.
type ConnectorInstance struct {
	ID              uuid.UUID         `json:"id" db:"id"`
	OrgID           uuid.UUID         `json:"org_id" db:"org_id"`
	ConnectorType   string            `json:"connector_type" db:"connector_type"`
	Category        ConnectorCategory `json:"category" db:"category"`
	Name            string            `json:"name" db:"name"`
	Description     *string           `json:"description,omitempty" db:"description"`
	Enabled         bool              `json:"enabled" db:"enabled"`
	Config          json.RawMessage   `json:"config" db:"config"`
	SecretRef       *string           `json:"secret_ref,omitempty" db:"secret_ref"`
	AuthMethod      string            `json:"auth_method" db:"auth_method"`
	HealthStatus    string            `json:"health_status" db:"health_status"`
	HealthCheckedAt *time.Time        `json:"health_checked_at,omitempty" db:"health_checked_at"`
	RateLimitConfig json.RawMessage   `json:"rate_limit_config" db:"rate_limit_config"`
	RetryConfig     json.RawMessage   `json:"retry_config" db:"retry_config"`
	FieldMappings   json.RawMessage   `json:"field_mappings" db:"field_mappings"`
	CreatedAt       time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at" db:"updated_at"`
}

// EngagementStatus for validation programs.
type EngagementStatus string

const (
	EngagementDraft     EngagementStatus = "draft"
	EngagementActive    EngagementStatus = "active"
	EngagementPaused    EngagementStatus = "paused"
	EngagementCompleted EngagementStatus = "completed"
	EngagementArchived  EngagementStatus = "archived"
)

// Engagement represents a validation program / campaign.
type Engagement struct {
	ID                uuid.UUID        `json:"id" db:"id"`
	OrgID             uuid.UUID        `json:"org_id" db:"org_id"`
	Name              string           `json:"name" db:"name"`
	Description       *string          `json:"description,omitempty" db:"description"`
	Status            EngagementStatus `json:"status" db:"status"`
	TargetAllowlist   []uuid.UUID      `json:"target_allowlist" db:"target_allowlist"`
	TargetExclusions  []uuid.UUID      `json:"target_exclusions" db:"target_exclusions"`
	AllowedTiers      []int            `json:"allowed_tiers" db:"allowed_tiers"`
	AllowedTechniques []string         `json:"allowed_techniques" db:"allowed_techniques"`
	ScheduleCron      *string          `json:"schedule_cron,omitempty" db:"schedule_cron"`
	RunWindowStart    *string          `json:"run_window_start,omitempty" db:"run_window_start"`
	RunWindowEnd      *string          `json:"run_window_end,omitempty" db:"run_window_end"`
	BlackoutPeriods   json.RawMessage  `json:"blackout_periods" db:"blackout_periods"`
	RateLimit         int              `json:"rate_limit" db:"rate_limit"`
	ConcurrencyCap    int              `json:"concurrency_cap" db:"concurrency_cap"`
	ConnectorIDs      []uuid.UUID      `json:"connector_ids" db:"connector_ids"`
	CreatedBy         *uuid.UUID       `json:"created_by,omitempty" db:"created_by"`
	CreatedAt         time.Time        `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time        `json:"updated_at" db:"updated_at"`
}

// RunStatus for execution instances.
type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunPaused    RunStatus = "paused"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
	RunCancelled RunStatus = "cancelled"
	RunKilled    RunStatus = "killed"
)

// Run represents an individual execution instance.
type Run struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	EngagementID   uuid.UUID       `json:"engagement_id" db:"engagement_id"`
	OrgID          uuid.UUID       `json:"org_id" db:"org_id"`
	Status         RunStatus       `json:"status" db:"status"`
	Tier           int             `json:"tier" db:"tier"`
	StartedAt      *time.Time      `json:"started_at,omitempty" db:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	StepsTotal     int             `json:"steps_total" db:"steps_total"`
	StepsCompleted int             `json:"steps_completed" db:"steps_completed"`
	StepsFailed    int             `json:"steps_failed" db:"steps_failed"`
	ReceiptID      *string         `json:"receipt_id,omitempty" db:"receipt_id"`
	Metadata       json.RawMessage `json:"metadata" db:"metadata"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at" db:"updated_at"`
}

// RunStepStatus for individual step tracking.
type RunStepStatus string

const (
	StepPending   RunStepStatus = "pending"
	StepRunning   RunStepStatus = "running"
	StepCompleted RunStepStatus = "completed"
	StepFailed    RunStepStatus = "failed"
	StepSkipped   RunStepStatus = "skipped"
	StepBlocked   RunStepStatus = "blocked"
)

// RunStep represents an individual action within a run.
type RunStep struct {
	ID              uuid.UUID       `json:"id" db:"id"`
	RunID           uuid.UUID       `json:"run_id" db:"run_id"`
	StepNumber      int             `json:"step_number" db:"step_number"`
	AgentType       string          `json:"agent_type" db:"agent_type"`
	Action          string          `json:"action" db:"action"`
	Tier            int             `json:"tier" db:"tier"`
	Status          RunStepStatus   `json:"status" db:"status"`
	Inputs          json.RawMessage `json:"inputs" db:"inputs"`
	Outputs         json.RawMessage `json:"outputs" db:"outputs"`
	EvidenceIDs     []string        `json:"evidence_ids" db:"evidence_ids"`
	ErrorMessage    *string         `json:"error_message,omitempty" db:"error_message"`
	StartedAt       *time.Time      `json:"started_at,omitempty" db:"started_at"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty" db:"completed_at"`
	CleanupVerified bool            `json:"cleanup_verified" db:"cleanup_verified"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
}

// Severity for findings.
type Severity string

const (
	SeverityCritical      Severity = "critical"
	SeverityHigh          Severity = "high"
	SeverityMedium        Severity = "medium"
	SeverityLow           Severity = "low"
	SeverityInformational Severity = "informational"
)

// Confidence levels for findings.
type Confidence string

const (
	ConfidenceConfirmed Confidence = "confirmed"
	ConfidenceHigh      Confidence = "high"
	ConfidenceMedium    Confidence = "medium"
	ConfidenceLow       Confidence = "low"
)

// FindingStatus lifecycle states.
type FindingStatus string

const (
	FindingObserved     FindingStatus = "observed"
	FindingNeedsReview  FindingStatus = "needs_review"
	FindingConfirmed    FindingStatus = "confirmed"
	FindingTicketed     FindingStatus = "ticketed"
	FindingFixed        FindingStatus = "fixed"
	FindingRetested     FindingStatus = "retested"
	FindingClosed       FindingStatus = "closed"
	FindingAcceptedRisk FindingStatus = "accepted_risk"
)

// Finding represents a security finding.
type Finding struct {
	ID                 uuid.UUID       `json:"id" db:"id"`
	OrgID              uuid.UUID       `json:"org_id" db:"org_id"`
	RunID              *uuid.UUID      `json:"run_id,omitempty" db:"run_id"`
	RunStepID          *uuid.UUID      `json:"run_step_id,omitempty" db:"run_step_id"`
	Title              string          `json:"title" db:"title"`
	Description        *string         `json:"description,omitempty" db:"description"`
	Severity           Severity        `json:"severity" db:"severity"`
	Confidence         Confidence      `json:"confidence" db:"confidence"`
	Status             FindingStatus   `json:"status" db:"status"`
	AffectedAssets     []uuid.UUID     `json:"affected_assets" db:"affected_assets"`
	TechniqueIDs       []string        `json:"technique_ids" db:"technique_ids"`
	EvidenceIDs        []string        `json:"evidence_ids" db:"evidence_ids"`
	Remediation        *string         `json:"remediation,omitempty" db:"remediation"`
	TicketID           *string         `json:"ticket_id,omitempty" db:"ticket_id"`
	TicketConnectorID  *uuid.UUID      `json:"ticket_connector_id,omitempty" db:"ticket_connector_id"`
	RetestRunID        *uuid.UUID      `json:"retest_run_id,omitempty" db:"retest_run_id"`
	ClusterID          *uuid.UUID      `json:"cluster_id,omitempty" db:"cluster_id"`
	Metadata           json.RawMessage `json:"metadata" db:"metadata"`
	CreatedAt          time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at" db:"updated_at"`
}

// ApprovalStatus for approval workflow.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalDenied   ApprovalStatus = "denied"
	ApprovalExpired  ApprovalStatus = "expired"
)

// Approval represents an approval request.
type Approval struct {
	ID                uuid.UUID      `json:"id" db:"id"`
	OrgID             uuid.UUID      `json:"org_id" db:"org_id"`
	RequestType       string         `json:"request_type" db:"request_type"`
	RequestedBy       string         `json:"requested_by" db:"requested_by"`
	TargetEntityID    *uuid.UUID     `json:"target_entity_id,omitempty" db:"target_entity_id"`
	TargetEntityType  *string        `json:"target_entity_type,omitempty" db:"target_entity_type"`
	Description       string         `json:"description" db:"description"`
	Tier              *int           `json:"tier,omitempty" db:"tier"`
	Status            ApprovalStatus `json:"status" db:"status"`
	DecidedBy         *uuid.UUID     `json:"decided_by,omitempty" db:"decided_by"`
	DecisionRationale *string        `json:"decision_rationale,omitempty" db:"decision_rationale"`
	ExpiresAt         *time.Time     `json:"expires_at,omitempty" db:"expires_at"`
	DecidedAt         *time.Time     `json:"decided_at,omitempty" db:"decided_at"`
	CreatedAt         time.Time      `json:"created_at" db:"created_at"`
}

// AuditLog represents an immutable audit entry.
type AuditLog struct {
	ID           int64           `json:"id" db:"id"`
	OrgID        uuid.UUID       `json:"org_id" db:"org_id"`
	ActorType    string          `json:"actor_type" db:"actor_type"`
	ActorID      string          `json:"actor_id" db:"actor_id"`
	Action       string          `json:"action" db:"action"`
	ResourceType string          `json:"resource_type" db:"resource_type"`
	ResourceID   *string         `json:"resource_id,omitempty" db:"resource_id"`
	Details      json.RawMessage `json:"details" db:"details"`
	IPAddress    *net.IP         `json:"ip_address,omitempty" db:"ip_address"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
}

// CoverageEntry represents a coverage matrix cell.
type CoverageEntry struct {
	ID              uuid.UUID  `json:"id" db:"id"`
	OrgID           uuid.UUID  `json:"org_id" db:"org_id"`
	TechniqueID     string     `json:"technique_id" db:"technique_id"`
	AssetID         *uuid.UUID `json:"asset_id,omitempty" db:"asset_id"`
	TelemetrySource *string    `json:"telemetry_source,omitempty" db:"telemetry_source"`
	HasTelemetry    bool       `json:"has_telemetry" db:"has_telemetry"`
	HasDetection    bool       `json:"has_detection" db:"has_detection"`
	HasAlert        bool       `json:"has_alert" db:"has_alert"`
	LastValidatedAt *time.Time `json:"last_validated_at,omitempty" db:"last_validated_at"`
	LastRunID       *uuid.UUID `json:"last_run_id,omitempty" db:"last_run_id"`
	CreatedAt       time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at" db:"updated_at"`
}

// PolicyPack represents a configurable policy pack.
type PolicyPack struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	OrgID       uuid.UUID       `json:"org_id" db:"org_id"`
	Name        string          `json:"name" db:"name"`
	Description *string         `json:"description,omitempty" db:"description"`
	IsDefault   bool            `json:"is_default" db:"is_default"`
	Rules       json.RawMessage `json:"rules" db:"rules"`
	Version     int             `json:"version" db:"version"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// Report represents a generated report.
type Report struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	OrgID       uuid.UUID       `json:"org_id" db:"org_id"`
	Title       string          `json:"title" db:"title"`
	ReportType  string          `json:"report_type" db:"report_type"`
	Status      string          `json:"status" db:"status"`
	Format      string          `json:"format" db:"format"`
	StoragePath *string         `json:"storage_path,omitempty" db:"storage_path"`
	GeneratedBy *uuid.UUID      `json:"generated_by,omitempty" db:"generated_by"`
	Metadata    json.RawMessage `json:"metadata" db:"metadata"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// Tier constants for the governance model.
const (
	Tier0Passive    = 0 // Telemetry health, config posture — fully autonomous
	Tier1Benign     = 1 // Safe atomic tests — fully autonomous with cleanup
	Tier2Sensitive  = 2 // Auth flows, operational impact — requires approval
	Tier3Prohibited = 3 // DoS, exfil, destructive — blocked by default
)

// APIResponse is the standard JSON envelope for all API responses.
type APIResponse struct {
	Data  any        `json:"data,omitempty"`
	Error *APIError  `json:"error,omitempty"`
	Meta  *APIMeta   `json:"meta,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type APIMeta struct {
	Total   int `json:"total,omitempty"`
	Page    int `json:"page,omitempty"`
	PerPage int `json:"per_page,omitempty"`
}

// PaginationParams holds standard pagination parameters.
type PaginationParams struct {
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

func (p PaginationParams) Offset() int {
	return (p.Page - 1) * p.PerPage
}

func (p PaginationParams) Limit() int {
	return p.PerPage
}

func DefaultPagination() PaginationParams {
	return PaginationParams{Page: 1, PerPage: 50}
}

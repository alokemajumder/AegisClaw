package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alokemajumder/AegisClaw/internal/auth"
	connectorsvc "github.com/alokemajumder/AegisClaw/internal/connector"
	"github.com/alokemajumder/AegisClaw/internal/database/repository"
	"github.com/alokemajumder/AegisClaw/internal/evidence"
	"github.com/alokemajumder/AegisClaw/internal/models"
	natspkg "github.com/alokemajumder/AegisClaw/internal/nats"
	"github.com/alokemajumder/AegisClaw/internal/reporting"
)

// Handler holds all API dependencies.
type Handler struct {
	DB        *pgxpool.Pool
	TokenSvc  *auth.TokenService
	Publisher *natspkg.Publisher
	Logger    *slog.Logger

	// Repositories
	Orgs       *repository.OrganizationRepo
	Users      *repository.UserRepo
	Assets     *repository.AssetRepo
	ConnReg    *repository.ConnectorRegistryRepo
	ConnInst   *repository.ConnectorInstanceRepo
	Engagements *repository.EngagementRepo
	Runs       *repository.RunRepo
	RunSteps   *repository.RunStepRepo
	Findings   *repository.FindingRepo
	Approvals  *repository.ApprovalRepo
	AuditLogs  *repository.AuditLogRepo
	Coverage   *repository.CoverageRepo
	Policies   *repository.PolicyPackRepo
	Reports    *repository.ReportRepo

	// Connector service (optional — nil when not configured)
	ConnectorSvc *connectorsvc.Service

	// Reporting service and evidence store (optional — nil when not configured)
	ReportSvc     *reporting.Service
	EvidenceStore *evidence.Store

	// Login lockout store (optional — nil disables lockout)
	LockoutStore LoginLockoutStore

	// NATS client (optional — nil when not configured)
	NATSClient *natspkg.Client

	// Kill switch state
	killSwitchMu      sync.RWMutex
	killSwitchEngaged bool
}

// NewHandler creates a Handler with all repositories initialized.
// It also loads the persisted kill switch state from the audit log.
func NewHandler(pool *pgxpool.Pool, tokenSvc *auth.TokenService, publisher *natspkg.Publisher, logger *slog.Logger) *Handler {
	auditLogs := repository.NewAuditLogRepo(pool)

	h := &Handler{
		DB:          pool,
		TokenSvc:    tokenSvc,
		Publisher:   publisher,
		Logger:      logger,
		Orgs:        repository.NewOrganizationRepo(pool),
		Users:       repository.NewUserRepo(pool),
		Assets:      repository.NewAssetRepo(pool),
		ConnReg:     repository.NewConnectorRegistryRepo(pool),
		ConnInst:    repository.NewConnectorInstanceRepo(pool),
		Engagements: repository.NewEngagementRepo(pool),
		Runs:        repository.NewRunRepo(pool),
		RunSteps:    repository.NewRunStepRepo(pool),
		Findings:    repository.NewFindingRepo(pool),
		Approvals:   repository.NewApprovalRepo(pool),
		AuditLogs:   auditLogs,
		Coverage:    repository.NewCoverageRepo(pool),
		Policies:    repository.NewPolicyPackRepo(pool),
		Reports:     repository.NewReportRepo(pool),
	}

	// Restore kill switch state from DB (survives restarts)
	engaged, err := auditLogs.GetLastKillSwitchState(context.Background())
	if err != nil {
		logger.Warn("failed to load kill switch state from DB, defaulting to disengaged", "error", err)
	} else if engaged {
		h.killSwitchEngaged = engaged
		logger.Warn("kill switch restored as ENGAGED from database")
	}

	return h
}

// IsKillSwitchEngaged returns true if the kill switch is active.
func (h *Handler) IsKillSwitchEngaged() bool {
	h.killSwitchMu.RLock()
	defer h.killSwitchMu.RUnlock()
	return h.killSwitchEngaged
}

// SetKillSwitch sets the kill switch state.
func (h *Handler) SetKillSwitch(engaged bool) {
	h.killSwitchMu.Lock()
	defer h.killSwitchMu.Unlock()
	h.killSwitchEngaged = engaged
}

// Helper functions

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

func writeData(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, models.APIResponse{Data: data})
}

func writeDataWithMeta(w http.ResponseWriter, data any, total, page, perPage int) {
	writeJSON(w, http.StatusOK, models.APIResponse{
		Data: data,
		Meta: &models.APIMeta{Total: total, Page: page, PerPage: perPage},
	})
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, models.APIResponse{
		Error: &models.APIError{Code: code, Message: message},
	})
}

func parseUUID(r *http.Request, param string) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, param))
}

func parsePagination(r *http.Request) models.PaginationParams {
	p := models.DefaultPagination()
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.Page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			p.PerPage = n
		}
	}
	return p
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func claimsFromRequest(r *http.Request) (*auth.Claims, bool) {
	return auth.UserFromContext(r.Context())
}

// audit writes an entry to the immutable audit log. Failures are logged as
// warnings but never fail the parent request.
func (h *Handler) audit(ctx context.Context, r *http.Request, claims *auth.Claims, action, resourceType string, resourceID *string, details json.RawMessage) {
	if details == nil {
		details = json.RawMessage(`{}`)
	}
	ip := parseIP(r.RemoteAddr)
	entry := &models.AuditLog{
		OrgID:        claims.OrgID,
		ActorType:    "user",
		ActorID:      claims.UserID.String(),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
		IPAddress:    ip,
	}
	if err := h.AuditLogs.Create(ctx, entry); err != nil {
		h.Logger.Warn("failed to write audit log",
			"action", action,
			"resource_type", resourceType,
			"error", err,
		)
	}
}

// parseIP extracts the IP portion from an address that may include a port.
func parseIP(remoteAddr string) *net.IP {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	return &ip
}

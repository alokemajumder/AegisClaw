package api

import (
	"encoding/json"
	"net/http"

	"github.com/alokemajumder/AegisClaw/internal/models"
)

type createAssetRequest struct {
	Name            string           `json:"name"`
	AssetType       string           `json:"asset_type"`
	Metadata        json.RawMessage  `json:"metadata"`
	Owner           *string          `json:"owner,omitempty"`
	Criticality     *string          `json:"criticality,omitempty"`
	Environment     *string          `json:"environment,omitempty"`
	BusinessService *string          `json:"business_service,omitempty"`
	Tags            []string         `json:"tags"`
}

func (h *Handler) ListAssets(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	p := parsePagination(r)
	assetType := r.URL.Query().Get("asset_type")

	assets, total, err := h.Assets.ListByOrgID(r.Context(), claims.OrgID, p, assetType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list assets")
		return
	}
	if assets == nil {
		assets = []models.Asset{}
	}
	writeDataWithMeta(w, assets, total, p.Page, p.PerPage)
}

func (h *Handler) CreateAsset(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)

	var req createAssetRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}
	if err := validateRequired(map[string]string{"name": req.Name, "asset_type": req.AssetType}); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	if err := validateAssetType(req.AssetType); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	metadata := req.Metadata
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}

	var crit *models.Criticality
	if req.Criticality != nil {
		c := models.Criticality(*req.Criticality)
		crit = &c
	}
	var env *models.Environment
	if req.Environment != nil {
		e := models.Environment(*req.Environment)
		env = &e
	}

	asset := &models.Asset{
		OrgID:           claims.OrgID,
		Name:            req.Name,
		AssetType:       models.AssetType(req.AssetType),
		Metadata:        metadata,
		Owner:           req.Owner,
		Criticality:     crit,
		Environment:     env,
		BusinessService: req.BusinessService,
		Tags:            req.Tags,
	}
	if asset.Tags == nil {
		asset.Tags = []string{}
	}

	if err := h.Assets.Create(r.Context(), asset); err != nil {
		h.Logger.Error("creating asset", "error", err)
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to create asset")
		return
	}

	resID := asset.ID.String()
	h.audit(r.Context(), r, claims, "asset.create", "asset", &resID, nil)

	writeJSON(w, http.StatusCreated, models.APIResponse{Data: asset})
}

func (h *Handler) GetAsset(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "assetID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid asset ID")
		return
	}

	asset, err := h.Assets.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Asset not found")
		return
	}
	if asset.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Asset not found")
		return
	}
	writeData(w, asset)
}

func (h *Handler) UpdateAsset(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "assetID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid asset ID")
		return
	}

	asset, err := h.Assets.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Asset not found")
		return
	}
	if asset.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Asset not found")
		return
	}

	var req createAssetRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if req.Name != "" {
		asset.Name = req.Name
	}
	if req.AssetType != "" {
		if err := validateAssetType(req.AssetType); err != nil {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		asset.AssetType = models.AssetType(req.AssetType)
	}
	if req.Metadata != nil {
		asset.Metadata = req.Metadata
	}
	if req.Owner != nil {
		asset.Owner = req.Owner
	}
	if req.Criticality != nil {
		c := models.Criticality(*req.Criticality)
		asset.Criticality = &c
	}
	if req.Environment != nil {
		e := models.Environment(*req.Environment)
		asset.Environment = &e
	}
	if req.BusinessService != nil {
		asset.BusinessService = req.BusinessService
	}
	if req.Tags != nil {
		asset.Tags = req.Tags
	}

	if err := h.Assets.Update(r.Context(), asset); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to update asset")
		return
	}

	resID := asset.ID.String()
	h.audit(r.Context(), r, claims, "asset.update", "asset", &resID, nil)

	writeData(w, asset)
}

func (h *Handler) DeleteAsset(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "assetID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid asset ID")
		return
	}

	asset, err := h.Assets.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Asset not found")
		return
	}
	if asset.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Asset not found")
		return
	}

	if err := h.Assets.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Asset not found")
		return
	}

	resID := id.String()
	h.audit(r.Context(), r, claims, "asset.delete", "asset", &resID, nil)

	writeData(w, map[string]string{"status": "deleted"})
}

func (h *Handler) ListAssetFindings(w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromRequest(r)
	id, err := parseUUID(r, "assetID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid asset ID")
		return
	}

	// Verify the asset belongs to the requesting org before listing its findings
	asset, err := h.Assets.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Asset not found")
		return
	}
	if asset.OrgID != claims.OrgID {
		writeError(w, http.StatusNotFound, "not_found", "Asset not found")
		return
	}

	p := parsePagination(r)

	findings, total, err := h.Findings.ListByAssetID(r.Context(), id, p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", "Failed to list findings")
		return
	}
	if findings == nil {
		findings = []models.Finding{}
	}
	writeDataWithMeta(w, findings, total, p.Page, p.PerPage)
}

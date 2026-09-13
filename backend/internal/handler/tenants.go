package handler

import (
	"github.com/gin-gonic/gin"

	"fihos/backend/internal/middleware"
	"fihos/backend/internal/response"
)

type createTenantReq struct {
	Name     string         `json:"name"`
	Slug     string         `json:"slug"`
	Branding map[string]any `json:"branding"`
}

func (h *H) PostTenant(c *gin.Context) {
	var req createTenantReq
	if !h.bind(c, &req) {
		return
	}
	if req.Name == "" || req.Slug == "" {
		response.ValidationFailed(c, gin.H{"name": "name and slug are required"})
		return
	}
	t, err := h.St.CreateTenant(c.Request.Context(), req.Name, req.Slug, req.Branding)
	if err != nil {
		h.fail(c, err, "")
		return
	}
	h.audit(c, "tenant.create", "tenant", &t.ID, gin.H{"name": t.Name})
	response.Ok(c, 201, t)
}

func (h *H) ListTenants(c *gin.Context) {
	page, size := h.page(c)
	items, total, err := h.St.ListTenants(c.Request.Context(), c.Query("status"), page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *H) GetTenant(c *gin.Context) {
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	t, err := h.St.TenantByID(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err, "Tenant not found")
		return
	}
	response.Ok(c, 200, t)
}

type updateTenantReq struct {
	Name     string         `json:"name"`
	Branding map[string]any `json:"branding"`
}

func (h *H) PatchTenant(c *gin.Context) {
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	var req updateTenantReq
	if !h.bind(c, &req) {
		return
	}
	if err := h.St.UpdateTenant(c.Request.Context(), id, req.Name, req.Branding); err != nil {
		h.fail(c, err, "Tenant not found")
		return
	}
	h.audit(c, "tenant.update", "tenant", &id, nil)
	t, _ := h.St.TenantByID(c.Request.Context(), id)
	response.Ok(c, 200, t)
}

type tenantStatusReq struct {
	Status string `json:"status"`
}

func (h *H) PatchTenantStatus(c *gin.Context) {
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	var req tenantStatusReq
	if !h.bind(c, &req) {
		return
	}
	if req.Status != "active" && req.Status != "suspended" && req.Status != "closed" {
		response.ValidationFailed(c, gin.H{"status": "must be one of active|suspended|closed"})
		return
	}
	if err := h.St.UpdateTenantStatus(c.Request.Context(), id, req.Status); err != nil {
		h.fail(c, err, "Tenant not found")
		return
	}
	h.audit(c, "tenant.status", "tenant", &id, gin.H{"status": req.Status})
	c.Status(204)
}

type settingsReq struct {
	PortalTitle   string `json:"portal_title"`
	PortalMessage string `json:"portal_message"`
}

func (h *H) GetSettings(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	settings, err := h.St.Settings(c.Request.Context(), tid)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Ok(c, 200, settings)
}

func (h *H) PutSettings(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	var req settingsReq
	if !h.bind(c, &req) {
		return
	}
	if err := h.St.UpsertSettings(c.Request.Context(), tid, req.PortalTitle, req.PortalMessage); err != nil {
		response.Internal(c, err)
		return
	}
	h.audit(c, "settings.update", "settings", nil, gin.H{"portal_title": req.PortalTitle})
	settings, _ := h.St.Settings(c.Request.Context(), tid)
	response.Ok(c, 200, settings)
}
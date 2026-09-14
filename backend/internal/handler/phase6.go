package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"fihos/backend/internal/middleware"
	"fihos/backend/internal/response"
	"fihos/backend/internal/store"
)

type rateWindowReq struct {
	HotspotID       int64          `json:"hotspot_id"`
	BoostMultiplier float64        `json:"boost_multiplier"`
	EffectiveFrom   time.Time      `json:"effective_from"`
	EffectiveUntil  time.Time      `json:"effective_until"`
	Repeat          map[string]any `json:"repeat"`
	Note            *string        `json:"note"`
}

// rateWindowUpsert handles create; scale-out push is deferred to the worker
// scheduler which re-applies on start and reverts on end.
func (h *H) PostRateWindow(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	var req rateWindowReq
	if !h.bind(c, &req) {
		return
	}
	if req.HotspotID == 0 || req.BoostMultiplier <= 1 {
		response.ValidationFailed(c, gin.H{"boost_multiplier": "must be > 1, hotspot_id required"})
		return
	}
	if req.EffectiveFrom.IsZero() || req.EffectiveUntil.IsZero() || !req.EffectiveUntil.After(req.EffectiveFrom) {
		response.ValidationFailed(c, gin.H{"effective_from": "window must have from < until"})
		return
	}
	uid := middleware.UserID(c)
	rw, err := h.St.CreateRateWindow(c.Request.Context(), &store.RateWindow{
		TenantID: tid, HotspotID: req.HotspotID, BoostMultiplier: req.BoostMultiplier,
		EffectiveFrom: req.EffectiveFrom, EffectiveUntil: req.EffectiveUntil,
		Repeat: req.Repeat, Note: req.Note, CreatedBy: &uid,
	})
	if err != nil {
		h.fail(c, err, "")
		return
	}
	h.audit(c, "rate_window.create", "rate_window", &rw.ID, gin.H{"multiplier": req.BoostMultiplier})
	response.Ok(c, 201, rw)
}

func (h *H) ListRateWindows(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	hsID, _ := store.ParseID(c.Query("hotspot_id"))
	upcoming := c.Query("upcoming") == "true"
	items, err := h.St.ListRateWindows(c.Request.Context(), tid, hsID, upcoming)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Ok(c, 200, items)
}

func (h *H) DeleteRateWindow(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	if err := h.St.DeleteRateWindow(c.Request.Context(), tid, id); err != nil {
		h.fail(c, err, "Rate window not found")
		return
	}
	h.audit(c, "rate_window.delete", "rate_window", &id, nil)
	c.Status(204)
}

func (h *H) ListAlerts(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	page, size := h.page(c)
	hsID, _ := store.ParseID(c.Query("hotspot_id"))
	items, total, err := h.St.ListAlerts(c.Request.Context(), tid, c.Query("status"), c.Query("severity"), hsID, page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

type alertStatusReq struct {
	Status string `json:"status"`
}

func (h *H) PatchAlertStatus(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	var req alertStatusReq
	if !h.bind(c, &req) {
		return
	}
	if req.Status != "acknowledged" && req.Status != "resolved" && req.Status != "dismissed" {
		response.ValidationFailed(c, gin.H{"status": "must be one of acknowledged|resolved|dismissed"})
		return
	}
	alert, err := h.St.UpdateAlertStatus(c.Request.Context(), tid, id, req.Status)
	if err != nil {
		h.fail(c, err, "Alert not found")
		return
	}
	h.audit(c, "alert.update", "alert", &id, gin.H{"status": req.Status})
	response.Ok(c, 200, alert)
}

func (h *H) ListConfigJobs(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	// Scope to tenant by joining pending jobs; worker marks done.
	items, err := h.St.PendingJobs(c.Request.Context(), 100)
	if err != nil {
		response.Internal(c, err)
		return
	}
	filtered := make([]store.ConfigJob, 0, len(items))
	for _, j := range items {
		if j.TenantID == tid {
			filtered = append(filtered, j)
		}
	}
	response.Ok(c, 200, filtered)
}
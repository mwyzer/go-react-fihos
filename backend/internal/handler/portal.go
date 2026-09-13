package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"fihos/backend/internal/mikrotik"
	"fihos/backend/internal/response"
	"fihos/backend/internal/service"
	"fihos/backend/internal/store"
)

// PortalGetHealth serves portal branding for a tenant slug.
func (h *H) PortalGetHealth(c *gin.Context) {
	slug := c.Param("slug")
	tenant, err := h.St.TenantBySlug(c.Request.Context(), slug)
	if err != nil {
		response.NotFound(c, "Portal not found")
		return
	}
	if tenant.Status != "active" {
		response.Error(c, 403, "tenant_suspended", "Portal is not active", nil)
		return
	}
	settings, _ := h.St.Settings(c.Request.Context(), tenant.ID)
	response.Ok(c, 200, gin.H{
		"tenant":  gin.H{"name": tenant.Name, "slug": tenant.Slug, "branding": tenant.Branding},
		"settings": settings,
	})
}

type portalRedeemReq struct {
	HotspotID int64  `json:"hotspot_id"`
	Code      string `json:"code"`
	MAC       string `json:"mac_address"`
}

func (h *H) PortalPostRedeem(c *gin.Context) {
	slug := c.Param("slug")
	tenant, err := h.St.TenantBySlug(c.Request.Context(), slug)
	if err != nil {
		response.NotFound(c, "Portal not found")
		return
	}
	if tenant.Status != "active" {
		response.Error(c, 403, "tenant_suspended", "Portal is not active", nil)
		return
	}
	var req portalRedeemReq
	if !h.bind(c, &req) {
		return
	}
	code, err := service.NormalizeCode(req.Code)
	if err != nil {
		response.Error(c, 422, "invalid_code", "Invalid voucher format", nil)
		return
	}
	voucher, err := h.St.VoucherByCode(c.Request.Context(), tenant.ID, code)
	if err != nil {
		response.Error(c, 404, "invalid_code", "Voucher not found", nil)
		return
	}
	if voucher.ExpiresAt != nil && voucher.ExpiresAt.Before(time.Now()) {
		response.Error(c, 409, "expired", "Voucher has expired", nil)
		return
	}
	switch voucher.Status {
	case "unused":
	case "redeemed":
		response.Error(c, 409, "already_used", "Voucher has already been used", gin.H{"voucher_id": voucher.ID})
		return
	case "revoked":
		response.Error(c, 409, "revoked", "Voucher has been revoked", nil)
		return
	case "expired":
		response.Error(c, 409, "expired", "Voucher has expired", nil)
		return
	}

	hotspot, err := h.St.HotspotByID(c.Request.Context(), tenant.ID, req.HotspotID)
	if err != nil {
		response.Error(c, 400, "hotspot_disabled", "Hotspot not found", nil)
		return
	}
	if hotspot.Status == "disabled" || hotspot.Status == "error" {
		response.Error(c, 409, "hotspot_disabled", "Hotspot is not currently available", nil)
		return
	}
	router, err := h.St.RouterByID(c.Request.Context(), tenant.ID, hotspot.RouterID)
	if err != nil {
		response.Internal(c, err)
		return
	}
	online, _, perr := h.Mt.ProbeWithTime(h.mtRouter(router))
	if perr != nil || !online {
		response.Error(c, 503, "router_offline", "Hotspot router is unreachable", nil)
		return
	}

	profile, err := h.St.ProfileByID(c.Request.Context(), tenant.ID, hotspot.ProfileID)
	if err != nil {
		response.Internal(c, err)
		return
	}

	// Paid vouchers are charged through the mock gateway.
	batch, err := h.St.BatchByID(c.Request.Context(), tenant.ID, voucher.BatchID)
	if err == nil && batch.Price != nil && *batch.Price > 0 {
		if _, err := h.Billing.Charge(c.Request.Context(), tenant.ID, voucher.ID, *batch.Price); err != nil {
			response.Internal(c, err)
			return
		}
	}

	username := service.SessionUsername(profile.ID, code)
	password, err := service.GenerateSessionPassword()
	if err != nil {
		response.Internal(c, err)
		return
	}
	sess, err := h.St.RedeemVoucher(c.Request.Context(), tenant.ID, voucher, hotspot.ID, &req.MAC, username, password)
	if err == store.ErrConflict {
		response.Error(c, 409, "already_used", "Voucher has already been used", nil)
		return
	}
	if err != nil {
		response.Internal(c, err)
		return
	}

	h.Mt.SeedSessions(h.mtRouter(router), []mikrotik.SimSession{{Username: username, MAC: req.MAC}})

	response.Ok(c, 201, gin.H{
		"session_id": sess.ID,
		"username":   username,
		"password":   password,
		"duration_min": batch.Duration,
		"wifi":       gin.H{"ssid": hotspot.Name, "ip_range": hotspot.IPRange},
	})
}

type portalStatusReq struct {
	Code string `json:"code"`
	MAC  string `json:"mac_address"`
}

// PortalPostStatus returns session status for the guest device's captive check.
func (h *H) PortalPostStatus(c *gin.Context) {
	slug := c.Param("slug")
	tenant, err := h.St.TenantBySlug(c.Request.Context(), slug)
	if err != nil {
		response.NotFound(c, "Portal not found")
		return
	}
	var req portalStatusReq
	if !h.bind(c, &req) {
		return
	}
	code, err := service.NormalizeCode(req.Code)
	if err != nil {
		response.Error(c, 422, "invalid_code", "Invalid voucher format", nil)
		return
	}
	voucher, err := h.St.VoucherByCode(c.Request.Context(), tenant.ID, code)
	if err != nil {
		response.Error(c, 404, "invalid_code", "Voucher not found", nil)
		return
	}
	if voucher.Status != "redeemed" || voucher.RedeemedAt == nil {
		response.Ok(c, 200, gin.H{"connected": false, "status": voucher.Status})
		return
	}
	sess, err := h.St.ActiveSessionByVoucher(c.Request.Context(), tenant.ID, voucher.ID)
	if err != nil {
		if err == store.ErrNotFound {
			response.Ok(c, 200, gin.H{"connected": false, "status": "expired"})
			return
		}
		response.Internal(c, err)
		return
	}
	response.Ok(c, 200, gin.H{
		"connected": true,
		"session_id": sess.ID,
		"bytes_rx":   sess.BytesRX,
		"bytes_tx":   sess.BytesTX,
		"started_at": sess.StartedAt,
		"remaining_min": int(profileRemainingMin(h, c, tenant.ID, sess)),
	})
}

func profileRemainingMin(h *H, c *gin.Context, tenantID int64, sess *store.Session) float64 {
	if sess.ProfileID == nil {
		return -1
	}
	p, err := h.St.ProfileByID(c.Request.Context(), tenantID, *sess.ProfileID)
	if err != nil || p.SessionUptimeLimit <= 0 {
		return -1
	}
	return float64(p.SessionUptimeLimit) - ctime(sess).Sub(sess.StartedAt).Minutes()
}

func ctime(s *store.Session) time.Time {
	if s.EndTime != nil {
		return *s.EndTime
	}
	return time.Now()
}
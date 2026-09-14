package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"fihos/backend/internal/response"
	"fihos/backend/internal/store"
)

func (h *H) GetDashboard(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	dash, err := h.St.Dashboard(c.Request.Context(), tid)
	if err != nil {
		response.Internal(c, err)
		return
	}
	ser, _ := h.St.HotspotTrafficSeries(c.Request.Context(), tid, 0, 14)
	top, _ := h.St.TopHotspots(c.Request.Context(), tid, time.Now().AddDate(0, 0, -7), 5)
	response.Ok(c, 200, gin.H{"dashboard": dash, "series": ser, "top": top})
}

func (h *H) GetTrafficSeries(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	days, _ := strconv.Atoi(c.DefaultQuery("days", "14"))
	if days < 1 || days > 90 {
		days = 14
	}
	hsID, _ := store.ParseID(c.Query("hotspot_id"))
	_ = h.St.RollupDailies(c.Request.Context(), tid, days)
	rows, err := h.St.ReadDailyMetrics(c.Request.Context(), tid, days)
	if err != nil {
		response.Internal(c, err)
		return
	}
	if hsID != 0 {
		rows, err = h.St.HotspotTrafficSeries(c.Request.Context(), tid, hsID, days)
		if err != nil {
			response.Internal(c, err)
			return
		}
	}
	response.Ok(c, 200, rows)
}

func (h *H) GetTopHotspots(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	since := time.Now().AddDate(0, 0, -7)
	top, err := h.St.TopHotspots(c.Request.Context(), tid, since, 10)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Ok(c, 200, top)
}

func (h *H) ListPayments(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	page, size := h.page(c)
	kind := c.Query("kind")
	if kind != "wallet_topup" && kind != "voucher" {
		kind = ""
	}
	items, total, err := h.St.ListPayments(c.Request.Context(), tid, c.Query("status"), kind, page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}
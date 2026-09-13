package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"fihos/backend/internal/middleware"
	"fihos/backend/internal/response"
	"fihos/backend/internal/store"
)

func (h *H) ListSessions(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	page, size := h.page(c)
	hsID, _ := store.ParseID(c.Query("hotspot_id"))
	items, total, err := h.St.ListSessions(c.Request.Context(), tid, c.Query("state"), hsID, c.Query("search"), page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

func (h *H) GetSession(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	sess, err := h.St.SessionByID(c.Request.Context(), tid, id)
	if err != nil {
		h.fail(c, err, "Session not found")
		return
	}
	h.audit(c, "session.view", "session", &id, nil)
	response.Ok(c, 200, sess)
}

func (h *H) ListSessionUsage(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	page, size := h.page(c)
	var from, to time.Time
	if v := c.Query("from"); v != "" {
		from, _ = time.Parse(time.RFC3339, v)
	}
	if v := c.Query("to"); v != "" {
		to, _ = time.Parse(time.RFC3339, v)
	}
	items, total, err := h.St.ListUsage(c.Request.Context(), tid, id, from, to, page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

// PostSessionDisconnect forces a session off the (simulated) router and
// closes it. The worker confirms through the router polling loop.
func (h *H) PostSessionDisconnect(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	sess, err := h.St.SessionByID(c.Request.Context(), tid, id)
	if err != nil {
		h.fail(c, err, "Session not found")
		return
	}
	if sess.State != "active" {
		response.Ok(c, 200, sess)
		return
	}
	hotspot, err := h.St.HotspotByID(c.Request.Context(), tid, sess.HotspotID)
	if err != nil {
		h.fail(c, err, "")
		return
	}
	router, err := h.St.RouterByID(c.Request.Context(), tid, hotspot.RouterID)
	if err != nil {
		h.fail(c, err, "")
		return
	}
	_ = h.Mt.Disconnect(h.mtRouter(router), sess.Username)
	closed, err := h.St.CloseSession(c.Request.Context(), id, time.Now(), sess.BytesRX, sess.BytesTX)
	if err != nil {
		h.fail(c, err, "Session not found")
		return
	}
	h.audit(c, "session.disconnect", "session", &id, gin.H{"mac": sess.MacAddress})
	response.Ok(c, 200, closed)
}
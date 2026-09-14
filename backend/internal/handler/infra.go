package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"fihos/backend/internal/middleware"
	"fihos/backend/internal/mikrotik"
	"fihos/backend/internal/response"
	"fihos/backend/internal/store"
)

type profileReq struct {
	Name               string `json:"name"`
	RxRate             int64  `json:"rx_rate"`
	TxRate             int64  `json:"tx_rate"`
	SessionUptimeLimit int    `json:"session_uptime_limit"`
	KeepaliveTimeout   int    `json:"keepalive_timeout"`
}

func (h *H) PostProfile(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	var req profileReq
	if !h.bind(c, &req) {
		return
	}
	if req.Name == "" || req.SessionUptimeLimit < 0 {
		response.ValidationFailed(c, gin.H{"name": "name required, session_uptime_limit >= 0"})
		return
	}
	uid := middleware.UserID(c)
	p, err := h.St.CreateProfile(c.Request.Context(), tid, &store.Profile{
		Name: req.Name, RxRate: req.RxRate, TxRate: req.TxRate,
		SessionUptimeLimit: req.SessionUptimeLimit, KeepaliveTimeout: req.KeepaliveTimeout, CreatedBy: &uid,
	})
	if err != nil {
		h.fail(c, err, "")
		return
	}
	h.audit(c, "profile.create", "profile", &p.ID, nil)
	response.Ok(c, 201, p)
}

func (h *H) ListProfiles(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	items, err := h.St.ListProfiles(c.Request.Context(), tid)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Ok(c, 200, items)
}

func (h *H) PatchProfile(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	var req profileReq
	if !h.bind(c, &req) {
		return
	}
	if req.Name == "" {
		response.ValidationFailed(c, gin.H{"name": "name is required"})
		return
	}
	if err := h.St.UpdateProfile(c.Request.Context(), tid, id, &store.Profile{
		Name: req.Name, RxRate: req.RxRate, TxRate: req.TxRate,
		SessionUptimeLimit: req.SessionUptimeLimit, KeepaliveTimeout: req.KeepaliveTimeout,
	}); err != nil {
		h.fail(c, err, "Profile not found")
		return
	}
	h.audit(c, "profile.update", "profile", &id, nil)
	c.Status(204)
}

type routerReq struct {
	Name      string `json:"name"`
	IPAddress string `json:"ip_address"`
	APIPort   int    `json:"api_port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

func (h *H) mtRouter(r *store.Router) mikrotik.Router {
	return mikrotik.Router{ID: r.ID, Name: r.Name, IPAddress: r.IPAddress, APIPort: r.APIPort, Username: r.Username, Password: r.Password}
}

func (h *H) PostRouter(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	var req routerReq
	if !h.bind(c, &req) {
		return
	}
	if req.Name == "" || req.IPAddress == "" || req.Username == "" {
		response.ValidationFailed(c, gin.H{"name": "name, ip_address and username are required"})
		return
	}
	if req.APIPort == 0 {
		req.APIPort = 8728
	}
	r, err := h.St.CreateRouter(c.Request.Context(), tid, &store.Router{
		Name: req.Name, IPAddress: req.IPAddress, APIPort: req.APIPort, Username: req.Username, Password: req.Password,
	})
	if err != nil {
		h.fail(c, err, "")
		return
	}
	h.Sim.RegisterRouter(r.ID, r.Name)
	h.St.SetRouterStatus(c.Request.Context(), r.ID, "online")
	h.audit(c, "router.create", "router", &r.ID, nil)
	response.Ok(c, 201, r)
}

func (h *H) ListRouters(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	items, err := h.St.ListRouters(c.Request.Context(), tid, c.Query("status"))
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Ok(c, 200, items)
}

func (h *H) GetRouter(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	r, err := h.St.RouterByID(c.Request.Context(), tid, id)
	if err != nil {
		h.fail(c, err, "Router not found")
		return
	}
	response.Ok(c, 200, r)
}

func (h *H) PatchRouter(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	var req routerReq
	if !h.bind(c, &req) {
		return
	}
	if req.APIPort == 0 {
		req.APIPort = 8728
	}
	r, err := h.St.UpdateRouter(c.Request.Context(), tid, id, &store.Router{
		Name: req.Name, IPAddress: req.IPAddress, APIPort: req.APIPort, Username: req.Username, Password: req.Password,
	})
	if err != nil {
		h.fail(c, err, "Router not found")
		return
	}
	h.Sim.RegisterRouter(r.ID, r.Name)
	h.audit(c, "router.update", "router", &id, nil)
	response.Ok(c, 200, r)
}

func (h *H) DeleteRouter(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	removed, err := h.St.DeleteRouter(c.Request.Context(), tid, id)
	if err != nil {
		h.fail(c, err, "Router not found")
		return
	}
	if removed {
		h.Sim.UnregisterRouter(id)
	}
	h.audit(c, "router.delete", "router", &id, nil)
	c.Status(204)
}

// PostRouterProbe checks the simulated device and syncs DB status.
func (h *H) PostRouterProbe(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	r, err := h.St.RouterByID(c.Request.Context(), tid, id)
	if err != nil {
		h.fail(c, err, "Router not found")
		return
	}
	okNet, ts, err := h.Mt.ProbeWithTime(h.mtRouter(r))
	if err != nil {
		h.fail(c, err, "Router not found")
		return
	}
	if okNet {
		h.St.SetRouterOnline(c.Request.Context(), id)
		response.Ok(c, 200, gin.H{"status": "online", "last_seen_at": ts})
		return
	}
	h.St.SetRouterOffline(c.Request.Context(), id)
	response.Error(c, 503, "router_offline", "Router did not respond", gin.H{"last_probe": ts})
}

type simulateReq struct {
	Connected bool `json:"connected"`
}

// PostRouterSimulate toggles the simulated device state for demos/tests.
func (h *H) PostRouterSimulate(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	if _, err := h.St.RouterByID(c.Request.Context(), tid, id); err != nil {
		h.fail(c, err, "Router not found")
		return
	}
	var req simulateReq
	if !h.bind(c, &req) {
		return
	}
	if req.Connected {
		h.Sim.RegisterRouter(id, fmt.Sprintf("router-%d", id))
		h.St.SetRouterOnline(c.Request.Context(), id)
	} else {
		h.Sim.Reset(id)
		h.St.SetRouterOffline(c.Request.Context(), id)
	}
	h.audit(c, "router.simulate", "router", &id, gin.H{"connected": req.Connected})
	response.Ok(c, 200, gin.H{"status": boolStatus(req.Connected)})
}

func boolStatus(ok bool) string {
	if ok {
		return "online"
	}
	return "offline"
}

type hotspotReq struct {
	RouterID  int64  `json:"router_id"`
	ProfileID int64  `json:"profile_id"`
	Name      string `json:"name"`
	IPRange   string `json:"ip_range"`
}

func (h *H) PostHotspot(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	var req hotspotReq
	if !h.bind(c, &req) {
		return
	}
	if req.RouterID == 0 || req.ProfileID == 0 || req.Name == "" {
		response.ValidationFailed(c, gin.H{"name": "router_id, profile_id and name are required"})
		return
	}
	var ir *string
	if req.IPRange != "" {
		ir = &req.IPRange
	}
	hs, err := h.St.CreateHotspot(c.Request.Context(), tid, &store.Hotspot{
		RouterID: req.RouterID, ProfileID: req.ProfileID, Name: req.Name, IPRange: ir,
	})
	if err != nil {
		h.fail(c, err, "")
		return
	}
	_ = h.St.EnqueueJob(c.Request.Context(), tid, hs.ID, "apply", gin.H{"name": hs.Name})
	h.pushHotspotConfig(c.Request.Context(), tid, hs, 1)
	h.audit(c, "hotspot.create", "hotspot", &hs.ID, nil)
	response.Ok(c, 201, hs)
}

func (h *H) ListHotspots(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	rid, _ := store.ParseID(c.Query("router_id"))
	items, err := h.St.ListHotspots(c.Request.Context(), tid, rid, c.Query("status"))
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Ok(c, 200, items)
}

func (h *H) GetHotspot(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	hs, err := h.St.HotspotByID(c.Request.Context(), tid, id)
	if err != nil {
		h.fail(c, err, "Hotspot not found")
		return
	}
	response.Ok(c, 200, hs)
}

func (h *H) PatchHotspot(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	var req hotspotReq
	if !h.bind(c, &req) {
		return
	}
	var ir *string
	if req.IPRange != "" {
		ir = &req.IPRange
	}
	hs, err := h.St.UpdateHotspot(c.Request.Context(), tid, id, &store.Hotspot{
		Name: req.Name, RouterID: req.RouterID, ProfileID: req.ProfileID, IPRange: ir,
	})
	if err != nil {
		h.fail(c, err, "Hotspot not found")
		return
	}
	_ = h.St.EnqueueJob(c.Request.Context(), tid, hs.ID, "apply", gin.H{"name": hs.Name})
	h.pushHotspotConfig(c.Request.Context(), tid, hs, hs.AppliedMultiplier)
	h.audit(c, "hotspot.update", "hotspot", &id, nil)
	response.Ok(c, 200, hs)
}

type hotspotStatusReq struct {
	Status string `json:"status"`
}

func (h *H) PatchHotspotStatus(c *gin.Context) {
	tid := h.requireTenant(c)
	if tid == 0 {
		return
	}
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	var req hotspotStatusReq
	if !h.bind(c, &req) {
		return
	}
	if req.Status != "active" && req.Status != "disabled" {
		response.ValidationFailed(c, gin.H{"status": "must be one of active|disabled"})
		return
	}
	if err := h.St.SetHotspotStatus(c.Request.Context(), tid, id, req.Status); err != nil {
		h.fail(c, err, "Hotspot not found")
		return
	}
	h.audit(c, "hotspot.status", "hotspot", &id, gin.H{"status": req.Status})
	c.Status(204)
}

// pushHotspotConfig applies the profile+multiplier to the simulated device
// when the router is reachable, and marks the hotspot configured.
func (h *H) pushHotspotConfig(ctx context.Context, tid int64, hs *store.Hotspot, multiplier float64) {
	router, err := h.St.RouterByID(ctx, tid, hs.RouterID)
	if err != nil {
		return
	}
	profile, err := h.St.ProfileByID(ctx, tid, hs.ProfileID)
	if err != nil {
		return
	}
	mt := h.mtRouter(router)
	online, _, perr := h.Mt.ProbeWithTime(mt)
	if perr != nil || !online {
		return
	}
	if multiplier < 1 {
		multiplier = 1
	}
	_ = h.Mt.ApplyConfig(mt, mikrotik.HotspotConfig{
		Name:        hs.Name,
		RxRate:      int64(float64(profile.RxRate) * multiplier),
		TxRate:      int64(float64(profile.TxRate) * multiplier),
		UptimeLimit: profile.SessionUptimeLimit,
	})
	mkID := time.Now().Format("hs-20060102150405")
	_ = h.St.SetHotspotConfigured(ctx, hs.ID, mkID, multiplier)
}
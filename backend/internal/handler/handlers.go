package handler

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/redis/go-redis/v9"

	"fihos/backend/internal/auth"
	"fihos/backend/internal/middleware"
	"fihos/backend/internal/mikrotik"
	"fihos/backend/internal/response"
	"fihos/backend/internal/service"
	"fihos/backend/internal/store"
)

type H struct {
	St      *store.Store
	Rdb     *redis.Client
	Auth    *auth.Service
	Billing *service.Billing
	Anomaly *service.AnomalyEngine
	Sim     *mikrotik.Simulator
	Mt      *mikrotik.Client
}

func New(st *store.Store, rdb *redis.Client, a *auth.Service, billing *service.Billing, anomaly *service.AnomalyEngine, sim *mikrotik.Simulator, mt *mikrotik.Client) *H {
	return &H{St: st, Rdb: rdb, Auth: a, Billing: billing, Anomaly: anomaly, Sim: sim, Mt: mt}
}

// tenant resolves the caller's tenant id from the JWT (admin global users
// overlay the tenant via X-Tenant-ID header).
func (h *H) tenant(c *gin.Context) int64 {
	if tid, ok := middleware.TenantID(c); ok {
		return tid
	}
	if tid := c.GetHeader("X-Tenant-Id"); tid != "" {
		if id, err := store.ParseID(tid); err == nil {
			return id
		}
	}
	return 0
}

func (h *H) requireTenant(c *gin.Context) int64 {
	tid := h.tenant(c)
	if tid == 0 {
		response.Forbidden(c)
		return 0
	}
	return tid
}

func (h *H) bind(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		response.ValidationFailed(c, gin.H{"body": err.Error()})
		return false
	}
	return true
}

// fail maps store/protocol errors to HTTP responses.
func (h *H) fail(c *gin.Context, err error, notFoundMsg string) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		response.NotFound(c, notFoundMsg)
	case errors.Is(err, store.ErrConflict):
		response.Conflict(c, "conflict", "Resource already exists or is in use")
	case errors.Is(err, mikrotik.ErrRouterOffline):
		response.Error(c, 503, "router_offline", "Router is offline", nil)
	case errors.Is(err, mikrotik.ErrUnknownRouter):
		response.NotFound(c, "Router not found in network mesh")
	case errors.Is(err, redis.Nil):
		response.Error(c, 401, "invalid_grant", "Invalid or expired refresh token", nil)
	default:
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			response.Conflict(c, "conflict", "Resource already exists or is in use")
			return
		}
		response.Internal(c, err)
	}
}

func (h *H) page(c *gin.Context) (int, int) {
	return store.PageSize(c.Query("page"), c.Query("size"))
}

func (h *H) audit(c *gin.Context, action, entity string, id *int64, meta map[string]any) {
	var u *int64
	if uid := middleware.UserID(c); uid != 0 {
		u = &uid
	}
	var tid *int64
	if v := h.tenant(c); v != 0 {
		tid = &v
	}
	_ = h.St.Audit(c.Request.Context(), &store.AuditLog{
		TenantID: tid, UserID: u, Action: action, EntityType: &entity, EntityID: id, Meta: meta,
	})
}
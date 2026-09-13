package handler

import (
	"strings"

	"github.com/gin-gonic/gin"

	"fihos/backend/internal/auth"
	"fihos/backend/internal/middleware"
	"fihos/backend/internal/response"
	"fihos/backend/internal/store"
)

type createUserReq struct {
	TenantID int64  `json:"tenant_id"`
	Email    string `json:"email"`
	Password string `json:"password"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
	IsActive *bool  `json:"is_active"`
}

func (h *H) PostUser(c *gin.Context) {
	var req createUserReq
	if !h.bind(c, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Password == "" || len(req.Password) < 8 {
		response.ValidationFailed(c, gin.H{"email": "email and password (min 8) are required"})
		return
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}
	// Tenant-scoped callers create users inside their own tenant.
	tid := h.tenant(c)
	if tid != 0 {
		req.TenantID = tid
	}
	if req.Role == "" {
		req.Role = "staff"
	}
	if req.Role != "staff" && req.Role != "owner" && req.Role != "admin" {
		response.ValidationFailed(c, gin.H{"role": "must be one of staff|owner|admin"})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		response.Internal(c, err)
		return
	}
	u, err := h.St.CreateUser(c.Request.Context(), &store.User{
		TenantID: &req.TenantID, Email: email, FullName: req.FullName, Role: req.Role, IsActive: active,
	}, hash)
	if err != nil {
		h.fail(c, err, "")
		return
	}
	h.audit(c, "user.create", "user", &u.ID, gin.H{"email": u.Email, "role": u.Role})
	response.Ok(c, 201, u)
}

func (h *H) ListUsers(c *gin.Context) {
	page, size := h.page(c)
	var tenantFilter *int64
	if tid := h.tenant(c); tid != 0 {
		tenantFilter = &tid
	}
	items, total, err := h.St.ListUsers(c.Request.Context(), tenantFilter, page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}

type patchUserReq struct {
	FullName string `json:"full_name"`
	Role     string `json:"role"`
	IsActive *bool  `json:"is_active"`
	Email    string `json:"email"`
}

func (h *H) PatchUser(c *gin.Context) {
	id, ok := middleware.ParamID(c, "id")
	if !ok {
		return
	}
	var req patchUserReq
	if !h.bind(c, &req) {
		return
	}
	var tenantFilter *int64
	if tid := h.tenant(c); tid != 0 {
		tenantFilter = &tid
	}
	u, err := h.St.UserByID(c.Request.Context(), id)
	if err != nil {
		h.fail(c, err, "User not found")
		return
	}
	if tenantFilter != nil && (u.TenantID == nil || *u.TenantID != *tenantFilter) {
		response.Forbidden(c)
		return
	}
	if err := h.St.UpdateUser(c.Request.Context(), id, tenantFilter, req.FullName, req.Role, req.IsActive, req.Email); err != nil {
		h.fail(c, err, "User not found")
		return
	}
	if req.IsActive != nil && !*req.IsActive {
		h.revokeUserRefreshTokens(c.Request.Context(), id)
	}
	h.audit(c, "user.update", "user", &id, gin.H{"role": req.Role, "is_active": req.IsActive != nil && *req.IsActive})
	c.Status(204)
}

func (h *H) GetAudit(c *gin.Context) {
	page, size := h.page(c)
	var tenantFilter *int64
	if tid := h.tenant(c); tid != 0 {
		tenantFilter = &tid
	}
	items, total, err := h.St.ListAudit(c.Request.Context(), tenantFilter, c.Query("action"), nil, page, size)
	if err != nil {
		response.Internal(c, err)
		return
	}
	response.Paginated(c, items, total, page, size)
}
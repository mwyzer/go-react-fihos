package middleware

import (
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"

	"fihos/backend/internal/response"
)

const (
	ctxUserID   = "user_id"
	ctxTenantID = "tenant_id"
	ctxRole     = "role"
	ctxIsAdmin  = "is_admin"
)

type Claims struct {
	UserID   int64  `json:"uid"`
	TenantID *int64 `json:"tenant_id,omitempty"`
	Role     string `json:"role"`
	Exp      int64  `json:"exp"`
}

type AuthVerifier interface {
	Verify(token string) (*Claims, error)
	IsRoleAuthorized(role string) bool
}

func Auth(verifier AuthVerifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if len(header) < 8 || header[:7] != "Bearer " {
			response.Unauthorized(c)
			return
		}
		claims, err := verifier.Verify(header[7:])
		if err != nil {
			response.Unauthorized(c)
			return
		}
		if !verifier.IsRoleAuthorized(claims.Role) {
			response.Forbidden(c)
			return
		}
		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxRole, claims.Role)
		if claims.TenantID != nil {
			c.Set(ctxTenantID, *claims.TenantID)
		}
		c.Set(ctxIsAdmin, claims.Role == "admin")
		c.Next()
	}
}

func Roles(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !slices.Contains(roles, c.GetString(ctxRole)) {
			response.Forbidden(c)
			return
		}
		c.Next()
	}
}

func UserID(c *gin.Context) int64 {
	return c.GetInt64(ctxUserID)
}

func Role(c *gin.Context) string {
	return c.GetString(ctxRole)
}

func TenantID(c *gin.Context) (int64, bool) {
	v, ok := c.Get(ctxTenantID)
	if !ok {
		return 0, false
	}
	id, _ := v.(int64)
	return id, ok && id != 0
}

func RequireTenant(c *gin.Context) {
	_, ok := TenantID(c)
	if !ok {
		// Platform admins (no JWT tenant) may overlay one via header.
		if tid := c.GetHeader("X-Tenant-Id"); tid != "" {
			if id, err := strconv.ParseInt(tid, 10, 64); err == nil && id > 0 {
				c.Set(ctxTenantID, id)
				ok = true
			}
		}
	}
	if !ok {
		response.Forbidden(c)
		return
	}
	c.Next()
}

// ParseParamID parses an int64 path parameter.
func ParamID(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, name+" must be a positive integer", nil)
		return 0, false
	}
	return id, true
}
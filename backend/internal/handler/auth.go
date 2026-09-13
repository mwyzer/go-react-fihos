package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"fihos/backend/internal/auth"
	"fihos/backend/internal/middleware"
	"fihos/backend/internal/response"
	"fihos/backend/internal/store"
)

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *H) refreshKey(userID int64, jti string) string {
	return fmt.Sprintf("refresh:%d:%s", userID, jti)
}

// storeRefresh records a refresh token as active.
func (h *H) storeRefresh(ctx context.Context, userID int64, jti string) error {
	return h.Rdb.Set(ctx, h.refreshKey(userID, jti), "1", h.Auth.RefreshTTL()).Err()
}

const rotateLua = `
local old = redis.call('GET', KEYS[1])
if not old then return 0 end
redis.call('DEL', KEYS[1])
redis.call('SET', KEYS[2], '1', 'EX', ARGV[1])
return 1`

// rotateRefresh atomically consumes the old refresh token and activates the new one.
func (h *H) rotateRefresh(ctx context.Context, oldKey, newKey string) (bool, error) {
	res, err := h.Rdb.Eval(ctx, rotateLua, []string{oldKey, newKey}, int(h.Auth.RefreshTTL().Seconds())).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

func (h *H) PostLogin(c *gin.Context) {
	var req loginReq
	if !h.bind(c, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" || req.Password == "" {
		response.ValidationFailed(c, gin.H{"email": "email and password are required"})
		return
	}
	user, err := h.St.UserByEmail(c.Request.Context(), email)
	if err != nil {
		response.Error(c, 401, "invalid_credentials", "Email or password is incorrect", nil)
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		response.Error(c, 401, "invalid_credentials", "Email or password is incorrect", nil)
		return
	}
	if !user.IsActive {
		response.Forbidden(c)
		return
	}
	if user.TenantID != nil {
		tenant, err := h.St.TenantByID(c.Request.Context(), *user.TenantID)
		if err != nil {
			response.Internal(c, err)
			return
		}
		if tenant.Status == "suspended" {
			response.TenantSuspended(c)
			return
		}
	}

	pair, jti, err := h.Auth.Issue(auth.UserAccess{
		ID: user.ID, Email: user.Email, FullName: user.FullName, Role: user.Role, TenantID: user.TenantID,
	})
	if err != nil {
		response.Internal(c, err)
		return
	}
	if err := h.storeRefresh(c.Request.Context(), user.ID, jti); err != nil {
		response.Internal(c, err)
		return
	}
	_ = h.St.TouchLogin(c.Request.Context(), user.ID)
	h.audit(c, "user.login", "user", &user.ID, gin.H{"email": user.Email})
	response.Ok(c, 200, pair)
}

type refreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *H) PostRefresh(c *gin.Context) {
	var req refreshReq
	if !h.bind(c, &req) {
		return
	}
	info, err := h.Auth.VerifyRefresh(req.RefreshToken)
	if err != nil {
		response.Error(c, 401, "invalid_grant", "Invalid or expired refresh token", nil)
		return
	}
	user, err := h.St.UserByID(c.Request.Context(), info.Claims.UserID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !user.IsActive) {
		response.Error(c, 401, "invalid_grant", "Account is no longer active", nil)
		return
	}
	if err != nil {
		response.Internal(c, err)
		return
	}
	if user.TenantID != nil {
		tenant, err := h.St.TenantByID(c.Request.Context(), *user.TenantID)
		if err == nil && tenant.Status == "suspended" {
			response.TenantSuspended(c)
			return
		}
	}

	newPair, newJTI, err := h.Auth.Issue(auth.UserAccess{
		ID: user.ID, Email: user.Email, FullName: user.FullName, Role: user.Role, TenantID: user.TenantID,
	})
	if err != nil {
		response.Internal(c, err)
		return
	}
	ok, err := h.rotateRefresh(c.Request.Context(), h.refreshKey(user.ID, info.JTI), h.refreshKey(user.ID, newJTI))
	if err != nil {
		response.Internal(c, err)
		return
	}
	if !ok {
		// Reuse or expiry: revoke this user's refresh tokens.
		h.revokeUserRefreshTokens(c.Request.Context(), user.ID)
		response.Error(c, 401, "invalid_grant", "Refresh token already used; all sessions revoked", nil)
		return
	}
	response.Ok(c, 200, newPair)
}

func (h *H) PostLogout(c *gin.Context) {
	uid := middleware.UserID(c)
	if c.Request.ContentLength > 0 {
		var req refreshReq
		if err := c.ShouldBindJSON(&req); err == nil && req.RefreshToken != "" {
			if info, err := h.Auth.VerifyRefresh(req.RefreshToken); err == nil && info.Claims.UserID == uid {
				_ = h.Rdb.Del(c.Request.Context(), h.refreshKey(uid, info.JTI)).Err()
			}
		}
	}
	h.audit(c, "user.logout", "user", &uid, nil)
	c.Status(204)
}

func (h *H) revokeUserRefreshTokens(ctx context.Context, userID int64) {
	iter := h.Rdb.Scan(ctx, 0, fmt.Sprintf("refresh:%d:*", userID), 100).Iterator()
	for iter.Next(ctx) {
		_ = h.Rdb.Del(ctx, iter.Val()).Err()
	}
}

type changePasswordReq struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h *H) PostChangePassword(c *gin.Context) {
	var req changePasswordReq
	if !h.bind(c, &req) {
		return
	}
	uid := middleware.UserID(c)
	user, err := h.St.UserByID(c.Request.Context(), uid)
	if err != nil {
		h.fail(c, err, "User not found")
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.CurrentPassword) {
		response.ValidationFailed(c, gin.H{"current_password": "Current password is incorrect"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		response.Internal(c, err)
		return
	}
	if err := h.St.SetPassword(c.Request.Context(), uid, hash); err != nil {
		response.Internal(c, err)
		return
	}
	h.revokeUserRefreshTokens(c.Request.Context(), uid)
	h.audit(c, "user.change_password", "user", &uid, nil)
	c.Status(204)
}

type resetPasswordReq struct {
	UserID      int64  `json:"user_id"`
	NewPassword string `json:"new_password"`
}

func (h *H) PostResetPassword(c *gin.Context) {
	var req resetPasswordReq
	if !h.bind(c, &req) {
		return
	}
	if len(req.NewPassword) < 8 {
		response.ValidationFailed(c, gin.H{"new_password": "min 8 characters"})
		return
	}
	target, err := h.St.UserByID(c.Request.Context(), req.UserID)
	if err != nil {
		h.fail(c, err, "User not found")
		return
	}
	if target.TenantID != nil {
		tid := *target.TenantID
		caller := h.tenant(c)
		if caller != 0 && caller != tid {
			response.Forbidden(c)
			return
		}
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		response.Internal(c, err)
		return
	}
	if err := h.St.SetPassword(c.Request.Context(), req.UserID, hash); err != nil {
		response.Internal(c, err)
		return
	}
	h.revokeUserRefreshTokens(c.Request.Context(), req.UserID)
	h.audit(c, "user.reset_password", "user", &req.UserID, nil)
	c.Status(204)
}

func (h *H) GetMe(c *gin.Context) {
	uid := middleware.UserID(c)
	user, err := h.St.UserByID(c.Request.Context(), uid)
	if err != nil {
		h.fail(c, err, "User not found")
		return
	}
	var tenant *store.Tenant
	if user.TenantID != nil {
		tenant, _ = h.St.TenantByID(c.Request.Context(), *user.TenantID)
	}
	response.Ok(c, 200, gin.H{"user": user, "tenant": tenant})
}
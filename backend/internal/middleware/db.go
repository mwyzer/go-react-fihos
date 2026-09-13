package middleware

import (
	"errors"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"fihos/backend/internal/response"
	"fihos/backend/internal/store"
)

const ctxAuthUser = "auth_user"

// AuthDB rehydrates the authenticated user and tenant from the database on
// every request so deactivation and suspension apply immediately.
func AuthDB(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := UserID(c)
		user, err := st.UserByID(c.Request.Context(), uid)
		if err != nil || !user.IsActive {
			if errors.Is(err, store.ErrNotFound) {
				response.Unauthorized(c)
				return
			}
			response.Forbidden(c)
			return
		}
		c.Set(ctxAuthUser, *user)

		if tid, ok := TenantID(c); ok {
			tenant, err := st.TenantByID(c.Request.Context(), tid)
			if err != nil {
				response.Unauthorized(c)
				return
			}
			if tenant.Status == "suspended" {
				response.TenantSuspended(c)
				return
			}
		}
		c.Next()
	}
}

// AuthUser returns the user loaded by AuthDB, if present.
func AuthUser(c *gin.Context) (store.User, bool) {
	v, ok := c.Get(ctxAuthUser)
	if !ok {
		return store.User{}, false
	}
	u, ok := v.(store.User)
	return u, ok
}

// RateLimit imposes a token-bucket style cap per authenticated user per window.
func RateLimit(rdb *redis.Client, key string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := UserID(c)
		k := "rl:" + key + ":" + strconv.FormatInt(uid, 10)
		ok := rateAllow(rdb, c, k, limit, window)
		if !ok {
			response.Error(c, 429, "too_many_requests", "Rate limit exceeded", nil)
			return
		}
		c.Next()
	}
}

// RateLimitIP imposes a per-IP cap (used for the anonymous captive portal).
func RateLimitIP(rdb *redis.Client, key string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		k := "rl:" + key + ":ip:" + ip
		if !rateAllow(rdb, c, k, limit, window) {
			response.Error(c, 429, "too_many_requests", "Rate limit exceeded", nil)
			return
		}
		c.Next()
	}
}

func rateAllow(rdb *redis.Client, c *gin.Context, k string, limit int, window time.Duration) bool {
	windowSec := int64(window.Seconds())
	res, err := rdb.Eval(c.Request.Context(), `
		local n = tonumber(redis.call('GET', KEYS[1]) or '0')
		local exp = tonumber(redis.call('TTL', KEYS[1]) or '0')
		if n >= tonumber(ARGV[1]) and exp > 0 then
			return 0
		end
		if exp <= 0 then
			redis.call('SET', KEYS[1], 1, 'EX', ARGV[2])
		else
			redis.call('INCR', KEYS[1])
		end
		return 1`, []string{k}, limit, windowSec).Int()
	return err == nil && res == 1
}
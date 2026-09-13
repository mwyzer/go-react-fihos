package server

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"fihos/backend/internal/auth"
	"fihos/backend/internal/handler"
	"fihos/backend/internal/health"
	"fihos/backend/internal/middleware"
	"fihos/backend/internal/mikrotik"
	"fihos/backend/internal/service"
	"fihos/backend/internal/store"
)

type Deps struct {
	DB       *pgxpool.Pool
	Redis    *redis.Client
	Verifier middleware.AuthVerifier
	AuthSvc  *auth.Service
	Store    *store.Store
	Sim      *mikrotik.Simulator
}

func New(engine *gin.Engine, deps Deps) *gin.Engine {
	engine.Use(
		gin.Recovery(),
		middleware.Logger("info"),
	)

	healthHandler := health.New(deps.DB, deps.Redis)
	engine.GET("/healthz", healthHandler.Live)
	engine.GET("/readyz", healthHandler.Ready)

	mt := mikrotik.NewClient(deps.Sim)
	h := handler.New(deps.Store, deps.Redis, deps.AuthSvc,
		service.NewBilling(deps.Store), service.NewAnomalyEngine(deps.Store), deps.Sim, mt)

	rate := func(rc *gin.RouterGroup) {
		rc.POST("/login", h.PostLogin)
		rc.POST("/refresh", h.PostRefresh)
		rc.POST("/logout", h.PostLogout)
	}

	// Public auth endpoints (IP rate-limited against brute force).
	authPublic := engine.Group("/api/v1/auth", middleware.RateLimitIP(deps.Redis, "auth", 20, time.Minute))
	rate(authPublic)

	// Captive portal (public, IP rate-limited).
	portal := engine.Group("/api/v1/portal", middleware.RateLimitIP(deps.Redis, "portal", 30, time.Minute))
	{
		portal.GET("/:slug/health", h.PortalGetHealth)
		portal.POST("/:slug/redeem", h.PortalPostRedeem)
		portal.POST("/:slug/status", h.PortalPostStatus)
	}

	api := engine.Group("/api/v1")
	api.Use(middleware.Auth(deps.Verifier), middleware.AuthDB(deps.Store))
	{
		api.GET("/me", h.GetMe)
		api.POST("/change-password", h.PostChangePassword)
		api.POST("/reset-password", middleware.Roles("admin"), h.PostResetPassword)

		admin := api.Group("/admin", middleware.Roles("admin"))
		{
			admin.POST("/tenants", h.PostTenant)
			admin.GET("/tenants", h.ListTenants)
			admin.GET("/tenants/:id", h.GetTenant)
			admin.PATCH("/tenants/:id", h.PatchTenant)
			admin.PATCH("/tenants/:id/status", h.PatchTenantStatus)
			admin.POST("/users", h.PostUser)
			admin.GET("/users", h.ListUsers)
			admin.PATCH("/users/:id", h.PatchUser)
			admin.GET("/audit", h.GetAudit)
		}

		// Tenant-scoped team operations (owner/staff).
		team := api.Group("")
		{
			team.GET("/users", h.ListUsers)
			team.POST("/users", h.PostUser)
			team.PATCH("/users/:id", h.PatchUser)
			team.GET("/settings", h.GetSettings)
			team.PUT("/settings", h.PutSettings)
			team.GET("/audit", h.GetAudit)
		}

		// Phase 2: profiles, routers, hotspots.
		infra := api.Group("", middleware.RequireTenant)
		{
			infra.GET("/profiles", h.ListProfiles)
			infra.POST("/profiles", h.PostProfile)
			infra.PATCH("/profiles/:id", h.PatchProfile)

			infra.GET("/routers", h.ListRouters)
			infra.POST("/routers", h.PostRouter)
			infra.GET("/routers/:id", h.GetRouter)
			infra.PATCH("/routers/:id", h.PatchRouter)
			infra.PATCH("/routers/:id/config", h.PostRouterProbe)
			infra.DELETE("/routers/:id", h.DeleteRouter)
			infra.POST("/routers/:id/probe", h.PostRouterProbe)
			infra.POST("/routers/:id/simulate", h.PostRouterSimulate)

			infra.GET("/hotspots", h.ListHotspots)
			infra.POST("/hotspots", h.PostHotspot)
			infra.GET("/hotspots/:id", h.GetHotspot)
			infra.PATCH("/hotspots/:id", h.PatchHotspot)
			infra.PATCH("/hotspots/:id/status", h.PatchHotspotStatus)
		}

		// Phase 3: vouchers.
		vouchers := api.Group("/batches", middleware.RequireTenant)
		{
			vouchers.GET("", h.ListBatches)
			vouchers.POST("", h.PostBatch)
			vouchers.GET("/:id", h.GetBatch)
			vouchers.GET("/:id/vouchers", h.ListBatchVouchers)
			vouchers.POST("/:id/vouchers/:vid/revoke", h.PostVoucherRevoke)
		}

		// Phase 4: sessions.
		sessions := api.Group("/sessions", middleware.RequireTenant)
		{
			sessions.GET("", h.ListSessions)
			sessions.GET("/:id", h.GetSession)
			sessions.GET("/:id/usage", h.ListSessionUsage)
			sessions.POST("/:id/disconnect", h.PostSessionDisconnect)
		}

		// Phase 5: analytics + billing.
		analytics := api.Group("", middleware.RequireTenant)
		{
			analytics.GET("/dashboard", h.GetDashboard)
			analytics.GET("/analytics/traffic", h.GetTrafficSeries)
			analytics.GET("/analytics/hotspots/top", h.GetTopHotspots)
			analytics.GET("/payments", h.ListPayments)
		}

		// Phase 6: rate windows, anomaly alerts, config jobs.
		phase6 := api.Group("", middleware.RequireTenant)
		{
			phase6.GET("/rate-windows", h.ListRateWindows)
			phase6.POST("/rate-windows", h.PostRateWindow)
			phase6.DELETE("/rate-windows/:id", h.DeleteRateWindow)
			phase6.GET("/alerts", h.ListAlerts)
			phase6.PATCH("/alerts/:id/status", h.PatchAlertStatus)
			phase6.GET("/config-jobs", h.ListConfigJobs)
		}

		api.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"pong": true})
		})
	}
	return engine
}
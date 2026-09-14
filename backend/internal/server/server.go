package server

import (
	"net/http"
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
	DB                   *pgxpool.Pool
	Redis                *redis.Client
	Verifier             middleware.AuthVerifier
	AuthSvc              *auth.Service
	Store                *store.Store
	Sim                  mikrotik.SimState
	MikrotikMode         string
	MikrotikCreds        mikrotik.Credentials
	MikrotikTimeout      time.Duration
	PaymentProvider      string
	PaymentSandboxURL    string
	PaymentWebhookSecret string
}

func New(engine *gin.Engine, deps Deps) *gin.Engine {
	engine.Use(
		gin.Recovery(),
		middleware.Logger("info"),
	)

	healthHandler := health.New(deps.DB, deps.Redis)
	engine.GET("/healthz", healthHandler.Live)
	engine.GET("/readyz", healthHandler.Ready)

	mt := mikrotik.New(deps.MikrotikMode, deps.Sim, deps.MikrotikCreds, &http.Client{Timeout: deps.MikrotikTimeout})
	provider := service.NewProvider(deps.PaymentProvider, service.ProviderConfig{SandboxBaseURL: deps.PaymentSandboxURL})
	billing := service.NewBilling(deps.Store, provider)
	h := handler.New(deps.Store, deps.Redis, deps.AuthSvc,
		billing, service.NewAnomalyEngine(deps.Store), deps.Sim, mt, deps.PaymentWebhookSecret)

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

	// Payment gateway callbacks (public: the gateway settles, not our tenants).
	payments := engine.Group("/api/v1/payments", middleware.RateLimitIP(deps.Redis, "payments", 30, time.Minute))
	{
		payments.POST("/:ref/complete", h.PostPaymentComplete)
		payments.GET("/:ref", h.GetPaymentStatus)
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

		// Tenant-scoped team operations (owner/staff reads, owner/admin writes).
		team := api.Group("")
		{
			team.GET("/users", h.ListUsers)
			teamWrite := team.Group("", middleware.Roles("owner", "admin"))
			teamWrite.POST("/users", h.PostUser)
			teamWrite.PATCH("/users/:id", h.PatchUser)
			team.GET("/settings", h.GetSettings)
			teamWrite.PUT("/settings", h.PutSettings)
			team.GET("/audit", h.GetAudit)
		}

		// Phase 2: reads open to every tenant role.
		infra := api.Group("", middleware.RequireTenant)
		{
			infra.GET("/profiles", h.ListProfiles)
			infra.GET("/routers", h.ListRouters)
			infra.GET("/routers/:id", h.GetRouter)
			infra.PATCH("/routers/:id/config", h.PostRouterProbe)
			infra.POST("/routers/:id/probe", h.PostRouterProbe)
			infra.POST("/routers/:id/simulate", h.PostRouterSimulate)

			infra.GET("/hotspots", h.ListHotspots)
			infra.GET("/hotspots/:id", h.GetHotspot)
		}

		// Writes that change router/hotspot/profile configuration (owner/admin only).
		infraWrite := api.Group("", middleware.RequireTenant, middleware.Roles("owner", "admin"))
		{
			infraWrite.POST("/profiles", h.PostProfile)
			infraWrite.PATCH("/profiles/:id", h.PatchProfile)
			infraWrite.POST("/routers", h.PostRouter)
			infraWrite.PATCH("/routers/:id", h.PatchRouter)
			infraWrite.DELETE("/routers/:id", h.DeleteRouter)
			infraWrite.POST("/hotspots", h.PostHotspot)
			infraWrite.PATCH("/hotspots/:id", h.PatchHotspot)
			infraWrite.PATCH("/hotspots/:id/status", h.PatchHotspotStatus)
		}

		// Phase 3: vouchers.
		vouchers := api.Group("/batches", middleware.RequireTenant)
		{
			vouchers.GET("", h.ListBatches)
			vouchers.POST("", middleware.Roles("owner", "admin"), h.PostBatch)
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
			phase6.POST("/rate-windows", middleware.Roles("owner", "admin"), h.PostRateWindow)
			phase6.DELETE("/rate-windows/:id", middleware.Roles("owner", "admin"), h.DeleteRateWindow)
			phase6.GET("/alerts", h.ListAlerts)
			phase6.PATCH("/alerts/:id/status", middleware.Roles("owner", "admin"), h.PatchAlertStatus)
			phase6.GET("/config-jobs", h.ListConfigJobs)
		}

		// Phase 7: customers & billing. Reads are open to any tenant role;
		// writes (customer mgmt, generate/mark-paid) are owner/admin only.
		customers := api.Group("/customers", middleware.RequireTenant)
		{
			customers.GET("", h.ListCustomers)
			customers.GET("/overview", h.CustomerOverview)
			customersWrite := customers.Group("", middleware.Roles("owner", "admin"))
			customersWrite.POST("", h.PostCustomer)
			customersWrite.PATCH("/:id/status", h.PatchCustomerStatus)
			customersWrite.POST("/:id/topup", h.PostCustomerTopup)
			customersWrite.POST("/:id/wallet/adjust", h.PostWalletAdjust)
			customers.GET("/:id/wallet", h.GetCustomerWallet)
		}

		billing := api.Group("/billing", middleware.RequireTenant)
		{
			billing.GET("", h.ListBillingWindows)
			billing.GET("/overview", h.GetBillingOverview)
			billingWrite := billing.Group("", middleware.Roles("owner", "admin"))
			billingWrite.POST("/generate", h.PostBillingGenerate)
			billingWrite.POST("/:id/pay", h.PostBillingPay)
		}

		api.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"pong": true})
		})
	}
	return engine
}

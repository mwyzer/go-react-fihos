package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"fihos/backend/internal/auth"
	"fihos/backend/internal/cache"
	"fihos/backend/internal/config"
	"fihos/backend/internal/database"
	"fihos/backend/internal/mikrotik"
	"fihos/backend/internal/server"
	"fihos/backend/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	db, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	rdb, err := cache.Connect(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	authSvc := auth.NewService(cfg.JWTSecret, cfg.TokenTTL, cfg.RefreshTTL)
	st := store.New(db)
	var sim mikrotik.SimState = mikrotik.NewSimulator()
	if cfg.SimShared {
		sim = mikrotik.NewRedisSimulator(rdb)
	}
	registerAllRouters(ctx, st, sim)

	engine := gin.New()
	server.New(engine, server.Deps{
		DB:           db,
		Redis:        rdb,
		Verifier:     authSvc,
		AuthSvc:      authSvc,
		Store:        st,
		Sim:          sim,
		MikrotikMode: cfg.MikrotikMode,
		MikrotikCreds: mikrotik.Credentials{
			Username: cfg.MikrotikUser,
			Password: cfg.MikrotikPass,
		},
		MikrotikTimeout:      cfg.MikrotikTimeout,
		PaymentProvider:      cfg.PaymentProvider,
		PaymentSandboxURL:    cfg.PaymentSandboxURL,
		PaymentWebhookSecret: cfg.PaymentWebhookSecret,
	})

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("API listening on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	shutdownCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("API stopped")
}

// registerAllRouters seeds the shared device state from the DB so seeded
// routers respond to probes without a manual simulate toggle.
func registerAllRouters(ctx context.Context, st *store.Store, sim mikrotik.SimState) {
	routers, err := st.RoutersAll(ctx)
	if err != nil {
		log.Printf("seed routers: %v", err)
		return
	}
	for _, r := range routers {
		sim.RegisterRouter(r.ID, r.Name)
	}
}

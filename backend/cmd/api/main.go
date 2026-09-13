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
	sim := mikrotik.NewSimulator()

	engine := gin.New()
	server.New(engine, server.Deps{
		DB:       db,
		Redis:    rdb,
		Verifier: authSvc,
		AuthSvc:  authSvc,
		Store:    st,
		Sim:      sim,
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
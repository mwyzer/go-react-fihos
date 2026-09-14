package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	HTTPPort             string
	GRPCPort             string
	DatabaseURL          string
	RedisAddr            string
	RedisPassword        string
	RedisDB              int
	JWTSecret            string
	TokenTTL             time.Duration
	RefreshTTL           time.Duration
	RouterSyncFreq       time.Duration
	RouterProbeFreq      time.Duration
	AnalyticsFreq        time.Duration
	BillingFreq          time.Duration
	LogLevel             string
	FrontendOrigin       string
	MikrotikMode         string
	MikrotikUser         string
	MikrotikPass         string
	MikrotikTimeout      time.Duration
	SimShared            bool
	PaymentProvider      string
	PaymentSandboxURL    string
	PaymentWebhookSecret string
	PaymentFeeRate       float64
}

func Load() (*Config, error) {
	port := getenv("HTTP_PORT", "8080")
	if _, err := strconv.Atoi(port); err != nil {
		return nil, fmt.Errorf("HTTP_PORT must be a number: %w", err)
	}

	redisDB, err := strconv.Atoi(getenv("REDIS_DB", "0"))
	if err != nil {
		return nil, fmt.Errorf("REDIS_DB must be a number: %w", err)
	}

	cfg := &Config{
		HTTPPort:             port,
		GRPCPort:             getenv("GRPC_PORT", "50051"),
		DatabaseURL:          getenv("DATABASE_URL", "postgres://fihos:fihos@localhost:55432/fihos?sslmode=disable"),
		RedisAddr:            getenv("REDIS_ADDR", "localhost:6380"),
		RedisPassword:        getenv("REDIS_PASSWORD", ""),
		RedisDB:              redisDB,
		JWTSecret:            getenv("JWT_SECRET", "dev-secret-change-me"),
		TokenTTL:             mustDuration("TOKEN_TTL", 15*time.Minute),
		RefreshTTL:           mustDuration("REFRESH_TTL", 7*24*time.Hour),
		RouterSyncFreq:       mustDuration("ROUTER_SYNC_FREQ", 30*time.Second),
		RouterProbeFreq:      mustDuration("ROUTER_PROBE_FREQ", 60*time.Second),
		AnalyticsFreq:        mustDuration("ANALYTICS_FREQ", 5*time.Minute),
		BillingFreq:          mustDuration("BILLING_FREQ", 6*time.Hour),
		LogLevel:             getenv("LOG_LEVEL", "info"),
		FrontendOrigin:       getenv("FRONTEND_ORIGIN", "http://localhost:5173"),
		MikrotikMode:         getenv("MIKROTIK_MODE", "simulate"),
		MikrotikUser:         getenv("MIKROTIK_USER", ""),
		MikrotikPass:         getenv("MIKROTIK_PASS", ""),
		MikrotikTimeout:      mustDuration("MIKROTIK_TIMEOUT", 5*time.Second),
		SimShared:            getenv("SIM_SHARED", "1") == "1",
		PaymentProvider:      getenv("PAYMENT_PROVIDER", "mock"),
		PaymentSandboxURL:    getenv("PAYMENT_SANDBOX_URL", ""),
		PaymentWebhookSecret: getenv("PAYMENT_WEBHOOK_SECRET", "dev-webhook-secret"),
		PaymentFeeRate:       mustFloat("PAYMENT_FEE_RATE", 0.03),
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func mustDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}

func mustFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}

//go:build integration

// Package itest also bootstraps its own environment with Testcontainers: a
// Postgres + Redis container, migrations applied, the API server started
// in-process, and the core smoke/regression checks re-run against it. No
// docker-compose stack required.
package itest

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/redis/go-redis/v9"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/modules/redis"
	"github.com/testcontainers/testcontainers-go/wait"

	"fihos/backend/internal/auth"
	"fihos/backend/internal/database"
	"fihos/backend/internal/mikrotik"
	"fihos/backend/internal/server"
	"fihos/backend/internal/store"
)

func TestContainerSmoke(t *testing.T) {
	if _, ok := os.LookupEnv("FIHOS_SKIP_CONTAINERS"); ok {
		t.Skip("FIHOS_SKIP_CONTAINERS set")
	}

	ctx := context.Background()

	pgc, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("fihos"),
		postgres.WithUsername("fihos"),
		postgres.WithPassword("fihos"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).WithStartupTimeout(120*time.Second)),
	)
	if err != nil {
		t.Fatalf("postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pgc.Terminate(ctx) })

	rc, err := redis.Run(ctx, "redis:7-alpine",
		testcontainers.WithWaitStrategy(wait.ForLog("* Ready to accept connections")))
	if err != nil {
		t.Fatalf("redis container: %v", err)
	}
	t.Cleanup(func() { _ = rc.Terminate(ctx) })

	dbURL := "postgres://" + strings.TrimPrefix(pgc.URI(), "postgres://")

	_, thisFile, _, _ := runtime.Caller(0)
	migrations := filepath.ToSlash(filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations"))
	m, err := migrate.New("file://"+migrations, dbURL)
	if err != nil {
		t.Fatalf("migrate init: %v", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("migrate up: %v", err)
	}
	m.Close()

	pool, err := database.Connect(ctx, dbURL)
	if err != nil {
		t.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: strings.TrimPrefix(rc.URI(), "redis://"), DB: 0})
	defer rdb.Close()

	st := store.New(pool)
	authSvc := auth.NewService("test-secret-change-me", 15*time.Minute, 7*24*time.Hour)
	seedContainerUsers(t, ctx, st)

	engine := gin.New()
	server.New(engine, server.Deps{
		DB:                   pool,
		Redis:                rdb,
		Verifier:             authSvc,
		AuthSvc:              authSvc,
		Store:                st,
		Sim:                  mikrotik.NewSimulator(),
		MikrotikMode:         mikrotik.ModeSimulate,
		MikrotikCreds:        mikrotik.Credentials{},
		MikrotikTimeout:      5 * time.Second,
		PaymentProvider:      "mock",
		PaymentSandboxURL:    "",
		PaymentWebhookSecret: "container-secret",
	})
	srv := httptest.NewServer(engine)
	defer srv.Close()

	oldBase := baseURL
	baseURL = srv.URL + "/api/v1"
	defer func() { baseURL = oldBase }()

	runContainerChecks(t)
}

// seedContainerUsers provisions the admin plus two tenants (one with
// owner/staff, one empty) in the fresh database.
func seedContainerUsers(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()

	adminHash, err := auth.HashPassword(adminPass)
	if err != nil {
		t.Fatalf("hash admin: %v", err)
	}
	if _, err := st.CreateUser(ctx, &store.User{
		Email: adminEmail, FullName: "Admin", Role: "admin", IsActive: true,
	}, adminHash); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	tenant, err := st.CreateTenant(ctx, "Demo WiFi", "demo", map[string]any{"color": "#3b82f6"})
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	tid := tenant.ID
	for _, u := range []struct {
		email, role, pass string
	}{
		{ownerEmail, "owner", ownerPass},
		{staffEmail, "staff", staffPass},
	} {
		hash, err := auth.HashPassword(u.pass)
		if err != nil {
			t.Fatalf("hash %s: %v", u.role, err)
		}
		if _, err := st.CreateUser(ctx, &store.User{
			TenantID: &tid, Email: u.email, FullName: "User " + u.role, Role: u.role, IsActive: true,
		}, hash); err != nil {
			t.Fatalf("create %s: %v", u.role, err)
		}
	}

	if _, err := st.CreateTenant(ctx, "Empty Net", "empty-net", map[string]any{"color": "#111827"}); err != nil {
		t.Fatalf("create empty tenant: %v", err)
	}
}

func runContainerChecks(t *testing.T) {
	adminTok, admin := login(t, adminEmail, adminPass)
	ownerTok, owner := login(t, ownerEmail, ownerPass)
	staffTok, staff := login(t, staffEmail, staffPass)

	if admin.User.Role != "admin" || admin.User.TenantID != nil {
		t.Fatalf("admin role/tenant mismatch: %+v", admin.User)
	}
	if owner.User.Role != "owner" || owner.User.TenantID == nil {
		t.Fatalf("owner mismatch: %+v", owner.User)
	}
	if staff.User.Role != "staff" {
		t.Fatalf("staff mismatch: %+v", staff.User)
	}

	code, _ := do(t, "GET", "/me", "", nil, nil)
	expectStatus(t, code, 401, "container unauthenticated /me")

	// Staff reads OK, writes 403 (role gates must not depend on seeded data).
	code, _ = do(t, "GET", "/customers", staffTok, staff.User.TenantID, nil)
	expectStatus(t, code, 200, "container staff GET /customers")
	code, _ = do(t, "POST", "/customers", staffTok, staff.User.TenantID, map[string]any{"name": "x"})
	expectStatus(t, code, 403, "container staff POST /customers")
	code, _ = do(t, "POST", "/billing/generate", staffTok, staff.User.TenantID, nil)
	expectStatus(t, code, 403, "container staff POST /billing/generate")

	// Owner write.
	code, raw := do(t, "POST", "/customers", ownerTok, owner.User.TenantID, map[string]any{
		"name": "Container Customer", "status": "active",
		"phone": fmt.Sprintf("08%09d", time.Now().UnixNano()%1000000000), "address": "tc",
	})
	expectStatus(t, code, 201, "container owner POST /customers")
	if !strings.Contains(string(raw), "id") {
		t.Fatalf("create customer body has no id: %s", raw)
	}

	tenant1 := int64(1)
	tenant2 := int64(2)

	// Admin overlay: seeded tenant returns real arrays.
	code, raw = do(t, "GET", "/customers", adminTok, &tenant1, nil)
	expectStatus(t, code, 200, "container admin GET /customers tenant 1")
	assertArrayNotNil(t, raw, "items")
	code, raw = do(t, "GET", "/payments", adminTok, &tenant1, nil)
	expectStatus(t, code, 200, "container admin GET /payments tenant 1")
	assertArrayNotNil(t, raw, "items")

	// Empty tenant: the old blank-dashboard bug must stay fixed.
	for _, tc := range []struct {
		path string
		keys []string
	}{
		{"/customers", []string{"items"}},
		{"/payments", []string{"items"}},
		{"/alerts", []string{"items"}},
		{"/batches", []string{"items"}},
		{"/config-jobs", []string{}},
		{"/rate-windows", []string{}},
		{"/profiles", []string{}},
		{"/routers", []string{}},
		{"/hotspots", []string{}},
		{"/analytics/traffic", []string{}},
		{"/analytics/hotspots/top", []string{}},
		{"/dashboard", []string{"series", "top"}},
	} {
		c, r := do(t, "GET", tc.path, adminTok, &tenant2, nil)
		if c != 200 {
			t.Errorf("container GET %s tenant 2 = %d: %s", tc.path, c, r)
			continue
		}
		if len(tc.keys) == 0 {
			assertArrayNotNil(t, r)
		} else {
			assertArrayNotNil(t, r, tc.keys...)
		}
	}

	// Admin without overlay rejected.
	code, _ = do(t, "GET", "/customers", adminTok, nil, nil)
	expectStatus(t, code, 403, "container admin GET /customers without overlay")
}
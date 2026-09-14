//go:build integration

// Package itest also bootstraps its own environment with Testcontainers: a
// Postgres + Redis container, migrations applied, the API server started
// in-process, and the core smoke/regression checks re-run against it. No
// docker-compose stack required.
package itest

import (
	"context"
	"encoding/json"
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
	tcredismodule "github.com/testcontainers/testcontainers-go/modules/redis"
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

	rc, err := tcredismodule.Run(ctx, "redis:7-alpine",
		testcontainers.WithWaitStrategy(wait.ForLog("* Ready to accept connections")))
	if err != nil {
		t.Fatalf("redis container: %v", err)
	}
	t.Cleanup(func() { _ = rc.Terminate(ctx) })

	conn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("postgres connection string: %v", err)
	}
	dbURL := "postgres://" + strings.TrimPrefix(conn, "postgres://")

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

	connStr, err := rc.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("redis connection string: %v", err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: strings.TrimPrefix(connStr, "redis://"), DB: 0})
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

	runWalletChecks(t, ownerTok, owner.User.TenantID)

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

// runWalletChecks exercises the customer prepaid balance: top-up through the
// mock gateway (instant settle -> crediting), manual adjustment, and paying a
// monthly billing window from the balance.
func runWalletChecks(t *testing.T, ownerTok string, tid *int64) {
	t.Helper()

	// Token-status customer so billing generation creates a window for them.
	code, raw := do(t, "POST", "/customers", ownerTok, tid, map[string]any{
		"name": fmt.Sprintf("Wallet Customer %d", time.Now().UnixNano()%100000),
		"status": "token",
		"phone":  fmt.Sprintf("08%09d", time.Now().UnixNano()%1000000000),
	})
	expectStatus(t, code, 201, "wallet create customer")
	var customer struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &customer); err != nil || customer.ID == 0 {
		t.Fatalf("wallet create customer body: %s err=%v", raw, err)
	}

	// Top-up settles instantly (mock provider) and credits the wallet.
	code, raw = do(t, "POST", fmt.Sprintf("/customers/%d/topup", customer.ID), ownerTok, tid, map[string]any{"amount": 200000})
	expectStatus(t, code, 201, "wallet topup")
	var topup struct {
		Async      bool `json:"async"`
		PaymentURL any  `json:"payment_url"`
		Payment    struct {
			Status    string `json:"status"`
			ExternalRef string `json:"external_ref"`
			CustomerID *int64 `json:"customer_id"`
		} `json:"payment"`
	}
	if err := json.Unmarshal(raw, &topup); err != nil {
		t.Fatalf("wallet topup body: %v", err)
	}
	if topup.Async {
		t.Fatalf("wallet topup should settle instantly with mock provider: %s", raw)
	}
	if topup.Payment.Status != "succeeded" {
		t.Fatalf("wallet topup status = %q", topup.Payment.Status)
	}
	if topup.Payment.CustomerID == nil || *topup.Payment.CustomerID != customer.ID {
		t.Fatalf("wallet topup customer_id = %v", topup.Payment.CustomerID)
	}

	// Balance now reflects the credited top-up (net of the 3% fee).
	code, raw = do(t, "GET", fmt.Sprintf("/customers/%d/wallet", customer.ID), ownerTok, tid, nil)
	expectStatus(t, code, 200, "wallet get")
	var wallet struct {
		Balance      float64 `json:"balance"`
		Transactions struct {
			Items []struct {
				Type   string  `json:"type"`
				Amount float64 `json:"amount"`
			} `json:"items"`
			Total int64 `json:"total"`
		} `json:"transactions"`
	}
	if err := json.Unmarshal(raw, &wallet); err != nil {
		t.Fatalf("wallet get body: %v", err)
	}
	wantBalance := 200000 * 0.97
	if wallet.Balance < wantBalance-1 || wallet.Balance > wantBalance+1 {
		t.Fatalf("wallet balance = %v, want ~%v", wallet.Balance, wantBalance)
	}
	if wallet.Transactions.Total != 1 || wallet.Transactions.Items[0].Type != "topup" {
		t.Fatalf("wallet ledger = %s", raw)
	}

	// Manual adjustment credits cash received offline.
	code, raw = do(t, "POST", fmt.Sprintf("/customers/%d/wallet/adjust", customer.ID), ownerTok, tid, map[string]any{
		"amount": 50000, "note": "cash",
	})
	expectStatus(t, code, 200, "wallet adjust")
	var adjusted struct {
		Balance float64 `json:"balance"`
	}
	if err := json.Unmarshal(raw, &adjusted); err != nil {
		t.Fatalf("wallet adjust body: %v", err)
	}
	if adjusted.Balance < wantBalance+50000-1 || adjusted.Balance > wantBalance+50000+1 {
		t.Fatalf("wallet balance after adjust = %v, want ~%v", adjusted.Balance, wantBalance+50000)
	}

	// Negative adjustment beyond balance is rejected.
	code, _ = do(t, "POST", fmt.Sprintf("/customers/%d/wallet/adjust", customer.ID), ownerTok, tid, map[string]any{
		"amount": -9999999, "note": "nope",
	})
	expectStatus(t, code, 409, "wallet adjust over-debit")

	// Generate the monthly window, then pay it from the balance.
	code, raw = do(t, "POST", "/billing/generate", ownerTok, tid, nil)
	expectStatus(t, code, 201, "wallet billing generate")
	var gen struct {
		Generated int `json:"generated"`
	}
	if err := json.Unmarshal(raw, &gen); err != nil || gen.Generated < 1 {
		t.Fatalf("wallet billing generate = %d: %s", gen.Generated, raw)
	}
	var windowID int64
	code, raw = do(t, "GET", "/billing", ownerTok, tid, nil)
	expectStatus(t, code, 200, "wallet list billing")
	var billing struct {
		Items []struct {
			ID         int64  `json:"id"`
			CustomerID int64  `json:"customer_id"`
			Status     string `json:"status"`
			Amount     float64 `json:"amount"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &billing); err != nil {
		t.Fatalf("wallet billing body: %v", err)
	}
	for _, w := range billing.Items {
		if w.CustomerID == customer.ID && w.Status != "paid" {
			windowID = w.ID
			break
		}
	}
	if windowID == 0 {
		t.Fatalf("no unpaid window for wallet customer: %s", raw)
	}

	code, raw = do(t, "POST", fmt.Sprintf("/billing/%d/pay", windowID), ownerTok, tid, map[string]any{"method": "wallet"})
	expectStatus(t, code, 200, "wallet pay from balance")
	var paid struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &paid); err != nil || paid.Status != "paid" {
		t.Fatalf("wallet pay window body: %s", raw)
	}

	// Balance dropped by the window amount and the ledger shows both entries.
	code, raw = do(t, "GET", fmt.Sprintf("/customers/%d/wallet", customer.ID), ownerTok, tid, nil)
	expectStatus(t, code, 200, "wallet get after pay")
	var after struct {
		Balance float64 `json:"balance"`
		Transactions struct {
			Total int64 `json:"total"`
		} `json:"transactions"`
	}
	if err := json.Unmarshal(raw, &after); err != nil {
		t.Fatalf("wallet after-pay body: %v", err)
	}
	if after.Balance < adjusted.Balance-150000-1 || after.Balance > adjusted.Balance-150000+1 {
		t.Fatalf("wallet balance after pay = %v, want ~%v (adjusted %v - monthly fee)", after.Balance, adjusted.Balance-150000, adjusted.Balance)
	}
	if after.Transactions.Total != 3 {
		t.Fatalf("wallet ledger total = %d, want 3 (topup, adjust, bill_payment): %s", after.Transactions.Total, raw)
	}
}
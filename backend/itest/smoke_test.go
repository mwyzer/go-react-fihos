//go:build integration

// Package itest exercises the running FIHOS stack (docker-compose) end to end.
//
// Run:
//
//	FIHOS_API_URL=http://localhost:8081/api/v1 go test -tags integration ./itest/ -v
package itest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var baseURL = func() string {
	if v := os.Getenv("FIHOS_API_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return "http://localhost:8081/api/v1"
}()

var (
	adminEmail = os.Getenv("FIHOS_ADMIN_EMAIL")
	adminPass  = os.Getenv("FIHOS_ADMIN_PASS")
	ownerEmail = "owner@demo.dev"
	ownerPass  = "owner12345"
	staffEmail = "staff@demo.dev"
	staffPass  = "staff12345"
)

func init() {
	if adminEmail == "" {
		adminEmail = "admin@fihos.dev"
	}
	if adminPass == "" {
		adminPass = "admin12345"
	}
}

type loginResp struct {
	AccessToken string `json:"access_token"`
	User        struct {
		ID       int64  `json:"id"`
		Role     string `json:"role"`
		TenantID *int64 `json:"tenant_id"`
	} `json:"user"`
}

func do(t *testing.T, method, path string, token string, tenantID *int64, body any) (int, json.RawMessage) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if tenantID != nil {
		req.Header.Set("X-Tenant-Id", fmt.Sprintf("%d", *tenantID))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

func login(t *testing.T, email, pass string) (string, *loginResp) {
	t.Helper()
	code, raw := do(t, "POST", "/auth/login", "", nil, map[string]string{"email": email, "password": pass})
	if code != 200 {
		t.Fatalf("login %s returned %d: %s", email, code, raw)
	}
	var lr loginResp
	if err := json.Unmarshal(raw, &lr); err != nil {
		t.Fatalf("unmarshal login: %v", err)
	}
	if lr.AccessToken == "" {
		t.Fatal("empty access token")
	}
	return lr.AccessToken, &lr
}

func expectStatus(t *testing.T, got, want int, ctx string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: status = %d, want %d", ctx, got, want)
	}
}

// assertArrayNotNil guards against the nil-slice -> JSON "null" bug that
// crashed the React frontend on ".map()". Object endpoints can assert specific
// keys (e.g. items/series/top); a raw-array endpoint (keys empty) must simply
// not serialize to "null".
func assertArrayNotNil(t *testing.T, raw json.RawMessage, keys ...string) {
	t.Helper()
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "null" {
		t.Fatalf("response is JSON null (frontend .map() crash): %s", raw)
	}
	if len(keys) == 0 {
		return
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("body not an object: %s", raw)
	}
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			t.Fatalf("key %q missing: %s", k, raw)
		}
		if string(v) == "null" {
			t.Fatalf("key %q is null (frontend .map() crash): %s", k, raw)
		}
		if !strings.HasPrefix(strings.TrimSpace(string(v)), "[") {
			t.Fatalf("key %q is not an array: %s", k, v)
		}
	}
}

func TestHealthz(t *testing.T) {
	resp, err := http.Get(strings.Replace(baseURL, "/api/v1", "", 1) + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}
}

func TestSmokeLoginAndMe(t *testing.T) {
	adminTok, admin := login(t, adminEmail, adminPass)
	ownerTok, owner := login(t, ownerEmail, ownerPass)
	staffTok, staff := login(t, staffEmail, staffPass)

	if admin.User.Role != "admin" || admin.User.TenantID != nil {
		t.Fatalf("admin role/tenant mismatch: %+v", admin.User)
	}
	if owner.User.Role != "owner" || owner.User.TenantID == nil {
		t.Fatalf("owner role/tenant mismatch: %+v", owner.User)
	}
	if staff.User.Role != "staff" {
		t.Fatalf("staff role mismatch: %+v", staff.User)
	}

	// /me for each role
	for name, tok := range map[string]string{"admin": adminTok, "owner": ownerTok, "staff": staffTok} {
		code, raw := do(t, "GET", "/me", tok, nil, nil)
		if code != 200 {
			t.Fatalf("GET /me (%s) = %d: %s", name, code, raw)
		}
	}

	// Unauthenticated request is rejected.
	code, _ := do(t, "GET", "/me", "", nil, nil)
	expectStatus(t, code, 401, "GET /me unauthenticated")
}

func TestSmokeRoleGates(t *testing.T) {
	_, owner := login(t, ownerEmail, ownerPass)
	_, staff := login(t, staffEmail, staffPass)

	// staff may read tenant-scoped data...
	code, _ := do(t, "GET", "/customers", staff.AccessToken, staff.User.TenantID, nil)
	expectStatus(t, code, 200, "staff GET /customers")
	code, _ = do(t, "GET", "/sessions", staff.AccessToken, staff.User.TenantID, nil)
	expectStatus(t, code, 200, "staff GET /sessions")
	code, _ = do(t, "GET", "/billing", staff.AccessToken, staff.User.TenantID, nil)
	expectStatus(t, code, 200, "staff GET /billing")

	// ...but writes are forbidden (middleware rejects before validation).
	code, _ = do(t, "POST", "/customers", staff.AccessToken, staff.User.TenantID, map[string]any{"name": "x"})
	expectStatus(t, code, 403, "staff POST /customers")
	code, _ = do(t, "POST", "/billing/generate", staff.AccessToken, staff.User.TenantID, nil)
	expectStatus(t, code, 403, "staff POST /billing/generate")
	code, _ = do(t, "POST", "/batches", staff.AccessToken, staff.User.TenantID, map[string]any{"quantity": 2})
	expectStatus(t, code, 403, "staff POST /batches")
	code, _ = do(t, "POST", "/rate-windows", staff.AccessToken, staff.User.TenantID, map[string]any{})
	expectStatus(t, code, 403, "staff POST /rate-windows")

	// owner writes succeed.
	code, raw := do(t, "POST", "/customers", owner.AccessToken, owner.User.TenantID, map[string]any{
		"name":    "Smoke Test Customer",
		"status":  "active",
		"phone":   fmt.Sprintf("08%09d", time.Now().UnixNano()%1000000000),
		"address": "smoke",
	})
	expectStatus(t, code, 201, "owner POST /customers")
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.ID == 0 {
		t.Fatalf("create customer response: %s err=%v", raw, err)
	}
}

func TestSmokeBillingGenerateIdempotent(t *testing.T) {
	_, owner := login(t, ownerEmail, ownerPass)
	code, raw := do(t, "POST", "/billing/generate", owner.AccessToken, owner.User.TenantID, nil)
	expectStatus(t, code, 201, "billing generate (1)")
	var first struct {
		Generated int `json:"generated"`
	}
	if err := json.Unmarshal(raw, &first); err != nil {
		t.Fatalf("unmarshal billing generate: %v", err)
	}
	// Second run must not duplicate windows for the same month.
	code2, raw2 := do(t, "POST", "/billing/generate", owner.AccessToken, owner.User.TenantID, nil)
	expectStatus(t, code2, 201, "billing generate (2)")
	var second struct {
		Generated int `json:"generated"`
	}
	if err := json.Unmarshal(raw2, &second); err != nil {
		t.Fatalf("unmarshal billing generate timer: %v", err)
	}
	if second.Generated != 0 {
		t.Fatalf("billing generate not idempotent: first=%d second=%d", first.Generated, second.Generated)
	}
}

func TestSmokeAdminTenantOverlay(t *testing.T) {
	adminTok, _ := login(t, adminEmail, adminPass)

	// Admin without tenant overlay gets 403 on tenant-scoped endpoints.
	code, _ := do(t, "GET", "/customers", adminTok, nil, nil)
	expectStatus(t, code, 403, "admin GET /customers without X-Tenant-Id")

	// Admin with overlay succeeds. (Exact regression for gold pages.)
	tenant1 := int64(1)
	code, raw := do(t, "GET", "/customers", adminTok, &tenant1, nil)
	expectStatus(t, code, 200, "admin GET /customers tenant 1")
	assertArrayNotNil(t, raw, "items")

	code, raw = do(t, "GET", "/sessions", adminTok, &tenant1, nil)
	expectStatus(t, code, 200, "admin GET /sessions tenant 1")
	assertArrayNotNil(t, raw, "items")

	// List tenants (admin only route).
	code, raw = do(t, "GET", "/admin/tenants", adminTok, nil, nil)
	expectStatus(t, code, 200, "admin GET /admin/tenants")
	assertArrayNotNil(t, raw, "items")

	// Owners are forbidden from admin-only routes.
	ownerTok, owner := login(t, ownerEmail, ownerPass)
	code, _ = do(t, "GET", "/admin/tenants", ownerTok, owner.User.TenantID, nil)
	expectStatus(t, code, 403, "owner GET /admin/tenants")
}

// TestRegressionNoNullArrays walks every tenant-scoped list endpoint and
// asserts items/series/top are real arrays, never JSON null.
func TestRegressionNoNullArrays(t *testing.T) {
	adminTok, _ := login(t, adminEmail, adminPass)
	// tenant 6 is the empty "epsilon" fixture that used to produce null arrays.

	cases := []struct {
		path   string
		tenant int64
		keys   []string
	}{
		{"/customers", 1, []string{"items"}},
		{"/customers/overview", 1, []string{}},
		{"/billing", 1, []string{"items"}},
		{"/billing/overview", 1, []string{}},
		{"/batches", 1, []string{"items"}},
		{"/sessions", 1, []string{"items"}},
		{"/sessions", 6, []string{"items"}},
		{"/payments", 1, []string{"items"}},
		{"/alerts", 1, []string{"items"}},
		{"/config-jobs", 1, []string{}},
		{"/audit", 1, []string{"items"}},
		{"/users", 1, []string{"items"}},
		{"/rate-windows", 1, []string{}},
		{"/profiles", 1, []string{}},
		{"/routers", 1, []string{}},
		{"/hotspots", 1, []string{}},
		{"/analytics/traffic", 1, []string{}},
		{"/analytics/hotspots/top", 1, []string{}},
		// The original blank-dashboard case: empty tenant, JSON must be arrays.
		{"/dashboard", 6, []string{"series", "top"}},
		{"/dashboard", 1, []string{"series", "top"}},
		{"/analytics/traffic", 6, []string{}},
		{"/analytics/hotspots/top", 6, []string{}},
		{"/rate-windows", 6, []string{}},
		{"/profiles", 6, []string{}},
		{"/routers", 6, []string{}},
		{"/hotspots", 6, []string{}},
		{"/batches", 6, []string{"items"}},
		{"/alerts", 6, []string{"items"}},
	}

	for _, tc := range cases {
		code, raw := do(t, "GET", tc.path, adminTok, &tc.tenant, nil)
		if code != 200 {
			t.Errorf("%s (tenant %d) = %d: %s", tc.path, tc.tenant, code, raw)
			continue
		}
		if len(tc.keys) == 0 {
			assertArrayNotNil(t, raw)
		} else {
			assertArrayNotNil(t, raw, tc.keys...)
		}
	}
}
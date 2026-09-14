package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeVerifier struct {
	claims *Claims
	err    error
}

func (f *fakeVerifier) Verify(string) (*Claims, error) {
	if f.err != nil {
		return nil, f.err
	}
	c := *f.claims
	if c.TenantID != nil {
		v := *c.TenantID
		c.TenantID = &v
	}
	return &c, nil
}
func (*fakeVerifier) IsRoleAuthorized(role string) bool {
	return role == "admin" || role == "owner" || role == "staff"
}

func run(handlers ...gin.HandlerFunc) (*httptest.ResponseRecorder, map[string]any) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handlers...)
	carrier := map[string]any{}
	r.GET("/", func(c *gin.Context) {
		tid, hasTenant := TenantID(c)
		carrier["tenant_id"] = tid
		carrier["has_tenant"] = hasTenant
		carrier["user_id"] = UserID(c)
		carrier["role"] = Role(c)
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer tok")
	r.ServeHTTP(w, req)
	return w, carrier
}

func TestAuthNoHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(Auth(&fakeVerifier{claims: &Claims{Role: "owner"}}))
	r.GET("/", func(c *gin.Context) {})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestAuthInvalidToken(t *testing.T) {
	ver := &fakeVerifier{claims: nil, err: errors.New("bad")}
	w, _ := run(Auth(ver))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAuthRejectsBannedRole(t *testing.T) {
	w, _ := run(Auth(&fakeVerifier{claims: &Claims{Role: "banned"}}))
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestAuthAllowsTenantOwner(t *testing.T) {
	tid := int64(3)
	w, carrier := run(Auth(&fakeVerifier{claims: &Claims{UserID: 1, TenantID: &tid, Role: "owner"}}))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if carrier["tenant_id"] != int64(3) || carrier["role"] != "owner" {
		t.Fatalf("expected claims propagated, got %+v", carrier)
	}
}

func TestRequireTenantFromJWT(t *testing.T) {
	tid := int64(3)
	w, carrier := run(
		Auth(&fakeVerifier{claims: &Claims{UserID: 1, TenantID: &tid, Role: "owner"}}),
		RequireTenant,
	)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if carrier["tenant_id"] != int64(3) || carrier["has_tenant"] != true {
		t.Fatalf("tenant not resolved: %+v", carrier)
	}
}

func TestRequireTenantAdminViaHeader(t *testing.T) {
	// run() hardcodes Authorization; rebuild with explicit header for admin+header case.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(
		Auth(&fakeVerifier{claims: &Claims{UserID: 9, Role: "admin"}}),
		RequireTenant,
	)
	r.GET("/", func(c *gin.Context) {
		id, ok := TenantID(c)
		if !ok || id != 7 {
			c.String(http.StatusTeapot, "bad tenant")
			return
		}
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("X-Tenant-Id", "7")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestRequireTenantAdminMissingTenantDenied(t *testing.T) {
	w, _ := run(Auth(&fakeVerifier{claims: &Claims{UserID: 9, Role: "admin"}}), RequireTenant)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestRolesGate(t *testing.T) {
	for _, tc := range []struct {
		role string
		want int
	}{
		{"admin", http.StatusOK},
		{"owner", http.StatusOK},
		{"staff", http.StatusForbidden},
	} {
		w, _ := run(
			Auth(&fakeVerifier{claims: &Claims{Role: tc.role}}),
			Roles("admin", "owner"),
		)
		if w.Code != tc.want {
			t.Fatalf("role %q: status = %d, want %d", tc.role, w.Code, tc.want)
		}
	}
}

func TestParamIDValidation(t *testing.T) {
	for _, tc := range []struct {
		param string
		want  bool
	}{
		{"42", true},
		{"0", false},
		{"-1", false},
		{"abc", false},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Params = gin.Params{{Key: "id", Value: tc.param}}
		id, ok := ParamID(c, "id")
		if ok != tc.want || (tc.want && id != 42) {
			t.Fatalf("ParamID(%q) = %d,%v; want ok=%v", tc.param, id, ok, tc.want)
		}
	}
}
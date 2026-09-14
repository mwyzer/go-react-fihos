package auth

import (
	"testing"
	"time"
)

func TestIssueVerifyRoundtrip(t *testing.T) {
	s := NewService("test-secret-0123456789", time.Minute*5, time.Hour*24)

	tid := int64(42)
	tp, jti, err := s.Issue(UserAccess{ID: 1, Email: "a@b.c", FullName: "A", Role: "owner", TenantID: &tid})
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}
	if tp.User.TenantID == nil || *tp.User.TenantID != 42 {
		t.Fatalf("Issue did not echo tenant: %+v", tp.User)
	}
	if tp.RefreshToken == "" || jti == "" {
		t.Fatal("expected refresh token and jti")
	}

	c, err := s.Verify(tp.AccessToken)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if c.UserID != 1 || c.Role != "owner" || c.TenantID == nil || *c.TenantID != 42 {
		t.Fatalf("claims mismatch: %+v", c)
	}

	// refresh verify returns same identity
	ri, err := s.VerifyRefresh(tp.RefreshToken)
	if err != nil {
		t.Fatalf("VerifyRefresh error: %v", err)
	}
	if ri.JTI != jti {
		t.Fatalf("jti mismatch: %s != %s", ri.JTI, jti)
	}
	if ri.Claims.TenantID == nil || *ri.Claims.TenantID != 42 {
		t.Fatalf("refresh claims tenant mismatch: %+v", ri.Claims)
	}
}

func TestVerifyAdminNoTenant(t *testing.T) {
	s := NewService("secret", time.Minute, time.Hour)
	tp, _, err := s.Issue(UserAccess{ID: 9, Email: "admin@x.y", Role: "admin"})
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}
	c, err := s.Verify(tp.AccessToken)
	if err != nil {
		t.Fatalf("Verify error: %v", err)
	}
	if c.Role != "admin" {
		t.Fatalf("role = %q", c.Role)
	}
	if c.TenantID != nil {
		t.Fatalf("expected nil tenant for admin, got %v", *c.TenantID)
	}
}

func TestVerifyRejectsGarbage(t *testing.T) {
	s := NewService("secret", time.Minute, time.Hour)
	if _, err := s.Verify("not-a-jwt"); err == nil {
		t.Fatal("expected error for garbage token")
	}
	if _, err := s.VerifyRefresh("not-a-jwt"); err == nil {
		t.Fatal("expected error for garbage refresh token")
	}
}

func TestVerifyRejectsRefreshAsAccess(t *testing.T) {
	s := NewService("secret", time.Minute, time.Hour)
	tp, _, err := s.Issue(UserAccess{ID: 1, Role: "owner"})
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}
	if _, err := s.Verify(tp.RefreshToken); err == nil {
		t.Fatal("expected refresh token to be rejected by Verify")
	}
	if _, err := s.VerifyRefresh(tp.AccessToken); err == nil {
		t.Fatal("expected access token to be rejected by VerifyRefresh")
	}
}

func TestVerifyRejectsForgedSignature(t *testing.T) {
	s := NewService("correct-secret", time.Minute, time.Hour)
	other := NewService("wrong-secret", time.Minute, time.Hour)
	tp, _, err := s.Issue(UserAccess{ID: 1, Role: "owner"})
	if err != nil {
		t.Fatalf("Issue error: %v", err)
	}
	if _, err := other.Verify(tp.AccessToken); err == nil {
		t.Fatal("expected signature mismatch to fail")
	}
}

func TestPasswordHashCheck(t *testing.T) {
	h, err := HashPassword("s3cret!")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if !CheckPassword(h, "s3cret!") {
		t.Fatal("CheckPassword should match")
	}
	if CheckPassword(h, "wrong") {
		t.Fatal("CheckPassword should reject wrong password")
	}
}

func TestIsRoleAuthorized(t *testing.T) {
	s := NewService("secret", time.Minute, time.Hour)
	for _, role := range []string{"admin", "owner", "staff"} {
		if !s.IsRoleAuthorized(role) {
			t.Fatalf("role %q should be authorized", role)
		}
	}
	for _, role := range []string{"", "root", "banned", "viewer"} {
		if s.IsRoleAuthorized(role) {
			t.Fatalf("role %q should NOT be authorized", role)
		}
	}
}
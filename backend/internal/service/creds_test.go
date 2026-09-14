package service

import (
	"strings"
	"testing"
)

func TestSessionUsernameAndParse(t *testing.T) {
	u := SessionUsername(7, "ABCD-2345")
	if u != "7-abcd2345" {
		t.Fatalf("SessionUsername = %q, want 7-abcd2345", u)
	}
	id, ok := ParseProfileFromUsername(u)
	if !ok || id != 7 {
		t.Fatalf("ParseProfileFromUsername(%q) = %d,%v; want 7,true", u, id, ok)
	}
}

func TestParseProfileFromUsernameInvalid(t *testing.T) {
	for _, in := range []string{"", "abc", "-5", "5", "x-foo"} {
		if _, ok := ParseProfileFromUsername(in); ok {
			t.Fatalf("ParseProfileFromUsername(%q) expected false", in)
		}
	}
}

func TestGenerateSessionPassword(t *testing.T) {
	for i := 0; i < 20; i++ {
		pw, err := GenerateSessionPassword()
		if err != nil {
			t.Fatalf("GenerateSessionPassword error: %v", err)
		}
		if len(pw) != 12 {
			t.Fatalf("password len = %d, want 12", len(pw))
		}
		for _, r := range pw {
			if !strings.ContainsRune(pwAlphabet, r) {
				t.Fatalf("password contains forbidden char %q", r)
			}
		}
	}
}

func TestExternalRefUnique(t *testing.T) {
	a := ExternalRef("pay", 1)
	b := ExternalRef("pay", 1)
	if a == b {
		t.Fatalf("expected unique refs, got %q twice", a)
	}
	if !strings.HasPrefix(a, "pay-1-") {
		t.Fatalf("unexpected ref format: %q", a)
	}
}
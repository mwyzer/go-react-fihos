package service

import (
	"strings"
	"testing"
)

func TestGenerateCodeFormat(t *testing.T) {
	code, err := GenerateCode()
	if err != nil {
		t.Fatalf("GenerateCode error: %v", err)
	}
	if len(code) != 9 {
		t.Fatalf("expected len 9 (XXXX-XXXX), got %q", code)
	}
	if code[4] != '-' {
		t.Fatalf("expected dash at index 4, got %q", code)
	}
	if strings.ContainsRune(code, 'O') || strings.ContainsRune(code, '0') ||
		strings.ContainsRune(code, 'I') || strings.ContainsRune(code, '1') {
		t.Fatalf("ambiguous character in code: %q", code)
	}
	for _, r := range code {
		if r == '-' {
			continue
		}
		if !strings.ContainsRune(alphabet, r) {
			t.Fatalf("code char %q not in alphabet", r)
		}
	}
}

func TestGenerateBatchCodesUnique(t *testing.T) {
	codes, err := GenerateBatchCodes(50)
	if err != nil {
		t.Fatalf("GenerateBatchCodes error: %v", err)
	}
	if len(codes) != 50 {
		t.Fatalf("expected 50 codes, got %d", len(codes))
	}
	seen := map[string]bool{}
	for _, c := range codes {
		if seen[c] {
			t.Fatalf("duplicate code: %s", c)
		}
		seen[c] = true
	}
}

func TestNormalizeCode(t *testing.T) {
	valid := []string{"ABCD-2345", "abcd-2345", "ABCD 2345", "abcd2345", "ABCD2345"}
	for _, in := range valid {
		out, err := NormalizeCode(in)
		if err != nil {
			t.Fatalf("NormalizeCode(%q) unexpected error: %v", in, err)
		}
		if out != "ABCD2345" {
			t.Fatalf("NormalizeCode(%q) = %q, want ABCD2345", in, out)
		}
	}
	invalid := []string{"", "ABC", "ABCD234", "ABCD23456", "ABCD-234O", "A0CD-2345"}
	for _, in := range invalid {
		if _, err := NormalizeCode(in); err == nil {
			t.Fatalf("NormalizeCode(%q) expected error", in)
		}
	}
}

func TestDisplayCode(t *testing.T) {
	if got := DisplayCode("ABCD2345"); got != "ABCD-2345" {
		t.Fatalf("DisplayCode = %q", got)
	}
	if got := DisplayCode("ABCD-2345"); got != "ABCD-2345" {
		t.Fatalf("DisplayCode raw = %q", got)
	}
}
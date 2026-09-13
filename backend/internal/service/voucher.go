package service

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
)

// Codec generates and validates voucher codes in "XXXX-XXXX" form using a
// confusion-free base32 alphabet (no 0/O/1/I).
const alphabet = "234567ABCDEFGHJKLMNPQRSTUVWXYZ"

func GenerateCode() (string, error) {
	var b strings.Builder
	for i := 0; i < 8; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(alphabet[n.Int64()])
	}
	s := b.String()
	return s[:4] + "-" + s[4:], nil
}

// GenerateBatchCodes returns `count` unique codes.
func GenerateBatchCodes(count int) ([]string, error) {
	seen := make(map[string]bool, count)
	out := make([]string, 0, count)
	for len(out) < count {
		code, err := GenerateCode()
		if err != nil {
			return nil, err
		}
		if !seen[code] {
			seen[code] = true
			out = append(out, code)
		}
	}
	return out, nil
}

var ErrInvalidCode = errors.New("invalid voucher code format")

// NormalizeCode strips dashes/spaces and uppercases for storage lookup.
func NormalizeCode(raw string) (string, error) {
	s := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(raw))
	if len(s) != 8 {
		return "", ErrInvalidCode
	}
	for _, r := range s {
		if !strings.ContainsRune(alphabet, r) {
			return "", ErrInvalidCode
		}
	}
	return s, nil
}

func DisplayCode(code string) string {
	if len(code) == 8 {
		return code[:4] + "-" + code[4:]
	}
	return code
}
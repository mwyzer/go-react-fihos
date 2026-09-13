package service

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"strings"
)

// Credentials generates stable credentials that RouterOS sessions can carry.
// username encodes the profile so the worker can attribute sessions back to a
// profile without a lookup: "<profileID>-<opaque>". Opaque is lowercase, dash-safe.

const pwAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

// SessionUsername derives the router username for a voucher session.
// profileID makes sessions self-describing to the sync worker.
func SessionUsername(profileID int64, voucherCode string) string {
	return strconv.FormatInt(profileID, 10) + "-" + strings.ToLower(strings.ReplaceAll(voucherCode, "-", ""))
}

// ParseProfileFromUsername extracts the profile id prefix written by SessionUsername.
func ParseProfileFromUsername(username string) (int64, bool) {
	idx := strings.IndexByte(username, '-')
	if idx <= 0 {
		return 0, false
	}
	id, err := strconv.ParseInt(username[:idx], 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// GenerateSessionPassword returns a 12-char password from an unambiguous set.
func GenerateSessionPassword() (string, error) {
	var b strings.Builder
	for i := 0; i < 12; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(pwAlphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(pwAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// ExternalRef builds a unique idempotency key for the mock payment provider.
func ExternalRef(kind string, id int64) string {
	return kind + "-" + strconv.FormatInt(id, 10) + "-" + randomHex(6)
}

func randomHex(n int) string {
	const hex = "0123456789abcdef"
	var b strings.Builder
	for i := 0; i < n; i++ {
		v, _ := rand.Int(rand.Reader, big.NewInt(16))
		b.WriteByte(hex[v.Int64()])
	}
	return b.String()
}
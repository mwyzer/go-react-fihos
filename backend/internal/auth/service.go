package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"fihos/backend/internal/middleware"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpired      = errors.New("token expired")
)

type Service struct {
	secret     []byte
	tokenTTL   time.Duration
	refreshTTL time.Duration
}

type UserAccess struct {
	ID       int64  `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role"`
	TenantID *int64 `json:"tenant_id"`
}

type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         UserAccess `json:"user"`
}

func NewService(secret string, tokenTTL, refreshTTL time.Duration) *Service {
	return &Service{secret: []byte(secret), tokenTTL: tokenTTL, refreshTTL: refreshTTL}
}

func (s *Service) AccessTTL() time.Duration  { return s.tokenTTL }
func (s *Service) RefreshTTL() time.Duration { return s.refreshTTL }

func HashPassword(plain string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(h), err
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func (s *Service) Issue(user UserAccess) (*TokenPair, string, error) {
	now := time.Now()
	access, err := s.sign(user, now, now.Add(s.tokenTTL), jwt.SigningMethodHS256, "access")
	if err != nil {
		return nil, "", err
	}
	refresh, jti, err := s.signPair(user, now, now.Add(s.refreshTTL), jwt.SigningMethodHS256)
	if err != nil {
		return nil, "", err
	}
	return &TokenPair{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresAt:    now.Add(s.tokenTTL),
		User:         user,
	}, jti, nil
}

func (s *Service) signPair(user UserAccess, issuedAt, expiresAt time.Time, method jwt.SigningMethod) (string, string, error) {
	jti := newJTI()
	claims := jwt.MapClaims{
		"uid":  user.ID,
		"role": user.Role,
		"typ":  "refresh",
		"jti":  jti,
		"iat":  issuedAt.Unix(),
		"exp":  expiresAt.Unix(),
	}
	if user.TenantID != nil {
		claims["tenant_id"] = *user.TenantID
	}
	token := jwt.NewWithClaims(method, claims)
	signed, err := token.SignedString(s.secret)
	return signed, jti, err
}

func (s *Service) sign(user UserAccess, issuedAt, expiresAt time.Time, method jwt.SigningMethod, tokenType string) (string, error) {
	claims := jwt.MapClaims{
		"uid":  user.ID,
		"role": user.Role,
		"typ":  tokenType,
		"jti":  newJTI(),
		"iat":  issuedAt.Unix(),
		"exp":  expiresAt.Unix(),
	}
	if user.TenantID != nil {
		claims["tenant_id"] = *user.TenantID
	}
	token := jwt.NewWithClaims(method, claims)
	return token.SignedString(s.secret)
}

func newJTI() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) Verify(tokenStr string) (*middleware.Claims, error) {
	mc := make(jwt.MapClaims)
	token, err := jwt.ParseWithClaims(tokenStr, mc, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}
	if mc["typ"] != "access" {
		return nil, ErrInvalidToken
	}
	exp, err := mc.GetExpirationTime()
	if err != nil || exp.Before(time.Now()) {
		return nil, ErrExpired
	}
	uidVal, ok := mc["uid"].(float64)
	if !ok {
		return nil, ErrInvalidToken
	}
	role, _ := mc["role"].(string)
	claims := &middleware.Claims{UserID: int64(uidVal), Role: role, Exp: exp.Unix()}
	if tid, ok := mc["tenant_id"].(float64); ok {
		v := int64(tid)
		claims.TenantID = &v
	}
	return claims, nil
}

func (*Service) IsRoleAuthorized(role string) bool {
	return role == "admin" || role == "owner" || role == "staff"
}

type RefreshInfo struct {
	Claims *middleware.Claims
	JTI    string
}

func (s *Service) VerifyRefresh(tokenStr string) (*RefreshInfo, error) {
	mc := make(jwt.MapClaims)
	token, err := jwt.ParseWithClaims(tokenStr, mc, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, ErrInvalidToken
	}
	if !token.Valid {
		return nil, ErrInvalidToken
	}
	if mc["typ"] != "refresh" {
		return nil, ErrInvalidToken
	}
	jti, _ := mc["jti"].(string)
	if jti == "" {
		return nil, ErrInvalidToken
	}
	exp, err := mc.GetExpirationTime()
	if err != nil || exp.Before(time.Now()) {
		return nil, ErrExpired
	}
	uidVal, ok := mc["uid"].(float64)
	if !ok {
		return nil, ErrInvalidToken
	}
	role, _ := mc["role"].(string)
	claims := &middleware.Claims{UserID: int64(uidVal), Role: role, Exp: exp.Unix()}
	if tid, ok := mc["tenant_id"].(float64); ok {
		v := int64(tid)
		claims.TenantID = &v
	}
	return &RefreshInfo{Claims: claims, JTI: jti}, nil
}

var _ middleware.AuthVerifier = (*Service)(nil)
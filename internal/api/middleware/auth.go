package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	ctxUserID   = "user_id"
	ctxUsername = "username"
)

// JWTManager issues and verifies user JWTs.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
}

func NewJWTManager(secret string, ttl time.Duration) *JWTManager {
	return &JWTManager{secret: []byte(secret), ttl: ttl}
}

// Issue creates a signed JWT for the given user.
func (m *JWTManager) Issue(userID uint, username string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": userID,
		"usr": username,
		"iat": now.Unix(),
		"exp": now.Add(m.ttl).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(m.secret)
}

func (m *JWTManager) parse(tokenStr string) (uint, string, error) {
	tok, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrTokenSignatureInvalid
		}
		return m.secret, nil
	})
	if err != nil || !tok.Valid {
		return 0, "", err
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return 0, "", jwt.ErrTokenInvalidClaims
	}
	idF, _ := claims["sub"].(float64)
	usr, _ := claims["usr"].(string)
	return uint(idF), usr, nil
}

// RequireUser is gin middleware enforcing a valid Bearer JWT.
func (m *JWTManager) RequireUser() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		id, usr, err := m.parse(strings.TrimPrefix(h, "Bearer "))
		if err != nil || id == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(ctxUserID, id)
		c.Set(ctxUsername, usr)
		c.Next()
	}
}

// UserID returns the authenticated user id from context (0 if absent).
func UserID(c *gin.Context) uint {
	if v, ok := c.Get(ctxUserID); ok {
		if id, ok := v.(uint); ok {
			return id
		}
	}
	return 0
}

// Username returns the authenticated username from context.
func Username(c *gin.Context) string {
	if v, ok := c.Get(ctxUsername); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

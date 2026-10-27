package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ssh-mesh/ssh-mesh/internal/server/store"
)

var (
	ErrInvalidToken = errors.New("invalid or expired token")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden: insufficient permissions")
)

// JWTHeader defines standard JWT header
type JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// JWTClaims defines payload claims with user role
type JWTClaims struct {
	Sub  string `json:"sub"`  // Username
	Role string `json:"role"` // "admin", "operator", "viewer"
	Exp  int64  `json:"exp"`
	Iat  int64  `json:"iat"`
}

// UserClaims is the validated context of the authenticated user
type UserClaims struct {
	Username string
	Role     string
}

// Manager handles user authentication, JWT lifecycle, and role verification
type Manager struct {
	store     store.Store
	jwtSecret []byte
}

// NewManager initializes the auth manager backed by the store
func NewManager(st store.Store) *Manager {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)

	return &Manager{
		store:     st,
		jwtSecret: secret,
	}
}

// AuthenticateUser verifies user credentials via the store and generates a JWT with user role
func (m *Manager) AuthenticateUser(username, password string) (string, *store.User, error) {
	u, err := m.store.AuthenticateUser(username, password)
	if err != nil {
		return "", nil, err
	}

	claims := JWTClaims{
		Sub:  u.Username,
		Role: u.Role,
		Iat:  time.Now().Unix(),
		Exp:  time.Now().Add(24 * time.Hour).Unix(),
	}

	tokenStr, err := m.signJWT(claims)
	if err != nil {
		return "", nil, err
	}

	return tokenStr, u, nil
}

// VerifyJWT validates a JWT string and returns the user claims (username & role)
func (m *Manager) VerifyJWT(tokenStr string) (*UserClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidToken
	}

	payloadToSign := parts[0] + "." + parts[1]
	expectedSig := m.hmacSign([]byte(payloadToSign))
	actualSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(expectedSig, actualSig) {
		return nil, ErrInvalidToken
	}

	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimBytes, &claims); err != nil {
		return nil, ErrInvalidToken
	}

	if time.Now().Unix() > claims.Exp {
		return nil, errors.New("token expired")
	}

	return &UserClaims{
		Username: claims.Sub,
		Role:     claims.Role,
	}, nil
}

// GetStore returns the underlying store
func (m *Manager) GetStore() store.Store {
	return m.store
}

// ValidateAgentToken checks if an enrollment token is valid
func (m *Manager) ValidateAgentToken(token string) bool {
	return m.store.ValidateToken(token)
}

// Middleware verifies JWT from Authorization header or 'token' query param
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := ""

		authHeader := r.Header.Get("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = strings.TrimPrefix(authHeader, "Bearer ")
		} else {
			tokenStr = r.URL.Query().Get("token")
		}

		if tokenStr == "" {
			http.Error(w, "missing authentication token", http.StatusUnauthorized)
			return
		}

		claims, err := m.VerifyJWT(tokenStr)
		if err != nil {
			http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}

		r.Header.Set("X-Authenticated-User", claims.Username)
		r.Header.Set("X-Authenticated-Role", claims.Role)
		next.ServeHTTP(w, r)
	})
}

// RequireRole enforces role authorization
func (m *Manager) RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := r.Header.Get("X-Authenticated-Role")
			for _, allowed := range allowedRoles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}
			http.Error(w, "forbidden: your role ("+role+") lacks permission for this action", http.StatusForbidden)
		})
	}
}

func (m *Manager) signJWT(claims JWTClaims) (string, error) {
	header := JWTHeader{Alg: "HS256", Typ: "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	part1 := base64.RawURLEncoding.EncodeToString(headerJSON)
	part2 := base64.RawURLEncoding.EncodeToString(claimJSON)
	payload := part1 + "." + part2

	sig := m.hmacSign([]byte(payload))
	part3 := base64.RawURLEncoding.EncodeToString(sig)

	return payload + "." + part3, nil
}

func (m *Manager) hmacSign(data []byte) []byte {
	h := hmac.New(sha256.New, m.jwtSecret)
	h.Write(data)
	return h.Sum(nil)
}

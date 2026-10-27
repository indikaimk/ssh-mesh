package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid or expired token")
	ErrUnauthorized = errors.New("unauthorized")
)

// JWTHeader defines standard JWT header
type JWTHeader struct {
	Alg string `json:"alg"`
	Typ string `json:"typ"`
}

// JWTClaims defines payload claims
type JWTClaims struct {
	Sub string `json:"sub"`
	Exp int64  `json:"exp"`
	Iat int64  `json:"iat"`
}

// EnrollmentToken represents a generated token to enroll agents
type EnrollmentToken struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

// Manager handles user authentication and agent enrollment tokens
type Manager struct {
	mu              sync.RWMutex
	adminUser       string
	adminPassword   string
	jwtSecret       []byte
	enrollmentTokens map[string]*EnrollmentToken // token string -> EnrollmentToken
}

// NewManager initializes the auth manager
func NewManager(adminUser, adminPassword string, defaultAgentToken string) *Manager {
	if adminUser == "" {
		adminUser = "admin"
	}
	if adminPassword == "" {
		adminPassword = "admin"
	}

	secret := make([]byte, 32)
	_, _ = rand.Read(secret)

	m := &Manager{
		adminUser:        adminUser,
		adminPassword:    adminPassword,
		jwtSecret:        secret,
		enrollmentTokens: make(map[string]*EnrollmentToken),
	}

	// Register default agent token
	if defaultAgentToken != "" {
		m.enrollmentTokens[defaultAgentToken] = &EnrollmentToken{
			ID:        "default",
			Name:      "Default Shared Token",
			Token:     defaultAgentToken,
			CreatedAt: time.Now(),
			ExpiresAt: time.Now().Add(365 * 24 * time.Hour),
			Revoked:   false,
		}
	}

	return m
}

// AuthenticateUser verifies admin credentials and generates a JWT
func (m *Manager) AuthenticateUser(username, password string) (string, error) {
	if username != m.adminUser || password != m.adminPassword {
		return "", errors.New("invalid username or password")
	}

	claims := JWTClaims{
		Sub: username,
		Iat: time.Now().Unix(),
		Exp: time.Now().Add(24 * time.Hour).Unix(),
	}

	return m.signJWT(claims)
}

// VerifyJWT validates a JWT string and returns the subject
func (m *Manager) VerifyJWT(tokenStr string) (string, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return "", ErrInvalidToken
	}

	payloadToSign := parts[0] + "." + parts[1]
	expectedSig := m.hmacSign([]byte(payloadToSign))
	actualSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(expectedSig, actualSig) {
		return "", ErrInvalidToken
	}

	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrInvalidToken
	}

	var claims JWTClaims
	if err := json.Unmarshal(claimBytes, &claims); err != nil {
		return "", ErrInvalidToken
	}

	if time.Now().Unix() > claims.Exp {
		return "", errors.New("token expired")
	}

	return claims.Sub, nil
}

// ValidateAgentToken checks if an enrollment token is valid and not revoked
func (m *Manager) ValidateAgentToken(token string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, exists := m.enrollmentTokens[token]
	if !exists {
		return false
	}
	if entry.Revoked {
		return false
	}
	if !entry.ExpiresAt.IsZero() && time.Now().After(entry.ExpiresAt) {
		return false
	}
	return true
}

// CreateEnrollmentToken generates a new agent enrollment token
func (m *Manager) CreateEnrollmentToken(name string, duration time.Duration) *EnrollmentToken {
	m.mu.Lock()
	defer m.mu.Unlock()

	randBytes := make([]byte, 24)
	_, _ = rand.Read(randBytes)
	tokenStr := "mesh_" + hex.EncodeToString(randBytes)

	expiresAt := time.Time{}
	if duration > 0 {
		expiresAt = time.Now().Add(duration)
	}

	entry := &EnrollmentToken{
		ID:        fmt.Sprintf("tok_%d", time.Now().UnixNano()),
		Name:      name,
		Token:     tokenStr,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
		Revoked:   false,
	}

	m.enrollmentTokens[tokenStr] = entry
	return entry
}

// ListEnrollmentTokens returns all enrollment tokens
func (m *Manager) ListEnrollmentTokens() []*EnrollmentToken {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tokens := make([]*EnrollmentToken, 0, len(m.enrollmentTokens))
	for _, t := range m.enrollmentTokens {
		copyTok := *t
		tokens = append(tokens, &copyTok)
	}
	return tokens
}

// RevokeEnrollmentToken marks a token as revoked
func (m *Manager) RevokeEnrollmentToken(idOrToken string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	for k, t := range m.enrollmentTokens {
		if t.ID == idOrToken || k == idOrToken {
			t.Revoked = true
			return true
		}
	}
	return false
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

		user, err := m.VerifyJWT(tokenStr)
		if err != nil {
			http.Error(w, "unauthorized: "+err.Error(), http.StatusUnauthorized)
			return
		}

		// Attach user to header for downstream handlers if needed
		r.Header.Set("X-Authenticated-User", user)
		next.ServeHTTP(w, r)
	})
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

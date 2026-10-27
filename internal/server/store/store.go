package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/ssh-mesh/ssh-mesh/internal/protocol"
	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("username already exists")
	ErrInvalidPassword   = errors.New("invalid username or password")
	ErrCannotDeleteAdmin = errors.New("cannot delete the primary admin account")
)

// User represents an authenticated dashboard user
type User struct {
	ID           string  `db:"id" json:"id"`
	Username     string  `db:"username" json:"username"`
	PasswordHash string  `db:"password_hash" json:"-"`
	Role         string  `db:"role" json:"role"` // "admin", "operator", "viewer"
	AllowedNodes string  `db:"allowed_nodes" json:"allowed_nodes"` // JSON array, e.g. ["*"] or ["env:staging", "node:vps-1"]
	CreatedAt    string  `db:"created_at" json:"created_at"`
	LastLoginAt  *string `db:"last_login_at" json:"last_login_at,omitempty"`
}

// CanAccessNode evaluates whether the user has permission to view or connect to the agent node
func (u *User) CanAccessNode(agent *protocol.AgentInfo) bool {
	if u.Role == "admin" {
		return true
	}
	if agent == nil {
		return false
	}

	var rules []string
	if u.AllowedNodes == "" || u.AllowedNodes == `["*"]` {
		rules = []string{"*"}
	} else {
		_ = json.Unmarshal([]byte(u.AllowedNodes), &rules)
	}

	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "*" {
			return true
		}
		// Match direct Node ID: "node:vps-1" or "vps-1"
		if strings.HasPrefix(rule, "node:") {
			if strings.TrimPrefix(rule, "node:") == agent.ID || strings.TrimPrefix(rule, "node:") == agent.Hostname {
				return true
			}
		} else if rule == agent.ID || rule == agent.Hostname {
			return true
		}

		// Match Tags: "env:staging", "tag:env=staging", "env=staging"
		tagRule := rule
		if strings.HasPrefix(tagRule, "tag:") {
			tagRule = strings.TrimPrefix(tagRule, "tag:")
		}

		var key, val string
		if strings.Contains(tagRule, "=") {
			parts := strings.SplitN(tagRule, "=", 2)
			key, val = parts[0], parts[1]
		} else if strings.Contains(tagRule, ":") {
			parts := strings.SplitN(tagRule, ":", 2)
			key, val = parts[0], parts[1]
		}

		if key != "" && agent.Tags != nil {
			if agent.Tags[key] == val {
				return true
			}
		}
	}

	return false
}

// EnrollmentToken represents a token for agent enrollment
type EnrollmentToken struct {
	ID        string  `db:"id" json:"id"`
	Name      string  `db:"name" json:"name"`
	Token     string  `db:"token" json:"token"`
	CreatedAt string  `db:"created_at" json:"created_at"`
	ExpiresAt *string `db:"expires_at" json:"expires_at,omitempty"`
	Revoked   bool    `db:"revoked" json:"revoked"`
}

// Store defines persistence operations for users and tokens
type Store interface {
	AuthenticateUser(username, password string) (*User, error)
	CreateUser(username, password, role, allowedNodes string) (*User, error)
	GetUser(username string) (*User, error)
	ListUsers() ([]*User, error)
	DeleteUser(id string) error
	UpdateUser(id, role, allowedNodes string) error
	UpdatePassword(username, newPassword string) error
	CreateToken(name string, duration time.Duration) (*EnrollmentToken, error)
	ListTokens() ([]*EnrollmentToken, error)
	ValidateToken(token string) bool
	RevokeToken(id string) error
	Close() error
}

// SQLiteStore implements Store using sqlx and modernc.org/sqlite
type SQLiteStore struct {
	mu sync.RWMutex
	db *sqlx.DB
}

// NewSQLiteStore initializes SQLite database and runs initial schema migrations using sqlx
func NewSQLiteStore(dbPath string, defaultAdminUser, defaultAdminPass, defaultToken string) (*SQLiteStore, error) {
	if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	db, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database with sqlx: %w", err)
	}

	// Performance pragmas for SQLite
	if _, err := db.Exec(`
		PRAGMA journal_mode = WAL;
		PRAGMA busy_timeout = 5000;
	`); err != nil {
		return nil, fmt.Errorf("failed to set pragma: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}

	// Seed admin user if not exists
	if defaultAdminUser != "" && defaultAdminPass != "" {
		if err := s.seedAdmin(defaultAdminUser, defaultAdminPass); err != nil {
			return nil, fmt.Errorf("failed to seed admin: %w", err)
		}
	}

	// Seed default agent token if not exists
	if defaultToken != "" {
		_ = s.seedToken("Default Agent Token", defaultToken)
	}

	return s, nil
}

func (s *SQLiteStore) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'operator',
		allowed_nodes TEXT NOT NULL DEFAULT '["*"]',
		created_at TEXT NOT NULL,
		last_login_at TEXT
	);

	CREATE TABLE IF NOT EXISTS enrollment_tokens (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		token TEXT UNIQUE NOT NULL,
		created_at TEXT NOT NULL,
		expires_at TEXT,
		revoked INTEGER NOT NULL DEFAULT 0
	);
	`
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}

	// Ensure allowed_nodes column exists if migrating an existing database
	_, _ = s.db.Exec(`ALTER TABLE users ADD COLUMN allowed_nodes TEXT NOT NULL DEFAULT '["*"]'`)
	return nil
}

func (s *SQLiteStore) seedAdmin(username, password string) error {
	var count int
	err := s.db.Get(&count, "SELECT COUNT(*) FROM users WHERE username = ?", username)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	u := &User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         "admin",
		AllowedNodes: `["*"]`,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	_, err = s.db.NamedExec(`
		INSERT INTO users (id, username, password_hash, role, allowed_nodes, created_at)
		VALUES (:id, :username, :password_hash, :role, :allowed_nodes, :created_at)
	`, u)
	return err
}

func (s *SQLiteStore) seedToken(name, token string) error {
	var count int
	err := s.db.Get(&count, "SELECT COUNT(*) FROM enrollment_tokens WHERE token = ?", token)
	if err != nil || count > 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	exp := time.Now().Add(365 * 24 * time.Hour).UTC().Format(time.RFC3339)
	tok := &EnrollmentToken{
		ID:        "default",
		Name:      name,
		Token:     token,
		CreatedAt: now,
		ExpiresAt: &exp,
		Revoked:   false,
	}

	_, err = s.db.NamedExec(`
		INSERT INTO enrollment_tokens (id, name, token, created_at, expires_at, revoked)
		VALUES (:id, :name, :token, :created_at, :expires_at, :revoked)
	`, tok)
	return err
}

func (s *SQLiteStore) AuthenticateUser(username, password string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var u User
	err := s.db.Get(&u, "SELECT * FROM users WHERE username = ?", username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidPassword
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidPassword
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.Exec("UPDATE users SET last_login_at = ? WHERE id = ?", now, u.ID)
	u.LastLoginAt = &now

	return &u, nil
}

func (s *SQLiteStore) GetUser(username string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var u User
	err := s.db.Get(&u, "SELECT * FROM users WHERE username = ?", username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &u, nil
}

func (s *SQLiteStore) CreateUser(username, password, role, allowedNodes string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if role != "admin" && role != "operator" && role != "viewer" {
		role = "operator"
	}
	if allowedNodes == "" {
		allowedNodes = `["*"]`
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	u := &User{
		ID:           uuid.New().String(),
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
		AllowedNodes: allowedNodes,
		CreatedAt:    now,
	}

	_, err = s.db.NamedExec(`
		INSERT INTO users (id, username, password_hash, role, allowed_nodes, created_at)
		VALUES (:id, :username, :password_hash, :role, :allowed_nodes, :created_at)
	`, u)

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUserAlreadyExists, err)
	}

	return u, nil
}

func (s *SQLiteStore) ListUsers() ([]*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var users []*User
	err := s.db.Select(&users, "SELECT * FROM users ORDER BY created_at ASC")
	if err != nil {
		return nil, err
	}
	return users, nil
}

func (s *SQLiteStore) UpdateUser(id, role, allowedNodes string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if allowedNodes == "" {
		allowedNodes = `["*"]`
	}
	res, err := s.db.Exec("UPDATE users SET role = ?, allowed_nodes = ? WHERE id = ?", role, allowedNodes, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *SQLiteStore) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var u User
	err := s.db.Get(&u, "SELECT id, username, role FROM users WHERE id = ?", id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}

	if u.Role == "admin" {
		var adminCount int
		_ = s.db.Get(&adminCount, "SELECT COUNT(*) FROM users WHERE role = 'admin'")
		if adminCount <= 1 {
			return errors.New("cannot delete the only remaining admin account")
		}
	}

	_, err = s.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (s *SQLiteStore) UpdatePassword(username, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	res, err := s.db.Exec("UPDATE users SET password_hash = ? WHERE username = ?", string(hash), username)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (s *SQLiteStore) CreateToken(name string, duration time.Duration) (*EnrollmentToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	randBytes := make([]byte, 24)
	_, _ = rand.Read(randBytes)
	tokenStr := "mesh_" + hex.EncodeToString(randBytes)

	now := time.Now().UTC().Format(time.RFC3339)
	var expStr *string
	if duration > 0 {
		exp := time.Now().Add(duration).UTC().Format(time.RFC3339)
		expStr = &exp
	}

	t := &EnrollmentToken{
		ID:        uuid.New().String(),
		Name:      name,
		Token:     tokenStr,
		CreatedAt: now,
		ExpiresAt: expStr,
		Revoked:   false,
	}

	_, err := s.db.NamedExec(`
		INSERT INTO enrollment_tokens (id, name, token, created_at, expires_at, revoked)
		VALUES (:id, :name, :token, :created_at, :expires_at, :revoked)
	`, t)
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (s *SQLiteStore) ListTokens() ([]*EnrollmentToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var tokens []*EnrollmentToken
	err := s.db.Select(&tokens, "SELECT * FROM enrollment_tokens ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	return tokens, nil
}

func (s *SQLiteStore) ValidateToken(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var t EnrollmentToken
	err := s.db.Get(&t, "SELECT * FROM enrollment_tokens WHERE token = ?", token)
	if err != nil || t.Revoked {
		return false
	}

	if t.ExpiresAt != nil && *t.ExpiresAt != "" {
		if exp, err := time.Parse(time.RFC3339, *t.ExpiresAt); err == nil {
			if !exp.IsZero() && time.Now().After(exp) {
				return false
			}
		}
	}

	return true
}

func (s *SQLiteStore) RevokeToken(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	res, err := s.db.Exec("UPDATE enrollment_tokens SET revoked = 1 WHERE id = ? OR token = ?", id, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errors.New("token not found")
	}
	return nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

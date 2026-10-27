package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
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
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	Role        string     `json:"role"` // "admin", "operator", "viewer"
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// EnrollmentToken represents a token for agent enrollment
type EnrollmentToken struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

// Store defines persistence operations for users and tokens
type Store interface {
	AuthenticateUser(username, password string) (*User, error)
	CreateUser(username, password, role string) (*User, error)
	ListUsers() ([]*User, error)
	DeleteUser(id string) error
	UpdatePassword(username, newPassword string) error
	CreateToken(name string, duration time.Duration) (*EnrollmentToken, error)
	ListTokens() ([]*EnrollmentToken, error)
	ValidateToken(token string) bool
	RevokeToken(id string) error
	Close() error
}

// SQLiteStore implements Store using modernc.org/sqlite
type SQLiteStore struct {
	mu sync.RWMutex
	db *sql.DB
}

// NewSQLiteStore initializes SQLite database and runs initial schema migrations
func NewSQLiteStore(dbPath string, defaultAdminUser, defaultAdminPass, defaultToken string) (*SQLiteStore, error) {
	if dir := filepath.Dir(dbPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
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
	_, err := s.db.Exec(schema)
	return err
}

func (s *SQLiteStore) seedAdmin(username, password string) error {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", username).Scan(&count)
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

	id := uuid.New().String()
	_, err = s.db.Exec(`
		INSERT INTO users (id, username, password_hash, role, created_at)
		VALUES (?, ?, ?, 'admin', ?)
	`, id, username, string(hash), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) seedToken(name, token string) error {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM enrollment_tokens WHERE token = ?", token).Scan(&count)
	if err != nil || count > 0 {
		return nil
	}

	_, err = s.db.Exec(`
		INSERT INTO enrollment_tokens (id, name, token, created_at, expires_at, revoked)
		VALUES ('default', ?, ?, ?, ?, 0)
	`, name, token, time.Now().UTC().Format(time.RFC3339), time.Now().Add(365*24*time.Hour).UTC().Format(time.RFC3339))
	return err
}

func (s *SQLiteStore) AuthenticateUser(username, password string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var u User
	var hash string
	var createdAtStr string
	var lastLoginStr sql.NullString

	row := s.db.QueryRow(`
		SELECT id, username, password_hash, role, created_at, last_login_at
		FROM users
		WHERE username = ?
	`, username)

	if err := row.Scan(&u.ID, &u.Username, &hash, &u.Role, &createdAtStr, &lastLoginStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidPassword
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, ErrInvalidPassword
	}

	u.CreatedAt, _ = parseTime(createdAtStr)
	now := time.Now().UTC()
	_, _ = s.db.Exec("UPDATE users SET last_login_at = ? WHERE id = ?", now.Format(time.RFC3339), u.ID)
	u.LastLoginAt = &now

	return &u, nil
}

func (s *SQLiteStore) CreateUser(username, password, role string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if role != "admin" && role != "operator" && role != "viewer" {
		role = "operator"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	u := &User{
		ID:        uuid.New().String(),
		Username:  username,
		Role:      role,
		CreatedAt: now,
	}

	_, err = s.db.Exec(`
		INSERT INTO users (id, username, password_hash, role, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, u.ID, u.Username, string(hash), u.Role, now.Format(time.RFC3339))

	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUserAlreadyExists, err)
	}

	return u, nil
}

func (s *SQLiteStore) ListUsers() ([]*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, username, role, created_at, last_login_at
		FROM users
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]*User, 0)
	for rows.Next() {
		var u User
		var createdAtStr string
		var lastLoginStr sql.NullString
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &createdAtStr, &lastLoginStr); err != nil {
			return nil, err
		}
		u.CreatedAt, _ = parseTime(createdAtStr)
		if lastLoginStr.Valid {
			t, err := parseTime(lastLoginStr.String)
			if err == nil {
				u.LastLoginAt = &t
			}
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

func (s *SQLiteStore) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var username, role string
	err := s.db.QueryRow("SELECT username, role FROM users WHERE id = ?", id).Scan(&username, &role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}

	// Prevent deleting the primary admin account if it's the only admin
	if role == "admin" {
		var adminCount int
		_ = s.db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)
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

	now := time.Now().UTC()
	expiresAt := time.Time{}
	var expiresAtStr string
	if duration > 0 {
		expiresAt = now.Add(duration)
		expiresAtStr = expiresAt.Format(time.RFC3339)
	}

	t := &EnrollmentToken{
		ID:        uuid.New().String(),
		Name:      name,
		Token:     tokenStr,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		Revoked:   false,
	}

	_, err := s.db.Exec(`
		INSERT INTO enrollment_tokens (id, name, token, created_at, expires_at, revoked)
		VALUES (?, ?, ?, ?, ?, 0)
	`, t.ID, t.Name, t.Token, now.Format(time.RFC3339), sqlNullString(expiresAtStr))
	if err != nil {
		return nil, err
	}

	return t, nil
}

func (s *SQLiteStore) ListTokens() ([]*EnrollmentToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`
		SELECT id, name, token, created_at, expires_at, revoked
		FROM enrollment_tokens
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]*EnrollmentToken, 0)
	for rows.Next() {
		var t EnrollmentToken
		var createdAtStr string
		var expStr sql.NullString
		if err := rows.Scan(&t.ID, &t.Name, &t.Token, &createdAtStr, &expStr, &t.Revoked); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = parseTime(createdAtStr)
		if expStr.Valid && expStr.String != "" {
			t.ExpiresAt, _ = parseTime(expStr.String)
		}
		tokens = append(tokens, &t)
	}
	return tokens, rows.Err()
}

func (s *SQLiteStore) ValidateToken(token string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var expStr sql.NullString
	var revoked bool

	err := s.db.QueryRow(`
		SELECT expires_at, revoked
		FROM enrollment_tokens
		WHERE token = ?
	`, token).Scan(&expStr, &revoked)

	if err != nil || revoked {
		return false
	}

	if expStr.Valid && expStr.String != "" {
		exp, err := parseTime(expStr.String)
		if err == nil && !exp.IsZero() && time.Now().After(exp) {
			return false
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

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02 15:04:05", s)
}

func sqlNullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

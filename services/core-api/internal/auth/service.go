package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const SessionCookieName = "dawha_session"

const sessionCookieName = SessionCookieName

var (
	ErrDatabaseUnavailable = errors.New("authentication database is unavailable")
	ErrInvalidCredentials  = errors.New("invalid credentials")
	ErrEmailTaken          = errors.New("email is already registered")
	ErrWeakPassword        = errors.New("password must contain at least 12 characters")
	ErrQuickSignInUnsafe   = errors.New("the reserved local reviewer account cannot be used")
)

// devQuickSignInEmail is the one reserved account the local development
// shortcut signs in as.
//
// The domain is deliberately NOT one the test gate reserves. `testsupport`.
// SyntheticEmailSuffixes owns `@example.test` and `@dawha.test` for the actors
// a fixture creates, and the database gate fails a run when a row in that
// marker survives. This account is not a fixture actor: it is a real account
// that is meant to persist in a developer's own database, and it is created by
// pressing a button, not by a test. With the address in a reserved domain the
// gate read it as a leaked test actor and failed the acceptance run for a
// developer who had simply signed in locally.
//
// `@dawha.local` says what it is: a local account, outside every marker.
const devQuickSignInEmail = "local-reviewer@dawha.local"
const devQuickSignInName = "مراجع محلي مؤقت"

type User struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name_ar"`
	CreatedAt   time.Time `json:"created_at"`
}

type Service struct {
	Pool       *pgxpool.Pool
	SessionTTL time.Duration
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{Pool: pool, SessionTTL: 30 * 24 * time.Hour}
}

func (s *Service) Register(ctx context.Context, email, displayName, password string) (User, error) {
	if s == nil || s.Pool == nil {
		return User{}, ErrDatabaseUnavailable
	}
	email = strings.ToLower(strings.TrimSpace(email))
	displayName = strings.TrimSpace(displayName)
	if email == "" || displayName == "" {
		return User{}, errors.New("email and display name are required")
	}
	if len([]rune(password)) < 12 {
		return User{}, ErrWeakPassword
	}
	passwordHash, err := HashPassword(password)
	if err != nil {
		return User{}, err
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)

	var user User
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, display_name_ar)
		VALUES ($1, $2)
		RETURNING id, email, display_name_ar, created_at
	`, email, displayName).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt); err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return User{}, ErrEmailTaken
		}
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_credentials (user_id, password_hash) VALUES ($1, $2)`, user.ID, passwordHash); err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'registered')`, user.ID); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (User, string, error) {
	if s == nil || s.Pool == nil {
		return User{}, "", ErrDatabaseUnavailable
	}
	email = strings.ToLower(strings.TrimSpace(email))
	var user User
	var passwordHash string
	if err := s.Pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.display_name_ar, u.created_at, c.password_hash
		FROM users u
		JOIN user_credentials c ON c.user_id = u.id
		WHERE u.email = $1 AND u.status = 'active'
	`, email).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt, &passwordHash); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, "", ErrInvalidCredentials
		}
		return User{}, "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return User{}, "", ErrInvalidCredentials
	}
	token, err := s.createSession(ctx, user.ID)
	if err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

// QuickSignIn provisions the one ordinary reviewer account used by the local
// development shortcut. The router only exposes this operation when the
// development flag is on, and the HTTP handler accepts loopback requests only.
// This account deliberately receives no platform role beyond registered.
func (s *Service) QuickSignIn(ctx context.Context) (User, string, error) {
	if s == nil || s.Pool == nil {
		return User{}, "", ErrDatabaseUnavailable
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return User{}, "", err
	}
	defer tx.Rollback(ctx)

	var user User
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (email, display_name_ar)
		VALUES ($1, $2)
		ON CONFLICT (email) DO UPDATE SET
			display_name_ar = EXCLUDED.display_name_ar,
			updated_at = now()
		WHERE users.status = 'active'
		RETURNING id, email, display_name_ar, created_at
	`, devQuickSignInEmail, devQuickSignInName).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, "", ErrQuickSignInUnsafe
		}
		return User{}, "", err
	}

	var elevatedRoleCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM user_roles WHERE user_id = $1 AND role <> 'registered'`, user.ID).Scan(&elevatedRoleCount); err != nil {
		return User{}, "", err
	}
	if elevatedRoleCount != 0 {
		return User{}, "", ErrQuickSignInUnsafe
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_roles (user_id, role) VALUES ($1, 'registered') ON CONFLICT DO NOTHING`, user.ID); err != nil {
		return User{}, "", err
	}

	token, err := NewSessionToken()
	if err != nil {
		return User{}, "", err
	}
	expiresAt := time.Now().UTC().Add(s.SessionTTL)
	if _, err := tx.Exec(ctx, `
		INSERT INTO auth_sessions (id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, uuid.New(), user.ID, HashToken(token), expiresAt); err != nil {
		return User{}, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, "", err
	}
	return user, token, nil
}

func (s *Service) UserFromToken(ctx context.Context, token string) (User, error) {
	if s == nil || s.Pool == nil {
		return User{}, ErrDatabaseUnavailable
	}
	if strings.TrimSpace(token) == "" {
		return User{}, ErrInvalidCredentials
	}
	var user User
	if err := s.Pool.QueryRow(ctx, `
		SELECT u.id, u.email, u.display_name_ar, u.created_at
		FROM auth_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now() AND u.status = 'active'
	`, HashToken(token)).Scan(&user.ID, &user.Email, &user.DisplayName, &user.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}
	_, _ = s.Pool.Exec(ctx, `UPDATE auth_sessions SET last_seen_at = now() WHERE token_hash = $1`, HashToken(token))
	return user, nil
}

func (s *Service) UserFromRequest(ctx context.Context, r *http.Request) (User, error) {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return User{}, ErrInvalidCredentials
	}
	return s.UserFromToken(ctx, cookie.Value)
}

// CurrentRoles describes the existing session's platform permissions for the
// interface. Authorization of each operation remains in its own service.
func (s *Service) CurrentRoles(ctx context.Context, userID string) ([]Role, error) {
	if s == nil || s.Pool == nil {
		return nil, ErrDatabaseUnavailable
	}
	rows, err := s.Pool.Query(ctx, `SELECT role FROM user_roles WHERE user_id = $1 ORDER BY role`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := []Role{}
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (s *Service) RevokeToken(ctx context.Context, token string) error {
	if s == nil || s.Pool == nil {
		return ErrDatabaseUnavailable
	}
	if strings.TrimSpace(token) == "" {
		return nil
	}
	_, err := s.Pool.Exec(ctx, `UPDATE auth_sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, HashToken(token))
	return err
}

func HashPassword(password string) (string, error) {
	if len([]rune(password)) < 12 {
		return "", ErrWeakPassword
	}
	value, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(value), nil
}

func NewSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func HashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return fmt.Sprintf("%x", digest[:])
}

func (s *Service) createSession(ctx context.Context, userID string) (string, error) {
	token, err := NewSessionToken()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().UTC().Add(s.SessionTTL)
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO auth_sessions (id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, uuid.New(), userID, HashToken(token), expiresAt)
	if err != nil {
		return "", err
	}
	return token, nil
}

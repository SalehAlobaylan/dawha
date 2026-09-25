package auth

import (
	"errors"
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
)

// The register -> login -> session -> revoke journey, against real
// PostgreSQL.
//
// Every account, credential and session here belongs to this test's isolated
// schema, so the test can be rerun with -count=N, in any order, alongside every
// other package, without touching the developer's database.
func TestRegisterLoginSessionLifecycle(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	email := fixture.Email()
	password := fixture.Unique("correct-horse")

	user, err := service.Register(fixture.Ctx(), email, "باحث دورة", password)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if user.Email != email || user.ID == "" {
		t.Fatalf("registered user = %+v, want the email and id it was given", user)
	}
	// The public-facing journey depends on a registered account carrying the
	// registered role, because that role is what the permission policy reads.
	if got := fixture.Count(`SELECT count(*) FROM user_roles WHERE user_id = $1 AND role = 'registered'`, user.ID); got != 1 {
		t.Fatalf("registered role rows = %d, want 1", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM user_credentials WHERE user_id = $1`, user.ID); got != 1 {
		t.Fatalf("credential rows = %d, want 1", got)
	}
	// The password is stored hashed, never as the value the caller sent.
	var stored string
	if err := fixture.QueryRow(`SELECT password_hash FROM user_credentials WHERE user_id = $1`, user.ID).Scan(&stored); err != nil {
		t.Fatalf("read credential: %v", err)
	}
	if stored == password || stored == "" {
		t.Fatalf("stored credential %q is not a hash of the submitted password", stored)
	}
	if _, _, err := service.Login(fixture.Ctx(), email, fixture.Unique("not-the-password")); err == nil {
		t.Fatal("login with a wrong password succeeded")
	} else if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password = %v, want ErrInvalidCredentials", err)
	}

	loggedIn, token, err := service.Login(fixture.Ctx(), email, password)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if loggedIn.ID != user.ID {
		t.Fatalf("logged in as %s, want %s", loggedIn.ID, user.ID)
	}
	session, err := service.UserFromToken(fixture.Ctx(), token)
	if err != nil {
		t.Fatalf("resolve session: %v", err)
	}
	if session.ID != user.ID {
		t.Fatalf("session resolved to %s, want %s", session.ID, user.ID)
	}
	// The database stores a hash of the token, so a leaked table cannot be
	// replayed as a live session.
	if got := fixture.Count(`SELECT count(*) FROM auth_sessions WHERE token_hash = $1 AND token_hash <> $2`, HashToken(token), token); got != 1 {
		t.Fatalf("stored session rows = %d, want 1 keyed by a hash", got)
	}

	if err := service.RevokeToken(fixture.Ctx(), token); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := service.UserFromToken(fixture.Ctx(), token); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("revoked session = %v, want ErrInvalidCredentials", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM auth_sessions WHERE revoked_at IS NULL AND user_id = $1`, user.ID); got != 0 {
		t.Fatalf("live sessions after revoke = %d, want 0", got)
	}
}

func TestRegisterRejectsADuplicateEmail(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	email := fixture.Email()
	password := fixture.Unique("correct-horse")

	if _, err := service.Register(fixture.Ctx(), email, "أول", password); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if _, err := service.Register(fixture.Ctx(), email, "ثانٍ", password); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("second register = %v, want ErrEmailTaken", err)
	}
	if got := fixture.Count(`SELECT count(*) FROM users WHERE email = $1`, email); got != 1 {
		t.Fatalf("user rows for the email = %d, want 1", got)
	}
}

func TestRegisterRejectsAWeakPassword(t *testing.T) {
	fixture := testsupport.New(t)
	service := NewService(fixture.Pool())
	email := fixture.Email()

	if _, err := service.Register(fixture.Ctx(), email, "باحث", "قصير"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("short password = %v, want ErrWeakPassword", err)
	}
	// A refused registration leaves no half-written account behind.
	if got := fixture.Count(`SELECT count(*) FROM users WHERE email = $1`, email); got != 0 {
		t.Fatalf("user rows after a refused registration = %d, want 0", got)
	}
	if got := fixture.Count(`SELECT count(*) FROM user_credentials`); got != 0 {
		t.Fatalf("credential rows after a refused registration = %d, want 0", got)
	}
}

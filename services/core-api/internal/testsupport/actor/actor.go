// Package actor registers the user accounts the acceptance tests act as.
//
// It lives beside the fixtures rather than inside them so internal/auth can use
// it without an import cycle: a test in package auth registers through the
// production auth service directly, while every other package reaches for
// actor.Register.
package actor

import (
	"testing"

	"github.com/SalehAlobaylan/dawha/services/core-api/internal/auth"
	"github.com/SalehAlobaylan/dawha/services/core-api/internal/testsupport"
)

// Actor is a registered account together with the live session the HTTP layer
// would read out of its cookie.
type Actor struct {
	User     auth.User
	Email    string
	Password string
	Token    string
}

// Register creates an account through the production auth service, so the
// fixture exercises the same path a real signup takes rather than inserting a
// row behind the service's back.
func Register(t *testing.T, fixture *testsupport.Fixture, displayName string) Actor {
	t.Helper()
	email := fixture.Email()
	password := fixture.Unique("correct-horse-battery")
	user, err := auth.NewService(fixture.Pool()).Register(fixture.Ctx(), email, displayName, password)
	if err != nil {
		t.Fatalf("register %s: %v", email, err)
	}
	return Actor{User: user, Email: email, Password: password}
}

// RegisterAndLogin creates an account and opens a session for it.
func RegisterAndLogin(t *testing.T, fixture *testsupport.Fixture, displayName string) Actor {
	t.Helper()
	registered := Register(t, fixture, displayName)
	registered.Token = Login(t, fixture, registered.Email, registered.Password)
	return registered
}

// Login opens a session for an already registered account.
func Login(t *testing.T, fixture *testsupport.Fixture, email, password string) string {
	t.Helper()
	_, token, err := auth.NewService(fixture.Pool()).Login(fixture.Ctx(), email, password)
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}
	return token
}

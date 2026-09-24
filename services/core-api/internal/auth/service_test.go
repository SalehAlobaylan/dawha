package auth

import (
	"strings"
	"testing"
)

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("unexpected hash error: %v", err)
	}
	if len(hash) == 0 || hash == "correct horse battery staple" {
		t.Fatal("password was not hashed")
	}
	if _, err := HashPassword("short"); err != ErrWeakPassword {
		t.Fatalf("expected weak password error, got %v", err)
	}
}

func TestSessionTokensAreHashed(t *testing.T) {
	token, err := NewSessionToken()
	if err != nil {
		t.Fatalf("unexpected token error: %v", err)
	}
	if len(token) < 40 || strings.Contains(token, " ") {
		t.Fatal("expected an opaque random token")
	}
	if HashToken(token) == token || len(HashToken(token)) != 64 {
		t.Fatal("expected a one-way token hash")
	}
}

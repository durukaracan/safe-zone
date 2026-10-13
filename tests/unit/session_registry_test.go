package unit

import (
	"strings"
	"testing"

	"thyris-sz/internal/auth"
)

func TestUserSessionsKey_IsPerUser(t *testing.T) {
	if auth.UserSessionsKey(7) == auth.UserSessionsKey(8) {
		t.Fatal("different users must have different registers")
	}
	if got := auth.UserSessionsKey(7); got != "user_sessions:7" {
		t.Fatalf("unexpected key %q", got)
	}
}

func TestUserSessionsKey_NeverLooksLikeASessionKey(t *testing.T) {
	// A register must never be readable as a session, or the middleware
	// could treat a set of ids as a login.
	if strings.HasPrefix(auth.UserSessionsKey(1), "session:") {
		t.Fatal("register key must not start with the session prefix")
	}
}

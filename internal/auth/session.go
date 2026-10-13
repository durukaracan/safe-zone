package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"thyris-sz/internal/cache"
)

const (
	// SessionCookieName is the cookie the browser stores the session ID in.
	SessionCookieName = "sz_session"
	// SessionTTL controls how long a session stays valid in Redis. After
	// this, the user is logged out automatically even without clicking
	// "logout" -- Redis expires the key on its own.
	SessionTTL = 24 * time.Hour

	sessionKeyPrefix = "session:"

	// userSessionsKeyPrefix names a Redis SET per user that lists the ids of
	// that user's live sessions. It is the "register" that lets us find and
	// close every session of one user, for example after a password change.
	userSessionsKeyPrefix = "user_sessions:"
)

// UserSessionsKey returns the Redis key of the set holding a user's
// session ids.
func UserSessionsKey(userID uint) string {
	return userSessionsKeyPrefix + strconv.FormatUint(uint64(userID), 10)
}

// Session is what we store in Redis per logged-in user. Kept intentionally
// small -- anything else about the user (permissions, preferences) should
// be looked up from the DB by UserID when needed, not duplicated here.
type Session struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}

// CreateSession generates a new random session ID, stores the session data
// in Redis under that ID with a TTL, and returns the ID. The caller (the
// login handler, in step 5) is responsible for setting it as a cookie.
func CreateSession(ctx context.Context, s Session) (string, error) {
	id, err := generateSessionID()
	if err != nil {
		return "", err
	}

	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}

	if err := cache.RDB.Set(ctx, sessionKeyPrefix+id, data, SessionTTL).Err(); err != nil {
		return "", err
	}

	// Write the id into the user's register. The register lives as long as
	// the newest session, so it always outlives every session listed in it.
	userKey := UserSessionsKey(s.UserID)
	pipe := cache.RDB.TxPipeline()
	pipe.SAdd(ctx, userKey, id)
	pipe.Expire(ctx, userKey, SessionTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		// A session that cannot be revoked later is worse than a failed
		// sign-in, so remove it and report the error.
		_ = cache.RDB.Del(ctx, sessionKeyPrefix+id).Err()
		return "", err
	}

	return id, nil
}

// GetSession looks up a session by ID. Returns an error if the ID is empty,
// missing from Redis, or expired -- callers should treat any error as
// "not authenticated".
func GetSession(ctx context.Context, id string) (*Session, error) {
	if id == "" {
		return nil, errors.New("empty session id")
	}

	val, err := cache.RDB.Get(ctx, sessionKeyPrefix+id).Result()
	if err != nil {
		return nil, err
	}

	var s Session
	if err := json.Unmarshal([]byte(val), &s); err != nil {
		return nil, err
	}

	return &s, nil
}

// DeleteSession removes a session from Redis (logout) and from its user's
// register.
func DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}

	// Read the owner first: once the session is gone we cannot tell whose
	// register it belongs to. A session that is already gone or unreadable
	// simply has no register entry to clean.
	session, getErr := GetSession(ctx, id)

	if err := cache.RDB.Del(ctx, sessionKeyPrefix+id).Err(); err != nil {
		return err
	}

	if getErr == nil {
		return cache.RDB.SRem(ctx, UserSessionsKey(session.UserID), id).Err()
	}
	return nil
}

// generateSessionID produces a cryptographically random, URL-safe session
// identifier. 32 random bytes = 256 bits of entropy, which is far beyond
// what's guessable/brute-forceable.
func generateSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

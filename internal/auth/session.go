package auth

import (
	"context"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Claims embedded in every JWT issued by the platform.
type Claims struct {
	UserID   string `json:"uid"`
	Role     Role   `json:"role"`
	BranchID string `json:"branch_id,omitempty"` // empty for super_admin
	// LanguageScope restricts a teacher to one language's tests/content
	// ("en" | "zh"); empty for mad/prodlenka teachers and every other
	// role. Set once at login from teachers.language_scope.
	LanguageScope string `json:"language_scope,omitempty"`
	jwt.RegisteredClaims
}

type SessionManager struct {
	redis     *redis.Client
	jwtSecret []byte
	idleTTL   time.Duration // e.g. 30 minutes, only enforced for staff roles
}

func NewSessionManager(redisClient *redis.Client, jwtSecret string, idleTTL time.Duration) *SessionManager {
	return &SessionManager{redis: redisClient, jwtSecret: []byte(jwtSecret), idleTTL: idleTTL}
}

// staffRoles are subject to the 30-minute idle-timeout requirement.
// Student and parent cabinets are exempt per spec.
func isStaffRole(r Role) bool {
	return r == RoleTeacher || r == RoleDirector || r == RoleSuperAdmin
}

// IssueSession creates a JWT and, for staff roles, registers a Redis key
// that expires on idle timeout. Renewing activity is handled by
// TouchSession on every authenticated request.
func (sm *SessionManager) IssueSession(ctx context.Context, userID string, role Role, branchID, languageScope string) (string, error) {
	sessionID := uuid.NewString()

	claims := Claims{
		UserID:        userID,
		Role:          role,
		BranchID:      branchID,
		LanguageScope: languageScope,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ID:        sessionID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)), // hard cap; idle logout is separate
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(sm.jwtSecret)
	if err != nil {
		return "", err
	}

	if isStaffRole(role) {
		key := "session:" + sessionID
		if err := sm.redis.Set(ctx, key, userID, sm.idleTTL).Err(); err != nil {
			return "", err
		}
	}

	return signed, nil
}

// TouchSession extends the Redis idle-TTL for staff sessions and returns
// ErrSessionExpired if the key has already been evicted (idle > 30 min).
// Call this from auth middleware on every request to staff subdomains.
func (sm *SessionManager) TouchSession(ctx context.Context, claims *Claims) error {
	if !isStaffRole(claims.Role) {
		return nil // students/parents are not subject to idle logout
	}
	key := "session:" + claims.ID
	exists, err := sm.redis.Exists(ctx, key).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		return ErrSessionExpired
	}
	return sm.redis.Expire(ctx, key, sm.idleTTL).Err()
}

// RevokeSession deletes the Redis session key immediately (explicit logout).
func (sm *SessionManager) RevokeSession(ctx context.Context, sessionID string) error {
	return sm.redis.Del(ctx, "session:"+sessionID).Err()
}

// ParseToken validates signature + expiry and returns claims.
func (sm *SessionManager) ParseToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return sm.jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

var (
	ErrSessionExpired = errors.New("session expired due to inactivity")
	ErrInvalidToken   = errors.New("invalid or expired token")
)

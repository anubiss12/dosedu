package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
)

// RequireAuth validates the bearer JWT, enforces the Redis-backed idle
// timeout for staff roles, and (if roles are given) restricts access to
// those roles only — e.g. RequireAuth(sm, auth.RoleDirector) for
// director.dosedu.kz-only endpoints.
func RequireAuth(sm *auth.SessionManager, allowed ...auth.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")

		claims, err := sm.ParseToken(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		if err := sm.TouchSession(c.Request.Context(), claims); err != nil {
			// Idle timeout hit -> force logout, matching the spec's
			// "auto session close after 30 min inactivity" requirement.
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session_expired"})
			return
		}

		if len(allowed) > 0 && !roleAllowed(claims.Role, allowed) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient role"})
			return
		}

		c.Set("claims", claims)
		c.Next()
	}
}

func roleAllowed(role auth.Role, allowed []auth.Role) bool {
	for _, r := range allowed {
		if r == role {
			return true
		}
	}
	return false
}

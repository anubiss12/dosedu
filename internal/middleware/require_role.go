package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
)

// RequireRole centralizes the per-endpoint role checks that used to be
// scattered as inline `if claims.Role != auth.RoleX` blocks inside
// individual handlers (see tests.go, daily_logs.go). Declaring the
// allowed roles at the route registration in router.go makes each
// endpoint's access rule visible in one place instead of buried in
// handler bodies, and is what actually enforces "a parent can never
// reach a test endpoint" / "a student can never write a daily log" now
// that learn/parent are logical role-scopes on the same family.*
// route group rather than separate subdomains.
func RequireRole(roles ...auth.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := c.MustGet("claims").(*auth.Claims)
		for _, r := range roles {
			if claims.Role == r {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "wrong role for this endpoint"})
	}
}

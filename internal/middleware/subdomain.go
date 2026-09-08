package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
)

// subdomainForRole is the expected first label of the Host header for
// each role, matching the subdomain layout in the spec:
// dosedu.kz / student.dosedu.kz / teacher.dosedu.kz /
// director.dosedu.kz / admin.dosedu.kz.
var subdomainForRole = map[auth.Role]string{
	auth.RoleSuperAdmin: "admin",
	auth.RoleDirector:   "director",
	auth.RoleTeacher:    "teacher",
	auth.RoleStudent:    "student",
	auth.RoleParent:     "student",
}

// RequireSubdomain is a defense-in-depth check on top of the existing
// per-route-group role restriction: even if a request somehow reaches
// the wrong route group, a token's role must also match the subdomain
// it's presented on (via X-Forwarded-Host, falling back to Host).
// Local/dev hosts (localhost, 127.0.0.1, bare IPs) are exempt so
// direct-to-API testing still works without a real subdomain.
func RequireSubdomain() gin.HandlerFunc {
	return func(c *gin.Context) {
		claimsVal, exists := c.Get("claims")
		if !exists {
			c.Next()
			return
		}
		claims := claimsVal.(*auth.Claims)

		host := c.GetHeader("X-Forwarded-Host")
		if host == "" {
			host = c.Request.Host
		}
		host = strings.ToLower(strings.Split(host, ":")[0])

		if host == "" || host == "localhost" || host == "127.0.0.1" || isIP(host) {
			c.Next() // local/direct testing — skip
			return
		}

		expected, ok := subdomainForRole[claims.Role]
		if !ok {
			c.Next()
			return
		}

		label := strings.Split(host, ".")[0]
		if label != expected {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "wrong subdomain for this account's role"})
			return
		}
		c.Next()
	}
}

func isIP(host string) bool {
	for _, r := range host {
		if r != '.' && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

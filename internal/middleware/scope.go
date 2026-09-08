package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
)

// EffectiveBranchID returns "" for a super_admin or a network-owner
// director — meaning "no restriction, every branch" — otherwise the
// caller's own branch_id. Read/list repo queries treat "" as
// "WHERE ($1 = '' OR branch_id = $1::uuid)", i.e. no filter at all.
func EffectiveBranchID(c *gin.Context) string {
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Role == auth.RoleSuperAdmin || claims.IsNetworkOwner {
		return ""
	}
	return claims.BranchID
}

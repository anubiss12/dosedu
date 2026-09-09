package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/dosedu/lms/internal/auth"
)

// EffectiveBranchID returns "" for a super_admin or a network-owner
// director — meaning "no restriction, every branch" — otherwise the
// caller's own branch_id. Read/list repo queries treat "" as
// "WHERE ($1 = ” OR branch_id = $1::uuid)", i.e. no filter at all.
//
// A network-wide caller may narrow this to one branch at a time via
// ?branch_id= (the director frontend's branch switcher) — this is the
// only place that needs to know about it, since every List* handler
// already calls through here instead of reading claims.BranchID
// directly.
func EffectiveBranchID(c *gin.Context) string {
	claims := c.MustGet("claims").(*auth.Claims)
	if claims.Role == auth.RoleSuperAdmin || claims.IsNetworkOwner {
		if bid := c.Query("branch_id"); bid != "" {
			return bid
		}
		return ""
	}
	return claims.BranchID
}

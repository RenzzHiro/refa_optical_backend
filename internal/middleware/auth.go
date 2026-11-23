package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/refa-optical/backend/internal/auth"
	"net/http"
	"strings"
)

const UserIDKey = "userID"
const UserEmailKey = "userEmail"
const UserRoleKey = "userRole"

func RequireAuth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "Bearer token diperlukan."}})
			return
		}
		claims, err := auth.ParseAccess(parts[1], secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "Access token tidak valid atau kedaluwarsa."}})
			return
		}
		c.Set(UserIDKey, claims.UserID)
		c.Set(UserEmailKey, claims.Email)
		c.Set(UserRoleKey, claims.Role)
		c.Next()
	}
}
func AdminOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get(UserRoleKey)
		if role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{"code": "forbidden", "message": "Akses admin diperlukan."}})
			return
		}
		c.Next()
	}
}

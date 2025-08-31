// server/internal/api/middleware/auth.go
package middleware

import (
	"net/http"
	"strings"
	"fresh-meat-scm-api-server/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddleware creates a gin middleware for JWT authentication.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token format"})
			return
		}

		claims := &auth.JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			return auth.JwtSecret, nil
		})

		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			return
		}

		// === QUAN TRỌNG: Lưu thông tin user vào context của request ===
		// Các handler sau đó có thể lấy thông tin này ra để sử dụng.
		c.Set("user_enrollment_id", claims.FabricEnrollmentID)
		c.Set("user_role", claims.Role)
		c.Set("user_org", claims.OrgShortName)

		c.Next()
	}
}
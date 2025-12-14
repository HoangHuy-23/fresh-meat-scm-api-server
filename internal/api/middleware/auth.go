package middleware

import (
	"fresh-meat-scm-api-server/internal/auth"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Authenticate validates the JWT token.
// It checks the validity of the token and injects user info into the context.
func Authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Allow preflight OPTIONS requests to pass without a token
		// This is critical for CORS to work, as the browser doesn't send Auth headers on OPTIONS
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

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

		// Set context variables for use in handlers
		c.Set("user_enrollment_id", claims.FabricEnrollmentID)
		c.Set("user_role", claims.Role)
		c.Set("user_facility_id", claims.FacilityID)

		c.Next()
	}
}

// Authorize checks if the user has one of the allowed roles.
func Authorize(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		userRoleInterface, exists := c.Get("user_role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User role not found in context"})
			return
		}

		userRole, ok := userRoleInterface.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User role has an invalid type"})
			return
		}

		for _, role := range allowedRoles {
			if role == userRole {
				c.Next()
				return
			}
		}

		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "You do not have permission to access this resource"})
	}
}
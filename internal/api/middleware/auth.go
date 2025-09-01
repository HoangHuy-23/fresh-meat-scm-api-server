// server/internal/api/middleware/auth.go
package middleware

import (
	"net/http"
	"strings"
	"fresh-meat-scm-api-server/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Authenticate là middleware xác thực token JWT.
// Nó kiểm tra tính hợp lệ của token và đưa thông tin user vào context.
func Authenticate() gin.HandlerFunc {
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

		// Lưu thông tin user vào context của request
		c.Set("user_enrollment_id", claims.FabricEnrollmentID)
		c.Set("user_role", claims.Role)
		c.Set("user_facility_id", claims.FacilityID)

		c.Next()
	}
}

// Authorize là một middleware factory để kiểm tra vai trò của người dùng.
// Nó nhận vào một danh sách các vai trò được phép và trả về một middleware.
func Authorize(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Lấy vai trò của user từ context (đã được middleware Authenticate đặt vào)
		userRoleInterface, exists := c.Get("user_role")
		if !exists {
			// Lỗi này không nên xảy ra nếu Authenticate được gọi trước
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User role not found in context"})
			return
		}

		userRole, ok := userRoleInterface.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "User role has an invalid type"})
			return
		}

		// Kiểm tra xem vai trò của user có nằm trong danh sách được phép không
		for _, role := range allowedRoles {
			if role == userRole {
				c.Next() // Vai trò hợp lệ, cho phép tiếp tục
				return
			}
		}

		// Nếu không tìm thấy vai trò phù hợp, từ chối truy cập
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "You do not have permission to access this resource"})
	}
}
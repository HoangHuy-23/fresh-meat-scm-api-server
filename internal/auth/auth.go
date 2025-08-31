// server/internal/auth/auth.go
package auth

import (
	// "fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// JWTClaims defines the payload for the JWT.
type JWTClaims struct {
	Email              string `json:"email"`
	Role               string `json:"role"`
	OrgShortName       string `json:"orgShortName"`
	FabricEnrollmentID string `json:"fabricEnrollmentID"`
	jwt.RegisteredClaims
}

// Hashing
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// JWT Generation
// TODO: Move JWT_SECRET to a secure location like environment variables
var JwtSecret = []byte("YOUR_SUPER_SECRET_KEY")

func GenerateJWT(email, role, orgShortName, fabricEnrollmentID string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &JWTClaims{
		Email:              email,
		Role:               role,
		OrgShortName:       orgShortName,
		FabricEnrollmentID: fabricEnrollmentID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(JwtSecret)
}
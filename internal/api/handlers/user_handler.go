// server/internal/api/handlers/user_handler.go
package handlers

import (
	"context" // <-- THÊM IMPORT
	"fmt"
	"net/http"
	"fresh-meat-scm-api-server/internal/auth"
	"fresh-meat-scm-api-server/internal/ca"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
	"go.mongodb.org/mongo-driver/bson" // <-- THÊM IMPORT
	"go.mongodb.org/mongo-driver/mongo"
)

type UserHandler struct {
	CAService *ca.CAService
	Wallet    *gateway.Wallet
	OrgName   string
	DB        *mongo.Database
}

type CreateUserRequest struct {
	Email        string `json:"email" binding:"required"`
	Name         string `json:"name" binding:"required"`
	Password     string `json:"password" binding:"required"`
	Affiliation  string `json:"affiliation" binding:"required"`
	Role         string `json:"role" binding:"required"`
	OrgShortName string `json:"orgShortName" binding:"required"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// User struct matches the document in MongoDB
type User struct {
	Email              string `bson:"email"`
	Name               string `bson:"name"`
	Password           string `bson:"password"`
	Role               string `bson:"role"`
	OrgShortName       string `bson:"orgShortName"`
	Status             string `bson:"status"`
	FabricEnrollmentID string `bson:"fabricEnrollmentID"`
}

// Login handles user authentication.
func (h *UserHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Bước 1: Tìm user trong MongoDB bằng email
	user, err := h.findUserByEmail(req.Email) // <-- SỬA LỖI: Gọi như một method
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Bước 2: So sánh password
	match := auth.CheckPasswordHash(req.Password, user.Password) // <-- SỬA LỖI: user.Password
	if !match {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	// Bước 3: Tạo JWT
	token, err := auth.GenerateJWT(
		user.Email,
		user.Role,
		user.OrgShortName,
		user.FabricEnrollmentID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"token": token})
}

// CreateUser handles registration and enrollment of a new user.
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 1. Tạo Enrollment ID duy nhất
	enrollmentID := fmt.Sprintf("%s-%s", req.Role, uuid.New().String()[:8])

	// 2. Đăng ký user với CA
	attributes := []msp.Attribute{
		{Name: "role", Value: req.Role, ECert: true},
		{Name: "orgShortName", Value: req.OrgShortName, ECert: true},
	}
	secret, err := h.CAService.RegisterUser(enrollmentID, req.Affiliation, attributes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user with CA", "details": err.Error()})
		return
	}

	// 3. Ghi danh user để lấy cert/key
	cert, key, err := h.CAService.EnrollUser(enrollmentID, secret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enroll user with CA", "details": err.Error()})
		return
	}

	// 4. Lưu danh tính mới vào wallet của server
	identity := gateway.NewX509Identity(h.OrgName+"MSP", string(cert), string(key))
	if err := h.Wallet.Put(enrollmentID, identity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save new identity to wallet", "details": err.Error()})
		return
	}

	// 5. Lưu thông tin user vào MongoDB
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password", "details": err.Error()})
		return
	}

	user := User{
		Email:              req.Email,
		Name:               req.Name,
		Password:           hashedPassword,
		Role:               req.Role,
		OrgShortName:       req.OrgShortName,
		Status:             "active",
		FabricEnrollmentID: enrollmentID,
	}
	if err := h.createUserInDB(&user); err != nil { // <-- SỬA LỖI: Gọi như một method
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user to database", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":       "success",
		"message":      "User created and enrolled successfully",
		"enrollmentID": enrollmentID,
		"email":        req.Email,
	})
}

// --- Helper Methods ---

// findUserByEmail is a helper method to find a user in the database.
func (h *UserHandler) findUserByEmail(email string) (*User, error) {
	var user User
	collection := h.DB.Collection("users")
	err := collection.FindOne(context.TODO(), bson.M{"email": email}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// createUserInDB is a helper method to insert a new user into the database.
func (h *UserHandler) createUserInDB(user *User) error {
	collection := h.DB.Collection("users")
	_, err := collection.InsertOne(context.TODO(), user)
	return err
}
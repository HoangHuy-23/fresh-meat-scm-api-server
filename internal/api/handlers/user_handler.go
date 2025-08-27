// server/internal/api/handlers/user_handler.go
package handlers

import (
	"fmt"
	"net/http"
	"fresh-meat-scm-api-server/internal/ca"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
)

type UserHandler struct {
	CAService *ca.CAService
	Wallet    *gateway.Wallet
	OrgName   string
}

type CreateUserRequest struct {
	Email        string `json:"email" binding:"required"`
	Name         string `json:"name" binding:"required"`
	Affiliation  string `json:"affiliation" binding:"required"` // Ví dụ: "meatsupplyorg.farm"
	Role         string `json:"role" binding:"required"`        // "admin" hoặc "worker"
	OrgShortName string `json:"orgShortName" binding:"required"`  // Ví dụ: "farmA"
}

func (h *UserHandler) CreateUser(c *gin.Context) {
	var req CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 1. Tạo Enrollment ID duy nhất
	enrollmentID := fmt.Sprintf("%s-%s", req.Role, uuid.New().String()[:8])

	// 2. Đăng ký user với CA, sử dụng danh tính của SuperAdmin
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

	// TODO: Lưu thông tin user (email, name, enrollmentID) vào MongoDB

	c.JSON(http.StatusCreated, gin.H{
		"status":         "success",
		"message":        "User created and enrolled successfully",
		"enrollmentID":   enrollmentID,
		"email":          req.Email,
	})
}
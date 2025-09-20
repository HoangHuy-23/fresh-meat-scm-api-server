// server/internal/api/handlers/user_handler.go
package handlers

import (
	"context" 
	"fmt"
	"net/http"
	"fresh-meat-scm-api-server/internal/auth"
	"fresh-meat-scm-api-server/internal/ca"
	"fresh-meat-scm-api-server/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
	"go.mongodb.org/mongo-driver/bson" 
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
	FacilityID  string `json:"facilityID" binding:"required"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// ProfileResponse là struct an toàn để trả về thông tin user, không bao gồm password.
type ProfileResponse struct {
	Email              string `json:"email"`
	Name               string `json:"name"`
	Role               string `json:"role"`
	FacilityID         string `json:"facilityID"`
	Status             string `json:"status"`
	FabricEnrollmentID string `json:"fabricEnrollmentID"`
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
		user.FacilityID,
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

	// Khai báo biến attributes để sử dụng trong cả hai trường hợp
	var attributes []msp.Attribute

	// === LOGIC ĐIỀU KIỆN DỰA TRÊN VAI TRÒ ===
	if req.Role == "driver" {
		// TRƯỜNG HỢP 1: NẾU LÀ TÀI XẾ
		// - Không cần kiểm tra facilityID trong DB.
		// - Chỉ gán các thuộc tính cần thiết.
		attributes = []msp.Attribute{
			{Name: "role",       Value: req.Role,       ECert: true},
			{Name: "facilityID", Value: req.FacilityID, ECert: true}, 
			{Name: "facilityType", Value: "CARRIER",    ECert: true},
		}
	} else {
		// TRƯỜNG HỢP 2: CÁC VAI TRÒ KHÁC (worker, admin, ...)
		// - Bắt buộc phải kiểm tra facilityID trong DB.
		// - "Làm giàu" thuộc tính với facilityType từ DB.
		var facility models.Facility
		facilityCollection := h.DB.Collection("facilities")
		err := facilityCollection.FindOne(context.Background(), bson.M{"facilityID": req.FacilityID}).Decode(&facility)
		
		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Facility with the provided ID does not exist"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error checking for facility"})
			return
		}

		attributes = []msp.Attribute{
			{Name: "role",         Value: req.Role,       ECert: true},
			{Name: "facilityID",   Value: req.FacilityID, ECert: true},
			{Name: "facilityType", Value: facility.Type,  ECert: true},
		}
	}
	// ==========================================

	// Phần còn lại của logic là chung cho tất cả các vai trò
	enrollmentID := fmt.Sprintf("%s-%s", req.Role, uuid.New().String()[:8])

	// Đăng ký user với CA sử dụng 'attributes' đã được chuẩn bị ở trên
	secret, err := h.CAService.RegisterUser(enrollmentID, req.Affiliation, attributes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register user with CA", "details": err.Error()})
		return
	}

	// Ghi danh user để lấy cert/key
	cert, key, err := h.CAService.EnrollUser(enrollmentID, secret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enroll user with CA", "details": err.Error()})
		return
	}

	// Lưu danh tính mới vào wallet của server
	identity := gateway.NewX509Identity(h.OrgName+"MSP", string(cert), string(key))
	if err := h.Wallet.Put(enrollmentID, identity); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save new identity to wallet", "details": err.Error()})
		return
	}

	// Lưu thông tin user vào MongoDB
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash password", "details": err.Error()})
		return
	}

	user := models.User{
		Email:              req.Email,
		Name:               req.Name,
		Password:           hashedPassword,
		Role:               req.Role,
		FacilityID:         req.FacilityID,
		Status:             "active",
		FabricEnrollmentID: enrollmentID,
	}
	if err := h.createUserInDB(&user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save user to database", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":       "success",
		"message":      "User created and enrolled successfully",
		"enrollmentID": enrollmentID,
		"facilityID":   req.FacilityID,
	})
}

// GetProfile lấy thông tin của người dùng đang đăng nhập.
func (h *UserHandler) GetProfile(c *gin.Context) {
	// 1. Lấy enrollmentID từ context, đã được middleware xác thực và đưa vào.
	enrollmentID, exists := c.Get("user_enrollment_id")
	if !exists {
		// Lỗi này không nên xảy ra nếu middleware hoạt động đúng.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User enrollment ID not found in context"})
		return
	}

	// 2. Tìm user trong database bằng enrollmentID.
	var user models.User
	collection := h.DB.Collection("users")
	err := collection.FindOne(context.Background(), bson.M{"fabricEnrollmentID": enrollmentID}).Decode(&user)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			// Trường hợp hiếm: user có token hợp lệ nhưng đã bị xóa khỏi DB.
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query user profile"})
		return
	}

	// 3. Xây dựng và trả về response an toàn.
	// TUYỆT ĐỐI KHÔNG TRẢ VỀ `user` struct trực tiếp vì nó chứa password hash.
	response := ProfileResponse{
		Email:              user.Email,
		Name:               user.Name,
		Role:               user.Role,
		FacilityID:         user.FacilityID,
		Status:             user.Status,
		FabricEnrollmentID: user.FabricEnrollmentID,
	}

	c.JSON(http.StatusOK, response)
}

// --- Helper Methods ---

// findUserByEmail is a helper method to find a user in the database.
func (h *UserHandler) findUserByEmail(email string) (*models.User, error) {
	var user models.User
	collection := h.DB.Collection("users")
	err := collection.FindOne(context.TODO(), bson.M{"email": email}).Decode(&user)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// createUserInDB is a helper method to insert a new user into the database.
func (h *UserHandler) createUserInDB(user *models.User) error {
	collection := h.DB.Collection("users")
	_, err := collection.InsertOne(context.TODO(), user)
	return err
}
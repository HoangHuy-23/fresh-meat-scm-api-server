// server/internal/api/handlers/admin_handler.go
package handlers

import (
	"net/http"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/wallet"
	"strings"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/hyperledger/fabric-sdk-go/pkg/client/msp"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
)

type AdminHandler struct {
	SDK    *fabsdk.FabricSDK
	Wallet *gateway.Wallet
	Config config.Config
}

type RegisterUserRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Role     string `json:"role" binding:"required"`
	EntityID string `json:"entityID" binding:"required"`
	OrgName  string `json:"orgName" binding:"required"` // Ví dụ: "MeatSupplyOrg"
}

func (h *AdminHandler) RegisterUser(c *gin.Context) {
	// TODO: Thêm middleware để xác thực đây là Super Admin
	

	var req RegisterUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	adminUser := h.Config.Fabric.SuperAdmin.UserName
    
    // === THÊM DEBUG LOG ===
    fmt.Printf("Looking for admin user: %s\n", adminUser)
    fmt.Printf("Wallet exists check: %v\n", h.Wallet.Exists(adminUser))
    
    // List all identities in wallet để debug
    identities, err := h.Wallet.List()
    fmt.Printf("Available identities in wallet: %v\n", identities)
    
    if !h.Wallet.Exists(adminUser) {
        c.JSON(http.StatusInternalServerError, gin.H{
            "error": "Admin user not found in wallet",
            "adminUser": adminUser,
            "availableUsers": identities,
        })
        return
    }

	// Tạo các thuộc tính cho user mới
	attributes := []msp.Attribute{
		{Name: "role", Value: req.Role, ECert: true},
		{Name: "entityID", Value: req.EntityID, ECert: true},
	}

	// === SỬA LỖI: Truyền đúng orgName và mspID ===
	orgNameForContext := strings.ToLower(req.OrgName) // Tên trong connection-profile (chữ thường)
	mspIDForIdentity := req.OrgName + "MSP"           // MSP ID (viết hoa)

	// Đăng ký và ghi danh user mới
	err = wallet.RegisterAndEnrollUser(
		h.SDK,
		h.Wallet,
		h.Config.Fabric.SuperAdmin.UserName, // Thực hiện hành động với tư cách superadmin
		req.Username,                 // Tên user mới
		req.Password,                 // Secret của user mới
		orgNameForContext,           // Truyền orgName
		mspIDForIdentity,             // Truyền mspID
		attributes,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to register and enroll user", "details": err.Error()})
		return
	}

	// TODO: Lưu thông tin user (username, hashed password, role, entityID) vào MongoDB

	c.JSON(http.StatusCreated, gin.H{"status": "success", "message": "User " + req.Username + " created successfully"})
}
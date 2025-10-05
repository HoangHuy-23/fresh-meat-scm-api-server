// server/internal/api/handlers/product_handler.go
package handlers

import (
	"net/http"
	"time"
	"strings"
	"math/rand"
	"fmt"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/blockchain"

	"github.com/gin-gonic/gin"
)

type ProductHandler struct {
	Fabric *blockchain.FabricSetup
	Cfg    config.Config
}

type Product struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Unit        string `json:"unit"`
	SourceType  string `json:"sourceType"`
	Category    string `json:"category"`
	SKU         string `json:"sku"`
	Active      bool   `json:"active"`
}

type CreateProductRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Unit        string `json:"unit" binding:"required"`
	SourceType  string `json:"sourceType" binding:"required"`
	Category    string `json:"category" binding:"required"`
}

// CreateProduct xử lý việc tạo sản phẩm mới on-chain.
func (h *ProductHandler) CreateProduct(c *gin.Context) {
	// Struct để bind request body
	var req CreateProductRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Lấy identity của người gọi (superadmin)
	enrollmentID := c.GetString("user_enrollment_id")
	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway"})
		return
	}
	defer userGateway.Close()
	network, _ := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)

	// Gọi chaincode
	_, err = contract.SubmitTransaction(
		"CreateProduct",
		generateSKU(req.SourceType),
		req.Name,
		req.Description,
		req.Unit,
		req.SourceType,
		req.Category,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "success", "message": "Product created successfully"})
}

// GetAllProducts truy vấn danh sách sản phẩm on-chain.
func (h *ProductHandler) GetAllProducts(c *gin.Context) {
	sourceType := c.Query("sourceType")
	category := c.Query("category")

	// Dùng identity của server để truy vấn (chỉ đọc)
	resultJSON, err := h.Fabric.Contract.EvaluateTransaction("QueryProducts", sourceType, category)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query products", "details": err.Error()})
		return
	}

	c.Data(http.StatusOK, "application/json", resultJSON)
}

// CreateProducts xử lý việc tạo nhiều sản phẩm mới on-chain.
func (h *ProductHandler) CreateProducts(c *gin.Context) {
	// Struct để bind request body
	var reqs []CreateProductRequest
	if err := c.ShouldBindJSON(&reqs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Lấy identity của người gọi (superadmin)
	enrollmentID := c.GetString("user_enrollment_id")
	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway"})
		return
	}
	defer userGateway.Close()
	network, _ := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)
	// Tạo sản phẩm từng cái một
	for _, req := range reqs {
		_, err = contract.SubmitTransaction(
			"CreateProduct",
			generateSKU(req.SourceType),
			req.Name,
			req.Description,
			req.Unit,
			req.SourceType,
			req.Category,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
			return
		}
	}

	c.JSON(http.StatusCreated, gin.H{"status": "success", "message": "Products created successfully"})
}

// generateSKU tự động tạo SKU theo sourceType
func generateSKU(sourceType string) string {
	prefix := strings.ToUpper(strings.TrimSpace(sourceType))

	datePart := time.Now().Format("20060102")

	randomPart := randString(4)

	return fmt.Sprintf("%s-%s-%s", prefix, datePart, randomPart)
}

// Sinh chuỗi ngẫu nhiên
func randString(n int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
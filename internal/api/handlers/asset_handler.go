// internal/api/handlers/asset_handler.go
package handlers

import (
	"encoding/json"
	"net/http"
	"fresh-meat-scm-api-server/internal/blockchain"

	"github.com/gin-gonic/gin"
)

type AssetHandler struct {
	Fabric *blockchain.FabricSetup
}

// CreateFarmingBatchRequest defines the structure for the farming batch creation request.
type CreateFarmingBatchRequest struct {
	AssetID     string          `json:"assetID" binding:"required"`
	ProductName string          `json:"productName" binding:"required"`
	Details     json.RawMessage `json:"details" binding:"required"` // Use json.RawMessage to pass through
}

// ProcessAndSplitBatchRequest defines the structure for the splitting request.
type ProcessAndSplitBatchRequest struct {
	ParentAssetID string          `json:"parentAssetID" binding:"required"`
	ChildAssets   json.RawMessage `json:"childAssets" binding:"required"`
	Details       json.RawMessage `json:"details" binding:"required"`
}

// CreateFarmingBatch handles the API endpoint for creating a new farming batch.
func (h *AssetHandler) CreateFarmingBatch(c *gin.Context) {
	var req CreateFarmingBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.Fabric.Contract.SubmitTransaction(
		"CreateFarmingBatch",
		req.AssetID,
		req.ProductName,
		string(req.Details),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "success", "assetID": req.AssetID})
}

// ProcessAndSplitBatch handles the API endpoint for splitting a batch.
func (h *AssetHandler) ProcessAndSplitBatch(c *gin.Context) {
	var req ProcessAndSplitBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.Fabric.Contract.SubmitTransaction(
		"ProcessAndSplitBatch",
		req.ParentAssetID,
		string(req.ChildAssets),
		string(req.Details),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "parentAssetID": req.ParentAssetID})
}

// GetAssetHistory handles the API endpoint for retrieving an asset's full history and details.
func (h *AssetHandler) GetAssetHistory(c *gin.Context) {
	assetID := c.Param("id")

    // Gọi hàm chaincode mới
	result, err := h.Fabric.Contract.EvaluateTransaction("GetAssetWithFullHistory", assetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found or error evaluating transaction", "details": err.Error()})
		return
	}

	// Kết quả trả về từ chaincode đã là một đối tượng JSON hoàn chỉnh
	c.Data(http.StatusOK, "application/json", result)
}
// server/internal/api/handlers/asset_handler.go
package handlers

import (
	"encoding/json"
	"net/http"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fmt"
	"github.com/gin-gonic/gin"
)

type AssetHandler struct {
	Fabric *blockchain.FabricSetup
}

// --- Structs cho Request Body ---

type Quantity struct {
	Unit  string  `json:"unit" binding:"required"`
	Value float64 `json:"value" binding:"required"`
}

type CreateFarmingBatchRequest struct {
	AssetID     string          `json:"assetID" binding:"required"`
	ProductName string          `json:"productName" binding:"required"`
	Quantity    Quantity        `json:"quantity" binding:"required"`
	Details     json.RawMessage `json:"details" binding:"required"`
}

type ChildAssetInputAPI struct {
	AssetID     string   `json:"assetID" binding:"required"`
	ProductName string   `json:"productName" binding:"required"`
	Quantity    Quantity `json:"quantity" binding:"required"`
}

type ProcessAndSplitBatchRequest struct {
	ParentAssetID string               `json:"parentAssetID" binding:"required"`
	ChildAssets   []ChildAssetInputAPI `json:"childAssets" binding:"required"`
	Details       json.RawMessage      `json:"details" binding:"required"`
}

// GenericDetailsRequest can be used for simple requests with a 'details' body.
type GenericDetailsRequest struct {
	Details json.RawMessage `json:"details" binding:"required"`
}

// SplitBatchToUnitsRequest: Struct mới, đơn giản hơn
type SplitBatchToUnitsRequest struct {
	ParentAssetID string `json:"parentAssetID" binding:"required"`
	UnitCount     int    `json:"unitCount" binding:"required,gt=0"` // gt=0: phải lớn hơn 0
	UnitIDPrefix  string `json:"unitIDPrefix" binding:"required"`
}

// --- Handlers ---

func (h *AssetHandler) CreateFarmingBatch(c *gin.Context) {
	var req CreateFarmingBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	quantityJSON, _ := json.Marshal(req.Quantity)

	_, err := h.Fabric.Contract.SubmitTransaction(
		"CreateFarmingBatch",
		req.AssetID,
		req.ProductName,
		string(quantityJSON),
		string(req.Details),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"status": "success", "assetID": req.AssetID})
}

func (h *AssetHandler) UpdateFarmingDetails(c *gin.Context) {
	assetID := c.Param("id")
	var req GenericDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.Fabric.Contract.SubmitTransaction("UpdateFarmingDetails", assetID, string(req.Details))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Farming details updated for asset " + assetID})
}

func (h *AssetHandler) ProcessAndSplitBatch(c *gin.Context) {
	var req ProcessAndSplitBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	childAssetsJSON, _ := json.Marshal(req.ChildAssets)

	_, err := h.Fabric.Contract.SubmitTransaction(
		"ProcessAndSplitBatch",
		req.ParentAssetID,
		string(childAssetsJSON),
		string(req.Details),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "parentAssetID": req.ParentAssetID})
}

// ===============================================================
// HÀM BỊ THIẾU ĐÃ ĐƯỢC BỔ SUNG
// ===============================================================
// UpdateStorageInfo handles the API endpoint for updating storage conditions.
func (h *AssetHandler) UpdateStorageInfo(c *gin.Context) {
	assetID := c.Param("id")
	var req GenericDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.Fabric.Contract.SubmitTransaction("UpdateStorageInfo", assetID, string(req.Details))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Storage info updated for asset " + assetID})
}
// ===============================================================

func (h *AssetHandler) MarkAsSold(c *gin.Context) {
	assetID := c.Param("id")
	var req GenericDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.Fabric.Contract.SubmitTransaction("MarkAsSold", assetID, string(req.Details))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Asset " + assetID + " marked as sold"})
}

func (h *AssetHandler) GetAssetTrace(c *gin.Context) {
	assetID := c.Param("id")

	result, err := h.Fabric.Contract.EvaluateTransaction("GetAssetWithFullHistory", assetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found or error evaluating transaction", "details": err.Error()})
		return
	}

	c.Data(http.StatusOK, "application/json", result)
}

func (h *AssetHandler) SplitBatchToUnits(c *gin.Context) {
	var req SplitBatchToUnitsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.Fabric.Contract.SubmitTransaction(
		"SplitBatchToUnits",
		req.ParentAssetID,
		fmt.Sprintf("%d", req.UnitCount), // Chuyển int thành string
		req.UnitIDPrefix,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": fmt.Sprintf("%d units created from batch %s", req.UnitCount, req.ParentAssetID)})
}
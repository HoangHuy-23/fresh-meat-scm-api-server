// server/internal/api/handlers/asset_handler.go
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/blockchain"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

type AssetHandler struct {
	Fabric *blockchain.FabricSetup
	Cfg    config.Config
	DB     *mongo.Database
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

type SplitBatchToUnitsRequest struct {
	ParentAssetID string `json:"parentAssetID" binding:"required"`
	UnitCount     int    `json:"unitCount" binding:"required,gt=0"`
	UnitIDPrefix  string `json:"unitIDPrefix" binding:"required"`
}

type GenericDetailsRequest struct {
	Details json.RawMessage `json:"details" binding:"required"`
}

// --- Handlers ---

func (h *AssetHandler) CreateFarmingBatch(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.ChaincodeName)

	var req CreateFarmingBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	quantityJSON, _ := json.Marshal(req.Quantity)

	_, err = contract.SubmitTransaction("CreateFarmingBatch", req.AssetID, req.ProductName, string(quantityJSON), string(req.Details))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "assetID": req.AssetID})
}

func (h *AssetHandler) UpdateFarmingDetails(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.ChaincodeName)

	assetID := c.Param("id")
	var req GenericDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err = contract.SubmitTransaction("UpdateFarmingDetails", assetID, string(req.Details))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Farming details updated for asset " + assetID})
}

func (h *AssetHandler) ProcessAndSplitBatch(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.ChaincodeName)

	var req ProcessAndSplitBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	childAssetsJSON, _ := json.Marshal(req.ChildAssets)

	_, err = contract.SubmitTransaction("ProcessAndSplitBatch", req.ParentAssetID, string(childAssetsJSON), string(req.Details))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "parentAssetID": req.ParentAssetID})
}

func (h *AssetHandler) UpdateStorageInfo(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.ChaincodeName)

	assetID := c.Param("id")
	var req GenericDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err = contract.SubmitTransaction("UpdateStorageInfo", assetID, string(req.Details))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Storage info updated for asset " + assetID})
}

func (h *AssetHandler) MarkAsSold(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.ChaincodeName)

	assetID := c.Param("id")
	var req GenericDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err = contract.SubmitTransaction("MarkAsSold", assetID, string(req.Details))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Asset " + assetID + " marked as sold"})
}

func (h *AssetHandler) SplitBatchToUnits(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.ChaincodeName)

	var req SplitBatchToUnitsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err = contract.SubmitTransaction("SplitBatchToUnits", req.ParentAssetID, fmt.Sprintf("%d", req.UnitCount), req.UnitIDPrefix)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": fmt.Sprintf("%d units created from batch %s", req.UnitCount, req.ParentAssetID)})
}

func (h *AssetHandler) GetAssetTrace(c *gin.Context) {
	// Query can use the default gateway connection for simplicity
	result, err := h.Fabric.Contract.EvaluateTransaction("GetAssetWithFullHistory", c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found or error evaluating transaction", "details": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json", result)
}
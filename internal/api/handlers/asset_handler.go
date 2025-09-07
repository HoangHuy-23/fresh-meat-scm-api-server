// server/internal/api/handlers/asset_handler.go
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"context"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fresh-meat-scm-api-server/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
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
	enrollmentID := c.GetString("user_enrollment_id")
	userFacilityID := c.GetString("user_facility_id")

	var req CreateFarmingBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	quantityJSON, _ := json.Marshal(req.Quantity)

	// Truy vấn MongoDB để lấy thông tin đầy đủ của cơ sở
	var facility models.Facility
	facilityCollection := h.DB.Collection("facilities")
	err := facilityCollection.FindOne(context.Background(), bson.M{"facilityID": userFacilityID}).Decode(&facility)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Facility associated with user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query facility details"})
		return
	}

	// Unmarshal details từ request của user
	var userDetails map[string]interface{}
	if err := json.Unmarshal(req.Details, &userDetails); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid details JSON format"})
		return
	}

	// Xây dựng object `FarmDetails` cuối cùng để gửi tới chaincode
	finalFarmDetails := map[string]interface{}{
		"facilityID":  facility.FacilityID, // Ghi lại ID của cơ sở
		"facilityName": facility.Name,       // Ghi lại tên đầy đủ
		"address":      facility.Address,    // Gửi cả object address có tọa độ
		// Thêm các trường từ request của user
		"sowingDate":   userDetails["sowingDate"],
		"harvestDate":  userDetails["harvestDate"],
		"fertilizers":  userDetails["fertilizers"],
		"pesticides":   userDetails["pesticides"],
		"certificates": userDetails["certificates"],
	}
	finalFarmDetailsJSON, _ := json.Marshal(finalFarmDetails)
	// ===================================

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


	_, err = contract.SubmitTransaction("CreateFarmingBatch", req.AssetID, req.ProductName, string(quantityJSON), string(finalFarmDetailsJSON))
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
	enrollmentID := c.GetString("user_enrollment_id")
	userFacilityID := c.GetString("user_facility_id")

	var req ProcessAndSplitBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	childAssetsJSON, _ := json.Marshal(req.ChildAssets)

	// Truy vấn MongoDB để lấy thông tin đầy đủ của cơ sở
	var facility models.Facility
	facilityCollection := h.DB.Collection("facilities")
	err := facilityCollection.FindOne(context.Background(), bson.M{"facilityID": userFacilityID}).Decode(&facility)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Facility associated with user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query facility details"})
		return
	}

	// Unmarshal details từ request của user
	var userDetails map[string]interface{}
	if err := json.Unmarshal(req.Details, &userDetails); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid details JSON format"})
		return
	}

	finalProcessingDetails := map[string]interface{}{
		"facilityID":       facility.FacilityID,
		"facilityName":     facility.Name,
		"address":          facility.Address, // <-- LÀM GIÀU
		"processorOrgName": userDetails["processorOrgName"],
		"steps":            userDetails["steps"],
		"certificates":     userDetails["certificates"],
	}
	finalDetailsJSON, _ := json.Marshal(finalProcessingDetails)
	// ===================================

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

	_, err = contract.SubmitTransaction("ProcessAndSplitBatch", req.ParentAssetID, string(childAssetsJSON), string(finalDetailsJSON))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "parentAssetID": req.ParentAssetID})
}

func (h *AssetHandler) UpdateStorageInfo(c *gin.Context) {
	enrollmentID := c.GetString("user_enrollment_id")
	userFacilityID := c.GetString("user_facility_id")

	assetID := c.Param("id")
	var req GenericDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Truy vấn MongoDB để lấy thông tin đầy đủ của cơ sở
	var facility models.Facility
	facilityCollection := h.DB.Collection("facilities")
	err := facilityCollection.FindOne(context.Background(), bson.M{"facilityID": userFacilityID}).Decode(&facility)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Facility associated with user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query facility details"})
		return
	}

	// Unmarshal details từ request của user
	var userDetails map[string]interface{}
	if err := json.Unmarshal(req.Details, &userDetails); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid details JSON format"})
		return
	}

	finalStorageDetails := map[string]interface{}{
	"facilityID":      facility.FacilityID,
	"facilityName":    facility.Name,
	"address":         facility.Address,
	"ownerOrgName":    userDetails["ownerOrgName"],
	"locationInStore": userDetails["locationInStore"],
	"temperature":     userDetails["temperature"],
	"note":            userDetails["note"],
	}
	finalDetailsJSON, _ := json.Marshal(finalStorageDetails)

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

	_, err = contract.SubmitTransaction("UpdateStorageInfo", assetID, string(finalDetailsJSON))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Storage info updated for asset " + assetID})
}

func (h *AssetHandler) MarkAsSold(c *gin.Context) {
	enrollmentID := c.GetString("user_enrollment_id")
	userFacilityID := c.GetString("user_facility_id")

	assetID := c.Param("id")
	var req GenericDetailsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Truy vấn MongoDB để lấy thông tin đầy đủ của cơ sở
	var facility models.Facility
	facilityCollection := h.DB.Collection("facilities")
	err := facilityCollection.FindOne(context.Background(), bson.M{"facilityID": userFacilityID}).Decode(&facility)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Facility associated with user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query facility details"})
		return
	}

	// Unmarshal details từ request của user
	var userDetails map[string]interface{}
	if err := json.Unmarshal(req.Details, &userDetails); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid details JSON format"})
		return
	}

	finalSoldDetails := map[string]interface{}{
		"facilityID":      facility.FacilityID,
		"facilityName":    facility.Name,
		"address":         facility.Address, // <-- LÀM GIÀU
		"retailerOrgName": userDetails["retailerOrgName"],
	}
	finalDetailsJSON, _ := json.Marshal(finalSoldDetails)

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

	_, err = contract.SubmitTransaction("MarkAsSold", assetID, string(finalDetailsJSON))
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

// GetAssetsByFacility thực hiện một truy vấn on-chain để lấy các asset của một cơ sở
func (h *AssetHandler) GetAssetsByFacility(c *gin.Context) {
	facilityID := c.Param("id")

	// Sử dụng EvaluateTransaction vì đây là một truy vấn chỉ đọc (query)
	result, err := h.Fabric.Contract.EvaluateTransaction("QueryAssetsByFacility", facilityID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query assets by facility", "details": err.Error()})
		return
	}

	// Kết quả trả về từ chaincode đã là một mảng JSON, trả về trực tiếp
	c.Data(http.StatusOK, "application/json", result)
}
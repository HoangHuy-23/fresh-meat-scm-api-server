// server/internal/api/handlers/asset_handler.go
package handlers

import (
	"encoding/json"
	"fmt"
	"time"
	"strings"
	"net/http"
	"context"
	"io"
	"bytes"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fresh-meat-scm-api-server/internal/models"
	"fresh-meat-scm-api-server/internal/s3"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type AssetHandler struct {
	Fabric *blockchain.FabricSetup
	Cfg    config.Config
	S3Uploader     *s3.Uploader
	DB     *mongo.Database
}

// --- Structs cho Request Body ---

// type Quantity struct {
// 	Unit  string  `json:"unit" binding:"required"`
// 	Value float64 `json:"value" binding:"required"`
// }

type CreateFarmingBatchRequest struct {
	SKU         string          `json:"sku" binding:"required"`
	Quantity    models.Quantity `json:"quantity" binding:"required"`
	SourceType  string          `json:"sourceType" binding:"required"`
	AverageWeight *models.Weight `json:"averageWeight"`
	Details     json.RawMessage `json:"details" binding:"required"`
}

type ChildAssetInputAPI struct {
	SKU         string          `json:"sku" binding:"required"`
	Quantity    models.Quantity `json:"quantity" binding:"required"`
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

type MediaPointer struct {
	URL      string `json:"url"`
	MimeType string `json:"mimeType"`
}

// Feed lưu trữ thông tin về thức ăn.
type Feed struct {
    Name         string  `json:"name" binding:"required"`          // Tên loại thức ăn (vd: "Green Feed tập ăn")
    DosageKg     float64 `json:"dosageKg" binding:"required"`      // Liều lượng mỗi ngày (kg/con hoặc kg/tổng đàn)
    StartDate    string  `json:"startDate" binding:"required"`     // Ngày bắt đầu sử dụng (YYYY-MM-DD)
    EndDate      string  `json:"endDate"`       // (Tùy chọn) Ngày kết thúc sử dụng
    Notes        string  `json:"notes"`         // (Tùy chọn) Ghi chú thêm như "giai đoạn tập ăn", "tăng trọng"
}

// Medication lưu trữ thông tin về thuốc và chất bổ sung.
type Medication struct {
    Name         string  `json:"name" binding:"required"`          // Tên thuốc/chất bổ sung (vd: "Vaccine A", "Vitamin B")
    Dose         string  `json:"dose" binding:"required"`          // Liều lượng (vd: "1ml", "500mg")
    DateApplied  string  `json:"dateApplied" binding:"required"`   // Ngày áp dụng (YYYY-MM-DD)
    NextDueDate  string  `json:"nextDueDate" binding:"required"`   // Ngày đến hạn tiếp theo (YYYY-MM-DD)
}

// Certificate lưu trữ thông tin về chứng nhận.
type Certificate struct {
	Name  string       `json:"name"`
	Media MediaPointer `json:"media"`
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
		"facilityID":     		facility.FacilityID, // Ghi lại ID của cơ sở
		"facilityName": 		facility.Name,       // Ghi lại tên đầy đủ
		"address":      		facility.Address,    // Gửi cả object address có tọa độ
		"sowingDate":   		userDetails["sowingDate"],
		"startDate":    		userDetails["startDate"],
		"expectedHarvestDate":  userDetails["expectedHarvestDate"],
		"harvestDate":          userDetails["harvestDate"],
		"feeds":                userDetails["feeds"],
		"medications":          userDetails["medications"],
		"certificates":         userDetails["certificates"],
	}
	finalFarmDetailsJSON, _ := json.Marshal(finalFarmDetails)
	// ===================================

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)

	productJSON, err := h.Fabric.Contract.EvaluateTransaction("GetProduct", req.SKU)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Product with SKU '%s' not found on-chain", req.SKU)})
		return
	}
	var product struct {
		Name          string        `json:"name"`
		AverageWeight models.Weight `json:"averageWeight"`
	}
	if err := json.Unmarshal(productJSON, &product); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse product data from chaincode"})
		return
	}
	productName := product.Name

	var finalAverageWeight models.Weight
	if req.AverageWeight != nil {
		finalAverageWeight = *req.AverageWeight
	} else {
		finalAverageWeight = product.AverageWeight
	}
	finalAverageWeightJSON, _ := json.Marshal(finalAverageWeight)


	// Tự động tạo một ID mới
	assetID := generateAssetID(req.SourceType)

	_, err = contract.SubmitTransaction(
		"CreateFarmingBatch", 
		assetID, 
		productName, 
		req.SKU, 
		string(quantityJSON), 
		string(finalFarmDetailsJSON), 
		string(finalAverageWeightJSON),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "assetID": assetID})
}

// AddFeedToFarmingBatch thêm một bản ghi thức ăn mới vào một lô hàng.
func (h *AssetHandler) AddFeedToFarmingBatch(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)
	assetID := c.Param("id")
	var req Feed
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	feedJSON, _ := json.Marshal(req)
	_, err = contract.SubmitTransaction("AddFeedToFarmingBatch", assetID, string(feedJSON))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Feed added to farming batch"})
}

// AddMedicationToFarmingBatch thêm một bản ghi thuốc mới.
func (h *AssetHandler) AddMedicationToFarmingBatch(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)
	assetID := c.Param("id")
	var req Medication
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	medicationJSON, _ := json.Marshal(req)
	_, err = contract.SubmitTransaction("AddMedicationToFarmingBatch", assetID, string(medicationJSON))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Medication added to farming batch"})
}

// UpdateAverageWeight cập nhật trọng lượng trung bình của một lô hàng.
func (h *AssetHandler) UpdateAverageWeight(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)
	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)
	assetID := c.Param("id")
	var req models.Weight
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	weightJSON, _ := json.Marshal(req)
	_, err = contract.SubmitTransaction("UpdateAverageWeight", assetID, string(weightJSON))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Average weight updated for asset " + assetID})
}

// UpdateHarvestDate cập nhật ngày thu hoạch thực tế.
func (h *AssetHandler) UpdateHarvestDate(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)
	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)
	assetID := c.Param("id")
	var req struct {
		HarvestDate string `json:"harvestDate" binding:"required"` // YYYY-MM-DD
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Kiểm tra định dạng ngày tháng
	if _, err := time.Parse("2006-01-02", req.HarvestDate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format. Use YYYY-MM-DD."})
		return
	}
	_, err = contract.SubmitTransaction("UpdateHarvestDate", assetID, req.HarvestDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Harvest date updated for asset " + assetID})
}

// UpdateExpectedHarvestDate cập nhật ngày thu hoạch dự kiến.
func (h *AssetHandler) UpdateExpectedHarvestDate(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)
	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)
	assetID := c.Param("id")
	var req struct {
		ExpectedHarvestDate string `json:"expectedHarvestDate" binding:"required"` // YYYY-MM-DD
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Kiểm tra định dạng ngày tháng
	if _, err := time.Parse("2006-01-02", req.ExpectedHarvestDate); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid date format. Use YYYY-MM-DD."})
		return
	}
	_, err = contract.SubmitTransaction("UpdateExpectedHarvestDate", assetID, req.ExpectedHarvestDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Expected harvest date updated for asset " + assetID})
}

// AddCertificatesToFarmingBatch thêm các chứng chỉ mới cho một lô hàng.
func (h *AssetHandler) AddCertificatesToFarmingBatch(c *gin.Context) {
	enrollmentIDInterface, _ := c.Get("user_enrollment_id")
	enrollmentID := enrollmentIDInterface.(string)
	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)
	assetID := c.Param("id")

	fileHeader, err := c.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Photo file is required"})
		return
	}

	name := c.PostForm("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Certificate name is required"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open uploaded file"})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read uploaded file"})
		return
	}

	objectKey := fmt.Sprintf("certificates/%s/%d_%s", assetID, time.Now().Unix(), fileHeader.Filename)
	fileReader := bytes.NewReader(fileBytes)
	photoURL, err := h.S3Uploader.UploadFile(c.Request.Context(), fileReader, objectKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload file to S3", "details": err.Error})
		return
	}

	newCert := Certificate{
		Name: name, 
		Media: MediaPointer{
			URL: photoURL,
		},
	}

	certsSlice := []Certificate{newCert}

	certJSON, _ := json.Marshal(certsSlice)

	_, err = contract.SubmitTransaction("AddCertificatesToFarmingBatch", assetID, string(certJSON))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Certificate added successfully"})
}

// GetAssetAtFarmByID lấy chi tiết một lô hàng tại trang trại.
func (h *AssetHandler) GetAssetAtFarmByID(c *gin.Context) {
	enrollmentID := c.GetString("user_enrollment_id")
	assetID := c.Param("id")
	// Cần sử dụng gateway của người dùng để chaincode có thể xác thực quyền
	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()
	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)
	result, err := contract.EvaluateTransaction("GetAssetAtFarmByID", assetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found or access denied", "details": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json", result)
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

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)

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

	//childAssetsJSON, _ := json.Marshal(req.ChildAssets)

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

	// --- LÀM GIÀU THÔNG TIN SẢN PHẨM CHO CHILD ASSETS ---
	enrichedChildAssets := []map[string]interface{}{}
	// Giả định Product Catalog được lưu on-chain
	for _, child := range req.ChildAssets {
		productJSON, err := h.Fabric.Contract.EvaluateTransaction("GetProduct", child.SKU)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Product with SKU '%s' not found on-chain", child.SKU)})
			return
		}
		var product map[string]interface{}
		json.Unmarshal(productJSON, &product)

		enrichedChild := map[string]interface{}{
			"assetID":     generateChildAssetID(strings.Split(child.SKU, "-")[2]), // PORK-20251005-3BMD -> 3BMD-xxxxxx
			"productName": product["name"],
			"sku":         child.SKU,
			"quantity":    child.Quantity,
		}
		enrichedChildAssets = append(enrichedChildAssets, enrichedChild)
	}
	childAssetsJSON, _ := json.Marshal(enrichedChildAssets)
	// -------------------------------------------------

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)

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

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)

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

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)

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

	network, err := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)

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

// GetAssetByMyFacility cho phép một cơ sở truy xuất chi tiết một asset thuộc về cơ sở đó
func (h *AssetHandler) GetAssetsByMyFacility(c *gin.Context) {
	userFacilityID := c.GetString("user_facility_id")
	
	result, err := h.Fabric.Contract.EvaluateTransaction("QueryAssetsByFacility", userFacilityID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found or access denied", "details": err.Error()})
		return
	}

	c.Data(http.StatusOK, "application/json", result)
}

// GetUnprocessedAssetsByProcessor lấy các lô sản phẩm chưa chế biến tại một nhà máy.
func (h *AssetHandler) GetUnprocessedAssetsByProcessor(c *gin.Context) {
	facilityID := c.Param("id")
	enrollmentID := c.GetString("user_enrollment_id")

	// Cần sử dụng gateway của người dùng để chaincode có thể xác thực quyền
	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()
	network, _ := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)

	result, err := contract.EvaluateTransaction("QueryAssetsAtProcessorByStatus", facilityID, "AT_PROCESSOR")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query unprocessed assets", "details": err.Error()})
		return
	}

	c.Data(http.StatusOK, "application/json", result)
}

// GetProcessedAssetsByProcessor lấy các lô sản phẩm đã chế biến tại một nhà máy.
func (h *AssetHandler) GetProcessedAssetsByProcessor(c *gin.Context) {
	facilityID := c.Param("id")
	enrollmentID := c.GetString("user_enrollment_id")

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()
	network, _ := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)

	// Trạng thái sau khi chế biến là "PACKAGED"
	result, err := contract.EvaluateTransaction("QueryAssetsAtProcessorByStatus", facilityID, "PACKAGED")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query processed assets", "details": err.Error()})
		return
	}

	c.Data(http.StatusOK, "application/json", result)
}

// GetAssetsAtRetailerByStatus lấy các lô sản phẩm tại một cơ sở bán lẻ.
func (h *AssetHandler) GetAssetsAtRetailerByStatus(c *gin.Context) {
	facilityID := c.Param("id")
	status := c.Query("status")
	enrollmentID := c.GetString("user_enrollment_id")

	userGateway, err := h.Fabric.GetGatewayForUser(enrollmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create user gateway", "details": err.Error()})
		return
	}
	defer userGateway.Close()
	network, _ := userGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)
	// Trạng thái có thể là "AT_RETAILER" hoặc "ON_SHELF"
	result, err := contract.EvaluateTransaction("QueryAssetsAtRetailerByStatus", facilityID, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query retailer assets", "details": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json", result)
}

// QueryAssetsByFacilityAndSKU lấy các lô sản phẩm của một cơ sở theo SKU cụ thể.
// public API, cần xác thực người dùng
func (h *AssetHandler) QueryAssetsByFacilityAndSKU(c *gin.Context) {
	facilityID := c.Param("id")
	sku := c.Query("sku")
	
	resultJSON, err := h.Fabric.Contract.EvaluateTransaction("QueryAssetsByFacilityAndSKU", facilityID, sku)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query assets by facility and SKU", "details": err.Error()})
		return
	}
	if resultJSON == nil || string(resultJSON) == "null" {
		c.Data(http.StatusOK, "application/json", []byte("[]"))
		return
	}
	c.Data(http.StatusOK, "application/json", resultJSON)
}

// generateAssetID tạo một ID duy nhất cho asset dựa trên loại nguồn và ngày hiện tại
func generateAssetID(sourceType string) string {
	prefix := "FARM"
	if sourceType != "" {
		prefix = prefix + "-" + strings.ToUpper(sourceType)
	}

	datePart := time.Now().Format("20060102")

	randomPart := randString(4)

	return fmt.Sprintf("%s-%s-%s", prefix, datePart, randomPart)
}

// generateChildAssetID tạo một ID duy nhất cho child asset dựa trên SKU và ngày hiện tại
func generateChildAssetID(sku string) string {
	prefix := "PRO"
	if sku != "" {
		prefix = prefix + "-" + strings.ToUpper(sku)
	}
	datePart := time.Now().Format("20060102")

	randomPart := randString(6)
	return fmt.Sprintf("%s-%s-%s", prefix, datePart, randomPart)
}

func (h *AssetHandler) GetAssetByID(c *gin.Context) {
    assetID := c.Param("assetID")

    if assetID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "assetID is required"})
        return
    }

    result, err := h.Fabric.Contract.EvaluateTransaction("GetAsset", assetID)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found", "details": err.Error()})
        return
    }

    // Treat explicit null or empty result as "not found"
    if result == nil || len(result) == 0 || string(result) == "null" {
        c.JSON(http.StatusNotFound, gin.H{"error": "Asset not found"})
        return
    }

    c.Data(http.StatusOK, "application/json", result)
}

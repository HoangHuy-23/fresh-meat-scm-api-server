// internal/api/handlers/shipment_handler.go
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"fmt"
	"time"
	"crypto/sha256"
	"encoding/hex"
	"io/ioutil"
	"path/filepath"
	"bytes"
	"log"
	"github.com/google/uuid"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fresh-meat-scm-api-server/internal/models"
	"fresh-meat-scm-api-server/internal/s3"
	"fresh-meat-scm-api-server/internal/socket"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

)

type ShipmentHandler struct {
	Fabric *blockchain.FabricSetup
	Cfg    config.Config
	DB     *mongo.Database
	S3Uploader     *s3.Uploader
	Hub    *socket.Hub
}

// --- Structs cho Request Body ---

// StopInJourneyAPI là struct mà client gửi lên, rất đơn giản.
type StopInJourneyAPI struct {
	FacilityID string              `json:"facilityID" binding:"required"`
	Action     string              `json:"action" binding:"required"`
	Items      []ItemInShipmentAPI `json:"items" binding:"required"`
}

// StopInJourneyChaincode là struct được "làm giàu" để gửi tới chaincode.
type StopInJourneyChaincode struct {
	FacilityID      string              `json:"facilityID"`
	FacilityName    string              `json:"facilityName"`
	FacilityAddress models.Address      `json:"facilityAddress"`
	Action          string              `json:"action"`
	Items           []ItemInShipmentAPI `json:"items"`
}

type ItemInShipmentAPI struct {
	AssetID  string   `json:"assetID" binding:"required"`
	Quantity Quantity `json:"quantity" binding:"required"`
}

type CreateShipmentRequest struct {
	ShipmentID         string             `json:"shipmentID" binding:"required"`
	ShipmentType       string             `json:"shipmentType" binding:"required"`
	DriverName         string             `json:"driverName" binding:"required"`
	VehiclePlate       string             `json:"vehiclePlate" binding:"required"`
	Stops              []StopInJourneyAPI `json:"stops" binding:"required"`
}

type ConfirmPickupRequest struct {
	FacilityID  string              `json:"facilityID" binding:"required"`
	ActualItems []ItemInShipmentAPI `json:"actualItems" binding:"required"`
}

type ConfirmDeliveryRequest struct {
	FacilityID     string `json:"facilityID" binding:"required"`
	NewAssetPrefix string `json:"newAssetPrefix" binding:"required"`
}

type AddPickupPhotoRequest struct {
	PhotoURL  string `json:"photoURL" binding:"required"`
	PhotoHash string `json:"photoHash" binding:"required"`
}

// --- Handlers ---

func (h *ShipmentHandler) CreateShipment(c *gin.Context) {
	enrollmentID := c.GetString("user_enrollment_id")

	var req CreateShipmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// === BƯỚC LÀM GIÀU DỮ LIỆU CHO STOPS ===
	enrichedStops := []StopInJourneyChaincode{}
	facilityCollection := h.DB.Collection("facilities")

	for _, stop := range req.Stops {
		var facility models.Facility
		err := facilityCollection.FindOne(context.Background(), bson.M{"facilityID": stop.FacilityID}).Decode(&facility)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Facility with ID '%s' does not exist", stop.FacilityID)})
				return // Dừng ngay lập tức nếu có bất kỳ facility nào không hợp lệ
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error checking for facility"})
			return
		}

		// Tạo một stop đã được làm giàu thông tin
		enrichedStop := StopInJourneyChaincode{
			FacilityID:      facility.FacilityID,
			FacilityName:    facility.Name,
			FacilityAddress: facility.Address,
			Action:          stop.Action,
			Items:           stop.Items,
		}
		enrichedStops = append(enrichedStops, enrichedStop)
	}
	// =======================================

	// Lấy gateway và contract
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

	// Gửi dữ liệu đã được làm giàu tới chaincode
	stopsJSON, _ := json.Marshal(enrichedStops)
	_, err = contract.SubmitTransaction(
		"CreateShipment",
		req.ShipmentID,
		req.ShipmentType,
		enrollmentID, // Driver's enrollment ID from token
		req.DriverName,
		req.VehiclePlate,
		string(stopsJSON),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "shipmentID": req.ShipmentID})
}

func (h *ShipmentHandler) ConfirmPickup(c *gin.Context) {
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

	shipmentID := c.Param("id")
	var req ConfirmPickupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// === LOGIC MỚI: TÌM BẰNG CHỨNG ===
	proofCollection := h.DB.Collection("pickup_proofs")
	var proof models.PickupProof
	// Tìm bằng chứng mới nhất cho shipment và facility này
	opts := options.FindOne().SetSort(bson.D{{"createdAt", -1}})
	err = proofCollection.FindOne(context.Background(), bson.M{"shipmentID": shipmentID, "facilityID": req.FacilityID}, opts).Decode(&proof)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusPreconditionFailed, gin.H{"error": "Pickup proof photo has not been uploaded by the driver yet"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check for pickup proof"})
		return
	}

	proofDetails := map[string]string{
		"photoURL":  proof.PhotoURL,
		"photoHash": proof.PhotoHash,
		"uploadedBy": proof.UploadedBy,
	}
	proofJSON, _ := json.Marshal(proofDetails)
	// =================================

	actualItemsJSON, _ := json.Marshal(req.ActualItems)

	_, err = contract.SubmitTransaction(
		"ConfirmPickup",
		shipmentID,
		req.FacilityID,
		string(actualItemsJSON),
		string(proofJSON), // Gửi bằng chứng dưới dạng JSON string
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	// Gửi thông báo real-time qua WebSocket
	// === BƯỚC MỚI: LẤY THÔNG TIN TÀI XẾ CẦN THÔNG BÁO ===
	// Chúng ta cần gọi chaincode để lấy thông tin shipment trước
	shipmentData, err := h.Fabric.Contract.EvaluateTransaction("GetShipment", shipmentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Shipment not found", "details": err.Error()})
		return
	}
	var shipmentInfo struct {
		DriverEnrollmentID string `json:"driverEnrollmentID"`
	}
	if err := json.Unmarshal(shipmentData, &shipmentInfo); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse shipment data"})
		return
	}
	driverToNotify := shipmentInfo.DriverEnrollmentID
	// =================================================
	notification := map[string]string{
		"type":       "pickup_confirmed",
		"shipmentID": shipmentID,
		"driverID":   driverToNotify,
		"message":    "Pickup has been confirmed for shipment " + shipmentID,
	}
	notificationJSON, _ := json.Marshal(notification)
	if err := h.Hub.Send(driverToNotify, notificationJSON); err != nil {
		log.Printf("Failed to send WebSocket notification: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Pickup confirmed for shipment " + shipmentID})
}

func (h *ShipmentHandler) StartShipment(c *gin.Context) {
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

	shipmentID := c.Param("id")

	_, err = contract.SubmitTransaction("StartShipment", shipmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Shipment " + shipmentID + " has started."})
}

func (h *ShipmentHandler) ConfirmDelivery(c *gin.Context) {
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

	shipmentID := c.Param("id")
	var req ConfirmDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// === LOGIC MỚI: TÌM BẰNG CHỨNG GIAO HÀNG ===
	proofCollection := h.DB.Collection("delivery_proofs") 
	var proof models.DeliveryProof 
	
	opts := options.FindOne().SetSort(bson.D{{"createdAt", -1}})
	err = proofCollection.FindOne(context.Background(), bson.M{"shipmentID": shipmentID, "facilityID": req.FacilityID}, opts).Decode(&proof)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusPreconditionFailed, gin.H{"error": "Delivery proof photo has not been uploaded by the driver yet"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check for delivery proof"})
		return
	}

	proofDetails := map[string]string{
		"photoURL":   proof.PhotoURL,
		"photoHash":  proof.PhotoHash,
		"uploadedBy": proof.UploadedBy,
	}
	proofJSON, _ := json.Marshal(proofDetails)
	// ==========================================

	_, err = contract.SubmitTransaction(
		"ConfirmShipmentDelivery",
		shipmentID,
		req.FacilityID,
		req.NewAssetPrefix,
		string(proofJSON), // Gửi bằng chứng dưới dạng JSON string
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Delivery confirmed for shipment " + shipmentID})
}

// Getshipment lấy các chi tiết của một lô hàng duy nhất.
func (h *ShipmentHandler) GetShipment(c *gin.Context) {
	shipmentID := c.Param("id")

	// Queries can use the default gateway connection for simplicity.
	// This uses the API server's identity, which is fine for read-only operations.
	result, err := h.Fabric.Contract.EvaluateTransaction("GetShipment", shipmentID)
	if err != nil {
		// The error from Fabric will contain details, e.g., "asset not found"
		c.JSON(http.StatusNotFound, gin.H{"error": "Shipment not found or error evaluating transaction", "details": err.Error()})
		return
	}

	// The result from chaincode is already a JSON byte array, so we can return it directly.
	c.Data(http.StatusOK, "application/json", result)
}

// GetShipmentsByDriver thực hiện một truy vấn on-chain để lấy các lô hàng của một tài xế
func (h *ShipmentHandler) GetShipmentsByDriver(c *gin.Context) {
	driverID := c.Param("id")

	// Sử dụng EvaluateTransaction vì đây là một truy vấn chỉ đọc (query)
	result, err := h.Fabric.Contract.EvaluateTransaction("QueryShipmentsByDriver", driverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query shipments by driver", "details": err.Error()})
		return
	}

	// Kết quả trả về từ chaincode đã là một mảng JSON, trả về trực tiếp
	c.Data(http.StatusOK, "application/json", result)
}

// AddPickupPhoto cho phép tài xế gửi bằng chứng hình ảnh trước khi pickup
func (h *ShipmentHandler) AddPickupPhoto(c *gin.Context) {
	shipmentID := c.Param("id")
	facilityID := c.Param("facilityID")
	driverEnrollmentID := c.GetString("user_enrollment_id")

	var req AddPickupPhotoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newProof := models.PickupProof{
		ShipmentID: shipmentID,
		FacilityID: facilityID,
		PhotoURL:   req.PhotoURL,
		PhotoHash:  req.PhotoHash,
		UploadedBy: driverEnrollmentID,
		CreatedAt:  time.Now(),
	}

	collection := h.DB.Collection("pickup_proofs")
	// Có thể thêm logic kiểm tra xem proof đã tồn tại chưa và ghi đè (upsert)
	_, err := collection.InsertOne(context.Background(), newProof)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save pickup proof"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Pickup photo uploaded successfully"})
}

// AddDeliveryPhoto cho phép tài xế gửi bằng chứng hình ảnh trước khi giao hàng
func (h *ShipmentHandler) AddDeliveryPhoto(c *gin.Context) {
	shipmentID := c.Param("id")
	facilityID := c.Param("facilityID")
	driverEnrollmentID := c.GetString("user_enrollment_id")

	var req AddPickupPhotoRequest // Dùng lại struct này vì nó giống hệt
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	newProof := models.DeliveryProof{ 
		ShipmentID: shipmentID,
		FacilityID: facilityID,
		PhotoURL:   req.PhotoURL,
		PhotoHash:  req.PhotoHash,
		UploadedBy: driverEnrollmentID,
		CreatedAt:  time.Now(),
	}

	collection := h.DB.Collection("delivery_proofs") 
	_, err := collection.InsertOne(context.Background(), newProof)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save delivery proof"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Delivery photo uploaded successfully"})
}

// UploadPickupPhoto nhận file ảnh từ client, upload lên S3 và lưu bằng chứng.
func (h *ShipmentHandler) UploadPickupPhoto(c *gin.Context) {
	shipmentID := c.Param("id")
	facilityID := c.Param("facilityID")
	driverEnrollmentID := c.GetString("user_enrollment_id")

	// 1. Nhận file từ request multipart/form-data
	fileHeader, err := c.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Photo file is required"})
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer file.Close()

	// 2. Đọc toàn bộ nội dung file để tính hash
	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file content"})
		return
	}

	// 3. Tính toán SHA-256 hash
	hash := sha256.Sum256(fileBytes)
	photoHash := hex.EncodeToString(hash[:])

	// 4. Upload file lên S3
	// Tạo một object key duy nhất để tránh trùng lặp
	objectKey := fmt.Sprintf("proofs/%s-%s-%s%s", shipmentID, facilityID, uuid.New().String(), filepath.Ext(fileHeader.Filename))
	
	// Cần tạo lại reader từ byte slice đã đọc
	fileReader := bytes.NewReader(fileBytes)
	photoURL, err := h.S3Uploader.UploadFile(c.Request.Context(), fileReader, objectKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload photo", "details": err.Error()})
		return
	}

	// 5. Lưu bằng chứng vào MongoDB
	newProof := models.PickupProof{
		ShipmentID: shipmentID,
		FacilityID: facilityID,
		PhotoURL:   photoURL,
		PhotoHash:  photoHash,
		UploadedBy: driverEnrollmentID,
		CreatedAt:  time.Now(),
	}

	collection := h.DB.Collection("pickup_proofs")
	_, err = collection.InsertOne(context.Background(), newProof)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save pickup proof"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"message":   "Pickup photo uploaded successfully",
		"photoURL":  photoURL,
		"photoHash": photoHash,
	})
}

// UploadDeliveryPhoto nhận file ảnh từ client, upload lên S3 và lưu bằng chứng.
func (h *ShipmentHandler) UploadDeliveryPhoto(c *gin.Context) {
	shipmentID := c.Param("id")
	facilityID := c.Param("facilityID")
	driverEnrollmentID := c.GetString("user_enrollment_id")
	// 1. Nhận file từ request multipart/form-data
	fileHeader, err := c.FormFile("photo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Photo file is required"})
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open file"})
		return
	}
	defer file.Close()

	// 2. Đọc toàn bộ nội dung file để tính hash
	fileBytes, err := ioutil.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file content"})
		return
	}
	// 3. Tính toán SHA-256 hash
	hash := sha256.Sum256(fileBytes)
	photoHash := hex.EncodeToString(hash[:])
	// 4. Upload file lên S3
	// Tạo một object key duy nhất để tránh trùng lặp
	objectKey := fmt.Sprintf("proofs/%s-%s-%s%s", shipmentID, facilityID, uuid.New().String(), filepath.Ext(fileHeader.Filename))
	// Cần tạo lại reader từ byte slice đã đọc
	fileReader := bytes.NewReader(fileBytes)
	photoURL, err := h.S3Uploader.UploadFile(c.Request.Context(), fileReader, objectKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upload photo", "details": err.Error()})
		return
	}
	// 5. Lưu bằng chứng vào MongoDB
	newProof := models.DeliveryProof{ 
		ShipmentID: shipmentID,
		FacilityID: facilityID,
		PhotoURL:   photoURL,
		PhotoHash:  photoHash,
		UploadedBy: driverEnrollmentID,
		CreatedAt:  time.Now(),
	}
	collection := h.DB.Collection("delivery_proofs") 
	_, err = collection.InsertOne(context.Background(), newProof)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save delivery proof"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"message":   "Delivery photo uploaded successfully",
		"photoURL":  photoURL,
		"photoHash": photoHash,
	})
}
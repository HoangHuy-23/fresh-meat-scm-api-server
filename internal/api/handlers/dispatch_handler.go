package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"strings"
	"fresh-meat-scm-api-server/internal/models"
	"fresh-meat-scm-api-server/internal/socket"
	"fresh-meat-scm-api-server/internal/blockchain"
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type DispatchHandler struct {
	DB  *mongo.Database
	Hub *socket.Hub
	Fabric *blockchain.FabricSetup
}

type DispatchItemPayload struct {
	AssetID  string   		 `json:"assetID" binding:"required"`
	Quantity models.Quantity `json:"quantity" binding:"required"`
}

// Struct cho request body, không cần ToFacilityID
type CreateDispatchRequestPayload struct {
	Items []DispatchItemPayload `json:"items" binding:"required"`
}

// CreateDispatchRequest xử lý việc tạo yêu cầu và "làm giàu" với SKU.
func (h *DispatchHandler) CreateDispatchRequest(c *gin.Context) {
	creatorEnrollmentID := c.GetString("user_enrollment_id")
	creatorFacilityID := c.GetString("user_facility_id")

	// (Tùy chọn) Kiểm tra xem facility của người tạo có tồn tại không
	facilityCollection := h.DB.Collection("facilities")
	count, err := facilityCollection.CountDocuments(context.Background(), bson.M{"facilityID": creatorFacilityID})
	if err != nil || count == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Originating facility does not exist."})
		return
	}

	var payload CreateDispatchRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// --- BƯỚC LÀM GIÀU QUAN TRỌNG ---
	enrichedItems := []models.DispatchItemDetail{}
	for _, item := range payload.Items {
		// 1. Gọi chaincode để lấy thông tin asset
		// Dùng identity của server để truy vấn cho nhanh
		assetJSON, err := h.Fabric.Contract.EvaluateTransaction("GetAsset", item.AssetID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Asset with ID '%s' not found on-chain", item.AssetID)})
			return
		}
		
		// 2. Unmarshal để lấy SKU và kiểm tra quyền sở hữu
		var assetData struct {
			SKU      string `json:"sku"`
			OwnerOrg string `json:"ownerOrg"`
		}
		if err := json.Unmarshal(assetJSON, &assetData); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse asset data"})
			return
		}
		
		// 3. Kiểm tra xem asset có thực sự thuộc sở hữu của người tạo request không
		if assetData.OwnerOrg != creatorFacilityID {
			c.JSON(http.StatusForbidden, gin.H{"error": fmt.Sprintf("You are not the owner of asset %s", item.AssetID)})
			return
		}

		// 4. Tạo item đã được làm giàu
		enrichedItems = append(enrichedItems, models.DispatchItemDetail{
			AssetID:  item.AssetID,
			SKU:      assetData.SKU, // <-- LẤY SKU TỪ CHAINCODE
			Quantity: item.Quantity,
		})
	}
	// =================================

	newRequest := models.DispatchRequest{
		RequestID:      fmt.Sprintf("DR-%s", strings.ToUpper(uuid.New().String()[:8])),
		FromFacilityID: creatorFacilityID,
		Items:          enrichedItems, // <-- SỬ DỤNG DỮ LIỆU ĐÃ ĐƯỢC LÀM GIÀU
		Status:         "PENDING",
		CreatedBy:      creatorEnrollmentID,
		CreatedAt:      time.Now(),
	}

	collection := h.DB.Collection("dispatch_requests")
	result, err := collection.InsertOne(context.Background(), newRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create dispatch request"})
		return
	}

	newRequest.ID = result.InsertedID.(primitive.ObjectID)

	// ... (gửi webhook) ...

	c.JSON(http.StatusCreated, newRequest)
}

// GetAllDispatchRequests lấy danh sách các yêu cầu xuất hàng, có thể lọc theo trạng thái.
func (h *DispatchHandler) GetAllDispatchRequests(c *gin.Context) {
	// 1. Tạo một bộ lọc (filter) rỗng
	filter := bson.M{}

	// 2. Lấy query parameter "status" từ URL
	// Ví dụ: /dispatch-requests?status=PENDING
	status := c.Query("status")
	if status != "" {
		// Nếu có, thêm điều kiện lọc vào filter
		filter["status"] = status
	}

	// 3. Truy vấn collection "dispatch_requests"
	collection := h.DB.Collection("dispatch_requests")
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query dispatch requests"})
		return
	}
	defer cursor.Close(context.Background())

	// 4. Decode kết quả vào một slice
	var requests []models.DispatchRequest
	if err = cursor.All(context.Background(), &requests); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode dispatch requests"})
		return
	}

	// Đảm bảo trả về một mảng rỗng thay vì null nếu không có kết quả
	if requests == nil {
		requests = []models.DispatchRequest{}
	}

	// 5. Trả về kết quả
	c.JSON(http.StatusOK, requests)
}

// GetMyFacilityDispatchRequests lấy danh sách các yêu cầu xuất hàng của facility hiện tại.
func (h *DispatchHandler) GetMyFacilityDispatchRequests(c *gin.Context) {
	facilityID := c.GetString("user_facility_id")
	filter := bson.M{"fromFacilityID": facilityID}

	collection := h.DB.Collection("dispatch_requests")
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query dispatch requests"})
		return
	}
	defer cursor.Close(context.Background())

	var requests []models.DispatchRequest
	if err = cursor.All(context.Background(), &requests); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode dispatch requests"})
		return
	}
	if requests == nil {
		requests = []models.DispatchRequest{}
	}
	c.JSON(http.StatusOK, requests)
}

// GetDispatchRequestByID lấy chi tiết một yêu cầu xuất hàng theo ID.
func (h *DispatchHandler) GetDispatchRequestByID(c *gin.Context) {
	requestID := c.Param("id")
	collection := h.DB.Collection("dispatch_requests")
	var request models.DispatchRequest
	err := collection.FindOne(context.Background(), bson.M{"requestID": requestID}).Decode(&request)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Dispatch request not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve dispatch request"})
		return
	}
	c.JSON(http.StatusOK, request)
}


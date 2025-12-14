package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"fresh-meat-scm-api-server/internal/models"
	"fresh-meat-scm-api-server/internal/socket"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type ReplenishmentHandler struct {
	DB  *mongo.Database
	Hub *socket.Hub
}

// Struct cho request body, đã được nâng cấp
type CreateReplenishmentRequestPayload struct {
	Items []models.ReplenishmentItem `json:"items" binding:"required,dive"` // dive: validate từng phần tử trong mảng
}

// CreateReplenishmentRequest xử lý việc tạo yêu cầu nhập hàng mới với nhiều mặt hàng.
func (h *ReplenishmentHandler) CreateReplenishmentRequest(c *gin.Context) {
	creatorEnrollmentID := c.GetString("user_enrollment_id")
	creatorFacilityID := c.GetString("user_facility_id")

	var payload CreateReplenishmentRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Lặp qua payload.Items để kiểm tra xem tất cả các SKU có tồn tại không

	newRequest := models.ReplenishmentRequest{
		RequestID:            fmt.Sprintf("RREQ-%s", uuid.New().String()[:8]),
		RequestingFacilityID: creatorFacilityID,
		Items:                payload.Items, // <-- GÁN MẢNG ITEMS
		Status:               "PENDING",
		CreatedBy:            creatorEnrollmentID,
		CreatedAt:            time.Now(),
	}

	collection := h.DB.Collection("replenishment_requests")
	result, err := collection.InsertOne(context.Background(), newRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create replenishment request"})
		return
	}

	newRequest.ID = result.InsertedID.(primitive.ObjectID)
	
	c.JSON(http.StatusCreated, newRequest)
}

// GetAllReplenishmentRequests lấy danh sách các yêu cầu nhập hàng.
func (h *ReplenishmentHandler) GetAllReplenishmentRequests(c *gin.Context) {
	filter := bson.M{}
	status := c.Query("status")
	if status != "" {
		filter["status"] = status
	}

	collection := h.DB.Collection("replenishment_requests")
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query replenishment requests"})
		return
	}
	defer cursor.Close(context.Background())

	var requests []models.ReplenishmentRequest
	if err = cursor.All(context.Background(), &requests); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode requests"})
		return
	}

	if requests == nil {
		requests = []models.ReplenishmentRequest{}
	}

	c.JSON(http.StatusOK, requests)
}

// GetMyReplenishmentRequests lấy danh sách các yêu cầu nhập hàng của cửa hàng hiện tại query status.
func (h *ReplenishmentHandler) GetMyReplenishmentRequests(c *gin.Context) {
	status := c.Query("status")
	creatorFacilityID := c.GetString("user_facility_id")

	filter := bson.M{
		"requestingFacilityID": creatorFacilityID,
	}
	if status != "" {
		filter["status"] = status
	}

	collection := h.DB.Collection("replenishment_requests")
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query replenishment requests"})
		return
	}
	defer cursor.Close(context.Background())

	var requests []models.ReplenishmentRequest
	if err = cursor.All(context.Background(), &requests); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode requests"})
		return
	}

	if requests == nil {
		requests = []models.ReplenishmentRequest{}
	}

	c.JSON(http.StatusOK, requests)
}

// GetReplenishmentRequestByID lấy chi tiết một yêu cầu nhập hàng theo ID.
func (h *ReplenishmentHandler) GetReplenishmentRequestByID(c *gin.Context) {
	requestID := c.Param("id")
	collection := h.DB.Collection("replenishment_requests")
	var request models.ReplenishmentRequest
	if err := collection.FindOne(context.Background(), bson.M{"requestID": requestID}).Decode(&request); err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Replenishment request not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve replenishment request"})
		}
		return
	}

	c.JSON(http.StatusOK, request)
}


// GetReplenishmentRequestsByFacility lấy danh sách các yêu cầu nhập hàng theo facilityID vaf status.
func (h *ReplenishmentHandler) GetReplenishmentRequestsByFacility(c *gin.Context) {
	facilityID := c.Param("facilityID")
	status := c.Query("status")
	filter := bson.M{"requestingFacilityID": facilityID}
	if status != "" {
		filter["status"] = status
	}

	collection := h.DB.Collection("replenishment_requests")
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query replenishment requests"})
		return
	}
	defer cursor.Close(context.Background())

	var requests []models.ReplenishmentRequest
	if err = cursor.All(context.Background(), &requests); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode requests"})
		return
	}

	if requests == nil {
		requests = []models.ReplenishmentRequest{}
	}

	c.JSON(http.StatusOK, requests)
}

// server/internal/api/handlers/facility_handler.go
package handlers

import (
	"context"
	"net/http"
	"time"
	"fresh-meat-scm-api-server/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type FacilityHandler struct {
	DB *mongo.Database
}

type AddressRequest struct {
	FullText  string  `json:"fullText" binding:"required"`
	Latitude  float64 `json:"latitude" binding:"required,min=-90,max=90"`
	Longitude float64 `json:"longitude" binding:"required,min=-180,max=180"`
}

type CreateFacilityRequest struct {
	FacilityID string `json:"facilityID" binding:"required"`
	Name       string       `json:"name" binding:"required"`
	Type       string       `json:"type" binding:"required"`
	Address    AddressRequest `json:"address" binding:"required"`
}

// CreateFacility tạo một cơ sở mới
func (h *FacilityHandler) CreateFacility(c *gin.Context) {
	var req CreateFacilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	collection := h.DB.Collection("facilities")

	// Kiểm tra xem facilityID đã tồn tại chưa
	count, err := collection.CountDocuments(context.Background(), bson.M{"facilityID": req.FacilityID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error checking for facility"})
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Facility with this ID already exists"})
		return
	}

	fullAddress := models.Address{
		FullText:  req.Address.FullText,
		Latitude:  req.Address.Latitude,
		Longitude: req.Address.Longitude,
	}

	newFacility := models.Facility{
		FacilityID: req.FacilityID,
		Name:       req.Name,
		Type:       req.Type,
		Address:    fullAddress,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	result, err := collection.InsertOne(context.Background(), newFacility)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create facility"})
		return
	}

	if oid, ok := result.InsertedID.(primitive.ObjectID); ok {
		newFacility.ID = oid
	}

	c.JSON(http.StatusCreated, newFacility)
}

// GetAllFacilities lấy danh sách tất cả các cơ sở
func (h *FacilityHandler) GetAllFacilities(c *gin.Context) {
	collection := h.DB.Collection("facilities")

	cursor, err := collection.Find(context.Background(), bson.M{})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query facilities"})
		return
	}
	defer cursor.Close(context.Background())

	var facilities []models.Facility
	if err = cursor.All(context.Background(), &facilities); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode facilities"})
		return
	}
	
	if facilities == nil {
		facilities = []models.Facility{}
	}

	c.JSON(http.StatusOK, facilities)
}

// GetFacilityByID lấy thông tin cơ sở theo facilityID
func (h *FacilityHandler) GetFacilityByID(c *gin.Context) {
	facilityID := c.Param("id")

	collection := h.DB.Collection("facilities")
	var facility models.Facility
	err := collection.FindOne(context.Background(), bson.M{"facilityID": facilityID}).Decode(&facility)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "Facility not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve facility"})
		}
		return
	}

	c.JSON(http.StatusOK, facility)
}

// UpdateFacility cập nhật thông tin cơ sở theo facilityID
func (h *FacilityHandler) UpdateFacility(c *gin.Context) {
	facilityID := c.Param("id")

	var req CreateFacilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	collection := h.DB.Collection("facilities")

	// Cập nhật thông tin cơ sở
	_, err := collection.UpdateOne(context.Background(), bson.M{"facilityID": facilityID}, bson.M{"$set": bson.M{
		"name":     req.Name,
		"type":     req.Type,
		"address":  req.Address,
		"updatedAt": time.Now(),
	}})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update facility"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Facility updated successfully"})
}

// DeleteFacility xóa một cơ sở theo facilityID
func (h *FacilityHandler) DeleteFacility(c *gin.Context) {
	facilityID := c.Param("id")

	collection := h.DB.Collection("facilities")
	_, err := collection.DeleteOne(context.Background(), bson.M{"facilityID": facilityID})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete facility"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Facility deleted successfully"})
}
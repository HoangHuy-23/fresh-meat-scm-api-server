// server/internal/api/handlers/vehicle_handler.go
package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"fresh-meat-scm-api-server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type VehicleHandler struct {
	DB *mongo.Database
}

type CreateVehiclePayload struct {
	PlateNumber   string            `json:"plateNumber" binding:"required"`
	OwnerDriverID string            `json:"ownerDriverID" binding:"required"`
	Model         string            `json:"model" binding:"required"`
	Specs         models.VehicleSpecs `json:"specs" binding:"required"`
}

// CreateVehicle tạo một phương tiện mới
func (h *VehicleHandler) CreateVehicle(c *gin.Context) {
	var payload CreateVehiclePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Kiểm tra xem OwnerDriverID có tồn tại và có role là "driver" không
	var driver models.User
	if err := h.DB.Collection("users").FindOne(context.Background(), bson.M{"fabricEnrollmentID": payload.OwnerDriverID, "role": "driver"}).Decode(&driver); err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid driver ID or role"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check driver"})
		}
		return
	}

	newVehicle := models.Vehicle{
		VehicleID:     fmt.Sprintf("VEH-%s", uuid.New().String()[:8]),
		PlateNumber:   payload.PlateNumber,
		OwnerDriverID: payload.OwnerDriverID,
		Model:         payload.Model,
		Specs:         payload.Specs,
		Status:        "AVAILABLE",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	collection := h.DB.Collection("vehicles")
	// ... (lưu vào DB và trả về response) ...
	result, err := collection.InsertOne(context.Background(), newVehicle)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create vehicle"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"vehicleID": newVehicle.VehicleID, "id": result.InsertedID})
}


func (h *VehicleHandler) GetVehiclesByDriver(c *gin.Context) {
	driverID := c.Param("id")
	collection := h.DB.Collection("vehicles")
	filter := bson.M{"ownerDriverID": driverID}
	// ... (tìm và trả về danh sách xe) ...
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query vehicles"})
		return
	}
	defer cursor.Close(context.Background())

	var vehicles []models.Vehicle
	if err := cursor.All(context.Background(), &vehicles); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode vehicles"})
		return
	}

	c.JSON(http.StatusOK, vehicles)
}
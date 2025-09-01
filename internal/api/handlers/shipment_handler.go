// internal/api/handlers/shipment_handler.go
package handlers

import (
	"encoding/json"
	"net/http"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/blockchain"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

type ShipmentHandler struct {
	Fabric *blockchain.FabricSetup
	Cfg    config.Config
	DB     *mongo.Database
}

// --- Structs cho Request Body ---

type StopInJourneyAPI struct {
	FacilityID string              `json:"facilityID" binding:"required"`
	Action     string              `json:"action" binding:"required"`
	Items      []ItemInShipmentAPI `json:"items" binding:"required"`
}

type ItemInShipmentAPI struct {
	AssetID  string   `json:"assetID" binding:"required"`
	Quantity Quantity `json:"quantity" binding:"required"`
}

type CreateShipmentRequest struct {
	ShipmentID         string             `json:"shipmentID" binding:"required"`
	ShipmentType       string             `json:"shipmentType" binding:"required"`
	// DriverEnrollmentID string             `json:"driverEnrollmentID" binding:"required"` // Bỏ trường này, lấy từ token
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

// --- Handlers ---

func (h *ShipmentHandler) CreateShipment(c *gin.Context) {
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

	var req CreateShipmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	stopsJSON, _ := json.Marshal(req.Stops)

	_, err = contract.SubmitTransaction(
		"CreateShipment",
		req.ShipmentID,
		req.ShipmentType,
		// req.DriverEnrollmentID,
		enrollmentID,
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

	network, err := userGateway.GetNetwork(h.Cfg.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.ChaincodeName)

	shipmentID := c.Param("id")
	var req ConfirmPickupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	actualItemsJSON, _ := json.Marshal(req.ActualItems)

	_, err = contract.SubmitTransaction(
		"ConfirmPickup",
		shipmentID,
		req.FacilityID,
		string(actualItemsJSON),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
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

	network, err := userGateway.GetNetwork(h.Cfg.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.ChaincodeName)

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

	network, err := userGateway.GetNetwork(h.Cfg.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.ChaincodeName)

	shipmentID := c.Param("id")
	var req ConfirmDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err = contract.SubmitTransaction(
		"ConfirmShipmentDelivery",
		shipmentID,
		req.FacilityID,
		req.NewAssetPrefix,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Delivery confirmed for shipment " + shipmentID})
}






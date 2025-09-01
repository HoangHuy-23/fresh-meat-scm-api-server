// internal/api/handlers/shipment_handler.go
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"fmt"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fresh-meat-scm-api-server/internal/models"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

)

type ShipmentHandler struct {
	Fabric *blockchain.FabricSetup
	Cfg    config.Config
	DB     *mongo.Database
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
	FacilityAddress string              `json:"facilityAddress"`
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
	network, err := userGateway.GetNetwork(h.Cfg.ChannelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get network", "details": err.Error()})
		return
	}
	contract := network.GetContract(h.Cfg.ChaincodeName)

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






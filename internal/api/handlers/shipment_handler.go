// internal/api/handlers/shipment_handler.go
package handlers

import (
	"encoding/json"
	"net/http"
	"fresh-meat-scm-api-server/internal/blockchain"

	"github.com/gin-gonic/gin"
)

type ShipmentHandler struct {
	Fabric *blockchain.FabricSetup
}

// --- Structs cho Request Body ---

type CreateShipmentRequest struct {
	ShipmentID     string `json:"shipmentID" binding:"required"`
	CarrierOrgName string `json:"carrierOrgName" binding:"required"`
	DriverName     string `json:"driverName" binding:"required"`
	VehiclePlate   string `json:"vehiclePlate" binding:"required"`
	FromFacilityID string `json:"fromFacilityID" binding:"required"`
}

type LoadItemRequest struct {
	AssetID      string   `json:"assetID" binding:"required"`
	Quantity     Quantity `json:"quantity" binding:"required"`
	ToFacilityID string   `json:"toFacilityID" binding:"required"`
}

// ConfirmDeliveryRequest: Cập nhật lại request body
type ConfirmDeliveryRequest struct {
	FacilityID string `json:"facilityID" binding:"required"`
	NewAssetID string `json:"newAssetID" binding:"required"` // ID cho lô con mới sẽ được tạo
	NewStatus  string `json:"newStatus" binding:"required"`
}

// --- Handlers ---

func (h *ShipmentHandler) CreateShipment(c *gin.Context) {
	var req CreateShipmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.Fabric.Contract.SubmitTransaction(
		"CreateShipment",
		req.ShipmentID,
		req.CarrierOrgName,
		req.DriverName,
		req.VehiclePlate,
		req.FromFacilityID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "success", "shipmentID": req.ShipmentID})
}

func (h *ShipmentHandler) LoadItem(c *gin.Context) {
	shipmentID := c.Param("id")
	var req LoadItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	quantityJSON, _ := json.Marshal(req.Quantity)

	_, err := h.Fabric.Contract.SubmitTransaction(
		"LoadItemToShipment",
		shipmentID,
		req.AssetID,
		string(quantityJSON),
		req.ToFacilityID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Item loaded successfully"})
}

func (h *ShipmentHandler) ConfirmDelivery(c *gin.Context) {
	shipmentID := c.Param("id")
	var req ConfirmDeliveryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.Fabric.Contract.SubmitTransaction(
		"ConfirmShipmentDelivery",
		shipmentID,
		req.FacilityID,
		req.NewAssetID,
		req.NewStatus, // <-- TRUYỀN THAM SỐ MỚI
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Delivery confirmed, new asset created", "newAssetID": req.NewAssetID})
}

// StartShipment handles the API endpoint for starting a shipment.
func (h *ShipmentHandler) StartShipment(c *gin.Context) {
	shipmentID := c.Param("id")

	_, err := h.Fabric.Contract.SubmitTransaction("StartShipment", shipmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Shipment " + shipmentID + " has started."})
}
// internal/api/handlers/event_handler.go
package handlers

import (
	"encoding/json"
	"net/http"
	"fresh-meat-scm-api-server/internal/blockchain"

	"github.com/gin-gonic/gin"
)

type EventHandler struct {
	Fabric *blockchain.FabricSetup
}

// AddEventRequest defines the structure for adding a new event.
type AddEventRequest struct {
	EventType string          `json:"eventType" binding:"required"`
	NewStatus string          `json:"newStatus" binding:"required"`
	Details   json.RawMessage `json:"details" binding:"required"`
}

// AddEvent handles the API endpoint for adding a new event to an asset.
func (h *EventHandler) AddEvent(c *gin.Context) {
	assetID := c.Param("id")

	var req AddEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	_, err := h.Fabric.Contract.SubmitTransaction(
		"AddEvent",
		assetID,
		req.EventType,
		req.NewStatus,
		string(req.Details),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to submit transaction", "details": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Event added successfully", "assetID": assetID})
}
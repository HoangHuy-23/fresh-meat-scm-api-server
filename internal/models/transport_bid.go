package models

import (
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// BidAssignment đại diện cho việc gán một gói thầu cho một tài xế với một xe cụ thể.
type BidAssignment struct {
	DriverID  string `bson:"driverID" json:"driverID"`
	VehicleID string `bson:"vehicleID" json:"vehicleID"`
}

// BidStop đại diện cho một điểm dừng trong gói vận chuyển.
// Nó có thể là điểm lấy hàng (PICKUP) hoặc giao hàng (DELIVERY).
type BidStop struct {
	FacilityID string        `bson:"facilityID" json:"facilityID"`
	Action     string        `bson:"action" json:"action"` // PICKUP or DELIVERY
	Items      []ItemInShipmentAPI `bson:"items" json:"items"`
}

type TransportBid struct {
	ID                 primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	BidID              string             `bson:"bidID" json:"bidID"`
	ShipmentType       string             `bson:"shipmentType" json:"shipmentType"`
	Stops              []BidStop          `bson:"stops" json:"stops"`
	Status             string             `bson:"status" json:"status"`
	BiddingAssignments []BidAssignment    `bson:"biddingAssignments" json:"biddingAssignments"` 
	ConfirmedAssignment BidAssignment      `bson:"confirmedAssignment,omitempty" json:"confirmedAssignment"`
	CreatedAt          time.Time          `bson:"createdAt" json:"createdAt"`
	ConfirmedAt        time.Time          `bson:"confirmedAt,omitempty" json:"confirmedAt"`
	OriginalRequestIDs []string           `bson:"originalRequestIDs" json:"originalRequestIDs"`
	ShipmentID         string             `bson:"shipmentID,omitempty" json:"shipmentID"`
}
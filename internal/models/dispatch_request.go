package models

import (
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// type Quantity struct {
// 	Unit  string  `bson:"unit" json:"unit"`
// 	Value float64 `bson:"value" json:"value"`
// }


type DispatchRequest struct {
	ID             primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	RequestID      string             `bson:"requestID" json:"requestID"`
	FromFacilityID string             `bson:"fromFacilityID" json:"fromFacilityID"`
	Items          []ItemInShipmentAPI `bson:"items" json:"items"`
	Status         string             `bson:"status" json:"status"` // PENDING, PROCESSED
	CreatedBy      string             `bson:"createdBy" json:"createdBy"`
	CreatedAt      time.Time          `bson:"createdAt" json:"createdAt"`
}
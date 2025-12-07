package models

import (
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type DispatchItemDetail struct {
	AssetID  string   `bson:"assetID" json:"assetID"`   
	SKU      string   `bson:"sku" json:"sku"`
	Quantity Quantity `bson:"quantity" json:"quantity"`
}

type DispatchRequest struct {
	ID             primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	RequestID      string               `bson:"requestID" json:"requestID"`
	FromFacilityID string               `bson:"fromFacilityID" json:"fromFacilityID"`
	Items          []DispatchItemDetail `bson:"items" json:"items"`
	Status         string               `bson:"status" json:"status"` // PENDING, PROCESSED
	CreatedBy      string               `bson:"createdBy" json:"createdBy"`
	CreatedAt      time.Time            `bson:"createdAt" json:"createdAt"`
}
package models

import (
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ReplenishmentItem struct {
	SKU      string   `bson:"sku" json:"sku"`
	Quantity Quantity `bson:"quantity" json:"quantity"`
}

type ReplenishmentRequest struct {
	ID                   primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	RequestID            string              `bson:"requestID" json:"requestID"`
	RequestingFacilityID string              `bson:"requestingFacilityID" json:"requestingFacilityID"`
	Items                []ReplenishmentItem `bson:"items" json:"items"`
	Status               string              `bson:"status" json:"status"`
	CreatedBy            string              `bson:"createdBy" json:"createdBy"`
	CreatedAt            time.Time           `bson:"createdAt" json:"createdAt"`
}
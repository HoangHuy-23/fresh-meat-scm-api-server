package models

import (
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type DeliveryProof struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ShipmentID string             `bson:"shipmentID" json:"shipmentID"`
	FacilityID string             `bson:"facilityID" json:"facilityID"`
	PhotoURL   string             `bson:"photoURL" json:"photoURL"`
	PhotoHash  string             `bson:"photoHash" json:"photoHash"`
	UploadedBy string             `bson:"uploadedBy" json:"uploadedBy"` 
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
}
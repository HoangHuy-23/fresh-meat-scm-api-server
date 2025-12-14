// server/internal/models/facility.go
package models

import (
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Facility struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	FacilityID string             `bson:"facilityID" json:"facilityID"` // User-friendly unique ID, e.g., "farm-A"
	Name       string             `bson:"name" json:"name"`             // e.g., "Trang trại Hữu cơ A"
	Type       string             `bson:"type" json:"type"`             // e.g., "FARM", "PROCESSOR", "WAREHOUSE", "RETAILER"
	Address    Address            `bson:"address" json:"address"`
	Status	   string             `bson:"status" json:"status"` // e.g., "ACTIVE", "INACTIVE", "UNDER_MAINTENANCE", "FULL_CAPACITY"
	CreatedAt  time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time          `bson:"updatedAt" json:"updatedAt"`
}
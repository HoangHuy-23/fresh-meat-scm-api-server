// server/internal/models/vehicle.go
package models

import (
	"time"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type VehicleSpecs struct {
	Type          string  `bson:"type" json:"type"`                     // TRUCK, VAN, MOTORBIKE
	Refrigerated  bool    `bson:"refrigerated" json:"refrigerated"`     // Có phải xe đông lạnh không?
	PayloadTonnes float64 `bson:"payloadTonnes" json:"payloadTonnes"`   // Tải trọng (tấn)
	VolumeCBM     float64 `bson:"volumeCBM" json:"volumeCBM"`           // Thể tích (mét khối)
}

type Vehicle struct {
	ID               primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	VehicleID        string             `bson:"vehicleID" json:"vehicleID"`               // ID tự tạo, dễ đọc
	PlateNumber      string             `bson:"plateNumber" json:"plateNumber"`           // Biển số xe
	OwnerDriverID    string             `bson:"ownerDriverID" json:"ownerDriverID"`       // EnrollmentID của tài xế sở hữu/phụ trách chính
	Model            string             `bson:"model" json:"model"`                       // Ví dụ: "Hyundai Porter H150"
	Specs            VehicleSpecs       `bson:"specs" json:"specs"`                       // Thông số kỹ thuật
	Status           string             `bson:"status" json:"status"`                     // AVAILABLE, IN_TRIP, MAINTENANCE
	RegistrationDocs []MediaPointer     `bson:"registrationDocs,omitempty" json:"registrationDocs"` // Giấy tờ xe (tham chiếu S3)
	CreatedAt        time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt        time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// MediaPointer đại diện cho một tài liệu media được lưu trữ trên S3 hoặc dịch vụ tương tự.
type MediaPointer struct {
	ID       string `bson:"id" json:"id"`               // ID duy nhất trong hệ thống
	URL      string `bson:"url" json:"url"`           // URL truy cập tài liệu
	FileName string `bson:"fileName" json:"fileName"` // Tên file gốc
	FileType string `bson:"fileType" json:"fileType"` // Loại file, ví dụ: "image/png", "application/pdf"
}
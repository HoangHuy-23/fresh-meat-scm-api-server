// server/internal/models/common.go
package models

// Quantity định nghĩa đơn vị và giá trị số lượng.
type Quantity struct {
	Unit  string  `bson:"unit,omitempty" json:"unit"`
	Value float64 `bson:"value,omitempty" json:"value"`
}

// Address là một object có cấu trúc để lưu thông tin địa chỉ.
type Address struct {
	FullText  string  `bson:"fullText" json:"fullText"`
	Latitude  float64 `bson:"latitude" json:"latitude"`
	Longitude float64 `bson:"longitude" json:"longitude"`
}

// ItemInShipmentAPI là struct chung cho các item trong một lô hàng/yêu cầu.
type ItemInShipmentAPI struct {
	AssetID  string   `bson:"assetID,omitempty" json:"assetID"`
	Quantity Quantity `bson:"quantity,omitempty" json:"quantity"`
}

type Weight struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"` // e.g., kg, g
}
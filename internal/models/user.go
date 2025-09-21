package models

// User struct matches the document in MongoDB
type User struct {
	Email              string `bson:"email"`
	Name               string `bson:"name"`
	Password           string `bson:"password"`
	Role               string `bson:"role"`
	FacilityID         string `bson:"facilityID"`
	Status             string `bson:"status"`
	FabricEnrollmentID string `bson:"fabricEnrollmentID"`
}
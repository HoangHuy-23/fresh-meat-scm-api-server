// server/internal/database/seeder.go
package database

import (
	"context"
	"log"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/auth"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type User struct {
	Email              string `bson:"email"`
	Name               string `bson:"name"`
	Password           string `bson:"password"`
	Role               string `bson:"role"`
	FacilityID         string `bson:"facilityID"`
	Status             string `bson:"status"`
	FabricEnrollmentID string `bson:"fabricEnrollmentID"`
}

func SeedSuperAdmin(db *mongo.Database, cfg config.Config) error {
	userCollection := db.Collection("users")
	superAdminEmail := "superadmin@example.com"

	// Kiểm tra xem superadmin đã tồn tại chưa
	count, err := userCollection.CountDocuments(context.Background(), bson.M{"email": superAdminEmail})
	if err != nil {
		return err
	}

	if count > 0 {
		log.Println("Super admin already exists. Seeding skipped.")
		return nil
	}

	// Tạo superadmin nếu chưa có
	log.Println("Super admin not found. Seeding...")
	hashedPassword, err := auth.HashPassword("superadminpassword") // Đặt một password mặc định
	if err != nil {
		return err
	}

	superAdmin := User{
		Email:              superAdminEmail,
		Name:               "Super Admin",
		Password:           hashedPassword,
		Role:               "superadmin",
		FacilityID:         "system",
		Status:             "active",
		FabricEnrollmentID: cfg.UserName, // "superadmin"
	}

	_, err = userCollection.InsertOne(context.Background(), superAdmin)
	if err != nil {
		return err
	}

	log.Println("Super admin seeded successfully.")
	return nil
}
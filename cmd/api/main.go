// server/cmd/api/main.go
package main

import (
	"context"
	"log"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/api/routes"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fresh-meat-scm-api-server/internal/ca"
	"fresh-meat-scm-api-server/internal/database"
	"fresh-meat-scm-api-server/internal/s3"

	"github.com/joho/godotenv"
)

func main() {
	// === BƯỚC QUAN TRỌNG: LOAD FILE .ENV ===
	// Phải được gọi trước khi LoadConfig
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, reading from environment variables")
	}
	// =======================================

	// 1. Load configuration
	cfg, err := config.LoadConfig("./config")
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
	}

	// === BƯỚC DEBUG: IN CONFIG RA ĐỂ KIỂM TRA ===
	log.Printf("Loaded S3 Bucket: [%s]", cfg.S3.Bucket)
	log.Printf("Loaded S3 Region: [%s]", cfg.S3.Region)
	log.Printf("Loaded S3 Access Key ID: [%s]", cfg.S3.AccessKeyID)
	// Để an toàn, không nên in Secret Key ra log, chỉ cần kiểm tra Access Key là đủ
	if cfg.S3.AccessKeyID == "" {
		log.Fatal("FATAL: S3 Access Key ID is empty. Check .env file and config loading.")
	}
	// ==========================================

	// 2. Connect to MongoDB
	mongoClient, err := database.ConnectDB(cfg.Mongo.URI)
	if err != nil {
		log.Fatalf("Could not connect to MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(context.Background())
	db := mongoClient.Database(cfg.Mongo.DBName)
	log.Println("MongoDB connected successfully.")

	// 3. Initialize Fabric connection
	fabricSetup, err := blockchain.Initialize(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Fabric setup: %v", err)
	}
	defer fabricSetup.Gateway.Close()
	defer fabricSetup.SDK.Close()

	// 4. Initialize CA Service with admin user context
	// === KEY FIX: Truyền thêm orgName và adminUser ===
	caService, err := ca.NewCAService(
		fabricSetup.SDK, 
		"ca.meatsupply.example.com",
		cfg.Fabric.OrgName,     // "MeatSupplyOrg" 
		cfg.Fabric.UserName,    // "ApiServer" (hoặc "SuperAdmin")
	)
	if err != nil {
		log.Fatalf("Failed to initialize CA service: %v", err)
	}

	// 5. Initialize S3 Uploader
	s3Uploader, err := s3.NewUploader(cfg.S3)
	if err != nil {
		log.Fatalf("Failed to initialize S3 uploader: %v", err)
	}

	 // 5. (QUAN TRỌNG) Seed Super Admin user
    err = database.SeedSuperAdmin(db, cfg)
    if err != nil {
        log.Fatalf("Failed to seed super admin: %v", err)
    }

	// 6. Setup router
	router := routes.SetupRouter(fabricSetup, caService, cfg, db, s3Uploader)

	// 7. Start server
	log.Printf("Starting API server on port %s", cfg.Server.Port)
	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
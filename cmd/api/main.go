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
)

func main() {
	// 1. Load configuration
	cfg, err := config.LoadConfig("./config")
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
	}

	// 2. Connect to MongoDB
	mongoClient, err := database.ConnectDB(cfg.MongoURI)
	if err != nil {
		log.Fatalf("Could not connect to MongoDB: %v", err)
	}
	defer mongoClient.Disconnect(context.Background())
	db := mongoClient.Database(cfg.MongoDBName)
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
		cfg.OrgName,     // "MeatSupplyOrg" 
		cfg.UserName,    // "ApiServer" (hoặc "SuperAdmin")
	)
	if err != nil {
		log.Fatalf("Failed to initialize CA service: %v", err)
	}

	 // 5. (QUAN TRỌNG) Seed Super Admin user
    err = database.SeedSuperAdmin(db, cfg)
    if err != nil {
        log.Fatalf("Failed to seed super admin: %v", err)
    }

	// 6. Setup router
	router := routes.SetupRouter(fabricSetup, caService, cfg, db)

	// 7. Start server
	log.Printf("Starting API server on port %s", cfg.ServerPort)
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
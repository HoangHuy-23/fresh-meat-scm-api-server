// cmd/api/main.go
package main

import (
	"log"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/api/routes"
	"fresh-meat-scm-api-server/internal/blockchain"
)

func main() {
	// 1. Load configuration
	cfg, err := config.LoadConfig("./config") // Load từ thư mục gốc của server
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
	}

	// 2. Initialize Fabric connection
	fabricSetup, err := blockchain.Initialize(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize Fabric setup: %v", err)
	}
	defer fabricSetup.Gateway.Close()

	// 3. Setup router
	router := routes.SetupRouter(fabricSetup)

	// 4. Start server
	log.Printf("Starting API server on port %s", cfg.ServerPort)
	if err := router.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
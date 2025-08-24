// server/cmd/api/main.go
package main

import (
	"log"
	"path/filepath"
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/api/routes"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fresh-meat-scm-api-server/internal/wallet"

	fabconfig "github.com/hyperledger/fabric-sdk-go/pkg/core/config"
	"github.com/hyperledger/fabric-sdk-go/pkg/fabsdk"
	"github.com/hyperledger/fabric-sdk-go/pkg/gateway"
)

func main() {
	// 1. Load configuration
	cfg, err := config.LoadConfig("./config")
	if err != nil {
		log.Fatalf("Could not load config: %v", err)
	}

	// 2. Khởi tạo FabSDK (cần cho AdminHandler)
	sdk, err := fabsdk.New(fabconfig.FromFile(filepath.Clean(cfg.Fabric.ConnectionProfile)))
	if err != nil {
		log.Fatalf("Failed to create Fabric SDK: %v", err)
	}
	defer sdk.Close()

	// 3. Khởi tạo Wallet (cần cho AdminHandler và FabricSetup)
	fsWallet, err := gateway.NewFileSystemWallet("wallet")
	if err != nil {
		log.Fatalf("Failed to create wallet: %v", err)
	}

	// 4. Đảm bảo danh tính superadmin có trong wallet
	err = wallet.PopulateWallet(fsWallet, cfg.Fabric.SuperAdmin.OrgName, cfg.Fabric.SuperAdmin.UserName, cfg.Fabric.SuperAdmin.CertPath, cfg.Fabric.SuperAdmin.KeyDir)
	if err != nil {
		log.Fatalf("Failed to populate wallet with superadmin identity: %v", err)
	}

	// 5. Khởi tạo kết nối nghiệp vụ chính (dùng danh tính superadmin)
	fabricSetup, err := blockchain.Initialize(cfg, fsWallet, cfg.Fabric.SuperAdmin.UserName)
	if err != nil {
		log.Fatalf("Failed to initialize Fabric setup: %v", err)
	}
	defer fabricSetup.Gateway.Close()

	// 6. Truyền tất cả các thành phần cần thiết vào router
	router := routes.SetupRouter(fabricSetup, sdk, fsWallet, cfg)

	// 7. Start server
	log.Printf("Starting API server on port %s", cfg.Server.Port)
	if err := router.Run(":" + cfg.Server.Port); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
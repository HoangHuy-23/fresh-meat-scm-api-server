// server/internal/api/routes/routes.go
package routes

import (
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/api/handlers"
	"fresh-meat-scm-api-server/internal/api/middleware"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fresh-meat-scm-api-server/internal/ca"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

// SetupRouter nhận vào các thành phần phụ thuộc và thiết lập các route
func SetupRouter(
	fabricSetup *blockchain.FabricSetup,
	caService *ca.CAService,
	cfg config.Config,
	db *mongo.Database,
) *gin.Engine {
	router := gin.Default()
	router.Use(gin.Recovery())

	// Khởi tạo các handlers với đúng các thành phần chúng cần
	assetHandler := &handlers.AssetHandler{Fabric: fabricSetup, Cfg: cfg, DB: db}
	shipmentHandler := &handlers.ShipmentHandler{Fabric: fabricSetup, Cfg: cfg, DB: db}
	userHandler := &handlers.UserHandler{CAService: caService, Wallet: fabricSetup.Wallet, OrgName: cfg.OrgName, DB: db}

	apiV1 := router.Group("/api/v1")
	{
				// Nhóm các API authentication
		auth := apiV1.Group("/auth")
		{
			auth.POST("/login", userHandler.Login)
		}

		// Nhóm các API quản trị
        admin := apiV1.Group("/admin")
		admin.Use(middleware.AuthMiddleware()) // Chỉ admin mới được phép
        {
            admin.POST("/users", userHandler.CreateUser)
        }
		businessRoutes := apiV1.Group("/")
		businessRoutes.Use(middleware.AuthMiddleware()) 
		{
		assets := businessRoutes.Group("/assets")
		{
			assets.POST("/farming", assetHandler.CreateFarmingBatch)
			assets.PUT("/:id/farming-details", assetHandler.UpdateFarmingDetails)
			assets.POST("/split", assetHandler.ProcessAndSplitBatch)
			assets.POST("/:id/storage", assetHandler.UpdateStorageInfo)
			assets.POST("/:id/sell", assetHandler.MarkAsSold)
			assets.POST("/split-to-units", assetHandler.SplitBatchToUnits)
			assets.GET("/:id/trace", assetHandler.GetAssetTrace)
		}

		shipments := businessRoutes.Group("/shipments")
		{
			shipments.POST("/", shipmentHandler.CreateShipment)
			shipments.POST("/:id/pickup", shipmentHandler.ConfirmPickup)
			shipments.POST("/:id/start", shipmentHandler.StartShipment)
			shipments.POST("/:id/deliver", shipmentHandler.ConfirmDelivery)
		}
		}
	}

	return router
}
// server/internal/api/routes/routes.go
package routes

import (
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/api/handlers"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fresh-meat-scm-api-server/internal/ca"

	"github.com/gin-gonic/gin"
)

// SetupRouter nhận vào các thành phần phụ thuộc và thiết lập các route
func SetupRouter(
	fabricSetup *blockchain.FabricSetup,
	caService *ca.CAService,
	cfg config.Config,
) *gin.Engine {
	router := gin.Default()

	// Khởi tạo các handlers với đúng các thành phần chúng cần
	assetHandler := &handlers.AssetHandler{Fabric: fabricSetup}
	shipmentHandler := &handlers.ShipmentHandler{Fabric: fabricSetup}
	userHandler := &handlers.UserHandler{CAService: caService, Wallet: fabricSetup.Wallet, OrgName: cfg.OrgName}

	apiV1 := router.Group("/api/v1")
	{
		assets := apiV1.Group("/assets")
		{
			assets.POST("/farming", assetHandler.CreateFarmingBatch)
			assets.PUT("/:id/farming-details", assetHandler.UpdateFarmingDetails)
			assets.POST("/split", assetHandler.ProcessAndSplitBatch)
			assets.POST("/:id/storage", assetHandler.UpdateStorageInfo)
			assets.POST("/:id/sell", assetHandler.MarkAsSold)
			assets.POST("/split-to-units", assetHandler.SplitBatchToUnits)
			assets.GET("/:id/trace", assetHandler.GetAssetTrace)
		}

		shipments := apiV1.Group("/shipments")
		{
			shipments.POST("/", shipmentHandler.CreateShipment)
			shipments.POST("/:id/load", shipmentHandler.LoadItem)
			shipments.POST("/:id/start", shipmentHandler.StartShipment)
			shipments.POST("/:id/deliver", shipmentHandler.ConfirmDelivery)
		}

		// Nhóm các API quản trị
        admin := apiV1.Group("/admin")
        {
            admin.POST("/users", userHandler.CreateUser)
        }
	}


	return router
}
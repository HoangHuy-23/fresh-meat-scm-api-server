// server/internal/api/routes/routes.go
package routes

import (
	"fresh-meat-scm-api-server/internal/api/handlers"
	"fresh-meat-scm-api-server/internal/blockchain"

	"github.com/gin-gonic/gin"
)

func SetupRouter(fabricSetup *blockchain.FabricSetup) *gin.Engine {
	router := gin.Default()

	// Khởi tạo các handlers
	assetHandler := &handlers.AssetHandler{Fabric: fabricSetup}
	shipmentHandler := &handlers.ShipmentHandler{Fabric: fabricSetup}

	apiV1 := router.Group("/api/v1")
	{
		// Nhóm các API liên quan đến tài sản (lô hàng, sản phẩm)
		assets := apiV1.Group("/assets")
		{
			// --- Nghiệp vụ tạo và cập nhật tại nguồn ---
			assets.POST("/farming", assetHandler.CreateFarmingBatch)
			assets.PUT("/:id/farming-details", assetHandler.UpdateFarmingDetails) // <-- ENDPOINT MỚI
			assets.POST("/split", assetHandler.ProcessAndSplitBatch)

			// --- Nghiệp vụ cập nhật trạng thái & thông tin tại các điểm khác ---
			assets.POST("/:id/storage", assetHandler.UpdateStorageInfo)
			assets.POST("/:id/sell", assetHandler.MarkAsSold)

			// --- Nghiệp vụ Retail split to units
			assets.POST("/split-to-units", assetHandler.SplitBatchToUnits)

			// --- Nghiệp vụ truy xuất ---
			assets.GET("/:id/trace", assetHandler.GetAssetTrace)
		}

		// Nhóm các API liên quan đến vận chuyển
		shipments := apiV1.Group("/shipments")
		{
			shipments.POST("/", shipmentHandler.CreateShipment)
			shipments.POST("/:id/load", shipmentHandler.LoadItem)
			shipments.POST("/:id/start", shipmentHandler.StartShipment)
			shipments.POST("/:id/deliver", shipmentHandler.ConfirmDelivery)
		}
	}

	return router
}
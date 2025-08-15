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
	eventHandler := &handlers.EventHandler{Fabric: fabricSetup}

	// Định nghĩa các group API
	apiV1 := router.Group("/api/v1")
	{
		assets := apiV1.Group("/assets")
		{
			// Tạo một lô nông trại mới
			// POST /api/v1/assets/farming
			assets.POST("/farming", assetHandler.CreateFarmingBatch)

			// Chế biến và tách một lô
			// POST /api/v1/assets/split
			assets.POST("/split", assetHandler.ProcessAndSplitBatch)

			// Lấy lịch sử truy xuất đầy đủ của một tài sản
			// GET /api/v1/assets/:id/history
			assets.GET("/:id/history", assetHandler.GetAssetHistory)

			// Thêm một sự kiện mới vào một tài sản đã tồn tại
			// POST /api/v1/assets/:id/events
			assets.POST("/:id/events", eventHandler.AddEvent)
		}
	}

	return router
}
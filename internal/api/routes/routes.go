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

	// Khởi tạo các handlers
	assetHandler := &handlers.AssetHandler{Fabric: fabricSetup, Cfg: cfg, DB: db}
	shipmentHandler := &handlers.ShipmentHandler{Fabric: fabricSetup, Cfg: cfg, DB: db}
	userHandler := &handlers.UserHandler{CAService: caService, Wallet: fabricSetup.Wallet, OrgName: cfg.OrgName, DB: db}
	facilityHandler := &handlers.FacilityHandler{DB: db}

	apiV1 := router.Group("/api/v1")
	{
		// === CÁC ROUTE KHÔNG YÊU CẦU XÁC THỰC ===

		// Nhóm API authentication
		auth := apiV1.Group("/auth")
		{
			auth.POST("/login", userHandler.Login)
		}

		// Nhóm API công khai (Public)
		public := apiV1.Group("/")
		{
			// API truy xuất nguồn gốc, không cần JWT
			public.GET("/assets/:id/trace", assetHandler.GetAssetTrace)
		}


		// === CÁC ROUTE YÊU CẦU XÁC THỰC (PROTECTED) ===
		// Tất cả các route bên dưới sẽ đi qua middleware Authenticate trước

		// Nhóm API quản trị, yêu cầu vai trò "superadmin"
		admin := apiV1.Group("/admin")
		admin.Use(middleware.Authenticate())
		admin.Use(middleware.Authorize("superadmin"))
		{
			// User management
			admin.POST("/users", userHandler.CreateUser)

			// Facility management (CRUD) 
			facilities := admin.Group("/facilities")
			{
				facilities.POST("/", facilityHandler.CreateFacility)
				facilities.GET("/", facilityHandler.GetAllFacilities)
				facilities.GET("/:id", facilityHandler.GetFacilityByID)
				facilities.PUT("/:id", facilityHandler.UpdateFacility)
				facilities.DELETE("/:id", facilityHandler.DeleteFacility)
			}
		}

		// Nhóm các API nghiệp vụ chính, yêu cầu các vai trò cụ thể
		businessRoutes := apiV1.Group("/")
		businessRoutes.Use(middleware.Authenticate())
		businessRoutes.Use(middleware.Authorize("admin", "worker", "driver"))
		{
			assets := businessRoutes.Group("/assets")
			{
				assets.POST("/farming", assetHandler.CreateFarmingBatch)
				assets.PUT("/:id/farming-details", assetHandler.UpdateFarmingDetails)
				assets.POST("/split", assetHandler.ProcessAndSplitBatch)
				assets.POST("/:id/storage", assetHandler.UpdateStorageInfo)
				assets.POST("/:id/sell", assetHandler.MarkAsSold)
				assets.POST("/split-to-units", assetHandler.SplitBatchToUnits)	
			}

			shipments := businessRoutes.Group("/shipments")
			{
				shipments.GET("/:id", shipmentHandler.GetShipment)
				shipments.POST("/", shipmentHandler.CreateShipment)
				shipments.POST("/:id/pickup", shipmentHandler.ConfirmPickup)
				shipments.POST("/:id/start", shipmentHandler.StartShipment)
				shipments.POST("/:id/deliver", shipmentHandler.ConfirmDelivery)
				// Route mới cho tài xế upload ảnh
				// Đặt trong một group riêng để áp dụng middleware Authorize chỉ cho driver
				driverActions := shipments.Group("/:id/stops/:facilityID")
				driverActions.Use(middleware.Authorize("driver"))
				{
					driverActions.POST("/pickup-photo", shipmentHandler.AddPickupPhoto)
					driverActions.POST("/delivery-photo", shipmentHandler.AddDeliveryPhoto)
				}
			}
		}
	}

	return router
}
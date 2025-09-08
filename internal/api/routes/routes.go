// server/internal/api/routes/routes.go
package routes

import (
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/api/handlers"
	"fresh-meat-scm-api-server/internal/api/middleware"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fresh-meat-scm-api-server/internal/ca"
	"fresh-meat-scm-api-server/internal/s3"
	"fresh-meat-scm-api-server/internal/socket"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

// SetupRouter nhận vào các thành phần phụ thuộc và thiết lập các route
func SetupRouter(
	fabricSetup *blockchain.FabricSetup,
	caService *ca.CAService,
	cfg config.Config,
	db *mongo.Database,
	s3Uploader *s3.Uploader,  // <-- THÊM S3 UPLOADER VÀO ĐÂY
	wsHub *socket.Hub,      // <-- THÊM WEBSOCKET HUB VÀO ĐÂY
) *gin.Engine {
	router := gin.Default()
	router.Use(gin.Recovery())

	// Khởi tạo các handlers
	assetHandler := &handlers.AssetHandler{Fabric: fabricSetup, Cfg: cfg, DB: db}
	shipmentHandler := &handlers.ShipmentHandler{Fabric: fabricSetup, Cfg: cfg, DB: db, S3Uploader: s3Uploader, Hub: wsHub} // <-- TRUYỀN S3 UPLOADER VÀO ĐÂY
	userHandler := &handlers.UserHandler{CAService: caService, Wallet: fabricSetup.Wallet, OrgName: cfg.Fabric.OrgName, DB: db}
	facilityHandler := &handlers.FacilityHandler{DB: db}
	webSocketHandler := &handlers.WebSocketHandler{Hub: wsHub} // <-- KHỞI TẠO WEBSOCKET HANDLER

	apiV1 := router.Group("/api/v1")
	{
		// Route cho WebSocket
		apiV1.GET("/ws", webSocketHandler.ServeWs) // <-- THÊM ROUTE WEBSOCKET
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
		businessRoutes.Use(middleware.Authorize("admin", "worker", "driver", "superadmin"))
		{
			// Asset management
			assets := businessRoutes.Group("/assets")
			{
				assets.POST("/farming", assetHandler.CreateFarmingBatch)
				assets.PUT("/:id/farming-details", assetHandler.UpdateFarmingDetails)
				assets.POST("/split", assetHandler.ProcessAndSplitBatch)
				assets.POST("/:id/storage", assetHandler.UpdateStorageInfo)
				assets.POST("/:id/sell", assetHandler.MarkAsSold)
				assets.POST("/split-to-units", assetHandler.SplitBatchToUnits)	
			}

			// Shipment management
			shipments := businessRoutes.Group("/shipments")
			{
				// Các route chung cho worker, admin, driver
				generalShipmentRoutes := shipments.Group("/")
				generalShipmentRoutes.Use(middleware.Authorize("admin", "worker", "driver"))
				{
					generalShipmentRoutes.GET("/:id", shipmentHandler.GetShipment)
				}

				// Route chỉ cho admin hoặc driver tạo shipment
				createShipmentRoutes := shipments.Group("/")
				createShipmentRoutes.Use(middleware.Authorize("admin", "driver"))
				{
					createShipmentRoutes.POST("/", shipmentHandler.CreateShipment)
				}

				// Route chỉ cho worker xác nhận
				workerShipmentRoutes := shipments.Group("/")
				workerShipmentRoutes.Use(middleware.Authorize("admin", "worker"))
				{
					workerShipmentRoutes.POST("/:id/pickup", shipmentHandler.ConfirmPickup)
					workerShipmentRoutes.POST("/:id/deliver", shipmentHandler.ConfirmDelivery)
				}
				
				// Route chỉ cho driver
				driverShipmentRoutes := shipments.Group("/")
				driverShipmentRoutes.Use(middleware.Authorize("admin", "driver"))
				{
					driverShipmentRoutes.POST("/:id/start", shipmentHandler.StartShipment)
				}

				// === PHẦN THÊM MỚI QUAN TRỌNG ===
				// Group route cho tài xế upload ảnh tại một điểm dừng cụ thể
				driverPhotoUploadRoutes := shipments.Group("/:id/stops/:facilityID")
				driverPhotoUploadRoutes.Use(middleware.Authorize("driver"))
				{
					// Endpoint để upload ảnh minh chứng LẤY HÀNG
					driverPhotoUploadRoutes.POST("/pickup-photo", shipmentHandler.UploadPickupPhoto)
					// Endpoint để upload ảnh minh chứng GIAO HÀNG
					driverPhotoUploadRoutes.POST("/delivery-photo", shipmentHandler.UploadDeliveryPhoto)
				}
				// =================================
				// Route mới cho tài xế upload ảnh
				// Đặt trong một group riêng để áp dụng middleware Authorize chỉ cho driver
				// driverActions := shipments.Group("/:id/stops/:facilityID")
				// driverActions.Use(middleware.Authorize("driver"))
				// {
				// 	driverActions.POST("/pickup-photo", shipmentHandler.AddPickupPhoto)
				// 	driverActions.POST("/delivery-photo", shipmentHandler.AddDeliveryPhoto)
				// }
			}

			// Facility management (chỉ đọc)
			facilities := businessRoutes.Group("/facilities")
			{
				facilities.GET("/:id/assets", assetHandler.GetAssetsByFacility)
				facilities.GET("/my/assets", assetHandler.GetAssetsByMyFacility)
			}

			// Drivers 
			drivers := businessRoutes.Group("/drivers")
			{
				drivers.GET("/:id/shipments", shipmentHandler.GetShipmentsByDriver)
			}
		}
	}

	return router
}
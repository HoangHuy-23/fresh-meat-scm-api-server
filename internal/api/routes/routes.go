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
	assetHandler := &handlers.AssetHandler{Fabric: fabricSetup, Cfg: cfg, DB: db, S3Uploader: s3Uploader}
	shipmentHandler := &handlers.ShipmentHandler{Fabric: fabricSetup, Cfg: cfg, DB: db, S3Uploader: s3Uploader, Hub: wsHub} // <-- TRUYỀN S3 UPLOADER VÀO ĐÂY
	userHandler := &handlers.UserHandler{CAService: caService, Wallet: fabricSetup.Wallet, OrgName: cfg.Fabric.OrgName, DB: db}
	facilityHandler := &handlers.FacilityHandler{DB: db}
	webSocketHandler := &handlers.WebSocketHandler{Hub: wsHub} // <-- KHỞI TẠO WEBSOCKET HANDLER
	dispatchHandler := &handlers.DispatchHandler{DB: db, Hub: wsHub} // <-- KHỞI TẠO DISPATCH HANDLER
	replenishmentHandler := &handlers.ReplenishmentHandler{DB: db, Hub: wsHub} // <-- KHỞI TẠO REPLENISHMENT HANDLER
	bidHandler := &handlers.BidHandler{DB: db, Hub: wsHub, Fabric: fabricSetup, Cfg: cfg} // <-- KHỞI TẠO BID HANDLER
	vehicleHandler := &handlers.VehicleHandler{DB: db} // <-- KHỞI TẠO VEHICLE HANDLER
	productHandler := &handlers.ProductHandler{Fabric: fabricSetup, Cfg: cfg} // <-- KHỞI TẠO PRODUCT HANDLER

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
			// API lấy danh sách phương tiện theo status
			public.GET("/vehicles", vehicleHandler.GetVehicles)
			// API lấy danh sách sản phẩm
			public.GET("/products", productHandler.GetAllProducts)
			// API lấy thông tin facility công khai
			public.GET("/facilities/public", facilityHandler.GetAllFacilities)

			// API AI Agent transport bid
			public.POST("/ai/transport-bids", bidHandler.CreateTransportBid) 

			public.GET("/facilities/:id/inventory", assetHandler.QueryAssetsByFacilityAndSKU) 
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

			// Vehicle management (CRUD)
			vehicles := admin.Group("/vehicles")
			{
				vehicles.POST("/", vehicleHandler.CreateVehicle)
				// Chúng ta có thể thêm các route GET, PUT, DELETE tương tự nếu cần
			}

			// Product management (CRUD)
			products := admin.Group("/products")
			{
				products.POST("/", productHandler.CreateProducts)
				// Chúng ta có thể thêm các route GET, PUT, DELETE tương tự nếu cần
			}
		}

		// Nhóm các API nghiệp vụ chính, yêu cầu các vai trò cụ thể
		businessRoutes := apiV1.Group("/")
		businessRoutes.Use(middleware.Authenticate())
		businessRoutes.Use(middleware.Authorize("admin", "worker", "driver", "superadmin"))
		{

			// === THÊM GROUP VÀ ROUTE MỚI TẠI ĐÂY ===
			profile := businessRoutes.Group("/profile")
			{
				// Endpoint cho người dùng lấy thông tin của chính họ
				profile.GET("/me", userHandler.GetProfile)
			}
			// =======================================

			// Asset management
			assets := businessRoutes.Group("/assets")
			{
				assets.POST("/farming", assetHandler.CreateFarmingBatch)
				assets.GET("/:id/farming", assetHandler.GetAssetAtFarmByID)
				assets.POST("/:id/farming/feeds", assetHandler.AddFeedToFarmingBatch)
				assets.POST("/:id/farming/medications", assetHandler.AddMedicationToFarmingBatch)
				assets.POST("/:id/farming/certificates", assetHandler.AddCertificatesToFarmingBatch)
				assets.PATCH("/:id/farming/harvest-date", assetHandler.UpdateHarvestDate)
				assets.PATCH("/:id/farming/average-weight", assetHandler.UpdateAverageWeight)
				assets.PATCH("/:id/farming/expected-harvest-date", assetHandler.UpdateExpectedHarvestDate)

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

				// ===== dành cho admin test =====
				adminTestRoutes := shipments.Group("/test")
				adminTestRoutes.Use(middleware.Authorize("superadmin"))
				{
					adminTestRoutes.POST("/:id/:vehicleID/complete", shipmentHandler.CompleteShipment)
				}
			}

			// Facility management (chỉ đọc)
			facilities := businessRoutes.Group("/facilities")
			{
				facilities.GET("/:id/assets", assetHandler.GetAssetsByFacility)
				facilities.GET("/my/assets", assetHandler.GetAssetsByMyFacility)
				facilities.GET("/:id/shipments", shipmentHandler.GetShipmentsByFacility)
				//
				facilities.PATCH("/:id/status", facilityHandler.UpdateFacilityStatus) 
			}

			// Drivers 
			drivers := businessRoutes.Group("/drivers")
			{
				drivers.GET("/:id/shipments", shipmentHandler.GetShipmentsByDriver)
				// Lấy danh sách xe của một tài xế
				drivers.GET("/:id/vehicles", vehicleHandler.GetVehiclesByDriver)
			}

			// Group mới cho processors
			processors := businessRoutes.Group("/processors")
			processors.Use(middleware.Authorize("admin", "worker"))
			{
				// :id ở đây là facilityID của nhà máy chế biến
				processors.GET("/:id/assets/unprocessed", assetHandler.GetUnprocessedAssetsByProcessor)
				processors.GET("/:id/assets/processed", assetHandler.GetProcessedAssetsByProcessor)
			}

			// Group cho retailers
			retailers := businessRoutes.Group("/retailers")
			retailers.Use(middleware.Authorize("admin", "worker"))
			{
				// :id ở đây là facilityID của cửa hàng bán lẻ
				retailers.GET("/:id/assets", assetHandler.GetAssetsAtRetailerByStatus)

				// Yêu cầu nhập hàng
				retailers.POST("/replenishment-requests", replenishmentHandler.CreateReplenishmentRequest)
				retailers.GET("/replenishment-requests/:requestID", replenishmentHandler.GetReplenishmentRequestByID)
				retailers.GET("/replenishment-requests/mine", replenishmentHandler.GetMyReplenishmentRequests) // Lấy tất cả yêu cầu của cửa hàng hiện tại
			}

			dispatchRequests := businessRoutes.Group("/dispatch-requests")
			{
				// Route cho worker/admin tạo yêu cầu
				createRoute := dispatchRequests.Group("/")
				createRoute.Use(middleware.Authorize("admin", "worker"))
				{
					createRoute.POST("/", dispatchHandler.CreateDispatchRequest)
					createRoute.GET("/:id", dispatchHandler.GetDispatchRequestByID)
				}

				// === THÊM ROUTE MỚI CHO ADMIN XEM ===
				// Route chỉ cho admin/superadmin xem danh sách
				adminRoute := dispatchRequests.Group("/")
				adminRoute.Use(middleware.Authorize("admin", "superadmin"))
				{
					adminRoute.GET("/", dispatchHandler.GetAllDispatchRequests)
				}
				// =====================================

				// Route cho facility xem các yêu cầu của họ
				facilityRoute := dispatchRequests.Group("/my")
				facilityRoute.Use(middleware.Authorize("admin", "worker", "driver"))
				{
					facilityRoute.GET("/", dispatchHandler.GetMyFacilityDispatchRequests)
				}
			}

			// === THÊM GROUP MỚI CHO REPLENISHMENT REQUESTS ===
			replenishmentRequests := businessRoutes.Group("/replenishment-requests")
			{
				// Route cho super admin xem tất cả yêu cầu
				superAdminRoute := replenishmentRequests.Group("/")
				superAdminRoute.Use(middleware.Authorize("superadmin"))
				{
					superAdminRoute.GET("/", replenishmentHandler.GetAllReplenishmentRequests)
				}
			}

			// === THÊM GROUP MỚI CHO TRANSPORT BIDS ===
			transportBids := businessRoutes.Group("/transport-bids")
			{
				// Route cho Admin tạo gói thầu
				adminRoute := transportBids.Group("/")
				adminRoute.Use(middleware.Authorize("admin", "superadmin"))
				{
					adminRoute.POST("/", bidHandler.CreateTransportBid)
				}

				// === THÊM ROUTE MỚI CHO DRIVER ===
				driverRoute := transportBids.Group("/")
				driverRoute.Use(middleware.Authorize("driver"))
				{
					// Lấy danh sách các gói thầu của tôi
					driverRoute.GET("/mine", bidHandler.GetMyBids)
					// Chúng ta sẽ thêm route POST /:id/confirm vào đây
					driverRoute.POST("/:id/confirm", bidHandler.ConfirmBid)
				}
				// =================================
			}
			// =======================================
		}
	}

	return router
}
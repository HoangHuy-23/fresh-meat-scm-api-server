package routes

import (
	"fresh-meat-scm-api-server/config"
	"fresh-meat-scm-api-server/internal/api/handlers"
	"fresh-meat-scm-api-server/internal/api/middleware"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fresh-meat-scm-api-server/internal/ca"
	"fresh-meat-scm-api-server/internal/s3"
	"fresh-meat-scm-api-server/internal/socket"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

// SetupRouter receives dependencies and sets up routes
func SetupRouter(
	fabricSetup *blockchain.FabricSetup,
	caService *ca.CAService,
	cfg config.Config,
	db *mongo.Database,
	s3Uploader *s3.Uploader,
	wsHub *socket.Hub,
) *gin.Engine {
	// 1. Create the router instance
	router := gin.New()
	

	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173"}, // explicit origins are required with credentials
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowHeaders:     []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"X-Requested-With",
			"X-CSRF-Token",
			"withcredentials", // tolerate client sending this as a header
		},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 3. Add Logger and Recovery middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	

	// Initialize handlers
	assetHandler := &handlers.AssetHandler{Fabric: fabricSetup, Cfg: cfg, DB: db, S3Uploader: s3Uploader}
	shipmentHandler := &handlers.ShipmentHandler{Fabric: fabricSetup, Cfg: cfg, DB: db, S3Uploader: s3Uploader, Hub: wsHub}
	userHandler := &handlers.UserHandler{CAService: caService, Wallet: fabricSetup.Wallet, OrgName: cfg.Fabric.OrgName, DB: db}
	facilityHandler := &handlers.FacilityHandler{DB: db}
	webSocketHandler := &handlers.WebSocketHandler{Hub: wsHub}
	dispatchHandler := &handlers.DispatchHandler{DB: db, Hub: wsHub}
	replenishmentHandler := &handlers.ReplenishmentHandler{DB: db, Hub: wsHub}
	bidHandler := &handlers.BidHandler{DB: db, Hub: wsHub, Fabric: fabricSetup, Cfg: cfg}
	vehicleHandler := &handlers.VehicleHandler{DB: db}
	productHandler := &handlers.ProductHandler{Fabric: fabricSetup, Cfg: cfg}

	apiV1 := router.Group("/api/v1")
	{
		apiV1.GET("/ws", webSocketHandler.ServeWs)
		
		auth := apiV1.Group("/auth")
		{
			auth.POST("/login", userHandler.Login)
		}

		public := apiV1.Group("/")
		{
			public.GET("/assets/:id/trace", assetHandler.GetAssetTrace)
			public.GET("/vehicles", vehicleHandler.GetVehicles)
			public.GET("/products", productHandler.GetAllProducts)
			public.GET("/facilities/public", facilityHandler.GetAllFacilities)
			public.POST("/ai/transport-bids", bidHandler.CreateTransportBid)
			public.GET("/facilities/:id/inventory", assetHandler.QueryAssetsByFacilityAndSKU)
		}

		admin := apiV1.Group("/admin")
		admin.Use(middleware.Authenticate())
		admin.Use(middleware.Authorize("superadmin"))
		{
			admin.POST("/users", userHandler.CreateUser)

			facilities := admin.Group("/facilities")
			{
				// Use empty string to match "/admin/facilities" exactly without redirect
				facilities.POST("", facilityHandler.CreateFacility)
				facilities.GET("", facilityHandler.GetAllFacilities)
				facilities.GET("/:id", facilityHandler.GetFacilityByID)
				facilities.PUT("/:id", facilityHandler.UpdateFacility)
				facilities.DELETE("/:id", facilityHandler.DeleteFacility)
			}

			vehicles := admin.Group("/vehicles")
			{
				vehicles.POST("", vehicleHandler.CreateVehicle)
			}

			products := admin.Group("/products")
			{
				products.POST("", productHandler.CreateProducts)
			}
		}

		businessRoutes := apiV1.Group("/")
		businessRoutes.Use(middleware.Authenticate())
		businessRoutes.Use(middleware.Authorize("admin", "worker", "driver", "superadmin"))
		{
			profile := businessRoutes.Group("/profile")
			{
				profile.GET("/me", userHandler.GetProfile)
			}

			assets := businessRoutes.Group("/assets")
			{
				assets.GET("/by-id/:assetID", assetHandler.GetAssetByID)
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

			shipments := businessRoutes.Group("/shipments")
			{
				generalShipmentRoutes := shipments.Group("/")
				generalShipmentRoutes.Use(middleware.Authorize("admin", "worker", "driver"))
				{
					generalShipmentRoutes.GET("/:id", shipmentHandler.GetShipment)
				}

				createShipmentRoutes := shipments.Group("/")
				createShipmentRoutes.Use(middleware.Authorize("admin", "driver"))
				{
					createShipmentRoutes.POST("", shipmentHandler.CreateShipment)
				}

				workerShipmentRoutes := shipments.Group("/")
				workerShipmentRoutes.Use(middleware.Authorize("admin", "worker"))
				{
					workerShipmentRoutes.POST("/:id/pickup", shipmentHandler.ConfirmPickup)
					workerShipmentRoutes.POST("/:id/delivery", shipmentHandler.ConfirmDelivery)
				}

				driverShipmentRoutes := shipments.Group("/")
				driverShipmentRoutes.Use(middleware.Authorize("admin", "driver"))
				{
					driverShipmentRoutes.POST("/:id/start", shipmentHandler.StartShipment)
				}

				driverPhotoUploadRoutes := shipments.Group("/:id/stops/:facilityID")
				driverPhotoUploadRoutes.Use(middleware.Authorize("driver"))
				{
					driverPhotoUploadRoutes.POST("/pickup-photo", shipmentHandler.UploadPickupPhoto)
					driverPhotoUploadRoutes.POST("/delivery-photo", shipmentHandler.UploadDeliveryPhoto)
				}

				adminTestRoutes := shipments.Group("/test")
				adminTestRoutes.Use(middleware.Authorize("superadmin"))
				{
					adminTestRoutes.POST("/:id/:vehicleID/complete", shipmentHandler.CompleteShipment)
				}
			}

			facilities := businessRoutes.Group("/facilities")
			{
				facilities.GET("/:id/assets", assetHandler.GetAssetsByFacility)
				facilities.GET("/my/assets", assetHandler.GetAssetsByMyFacility)
				facilities.GET("/:id/shipments", shipmentHandler.GetShipmentsByFacility)
				facilities.PATCH("/:id/status", facilityHandler.UpdateFacilityStatus)
			}

			drivers := businessRoutes.Group("/drivers")
			{
				drivers.GET("/:id/shipments", shipmentHandler.GetShipmentsByDriver)
				drivers.GET("/:id/vehicles", vehicleHandler.GetVehiclesByDriver)
			}

			processors := businessRoutes.Group("/processors")
			processors.Use(middleware.Authorize("admin", "worker"))
			{
				processors.GET("/:id/assets/unprocessed", assetHandler.GetUnprocessedAssetsByProcessor)
				processors.GET("/:id/assets/processed", assetHandler.GetProcessedAssetsByProcessor)
			}

			retailers := businessRoutes.Group("/retailers")
			retailers.Use(middleware.Authorize("admin", "worker"))
			{
				retailers.GET("/:id/assets", assetHandler.GetAssetsAtRetailerByStatus)
				retailers.POST("/replenishment-requests", replenishmentHandler.CreateReplenishmentRequest)
				retailers.GET("/replenishment-requests/:requestID", replenishmentHandler.GetReplenishmentRequestByID)
				retailers.GET("/replenishment-requests/mine", replenishmentHandler.GetMyReplenishmentRequests)
			}

			dispatchRequests := businessRoutes.Group("/dispatch-requests")
			{
				createRoute := dispatchRequests.Group("/")
				createRoute.Use(middleware.Authorize("admin", "worker"))
				{
					createRoute.POST("", dispatchHandler.CreateDispatchRequest)
					createRoute.GET("/:id", dispatchHandler.GetDispatchRequestByID)
				}

				adminRoute := dispatchRequests.Group("/")
				adminRoute.Use(middleware.Authorize("admin", "superadmin"))
				{
					adminRoute.GET("", dispatchHandler.GetAllDispatchRequests)
				}

				facilityRoute := dispatchRequests.Group("/my")
				facilityRoute.Use(middleware.Authorize("admin", "worker", "driver"))
				{
					facilityRoute.GET("", dispatchHandler.GetMyFacilityDispatchRequests)
				}
			}

			// replenishmentRequests := businessRoutes.Group("/replenishment-requests")
			// {
			// 	superAdminRoute := replenishmentRequests.Group("")
			// 	superAdminRoute.Use(middleware.Authorize("superadmin"))
			// 	{
			// 		superAdminRoute.GET("", replenishmentHandler.GetAllReplenishmentRequests)
			// 	}
			// }
			
			replenishmentRequests := businessRoutes.Group("/replenishment-requests")
			{
				// Đổi "/" thành "" (chuỗi rỗng)
				superAdminRoute := replenishmentRequests.Group("/superadmin") 
				
				superAdminRoute.Use(middleware.Authorize("superadmin"))
				{
					superAdminRoute.GET("", replenishmentHandler.GetAllReplenishmentRequests)
				}
			}

			transportBids := businessRoutes.Group("/transport-bids")
			{
				adminRoute := transportBids.Group("/")
				adminRoute.Use(middleware.Authorize("admin", "superadmin"))
				{
					adminRoute.POST("", bidHandler.CreateTransportBid)
				}

				driverRoute := transportBids.Group("/")
				driverRoute.Use(middleware.Authorize("driver"))
				{
					driverRoute.GET("/mine", bidHandler.GetMyBids)
					driverRoute.POST("/:id/confirm", bidHandler.ConfirmBid)
				}
			}
		}
	}

	return router
}
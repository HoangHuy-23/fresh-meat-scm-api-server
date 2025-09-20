package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"log"
	"encoding/json"
	"fresh-meat-scm-api-server/internal/models"
	"fresh-meat-scm-api-server/internal/socket"
	"fresh-meat-scm-api-server/internal/blockchain"
	"fresh-meat-scm-api-server/config"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type DispatchHandler struct {
	DB  *mongo.Database
	Hub *socket.Hub
	Fabric *blockchain.FabricSetup
	Cfg config.Config
}

// Struct cho request body, không cần ToFacilityID
type CreateDispatchRequestPayload struct {
	Items []models.ItemInShipmentAPI `json:"items" binding:"required"`
}

// Struct cho request body của Admin
type CreateTransportBidPayload struct {
	OriginalRequestIDs []string               `json:"originalRequestIDs" binding:"required"`
	BiddingAssignments []models.BidAssignment `json:"biddingAssignments" binding:"required"` // <-- THAY ĐỔI
	ShipmentType       string                  `json:"shipmentType" binding:"required"`
	Stops              []models.BidStop       `json:"stops" binding:"required"`
}

// CreateDispatchRequest xử lý việc tạo một yêu cầu xuất hàng mới.
func (h *DispatchHandler) CreateDispatchRequest(c *gin.Context) {
	creatorEnrollmentID := c.GetString("user_enrollment_id")
	creatorFacilityID := c.GetString("user_facility_id")

	var payload CreateDispatchRequestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// (Tùy chọn) Kiểm tra xem facility của người tạo có tồn tại không
	facilityCollection := h.DB.Collection("facilities")
	count, err := facilityCollection.CountDocuments(context.Background(), bson.M{"facilityID": creatorFacilityID})
	if err != nil || count == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Originating facility does not exist."})
		return
	}

	newRequest := models.DispatchRequest{
		RequestID:      fmt.Sprintf("DREQ-%s", uuid.New().String()[:8]),
		FromFacilityID: creatorFacilityID, // Tự động lấy từ token của người tạo
		Items:          payload.Items,
		Status:         "PENDING",
		CreatedBy:      creatorEnrollmentID,
		CreatedAt:      time.Now(),
	}

	collection := h.DB.Collection("dispatch_requests") // Đổi tên collection
	result, err := collection.InsertOne(context.Background(), newRequest)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create dispatch request"})
		return
	}

	newRequest.ID = result.InsertedID.(primitive.ObjectID)

	// Gửi thông báo WebSocket đến các admin (nếu cần)

	c.JSON(http.StatusCreated, newRequest)
}

// GetAllDispatchRequests lấy danh sách các yêu cầu xuất hàng, có thể lọc theo trạng thái.
func (h *DispatchHandler) GetAllDispatchRequests(c *gin.Context) {
	// 1. Tạo một bộ lọc (filter) rỗng
	filter := bson.M{}

	// 2. Lấy query parameter "status" từ URL
	// Ví dụ: /dispatch-requests?status=PENDING
	status := c.Query("status")
	if status != "" {
		// Nếu có, thêm điều kiện lọc vào filter
		filter["status"] = status
	}

	// 3. Truy vấn collection "dispatch_requests"
	collection := h.DB.Collection("dispatch_requests")
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query dispatch requests"})
		return
	}
	defer cursor.Close(context.Background())

	// 4. Decode kết quả vào một slice
	var requests []models.DispatchRequest
	if err = cursor.All(context.Background(), &requests); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode dispatch requests"})
		return
	}

	// Đảm bảo trả về một mảng rỗng thay vì null nếu không có kết quả
	if requests == nil {
		requests = []models.DispatchRequest{}
	}

	// 5. Trả về kết quả
	c.JSON(http.StatusOK, requests)
}

// CreateTransportBid xử lý việc Admin gom nhóm và tạo gói mời thầu (VỚI TRANSACTION).
func (h *DispatchHandler) CreateTransportBid(c *gin.Context) {
	var payload CreateTransportBidPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// TODO: Thêm logic kiểm tra xem các tài xế và xe có tồn tại và hợp lệ không
	for _, assignment := range payload.BiddingAssignments {
		var driver models.User
		if err := h.DB.Collection("users").FindOne(context.Background(), bson.M{"fabricEnrollmentID": assignment.DriverID, "role": "driver"}).Decode(&driver); err != nil {
			if err == mongo.ErrNoDocuments {
				// Nếu không tìm thấy tài xế, trả về lỗi
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Driver not found: %s", assignment.DriverID)})
				return
			}
			// Nếu có lỗi khác, trả về lỗi
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check driver existence"})
			return
		}

		// Kiểm tra xem xe có tồn tại không
		var vehicle models.Vehicle
		if err := h.DB.Collection("vehicles").FindOne(context.Background(), bson.M{"vehicleID": assignment.VehicleID}).Decode(&vehicle); err != nil {
			if err == mongo.ErrNoDocuments {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Vehicle not found: %s", assignment.VehicleID)})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to check vehicle existence"})
			return
		}
	}

	// Bắt đầu một session mới với MongoDB để thực hiện transaction
	session, err := h.DB.Client().StartSession()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start database session"})
		return
	}
	defer session.EndSession(context.Background())

	// Định nghĩa logic transaction
	callback := func(sessCtx mongo.SessionContext) (interface{}, error) {
		// 1. Cập nhật trạng thái của tất cả OriginalRequestIDs thành "PROCESSED"
		dispatchCollection := h.DB.Collection("dispatch_requests")
		filter := bson.M{
			"requestID": bson.M{"$in": payload.OriginalRequestIDs},
			"status":    "PENDING",
		}
		update := bson.M{"$set": bson.M{"status": "PROCESSED"}}
		
		updateResult, err := dispatchCollection.UpdateMany(sessCtx, filter, update)
		if err != nil {
			return nil, err // Lỗi sẽ khiến transaction bị abort
		}

		// Kiểm tra xem có đúng số lượng request được cập nhật không
		if updateResult.ModifiedCount != int64(len(payload.OriginalRequestIDs)) {
			return nil, fmt.Errorf("could not process all requests. Some might have a non-PENDING status or do not exist")
		}

		// 2. Tạo TransportBid mới
		newBid := models.TransportBid{
			BidID:              fmt.Sprintf("BID-%s", uuid.New().String()[:8]),
			OriginalRequestIDs: payload.OriginalRequestIDs,
			ShipmentType:       payload.ShipmentType,
			BiddingAssignments: payload.BiddingAssignments,
			Stops:              payload.Stops,
			Status:             "BIDDING",
			CreatedAt:          time.Now(),
		}

		bidCollection := h.DB.Collection("transport_bids")
		result, err := bidCollection.InsertOne(sessCtx, newBid)
		if err != nil {
			return nil, err // Lỗi sẽ khiến transaction bị abort
		}

		// Gán lại ID đã được tạo để trả về
		newBid.ID = result.InsertedID.(primitive.ObjectID)
		return newBid, nil
	}

	// Thực thi transaction
	result, err := session.WithTransaction(context.Background(), callback)
	if err != nil {
		// Nếu có lỗi ở bất kỳ bước nào bên trong callback, transaction sẽ tự động được rollback
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Transaction failed", "details": err.Error()})
		return
	}

	// Transaction thành công, `result` chính là `newBid` đã được trả về từ callback
	createdBid := result.(models.TransportBid)

	// Gửi thông báo WebSocket đến các tài xế được mời thầu
	notification := map[string]interface{}{
		"event": "new_transport_bid",
		"bid":   createdBid,
	}
	notificationJSON, _ := json.Marshal(notification)
	for _, assignment := range payload.BiddingAssignments {
		h.Hub.Send(assignment.DriverID, notificationJSON)
	}

	c.JSON(http.StatusCreated, createdBid)
}

// GetMyBids lấy danh sách các gói thầu mà tài xế đang đăng nhập được mời.
func (h *DispatchHandler) GetMyBids(c *gin.Context) {
	driverID := c.GetString("user_enrollment_id")

	// Chỉ lấy các gói thầu đang ở trạng thái "BIDDING"
	filter := bson.M{
		"biddingDriverIDs": driverID,
		"status":           "BIDDING",
	}

	collection := h.DB.Collection("transport_bids")
	cursor, err := collection.Find(context.Background(), filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query transport bids"})
		return
	}
	defer cursor.Close(context.Background())

	var bids []models.TransportBid
	if err = cursor.All(context.Background(), &bids); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode transport bids"})
		return
	}

	if bids == nil {
		bids = []models.TransportBid{}
	}

	c.JSON(http.StatusOK, bids)
}

// ConfirmBid xử lý việc tài xế xác nhận một gói thầu đã được Admin chỉ định xe.
func (h *DispatchHandler) ConfirmBid(c *gin.Context) {
	bidID := c.Param("id")
	driverID := c.GetString("user_enrollment_id")

	bidCollection := h.DB.Collection("transport_bids")

	// --- BƯỚC 1: TÌM VÀ LẤY THÔNG TIN GÓI THẦU ---
	// Tìm một bid khớp ID, có tài xế này trong danh sách mời thầu, VÀ status là "BIDDING".
	initialFilter := bson.M{
		"bidID":  bidID,
		"status": "BIDDING",
		"biddingAssignments": bson.M{
			"$elemMatch": bson.M{"driverID": driverID},
		},
	}

	var bidToConfirm models.TransportBid
	err := bidCollection.FindOne(context.Background(), initialFilter).Decode(&bidToConfirm)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusConflict, gin.H{"error": "This bid is no longer available for confirmation."})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error checking bid availability."})
		return
	}

	// Tìm assignment của chính tài xế này để lấy vehicleID
	var confirmedAssignment models.BidAssignment
	for _, assignment := range bidToConfirm.BiddingAssignments {
		if assignment.DriverID == driverID {
			confirmedAssignment = assignment
			break
		}
	}

	// --- BƯỚC 2: LOGIC "AI NHANH HƠN" (CẬP NHẬT NGUYÊN TỬ) ---
	// Chỉ cập nhật nếu bidID và status vẫn là "BIDDING"
	atomicFilter := bson.M{"bidID": bidID, "status": "BIDDING"}
	update := bson.M{
		"$set": bson.M{
			"status":             "CONFIRMED",
			"confirmedAssignment": confirmedAssignment,
			"confirmedAt":        time.Now(),
		},
	}

	updateResult, err := bidCollection.UpdateOne(context.Background(), atomicFilter, update)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error during bid confirmation."})
		return
	}
	if updateResult.ModifiedCount == 0 {
		// Nếu không có document nào được sửa, nghĩa là ai đó đã nhanh tay hơn
		c.JSON(http.StatusConflict, gin.H{"error": "This bid was confirmed by another driver just now."})
		return
	}

	// --- BƯỚC 3: THÔNG BÁO CHO CÁC TÀI XẾ KHÁC ---
	notification := map[string]string{
		"event": "bid_confirmed_by_other",
		"bidID": bidID,
	}
	notificationJSON, _ := json.Marshal(notification)
	for _, assignment := range bidToConfirm.BiddingAssignments {
		if assignment.DriverID != driverID {
			h.Hub.Send(assignment.DriverID, notificationJSON)
		}
	}

	// --- BƯỚC 4: THU THẬP DỮ LIỆU ĐỂ TẠO SHIPMENT ---
	vehicleID := confirmedAssignment.VehicleID

	// Lấy thông tin xe và kiểm tra
	var vehicle models.Vehicle
	vehicleCollection := h.DB.Collection("vehicles")
	err = vehicleCollection.FindOne(context.Background(), bson.M{"vehicleID": vehicleID, "ownerDriverID": driverID, "status": "AVAILABLE"}).Decode(&vehicle)
	if err != nil {
		log.Printf("CRITICAL: Bid %s confirmed but vehicle %s is invalid. Rolling back...", bidID, vehicleID)
		bidCollection.UpdateOne(context.Background(), bson.M{"bidID": bidID}, bson.M{"$set": bson.M{"status": "BIDDING", "confirmedAssignment": nil, "confirmedAt": nil}})
		c.JSON(http.StatusConflict, gin.H{"error": "The assigned vehicle is not available or not owned by you."})
		return
	}

	// Lấy thông tin profile tài xế
	var driverProfile models.User
	userCollection := h.DB.Collection("users")
	err = userCollection.FindOne(context.Background(), bson.M{"fabricEnrollmentID": driverID}).Decode(&driverProfile)
	if err != nil {
		// Rollback...
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not retrieve driver profile."})
		return
	}

	// "Làm giàu" lại thông tin các điểm dừng
	enrichedStops := []StopInJourneyChaincode{} // Tái sử dụng struct từ shipment_handler
	facilityCollection := h.DB.Collection("facilities")
	for _, stop := range bidToConfirm.Stops {
		var facility models.Facility
		err := facilityCollection.FindOne(context.Background(), bson.M{"facilityID": stop.FacilityID}).Decode(&facility)
		if err != nil {
			// Rollback...
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to enrich facility data for %s", stop.FacilityID)})
			return
		}

		enrichedStops = append(enrichedStops, StopInJourneyChaincode{
			FacilityID:      facility.FacilityID,
			FacilityName:    facility.Name,
			FacilityAddress: facility.Address,
			Action:          stop.Action,
			Items:           stop.Items,
		})
	}

	// --- BƯỚC 5: TẠO SHIPMENT ON-CHAIN ---
	shipmentID := fmt.Sprintf("SHIP-%s", bidToConfirm.BidID)
	shipmentType := bidToConfirm.ShipmentType
	driverName := driverProfile.Name
	vehiclePlate := vehicle.PlateNumber
	stopsJSON, _ := json.Marshal(enrichedStops)

	driverGateway, err := h.Fabric.GetGatewayForUser(driverID)
	if err != nil {
		// Rollback...
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create driver gateway for shipment creation"})
		return
	}
	defer driverGateway.Close()
	network, _ := driverGateway.GetNetwork(h.Cfg.Fabric.ChannelName)
	contract := network.GetContract(h.Cfg.Fabric.ChaincodeName)

	_, err = contract.SubmitTransaction(
		"CreateShipment",
		shipmentID,
		shipmentType,
		driverID,
		driverName,
		vehiclePlate,
		string(stopsJSON),
	)
	if err != nil {
		log.Printf("CRITICAL: Bid %s confirmed but FAILED to create on-chain shipment. Rolling back...", bidID)
		bidCollection.UpdateOne(context.Background(), bson.M{"bidID": bidID}, bson.M{"$set": bson.M{"status": "BIDDING", "confirmedAssignment": nil, "confirmedAt": nil}})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create on-chain shipment", "details": err.Error()})
		return
	}

	// --- BƯỚC 6: CẬP NHẬT TRẠNG THÁI OFF-CHAIN CUỐI CÙNG ---
	// Cập nhật trạng thái xe
	_, err = vehicleCollection.UpdateOne(context.Background(), bson.M{"vehicleID": vehicle.VehicleID}, bson.M{"$set": bson.M{"status": "IN_TRIP"}})
	if err != nil {
		log.Printf("CRITICAL: Failed to update vehicle status for %s. Please check manually.", vehicle.VehicleID)
	}

	// Cập nhật trạng thái bid
	_, err = bidCollection.UpdateOne(context.Background(), bson.M{"bidID": bidID}, bson.M{"$set": bson.M{"status": "COMPLETED", "shipmentID": shipmentID}})
	if err != nil {
		log.Printf("CRITICAL: On-chain shipment %s was created but failed to update off-chain bid %s. Please check manually.", shipmentID, bidID)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     "success",
		"message":    "You have successfully confirmed the bid. A shipment has been created.",
		"shipmentID": shipmentID,
	})
}
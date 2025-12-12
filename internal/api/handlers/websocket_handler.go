// server/internal/api/handlers/websocket_handler.go
package handlers

import (
	"log"
	"net/http"
	"fresh-meat-scm-api-server/internal/auth"
	"fresh-meat-scm-api-server/internal/socket" // <-- Dùng tên package mới

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket" // <-- Giữ nguyên tên này cho thư viện gorilla
	"time"
)

// Thời gian chờ tối đa cho một tin nhắn từ client.
const pongWait = 30 * time.Second

var upgrader = websocket.Upgrader{ // <-- Phải là websocket.Upgrader của gorilla
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WebSocketHandler struct {
	Hub *socket.Hub // <-- Dùng kiểu dữ liệu từ package internal/socket
}

// ServeWs xử lý các yêu cầu kết nối WebSocket.
func (h *WebSocketHandler) ServeWs(c *gin.Context) {
	tokenString := c.Query("token")
	if tokenString == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Token is required"})
		return
	}

	claims := &auth.JWTClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return auth.JwtSecret, nil
	})

	if err != nil || !token.Valid {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
		return
	}
	userID := claims.FabricEnrollmentID

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	h.Hub.Register(userID, conn)

	defer func() {
		h.Hub.Unregister(userID)
		conn.Close()
	}()

	// BƯỚC 1: Thiết lập thời gian chờ đọc ban đầu
	conn.SetReadDeadline(time.Now().Add(pongWait))

	// Khởi chạy Vòng Lặp Đọc (Read Loop)
	for {
		// Đọc một tin nhắn bất kỳ từ client
		msgType, msg, err := conn.ReadMessage()
		if err != nil {
			// Thêm CloseNormalClosure vào danh sách các lỗi được mong đợi để không log ra khi client chủ động đóng
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure) {
				log.Printf("An actual unexpected close error occurred: %v", err)
			}
			break
		}

		// BƯỚC 2: ĐÂY LÀ DÒNG QUAN TRỌNG NHẤT
		// Reset lại deadline mỗi khi chúng ta nhận được một tin nhắn thành công.
		// Điều này báo cho server biết client vẫn còn "sống".
		conn.SetReadDeadline(time.Now().Add(pongWait))

		// BƯỚC 3: Xử lý tin nhắn heartbeat từ client
		// Nếu đó là tin nhắn "ping", chúng ta chỉ cần bỏ qua và tiếp tục vòng lặp.
		if msgType == websocket.TextMessage && string(msg) == "\"ping\"" {
			continue 
		}

		// Nếu không phải là tin nhắn ping, hãy xử lý nó (nếu cần)
		// Ví dụ: log.Printf("Received data message from %s: %s", userID, string(msg))
	}
}
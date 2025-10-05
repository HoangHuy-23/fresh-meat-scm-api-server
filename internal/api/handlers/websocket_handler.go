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

	// 4. Thiết lập cơ chế xử lý Pong (Heartbeat)
	// Đặt thời gian chờ tối đa để nhận một tin nhắn pong từ client.
	conn.SetReadDeadline(time.Now().Add(pongWait))
	// Khi nhận được một tin nhắn pong, hãy reset lại thời gian chờ.
	// Khi nhận được một tin nhắn PING từ client, chúng ta reset lại deadline.
	// Thư viện gorilla/websocket sẽ tự động gửi lại PONG.
	conn.SetPingHandler(func(string) error {
		log.Printf("Received PING from %s, extending deadline", userID)
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Khởi chạy Vòng Lặp Đọc (Read Loop)
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Unexpected close error: %v", err)
			}
			break
		}
	}
}
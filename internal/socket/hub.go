// server/internal/websocket/hub.go
package socket

import (
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Hub quản lý tất cả các client WebSocket.
type Hub struct {
	// clients là một map để lưu trữ các kết nối, key là enrollmentID của user.
	clients map[string]*websocket.Conn
	// mu là một Mutex để đảm bảo an toàn khi truy cập map clients từ nhiều goroutine.
	mu sync.RWMutex
}

// NewHub tạo một Hub mới.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]*websocket.Conn),
	}
}

// Register thêm một client mới vào Hub.
func (h *Hub) Register(userID string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[userID] = conn
	log.Printf("WebSocket client registered: %s", userID)
}

// Unregister xóa một client khỏi Hub.
func (h *Hub) Unregister(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[userID]; ok {
		delete(h.clients, userID)
		log.Printf("WebSocket client unregistered: %s", userID)
	}
}

// Send gửi một tin nhắn đến một client cụ thể.
func (h *Hub) Send(userID string, message []byte) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	conn, ok := h.clients[userID]
	if !ok {
		// Không tìm thấy client (có thể đã offline), không coi đây là lỗi nghiêm trọng.
		log.Printf("WebSocket client not found, could not send message: %s", userID)
		return nil
	}

	// Gửi tin nhắn JSON
	return conn.WriteMessage(websocket.TextMessage, message)
}
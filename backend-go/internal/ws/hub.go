// backend-go/internal/ws/hub.go
package ws

import (
	"encoding/json"
	"sync"

	"github.com/gofiber/websocket/v2"
)

type ClientInfo struct {
	UserID    string
	SessionID string
}

// Extracting this into a named struct fixes the IDE parsing errors
type RegisterPayload struct {
	Conn *websocket.Conn
	Info *ClientInfo
}

type Hub struct {
	Clients    map[*websocket.Conn]*ClientInfo
	Register   chan RegisterPayload
	Unregister chan *websocket.Conn
	mu         sync.Mutex
}

func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[*websocket.Conn]*ClientInfo),
		Register:   make(chan RegisterPayload), // Clean and readable
		Unregister: make(chan *websocket.Conn),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case reg := <-h.Register:
			h.mu.Lock()
			h.Clients[reg.Conn] = reg.Info
			h.mu.Unlock()
		case client := <-h.Unregister:
			h.mu.Lock()
			if _, ok := h.Clients[client]; ok {
				delete(h.Clients, client)
				client.Close()
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) BroadcastTargeted(userID, sessionID, msgType string, payload interface{}) {
	data, _ := json.Marshal(map[string]interface{}{
		"type":    msgType,
		"payload": payload,
	})

	h.mu.Lock()
	defer h.mu.Unlock()

	for client, info := range h.Clients {
		if info.UserID == userID && info.SessionID == sessionID {
			if err := client.WriteMessage(websocket.TextMessage, data); err != nil {
				client.Close()
				delete(h.Clients, client)
			}
		}
	}
}
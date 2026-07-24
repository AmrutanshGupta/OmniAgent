package ws

import (
	"encoding/json"
	"sync"

	"github.com/gofiber/contrib/websocket"
)

type Event struct {
	Type string      `json:"type"` // "task_update" | "cost_event" | "plan_ready"
	Data interface{} `json:"data"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[*websocket.Conn]bool
}

func NewHub() *Hub {
	return &Hub{clients: make(map[*websocket.Conn]bool)}
}

func (h *Hub) Register(c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = true
}

func (h *Hub) Unregister(c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
}

func (h *Hub) Broadcast(evt Event) {
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		_ = c.WriteMessage(websocket.TextMessage, data)
	}
}

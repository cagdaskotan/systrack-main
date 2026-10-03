package ws

import (
	"encoding/json"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type Hub struct {
	clients    map[*Client]bool
	Register   chan *Client
	Unregister chan *Client
	broadcast  chan []byte
}

type Client struct {
	Hub  *Hub
	Conn *websocket.Conn
	Send chan []byte
}

type Message struct {
	Type     string      `json:"type"`
	Data     interface{} `json:"data"`
	TargetID int         `json:"targetId,omitempty"`
}

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		broadcast:  make(chan []byte),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case client := <-h.Register:
			h.clients[client] = true
			log.Printf("WebSocket client connected. Total clients: %d", len(h.clients))

		case client := <-h.Unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				log.Printf("WebSocket client disconnected. Total clients: %d", len(h.clients))
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.Send <- message:
				default:
					close(client.Send)
					delete(h.clients, client)
				}
			}
		}
	}
}

func (h *Hub) BroadcastStatusUpdate(targetID int, status string) {
	message := Message{
		Type:     "status_update",
		TargetID: targetID,
		Data:     map[string]string{"status": status},
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal status update: %v", err)
		return
	}

	log.Printf("📡 Broadcasting status update: targetID=%d status=%s clients=%d", targetID, status, len(h.clients))
	h.broadcast <- data
}

func (h *Hub) BroadcastPingResult(targetID int, responseTime int, success bool) {
	message := Message{
		Type:     "ping_result",
		TargetID: targetID,
		Data: map[string]interface{}{
			"responseTime": responseTime,
			"success":      success,
			"timestamp":    time.Now().Format("15:04:05"),
		},
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal ping result: %v", err)
		return
	}

	h.broadcast <- data
}

func (h *Hub) BroadcastDashboardStats(stats map[string]interface{}) {
	message := Message{
		Type: "dashboard_stats",
		Data: stats,
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal dashboard stats: %v", err)
		return
	}

	h.broadcast <- data
}

func (h *Hub) BroadcastAlert(alertType string, targetID int, data interface{}) {
	message := Message{
		Type:     alertType,
		TargetID: targetID,
		Data:     data,
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal alert: %v", err)
		return
	}

	h.broadcast <- jsonData
}

func (h *Hub) BroadcastServerStats(stats interface{}) {
	message := Message{
		Type: "server_stats",
		Data: stats,
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal server stats: %v", err)
		return
	}

	h.broadcast <- data
}

// BroadcastSensorData broadcasts live sensor (ESP32/MQTT) data to all connected clients
func (h *Hub) BroadcastSensorData(data interface{}) {
	message := Message{
		Type: "sensor_data",
		Data: data,
	}

	jsonData, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal sensor data: %v", err)
		return
	}

	h.broadcast <- jsonData
}

// BroadcastDeviceMetrics broadcasts device metrics update to all connected clients
func (h *Hub) BroadcastDeviceMetrics(metrics interface{}) {
	message := Message{
		Type: "device_metrics",
		Data: metrics,
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("Failed to marshal device metrics: %v", err)
		return
	}

	h.broadcast <- data
}

func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister <- c
		c.Conn.Close()
	}()

	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.Conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}
	}
}

func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.Conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Buffered mesajları gönder
			n := len(c.Send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.Send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

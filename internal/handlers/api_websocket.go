package handlers

import (
	"net/http"

	"systrack/internal/auth"
	"systrack/internal/ws"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func WebSocketHandler(hub *ws.Hub, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// WebSocket upgrade
		upgrader := websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to upgrade to WebSocket"})
			return
		}

		// JWT token kontrolü (query parameter olarak)
		token := c.Query("token")
		if token == "" {
			conn.Close()
			return
		}

		// Token doğrulama
		_, err = auth.ValidateToken(token, jwtSecret)
		if err != nil {
			conn.Close()
			return
		}

		// Client oluştur
		client := &ws.Client{
			Hub:  hub,
			Conn: conn,
			Send: make(chan []byte, 256),
		}

		// Client'ı hub'a kaydet
		hub.Register <- client

		// Goroutine'leri başlat
		go client.WritePump()
		go client.ReadPump()
	}
}

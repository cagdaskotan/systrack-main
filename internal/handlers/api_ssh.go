package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var sshUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // iç ağ için serbest bırakıldı
	},
}

type SSHSession struct {
	ID         string
	TargetID   int
	TargetAddr string
	Username   string
	Cmd        *exec.Cmd          // Linux'ta dolu, Windows'ta genelde nil (conpty kullanılır)
	Term       io.ReadWriteCloser // PTY veya ConPTY burada birleşir
	WS         *websocket.Conn
	Mutex      sync.Mutex
	CreatedAt  time.Time
	LastActive time.Time
}

var (
	sshSessions = make(map[string]*SSHSession)
	sessionsMux sync.RWMutex
)

// Ortak WS handler — OS'e özel startSSHSession çağrılır
func SSHWebSocketHandler(db *sql.DB, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Token al
		token := c.Query("token")
		if token == "" {
			token = c.GetHeader("Authorization")
			if len(token) > 7 && token[:7] == "Bearer " {
				token = token[7:]
			}
		}
		if len(token) < 10 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Geçersiz veya eksik token"})
			return
		}

		// Parametreler
		targetIDStr := c.Query("target_id")
		username := c.Query("username")
		if targetIDStr == "" || username == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "target_id ve username gerekli"})
			return
		}
		targetID, err := strconv.Atoi(targetIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz target_id"})
			return
		}

		// Hedef IP/host'u al
		var targetAddr string
		if err := db.QueryRow("SELECT address FROM targets WHERE id = ?", targetID).Scan(&targetAddr); err != nil {
			log.Printf("Target bulunamadı: %v", err)
			c.JSON(http.StatusNotFound, gin.H{"error": "Hedef bulunamadı"})
			return
		}

		// WebSocket upgrade
		ws, err := sshUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("WebSocket upgrade hatası: %v", err)
			return
		}
		defer ws.Close()

		// OS'e özel başlatma
		session, err := startSSHSession(targetID, targetAddr, username, ws)
		if err != nil {
			log.Printf("SSH oturumu başlatılamadı: %v", err)
			_ = ws.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\n\033[31mHata: %v\033[0m\r\n", err)))
			return
		}
		defer cleanupSession(session)

		// Kaydet
		sessionsMux.Lock()
		sshSessions[session.ID] = session
		sessionsMux.Unlock()

		// Hoş geldin
		_ = ws.WriteMessage(websocket.TextMessage,
			[]byte(fmt.Sprintf("\033[32mSSH bağlantısı kuruluyor: %s@%s\033[0m\r\n\r\n", username, targetAddr)))

		// Term'den geleni WS'e aktar
		go func() {
			buf := make([]byte, 4096)
			for {
				n, err := session.Term.Read(buf)
				if err != nil {
					if err != io.EOF {
						log.Printf("Terminal okuma hatası: %v", err)
					}
					return
				}
				session.Mutex.Lock()
				session.LastActive = time.Now()
				werr := ws.WriteMessage(websocket.BinaryMessage, buf[:n])
				session.Mutex.Unlock()
				if werr != nil {
					log.Printf("WebSocket yazma hatası: %v", werr)
					return
				}
			}
		}()

		// WS'den geleni Term'e yaz
		for {
			_, msg, err := ws.ReadMessage()
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket bağlantısı kapandı: %v", err)
				}
				break
			}
			session.Mutex.Lock()
			session.LastActive = time.Now()
			_, werr := session.Term.Write(msg)
			session.Mutex.Unlock()
			if werr != nil {
				log.Printf("Terminal yazma hatası: %v", werr)
				break
			}
		}
	}
}

// Temizlik
func cleanupSession(session *SSHSession) {
	sessionsMux.Lock()
	delete(sshSessions, session.ID)
	sessionsMux.Unlock()

	if session.Term != nil {
		_ = session.Term.Close()
	}
	if session.Cmd != nil && session.Cmd.Process != nil {
		_ = session.Cmd.Process.Kill()
		_, _ = session.Cmd.Process.Wait()
	}
	log.Printf("SSH oturumu sonlandırıldı: %s", session.ID)
}

// Zaman aşımı temizliği
func init() {
	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			sessionsMux.Lock()
			for id, s := range sshSessions {
				if time.Since(s.LastActive) > 5*time.Minute {
					log.Printf("Timeout: SSH oturumu kapatılıyor: %s", id)
					cleanupSession(s)
				}
			}
			sessionsMux.Unlock()
		}
	}()
}

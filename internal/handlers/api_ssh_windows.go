//go:build windows

package handlers

import (
	"fmt"
	"time"

	"github.com/UserExistsError/conpty"
	"github.com/gorilla/websocket"
)

func startSSHSession(targetID int, targetAddr, username string, ws *websocket.Conn) (*SSHSession, error) {
	cmdLine := fmt.Sprintf(`ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=10 -tt %s@%s`, username, targetAddr)

	cpty, err := conpty.Start(cmdLine)
	if err != nil {
		return nil, fmt.Errorf("ConPTY başlatılamadı: %w", err)
	}

	session := &SSHSession{
		ID:         fmt.Sprintf("%d-%s-%d", targetID, username, time.Now().Unix()),
		TargetID:   targetID,
		TargetAddr: targetAddr,
		Username:   username,
		Cmd:        nil,  // ConPTY'de exec.Cmd yok
		Term:       cpty, // io.ReadWriteCloser
		WS:         ws,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}
	return session, nil
}

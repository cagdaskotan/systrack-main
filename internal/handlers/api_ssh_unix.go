//go:build linux || darwin

package handlers

import (
	"fmt"
	"os" // <-- eklendi (ENV için)
	"os/exec"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

func startSSHSession(targetID int, targetAddr, username string, ws *websocket.Conn) (*SSHSession, error) {
	sshArgs := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		"-o", "NumberOfPasswordPrompts=1",
		"-o", "PreferredAuthentications=keyboard-interactive,password",
		"-o", "PubkeyAuthentication=no",
		"-tt",
		fmt.Sprintf("%s@%s", username, targetAddr),
	}

	cmd := exec.Command("ssh", sshArgs...)

	// Prompt görünürlüğü için TERM ve locale ver
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"LC_ALL=C",
		"LANG=C",
	)

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("PTY başlatılamadı: %w", err)
	}

	_ = pty.Setsize(ptmx, &pty.Winsize{Rows: 32, Cols: 120})
	_, _ = ptmx.Write([]byte("\r"))

	return &SSHSession{
		ID:         fmt.Sprintf("%d-%s-%d", targetID, username, time.Now().Unix()),
		TargetID:   targetID,
		TargetAddr: targetAddr,
		Username:   username,
		Cmd:        cmd,
		Term:       ptmx, // <<< BURASI: PTY yerine Term
		WS:         ws,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
	}, nil
}

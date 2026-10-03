package remote_access

import (
	"fmt"
	"log"
	"os/exec"
	"sync"
)

// Config represents remote access tunnel configuration
type Config struct {
	Enabled     bool
	TunnelHost  string
	SSHUser     string
	SSHPassword string
	UIPort      int
	DBPort      *int
}

// Manager manages SSH reverse tunnel for remote access
type Manager struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	active      bool
	config      Config
	stopChan    chan struct{}
	stoppedChan chan struct{}
}

// NewManager creates a new remote access manager
func NewManager() *Manager {
	return &Manager{
		stopChan:    make(chan struct{}),
		stoppedChan: make(chan struct{}),
	}
}

// Start opens SSH reverse tunnel with given configuration
func (m *Manager) Start(config Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Already running with same config?
	if m.active && m.configEquals(config) {
		log.Printf("🔗 Remote access already active (port=%d)", config.UIPort)
		return nil
	}

	// Stop existing tunnel if config changed
	if m.active {
		log.Printf("🔄 Remote access config changed, restarting tunnel...")
		m.stopInternal()
	}

	// Validate config
	if !config.Enabled {
		return fmt.Errorf("remote access not enabled")
	}
	if config.TunnelHost == "" || config.SSHUser == "" || config.SSHPassword == "" {
		return fmt.Errorf("incomplete tunnel configuration")
	}
	if config.UIPort < 1024 || config.UIPort > 65535 {
		return fmt.Errorf("invalid UI port: %d", config.UIPort)
	}

	log.Printf("🔗 Starting remote access tunnel...")
	log.Printf("   Host: %s", config.TunnelHost)
	log.Printf("   User: %s", config.SSHUser)
	log.Printf("   UI Port: %d", config.UIPort)
	if config.DBPort != nil {
		log.Printf("   DB Port: %d", *config.DBPort)
	}

	// Build SSH command with sshpass
	args := []string{
		"-N", // No remote command
		"-o", "StrictHostKeyChecking=no", // Skip host key verification (MITM koruması ekleyeceğiz)
		"-o", "ServerAliveInterval=30",   // Keep-alive every 30 seconds
		"-o", "ServerAliveCountMax=3",    // Max 3 failed keep-alives before disconnect
		"-o", "ExitOnForwardFailure=yes", // Exit if port forwarding fails
		"-R", fmt.Sprintf("0.0.0.0:%d:localhost:8080", config.UIPort), // UI tunnel
	}

	// Add DB tunnel if specified
	if config.DBPort != nil && *config.DBPort >= 1024 && *config.DBPort <= 65535 {
		args = append(args, "-R", fmt.Sprintf("0.0.0.0:%d:localhost:8081", *config.DBPort))
	}

	// SSH destination
	args = append(args, fmt.Sprintf("%s@%s", config.SSHUser, config.TunnelHost))

	// Use sshpass to provide password
	cmd := exec.Command("sshpass", append([]string{"-p", config.SSHPassword, "ssh"}, args...)...)

	// Start the tunnel
	if err := cmd.Start(); err != nil {
		log.Printf("❌ Failed to start SSH tunnel: %v", err)
		return fmt.Errorf("failed to start tunnel: %w", err)
	}

	m.cmd = cmd
	m.config = config
	m.active = true

	// Monitor tunnel in background
	go m.monitorTunnel()

	log.Printf("✅ Remote access tunnel started successfully")
	return nil
}

// Stop closes the SSH tunnel
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.active {
		return nil
	}

	log.Printf("🔒 Stopping remote access tunnel...")
	m.stopInternal()
	log.Printf("✅ Remote access tunnel stopped")
	return nil
}

// stopInternal stops tunnel without locking (called from Start)
func (m *Manager) stopInternal() {
	if m.cmd != nil && m.cmd.Process != nil {
		close(m.stopChan)
		m.cmd.Process.Kill()
		<-m.stoppedChan // Wait for monitor goroutine to exit
		m.cmd = nil
		m.stopChan = make(chan struct{})
		m.stoppedChan = make(chan struct{})
	}
	m.active = false
}

// IsActive returns true if tunnel is currently active
func (m *Manager) IsActive() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

// GetConfig returns current tunnel configuration
func (m *Manager) GetConfig() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.config
}

// monitorTunnel watches SSH process and logs errors
func (m *Manager) monitorTunnel() {
	defer close(m.stoppedChan)

	if m.cmd == nil {
		return
	}

	// Wait for SSH process to exit
	err := m.cmd.Wait()

	select {
	case <-m.stopChan:
		// Tunnel was explicitly stopped
		return
	default:
		// Tunnel died unexpectedly
		m.mu.Lock()
		m.active = false
		m.mu.Unlock()

		if err != nil {
			log.Printf("⚠️  SSH tunnel died unexpectedly: %v", err)
			log.Printf("   Tunnel will restart on next heartbeat if still requested")
		} else {
			log.Printf("⚠️  SSH tunnel closed unexpectedly (exit code 0)")
		}
	}
}

// configEquals checks if two configs are equal
func (m *Manager) configEquals(other Config) bool {
	if m.config.Enabled != other.Enabled {
		return false
	}
	if m.config.TunnelHost != other.TunnelHost {
		return false
	}
	if m.config.SSHUser != other.SSHUser {
		return false
	}
	if m.config.SSHPassword != other.SSHPassword {
		return false
	}
	if m.config.UIPort != other.UIPort {
		return false
	}

	// Compare DBPort (handle nil cases)
	if (m.config.DBPort == nil) != (other.DBPort == nil) {
		return false
	}
	if m.config.DBPort != nil && other.DBPort != nil && *m.config.DBPort != *other.DBPort {
		return false
	}

	return true
}

// SetupVPSFingerprint adds VPS host key to known_hosts (MITM protection)
func SetupVPSFingerprint(tunnelHost string) error {
	log.Printf("🔐 Setting up VPS fingerprint for MITM protection...")

	// Run ssh-keyscan to get VPS host key
	cmd := exec.Command("ssh-keyscan", "-H", tunnelHost)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to scan VPS host key: %w", err)
	}

	if len(output) == 0 {
		return fmt.Errorf("no host key received from VPS")
	}

	// Append to known_hosts
	knownHostsPath := "/home/systrack/.ssh/known_hosts"
	appendCmd := exec.Command("sh", "-c", fmt.Sprintf("mkdir -p /home/systrack/.ssh && echo '%s' >> %s", string(output), knownHostsPath))
	if err := appendCmd.Run(); err != nil {
		return fmt.Errorf("failed to add VPS fingerprint to known_hosts: %w", err)
	}

	log.Printf("✅ VPS fingerprint added to known_hosts")
	log.Printf("   Future connections will verify host identity (MITM protection)")
	return nil
}

// EnableStrictHostKeyChecking switches to strict mode after fingerprint setup
func (m *Manager) EnableStrictHostKeyChecking() {
	// Bu fonksiyon future use için - şimdilik StrictHostKeyChecking=no kullanıyoruz
	// İlk setup'ta SetupVPSFingerprint() çağrıldıktan sonra bu fonksiyonu çağırarak
	// strict mode'a geçebiliriz
}

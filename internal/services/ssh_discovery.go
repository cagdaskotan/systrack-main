package services

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHDiscovery struct {
}

func NewSSHDiscovery() *SSHDiscovery {
	return &SSHDiscovery{}
}

// DiscoverLinuxServices - SSH ile Linux/Unix sunucusundaki servisleri keşfeder
func (s *SSHDiscovery) DiscoverLinuxServices(ctx context.Context, targetAddress, username, password string) ([]DiscoveredService, error) {
	var discovered []DiscoveredService

	// SSH client config
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Production'da proper host key verification gerekir
		Timeout:         10 * time.Second,
	}

	// SSH bağlantısı kur (port 22)
	addr := targetAddress
	if !strings.Contains(addr, ":") {
		addr = addr + ":22"
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH connection failed: %v", err)
	}
	defer client.Close()

	// Systemd servisleri için systemctl kullan
	systemdServices, err := s.getSystemdServices(ctx, client)
	if err == nil && len(systemdServices) > 0 {
		discovered = append(discovered, systemdServices...)
	}

	// SysVinit servisleri için service --status-all (fallback)
	if len(discovered) == 0 {
		sysvinitServices, err := s.getSysVInitServices(ctx, client)
		if err == nil {
			discovered = append(discovered, sysvinitServices...)
		}
	}

	// Docker container'ları da ekleyelim
	dockerContainers, err := s.getDockerContainers(ctx, client)
	if err == nil {
		discovered = append(discovered, dockerContainers...)
	}

	log.Printf("SSH discovered %d services on %s", len(discovered), targetAddress)
	return discovered, nil
}

// getSystemdServices - systemctl kullanarak aktif servisleri listeler
func (s *SSHDiscovery) getSystemdServices(ctx context.Context, client *ssh.Client) ([]DiscoveredService, error) {
	var services []DiscoveredService

	// systemctl list-units --type=service --state=running --no-pager
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	output, err := session.CombinedOutput("systemctl list-units --type=service --state=running --no-pager --no-legend")
	if err != nil {
		return nil, fmt.Errorf("systemctl failed: %v", err)
	}

	// Parse output
	// Format: service-name.service    loaded active running   Description
	lines := strings.Split(string(output), "\n")
	serviceRegex := regexp.MustCompile(`^\s*(\S+\.service)\s+loaded\s+active\s+running\s+(.+)$`)

	for _, line := range lines {
		matches := serviceRegex.FindStringSubmatch(line)
		if len(matches) >= 3 {
			serviceName := strings.TrimSuffix(matches[1], ".service")
			description := strings.TrimSpace(matches[2])

			// Yaygın servisleri filtrele
			if s.isRelevantService(serviceName) {
				services = append(services, DiscoveredService{
					ServiceName: fmt.Sprintf("%s (%s)", serviceName, description),
					Port:        0, // Port bilgisi için netstat/ss gerekir
					Protocol:    "systemd",
					Method:      "ssh",
				})
			}
		}
	}

	return services, nil
}

// getSysVInitServices - SysVinit servisleri için fallback
func (s *SSHDiscovery) getSysVInitServices(ctx context.Context, client *ssh.Client) ([]DiscoveredService, error) {
	var services []DiscoveredService

	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	output, err := session.CombinedOutput("service --status-all 2>/dev/null | grep '+' ")
	if err != nil {
		return nil, fmt.Errorf("service --status-all failed: %v", err)
	}

	// Parse output
	// Format:  [ + ]  servicename
	lines := strings.Split(string(output), "\n")
	serviceRegex := regexp.MustCompile(`\[\s*\+\s*\]\s+(\S+)`)

	for _, line := range lines {
		matches := serviceRegex.FindStringSubmatch(line)
		if len(matches) >= 2 {
			serviceName := matches[1]

			if s.isRelevantService(serviceName) {
				services = append(services, DiscoveredService{
					ServiceName: serviceName,
					Port:        0,
					Protocol:    "sysvinit",
					Method:      "ssh",
				})
			}
		}
	}

	return services, nil
}

// getDockerContainers - Çalışan Docker container'ları listeler
func (s *SSHDiscovery) getDockerContainers(ctx context.Context, client *ssh.Client) ([]DiscoveredService, error) {
	var services []DiscoveredService

	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	output, err := session.CombinedOutput("docker ps --format '{{.Names}}|{{.Image}}|{{.Status}}' 2>/dev/null")
	if err != nil {
		// Docker yüklü değil veya çalışmıyor, normal
		return services, nil
	}

	// Parse output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) >= 2 {
			containerName := parts[0]
			imageName := parts[1]

			services = append(services, DiscoveredService{
				ServiceName: fmt.Sprintf("Docker: %s (%s)", containerName, imageName),
				Port:        0,
				Protocol:    "docker",
				Method:      "ssh",
			})
		}
	}

	return services, nil
}

// isRelevantService - Sadece önemli servisleri filtreler (tüm systemd servislerini döndürmemek için)
func (s *SSHDiscovery) isRelevantService(serviceName string) bool {
	relevantKeywords := []string{
		"nginx", "apache", "httpd", "mysql", "mariadb", "postgres", "postgresql",
		"redis", "mongodb", "mongod", "docker", "ssh", "sshd", "firewall", "ufw",
		"iptables", "fail2ban", "cron", "tomcat", "jenkins", "gitlab", "grafana",
		"prometheus", "elasticsearch", "kibana", "rabbitmq", "kafka", "zookeeper",
		"haproxy", "traefik", "caddy", "php-fpm", "ftp", "vsftpd", "proftpd",
	}

	lowerService := strings.ToLower(serviceName)
	for _, keyword := range relevantKeywords {
		if strings.Contains(lowerService, keyword) {
			return true
		}
	}

	return false
}

// CheckLinuxService - Belirli bir servisin durumunu kontrol eder
func (s *SSHDiscovery) CheckLinuxService(ctx context.Context, targetAddress, username, password, serviceName string) (string, error) {
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	addr := targetAddress
	if !strings.Contains(addr, ":") {
		addr = addr + ":22"
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return "unknown", fmt.Errorf("SSH connection failed: %v", err)
	}
	defer client.Close()

	// systemctl is-active servicename
	session, err := client.NewSession()
	if err != nil {
		return "unknown", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(fmt.Sprintf("systemctl is-active %s 2>/dev/null", serviceName))
	if err != nil {
		// Fallback: service status
		session2, err2 := client.NewSession()
		if err2 != nil {
			return "unknown", err2
		}
		defer session2.Close()

		output2, err2 := session2.CombinedOutput(fmt.Sprintf("service %s status 2>/dev/null", serviceName))
		if err2 != nil {
			return "unknown", fmt.Errorf("service check failed: %v", err)
		}

		// Parse service status output (running kelimesini ara)
		if strings.Contains(strings.ToLower(string(output2)), "running") {
			return "online", nil
		}
		return "offline", nil
	}

	state := strings.TrimSpace(string(output))
	switch state {
	case "active":
		return "online", nil
	case "inactive", "failed":
		return "offline", nil
	default:
		return "unknown", nil
	}
}

// QuickSSHCheck - Hızlı SSH bağlantı testi (credential doğrulama)
func (s *SSHDiscovery) QuickSSHCheck(ctx context.Context, targetAddress, username, password string) error {
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	addr := targetAddress
	if !strings.Contains(addr, ":") {
		addr = addr + ":22"
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %v", err)
	}
	defer client.Close()

	// Simple command to verify connection
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("SSH session failed: %v", err)
	}
	defer session.Close()

	if err := session.Run("echo OK"); err != nil {
		return fmt.Errorf("SSH test command failed: %v", err)
	}

	return nil
}

// GetAllLinuxServices - Tüm Linux servislerini döndürür (LinuxService struct olarak)
func (s *SSHDiscovery) GetAllLinuxServices(ctx context.Context, targetAddress, username, password string) ([]LinuxService, error) {
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := targetAddress
	if !strings.Contains(addr, ":") {
		addr = addr + ":22"
	}

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("SSH connection failed: %v", err)
	}
	defer client.Close()

	// Get all systemd services (not just running ones)
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	// List all services with their status
	output, err := session.CombinedOutput("systemctl list-units --type=service --all --no-pager --no-legend --plain")
	if err != nil {
		return nil, fmt.Errorf("systemctl failed: %v", err)
	}

	return s.parseAllLinuxServices(string(output)), nil
}

// parseAllLinuxServices - Tüm servisleri parse eder
func (s *SSHDiscovery) parseAllLinuxServices(output string) []LinuxService {
	var services []LinuxService
	lines := strings.Split(output, "\n")

	// Format: service-name.service  loaded active running Description
	serviceRegex := regexp.MustCompile(`^\s*(\S+\.service)\s+(\S+)\s+(\S+)\s+(\S+)\s+(.*)$`)

	for _, line := range lines {
		matches := serviceRegex.FindStringSubmatch(line)
		if len(matches) >= 6 {
			serviceName := strings.TrimSuffix(matches[1], ".service")
			loadState := matches[2]   // loaded, not-found, etc.
			activeState := matches[3] // active, inactive, failed
			subState := matches[4]    // running, exited, dead
			description := strings.TrimSpace(matches[5])

			// Determine status
			status := "unknown"
			if activeState == "active" && subState == "running" {
				status = "running"
			} else if activeState == "inactive" || activeState == "failed" {
				status = "stopped"
			}

			service := LinuxService{
				Name:        serviceName,
				DisplayName: description,
				Description: description,
				Status:      status,
				StartupType: loadState,
			}

			services = append(services, service)
		}
	}

	return services
}

package services

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type WMIDiscovery struct {
}

func NewWMIDiscovery() *WMIDiscovery {
	return &WMIDiscovery{}
}

// decodeWindowsOutput - Converts Windows console output (CP857) to ASCII
func decodeWindowsOutput(data []byte) string {
	// CP857 byte values for Turkish characters
	// Map CP857 bytes directly to ASCII equivalents
	result := make([]byte, len(data))

	for i, b := range data {
		switch b {
		// Turkish characters in CP857 -> ASCII
		case 0x87: result[i] = 'c'  // ç
		case 0x80: result[i] = 'C'  // Ç
		case 0x94: result[i] = 'o'  // ö
		case 0x99: result[i] = 'O'  // Ö
		case 0x81: result[i] = 'u'  // ü
		case 0x9A: result[i] = 'U'  // Ü
		case 0x8D: result[i] = 'i'  // ı
		case 0x98: result[i] = 'I'  // İ
		case 0xFD: result[i] = 's'  // ş
		case 0xE7: result[i] = 'S'  // Ş
		case 0xA7: result[i] = 'g'  // ğ
		case 0xA6: result[i] = 'G'  // Ğ
		default:
			// Keep ASCII and other characters as-is
			result[i] = b
		}
	}

	output := string(result)
	log.Printf("[Encoding] Decoded CP857 to ASCII")
	return output
}

// sanitizeError - Sanitizes error messages to prevent information disclosure
func (w *WMIDiscovery) sanitizeError(err error, powershellOutput string) error {
	if err == nil {
		return nil
	}

	// Common error patterns and their user-friendly messages
	errorMap := map[string]string{
		"Access is denied":                                 "authentication_failed",
		"Erişim engellendi":                                "authentication_failed",
		"E_ACCESSDENIED":                                   "authentication_failed",
		"0x80070005":                                       "authentication_failed",
		"The user name or password is incorrect":          "invalid_credentials",
		"Kullanıcı adı veya parola yanlış":                "invalid_credentials",
		"WSMan":                                            "winrm_not_configured",
		"WinRM":                                            "winrm_not_configured",
		"host is unavailable":                              "host_unreachable",
		"RPC server is unavailable":                        "rpc_unavailable",
		"The RPC server is unavailable":                    "rpc_unavailable",
		"Connecting to remote server":                      "connection_failed",
		"TrustedHosts":                                     "trust_configuration_required",
		"güvenilir ana bilgisayar":                         "trust_configuration_required",
		"WMI hizmeti":                                      "wmi_service_error",
		"timeout":                                          "connection_timeout",
		"zaman aşımı":                                      "connection_timeout",
		"network path":                                     "network_error",
		"The network path was not found":                  "host_unreachable",
	}

	// Check output and error message for known patterns
	outputLower := strings.ToLower(powershellOutput)
	errorLower := strings.ToLower(err.Error())
	combinedText := outputLower + " " + errorLower

	for pattern, errorCode := range errorMap {
		if strings.Contains(combinedText, strings.ToLower(pattern)) {
			return fmt.Errorf(errorCode)
		}
	}

	// Default generic error
	return fmt.Errorf("connection_failed")
}

// DiscoveredService represents a discovered service (for backward compatibility)
type DiscoveredService struct {
	ServiceName string
	Port        int
	Protocol    string
	Method      string
}

// DiscoverWindowsServices - WMI kullanarak uzak Windows sunucusundaki servisleri keşfeder
func (w *WMIDiscovery) DiscoverWindowsServices(ctx context.Context, targetAddress, username, password, domain string) ([]DiscoveredService, error) {
	var discovered []DiscoveredService

	// WMI query için PowerShell kullanıyoruz (WMIC deprecated)
	// PowerShell remoting veya Get-WmiObject kullanarak uzak sunucuya bağlanıyoruz

	// Credential oluştur
	credential := username
	if domain != "" {
		credential = domain + "\\" + username
	}

	// PowerShell script: Uzak sunucudan servisleri çek
	psScript := fmt.Sprintf(`
		$secPassword = ConvertTo-SecureString '%s' -AsPlainText -Force
		$credential = New-Object System.Management.Automation.PSCredential('%s', $secPassword)

		try {
			# WinRM ile bağlan (veya WMI)
			$services = Get-WmiObject -Class Win32_Service -ComputerName '%s' -Credential $credential -ErrorAction Stop

			foreach ($svc in $services) {
				if ($svc.State -eq 'Running') {
					Write-Output "$($svc.Name)|$($svc.DisplayName)|$($svc.State)|$($svc.StartMode)|$($svc.ProcessId)|$($svc.Description)"
				}
			}
		} catch {
			Write-Error $_.Exception.Message
			exit 1
		}
	`, password, credential, targetAddress)

	// PowerShell çalıştır
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// WMI başarısız oldu, detaylı hata döndür
		return nil, fmt.Errorf("WMI query failed: %v - %s", err, string(output))
	}

	// Output'u parse et
	discovered = w.parseWMIOutput(string(output))

	log.Printf("WMI discovered %d services on %s", len(discovered), targetAddress)
	return discovered, nil
}

// parseWMIOutput - PowerShell output'unu parse eder
func (w *WMIDiscovery) parseWMIOutput(output string) []DiscoveredService {
	var services []DiscoveredService
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Format: Name|DisplayName|State|StartMode|ProcessId|Description
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}

		serviceName := strings.TrimSpace(parts[0])
		displayName := strings.TrimSpace(parts[1])
		state := strings.TrimSpace(parts[2])

		// Sadece çalışan servisleri döndür
		if state != "Running" {
			continue
		}

		// Servis bilgisini oluştur
		discovered := DiscoveredService{
			ServiceName: fmt.Sprintf("%s (%s)", displayName, serviceName),
			Port:        0, // WMI'da port bilgisi yok, ayrıca netstat ile bulunabilir
			Protocol:    "wmi",
			Method:      "wmi",
		}

		services = append(services, discovered)
	}

	return services
}

// GetServicePorts - Çalışan servislerin port bilgilerini netstat ile alır
func (w *WMIDiscovery) GetServicePorts(ctx context.Context, targetAddress, username, password, domain string) (map[string]int, error) {
	credential := username
	if domain != "" {
		credential = domain + "\\" + username
	}

	// PowerShell script: netstat ile port bilgilerini al
	psScript := fmt.Sprintf(`
		$secPassword = ConvertTo-SecureString '%s' -AsPlainText -Force
		$credential = New-Object System.Management.Automation.PSCredential('%s', $secPassword)

		try {
			Invoke-Command -ComputerName '%s' -Credential $credential -ScriptBlock {
				netstat -ano | Select-String "LISTENING"
			}
		} catch {
			Write-Error $_.Exception.Message
			exit 1
		}
	`, password, credential, targetAddress)

	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("netstat query failed: %v", err)
	}

	return w.parseNetstatOutput(string(output)), nil
}

// parseNetstatOutput - netstat output'unu parse eder
func (w *WMIDiscovery) parseNetstatOutput(output string) map[string]int {
	portMap := make(map[string]int)
	lines := strings.Split(output, "\n")

	// Regex: Extract port from netstat output
	// Format: TCP    0.0.0.0:80    0.0.0.0:0    LISTENING    1234
	portRegex := regexp.MustCompile(`TCP\s+\S+:(\d+)\s+.*LISTENING\s+(\d+)`)

	for _, line := range lines {
		matches := portRegex.FindStringSubmatch(line)
		if len(matches) >= 3 {
			port := matches[1]
			// pid := matches[2] // Process ID (ihtiyaç olursa kullanılabilir)

			var portNum int
			fmt.Sscanf(port, "%d", &portNum)

			// Port numarasını map'e ekle (PID ile eşleştirebiliriz)
			if portNum > 0 {
				portMap[port] = portNum
			}
		}
	}

	return portMap
}

// CheckWindowsService - Belirli bir servisin durumunu kontrol eder
func (w *WMIDiscovery) CheckWindowsService(ctx context.Context, targetAddress, username, password, domain, serviceName string) (string, error) {
	credential := username
	if domain != "" {
		credential = domain + "\\" + username
	}

	psScript := fmt.Sprintf(`
		$secPassword = ConvertTo-SecureString '%s' -AsPlainText -Force
		$credential = New-Object System.Management.Automation.PSCredential('%s', $secPassword)

		try {
			$service = Get-WmiObject -Class Win32_Service -Filter "Name='%s'" -ComputerName '%s' -Credential $credential -ErrorAction Stop
			if ($service) {
				Write-Output $service.State
			} else {
				Write-Output "NotFound"
			}
		} catch {
			Write-Error $_.Exception.Message
			exit 1
		}
	`, password, credential, serviceName, targetAddress)

	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript)
	output, err := cmd.Output()
	if err != nil {
		return "unknown", fmt.Errorf("service check failed: %v", err)
	}

	state := strings.TrimSpace(string(output))

	// State mapping: Running -> online, Stopped -> offline
	switch state {
	case "Running":
		return "online", nil
	case "Stopped", "Paused":
		return "offline", nil
	case "NotFound":
		return "unknown", fmt.Errorf("service not found")
	default:
		return "unknown", nil
	}
}

// QuickWMICheck - Hızlı WMI bağlantı testi (credential doğrulama)
func (w *WMIDiscovery) QuickWMICheck(ctx context.Context, targetAddress, username, password, domain string) error {
	credential := username
	if domain != "" {
		credential = domain + "\\" + username
	}

	// 10 saniyelik timeout (Invoke-Command için biraz daha fazla)
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Escape single quotes in password
	escapedPassword := strings.ReplaceAll(password, "'", "''")

	psScript := fmt.Sprintf(`
		$ErrorActionPreference = 'Continue'
		[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
		$OutputEncoding = [System.Text.Encoding]::UTF8
		chcp 65001 | Out-Null

		$secPassword = ConvertTo-SecureString '%s' -AsPlainText -Force
		$credential = New-Object System.Management.Automation.PSCredential('%s', $secPassword)

		$errors = @()

		# Method 1: SC.exe via UNC (Simplest, no WinRM/WMI needed)
		try {
			Write-Host "[DEBUG] Method 1: SC.exe"
			$password = '%s'
			$username = '%s'
			net use \\%s\IPC$ /user:$username $password 2>&1 | Out-Null
			$result = sc.exe \\%s query 2>&1
			net use \\%s\IPC$ /delete 2>&1 | Out-Null
			if ($result -match 'SERVICE_NAME') {
				Write-Output "OK:SC"
				exit 0
			}
		} catch {
			net use \\%s\IPC$ /delete 2>&1 | Out-Null
		}

		# Method 2: CIM with DCOM (Good for non-WinRM systems)
		try {
			Write-Host "[DEBUG] Method 2: CIM/DCOM"
			$sessionOption = New-CimSessionOption -Protocol Dcom
			$session = New-CimSession -ComputerName '%s' -Credential $credential -SessionOption $sessionOption -ErrorAction Stop
			$os = Get-CimInstance -ClassName Win32_OperatingSystem -CimSession $session -ErrorAction Stop
			Remove-CimSession $session
			Write-Output "OK:CIM"
			exit 0
		} catch {
			$errors += "Method2_Failed"
		}

		# Method 3: Direct WMI
		try {
			Write-Host "[DEBUG] Method 3: Direct WMI"
			$os = Get-WmiObject -Class Win32_OperatingSystem -ComputerName '%s' -Credential $credential -ErrorAction Stop
			Write-Output "OK:WMI"
			exit 0
		} catch {
			$errors += "Method3_Failed"
		}

		# Method 4: Invoke-Command with Negotiate
		try {
			Write-Host "[DEBUG] Method 4: Invoke-Command/Negotiate"
			$sessionOption = New-PSSessionOption -SkipCACheck -SkipCNCheck -SkipRevocationCheck
			$result = Invoke-Command -ComputerName '%s' -Credential $credential -Authentication Negotiate -SessionOption $sessionOption -ScriptBlock {
				Get-WmiObject -Class Win32_OperatingSystem | Select-Object Caption
			} -ErrorAction Stop
			Write-Output "OK:Invoke-Command"
			exit 0
		} catch {
			$errors += "Method4_Failed"
		}

		# Method 5: CIM with WSMAN
		try {
			Write-Host "[DEBUG] Method 5: CIM/WSMAN"
			$sessionOption = New-CimSessionOption -Protocol Wsman -SkipCACheck -SkipCNCheck -SkipRevocationCheck
			$session = New-CimSession -ComputerName '%s' -Credential $credential -SessionOption $sessionOption -ErrorAction Stop
			$os = Get-CimInstance -ClassName Win32_OperatingSystem -CimSession $session -ErrorAction Stop
			Remove-CimSession $session
			Write-Output "OK:CIM"
			exit 0
		} catch {
			$errors += "Method5_Failed"
		}

		# All methods failed
		Write-Host "[FAILED] All connection methods failed"
		Write-Error "connection_failed"
		exit 1
	`, escapedPassword, credential, password, username, targetAddress, targetAddress, targetAddress, targetAddress, targetAddress, targetAddress, targetAddress, targetAddress)

	cmd := exec.CommandContext(checkCtx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript)
	rawOutput, err := cmd.CombinedOutput()

	// Decode Windows output to UTF-8
	output := decodeWindowsOutput(rawOutput)

	// Log sanitized output for debugging (don't log passwords or sensitive data)
	log.Printf("[WMI Check] Target: %s, User: %s, Domain: %s", targetAddress, username, domain)

	// Only log detailed output in debug mode, otherwise log sanitized version
	if strings.Contains(output, "[DEBUG]") || strings.Contains(output, "[ERROR]") {
		// Extract only debug/error lines, skip full output
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "[DEBUG]") || strings.Contains(line, "[ERROR]") || strings.Contains(line, "[FAILED]") {
				log.Printf("[WMI Check] %s", line)
			}
		}
	}

	if err != nil {
		log.Printf("[WMI Check] Connection failed with error: %v", err)
		// Return sanitized error
		return w.sanitizeError(err, output)
	}

	if !strings.Contains(output, "OK:") {
		log.Printf("[WMI Check] No OK response found")
		// Return sanitized error
		return w.sanitizeError(fmt.Errorf("connection failed"), output)
	}

	// Extract method that worked
	if strings.Contains(output, "OK:Invoke-Command") {
		log.Printf("[WMI Check] Connection successful using: Invoke-Command (PowerShell Remoting)")
	} else if strings.Contains(output, "OK:WMI") {
		log.Printf("[WMI Check] Connection successful using: WMI")
	} else if strings.Contains(output, "OK:CIM") {
		log.Printf("[WMI Check] Connection successful using: CIM")
	} else if strings.Contains(output, "OK:SC") {
		log.Printf("[WMI Check] Connection successful using: SC.exe")
	}

	return nil
}

// GetAllWindowsServices - Tüm Windows servislerini döndürür (WindowsService struct olarak)
func (w *WMIDiscovery) GetAllWindowsServices(ctx context.Context, targetAddress, username, password, domain string) ([]WindowsService, error) {
	credential := username
	if domain != "" {
		credential = domain + "\\" + username
	}

	// Escape single quotes in password
	escapedPassword := strings.ReplaceAll(password, "'", "''")

	// PowerShell script: 6-tier fallback strategy with SC.exe
	psScript := fmt.Sprintf(`
		$ErrorActionPreference = 'Continue'
		[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
		$OutputEncoding = [System.Text.Encoding]::UTF8
		chcp 65001 | Out-Null

		$secPassword = ConvertTo-SecureString '%s' -AsPlainText -Force
		$credential = New-Object System.Management.Automation.PSCredential('%s', $secPassword)

		$services = @()

		# Method 1: SC.exe via UNC path (Simplest, works with basic credentials)
		try {
			Write-Host "[DEBUG] Method 1: SC.exe via UNC"
			$password = '%s'
			$username = '%s'

			# Map network drive temporarily
			net use \\%s\IPC$ /user:$username $password 2>&1 | Out-Null

			# Get all services with sc.exe (UTF-8 encoding)
			$scOutput = sc.exe \\%s query state=all 2>&1

			# Parse sc output - get name, display, state
			$services = @{}
			$currentName = ''
			foreach ($line in $scOutput) {
				if ($line -match '^SERVICE_NAME:\s+(.+)$') {
					$currentName = $matches[1].Trim()
					$services[$currentName] = @{ 'Name' = $currentName; 'Display' = ''; 'State' = 'Unknown'; 'StartMode' = 'Unknown' }
				}
				elseif ($line -match '^DISPLAY_NAME:\s+(.+)$' -and $currentName) {
					$services[$currentName]['Display'] = $matches[1].Trim()
				}
				elseif ($line -match '^\s+STATE\s+:\s+\d+\s+(\w+)' -and $currentName) {
					$state = $matches[1].Trim()
					if ($state -eq 'RUNNING') { $state = 'Running' }
					elseif ($state -eq 'STOPPED') { $state = 'Stopped' }
					$services[$currentName]['State'] = $state
				}
			}

			# Now get startup type for each service using sc qc
			foreach ($serviceName in $services.Keys) {
				$qcOutput = sc.exe \\%s qc $serviceName 2>&1
				foreach ($line in $qcOutput) {
					if ($line -match 'START_TYPE\s+:\s+\d+\s+(\w+)') {
						$startType = $matches[1].Trim()
						if ($startType -eq 'AUTO_START') { $startType = 'Auto' }
						elseif ($startType -eq 'DEMAND_START') { $startType = 'Manual' }
						elseif ($startType -eq 'DISABLED') { $startType = 'Disabled' }
						$services[$serviceName]['StartMode'] = $startType
						break
					}
				}

				$svc = $services[$serviceName]
				Write-Output "$($svc.Name)|$($svc.Display)|$($svc.State)|$($svc.StartMode)|0|"
			}

			net use \\%s\IPC$ /delete 2>&1 | Out-Null
			exit 0
		} catch {
			net use \\%s\IPC$ /delete 2>&1 | Out-Null
		}

		# Method 2: Invoke-Command with Negotiate
		try {
			Write-Host "[DEBUG] Method 2: Invoke-Command/Negotiate"
			$sessionOption = New-PSSessionOption -SkipCACheck -SkipCNCheck -SkipRevocationCheck
			$services = Invoke-Command -ComputerName '%s' -Credential $credential -Authentication Negotiate -SessionOption $sessionOption -ScriptBlock {
				Get-WmiObject -Class Win32_Service | Select-Object Name, DisplayName, State, StartMode, ProcessId, Description
			} -ErrorAction Stop

			foreach ($svc in $services) {
				Write-Output "$($svc.Name)|$($svc.DisplayName)|$($svc.State)|$($svc.StartMode)|$($svc.ProcessId)|$($svc.Description)"
			}
			exit 0
		} catch { }

		# Method 3: CIM with DCOM (Better for non-WinRM systems)
		try {
			Write-Host "[DEBUG] Method 3: CIM/DCOM"
			$sessionOption = New-CimSessionOption -Protocol Dcom
			$session = New-CimSession -ComputerName '%s' -Credential $credential -SessionOption $sessionOption -ErrorAction Stop
			$services = Get-CimInstance -ClassName Win32_Service -CimSession $session -ErrorAction Stop

			foreach ($svc in $services) {
				Write-Output "$($svc.Name)|$($svc.DisplayName)|$($svc.State)|$($svc.StartMode)|$($svc.ProcessId)|$($svc.Description)"
			}
			Remove-CimSession $session
			exit 0
		} catch { }

		# Method 4: Direct WMI
		try {
			Write-Host "[DEBUG] Method 4: Direct WMI"
			$services = Get-WmiObject -Class Win32_Service -ComputerName '%s' -Credential $credential -ErrorAction Stop

			foreach ($svc in $services) {
				Write-Output "$($svc.Name)|$($svc.DisplayName)|$($svc.State)|$($svc.StartMode)|$($svc.ProcessId)|$($svc.Description)"
			}
			exit 0
		} catch { }

		# Method 5: CIM with WSMAN
		try {
			Write-Host "[DEBUG] Method 5: CIM/WSMAN"
			$sessionOption = New-CimSessionOption -Protocol Wsman -SkipCACheck -SkipCNCheck -SkipRevocationCheck
			$session = New-CimSession -ComputerName '%s' -Credential $credential -SessionOption $sessionOption -ErrorAction Stop
			$services = Get-CimInstance -ClassName Win32_Service -CimSession $session -ErrorAction Stop

			foreach ($svc in $services) {
				Write-Output "$($svc.Name)|$($svc.DisplayName)|$($svc.State)|$($svc.StartMode)|$($svc.ProcessId)|$($svc.Description)"
			}
			Remove-CimSession $session
			exit 0
		} catch { }

		# Method 6: Invoke-Command with Basic
		try {
			Write-Host "[DEBUG] Method 6: Invoke-Command/Basic"
			$sessionOption = New-PSSessionOption -SkipCACheck -SkipCNCheck -SkipRevocationCheck
			$services = Invoke-Command -ComputerName '%s' -Credential $credential -Authentication Basic -SessionOption $sessionOption -ScriptBlock {
				Get-WmiObject -Class Win32_Service | Select-Object Name, DisplayName, State, StartMode, ProcessId, Description
			} -ErrorAction Stop

			foreach ($svc in $services) {
				Write-Output "$($svc.Name)|$($svc.DisplayName)|$($svc.State)|$($svc.StartMode)|$($svc.ProcessId)|$($svc.Description)"
			}
			exit 0
		} catch { }

		# All methods failed
		Write-Host "[FAILED] All methods failed"
		Write-Error "connection_failed"
		exit 1
	`, escapedPassword, credential, password, username, targetAddress, targetAddress, targetAddress, targetAddress, targetAddress, targetAddress, targetAddress, targetAddress, targetAddress, targetAddress)

	// PowerShell çalıştır
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript)
	rawOutput, err := cmd.CombinedOutput()

	// Decode Windows output to UTF-8
	output := decodeWindowsOutput(rawOutput)

	// Log sanitized information
	log.Printf("[GetAllWindowsServices] Target: %s, User: %s, Domain: %s", targetAddress, username, domain)

	// Only log debug/error lines
	if strings.Contains(output, "[DEBUG]") || strings.Contains(output, "[ERROR]") {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, "[DEBUG]") || strings.Contains(line, "[ERROR]") || strings.Contains(line, "[FAILED]") {
				log.Printf("[GetAllWindowsServices] %s", line)
			}
		}
	}

	if err != nil {
		log.Printf("[GetAllWindowsServices] Service query failed: %v", err)
		return nil, w.sanitizeError(err, output)
	}

	// Output'u parse et
	services := w.parseAllServicesOutput(output)
	if len(services) == 0 {
		log.Printf("[GetAllWindowsServices] No services found in output")
		return nil, w.sanitizeError(fmt.Errorf("no services found"), output)
	}

	log.Printf("[GetAllWindowsServices] Successfully retrieved %d services", len(services))
	return services, nil
}

// parseAllServicesOutput - Tüm servisleri parse eder
func (w *WMIDiscovery) parseAllServicesOutput(output string) []WindowsService {
	var services []WindowsService
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Format: Name|DisplayName|State|StartMode|ProcessId|Description
		parts := strings.Split(line, "|")
		if len(parts) < 6 {
			continue
		}

		var pid int
		fmt.Sscanf(strings.TrimSpace(parts[4]), "%d", &pid)

		service := WindowsService{
			Name:        strings.TrimSpace(parts[0]),
			DisplayName: strings.TrimSpace(parts[1]),
			State:       strings.TrimSpace(parts[2]),
			StartMode:   strings.TrimSpace(parts[3]),
			ProcessID:   pid,
			Description: strings.TrimSpace(parts[5]),
		}

		services = append(services, service)
	}

	return services
}

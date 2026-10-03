//go:generate go run ./internal/network/scanner/oui_gen.go

package scanner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/gopacket/macs"
	"github.com/gosnmp/gosnmp"
	"github.com/klauspost/oui"
	"golang.org/x/net/dns/dnsmessage"
)

type DeviceInfo struct {
	IP                string    `json:"ip"`
	Hostname          string    `json:"hostname,omitempty"`
	FQDN              string    `json:"-"` // Hidden from JSON for security (contains full domain)
	Domain            string    `json:"-"` // Hidden from JSON for security
	SystemName        string    `json:"system_name,omitempty"`
	SystemDescription string    `json:"system_description,omitempty"`
	SystemObjectID    string    `json:"system_object_id,omitempty"`
	LLDPSystemName    string    `json:"lldp_system_name,omitempty"`
	LLDPPortDesc      string    `json:"lldp_port_description,omitempty"`
	OperatingSystem   string    `json:"operating_system,omitempty"`
	MACAddress        string    `json:"mac_address,omitempty"`
	Vendor            string    `json:"vendor,omitempty"`
	Reachable         bool      `json:"reachable"`
	LatencyMs         int64     `json:"latency_ms"`
	CheckedAt         time.Time `json:"checked_at"`
	IsLocal           bool      `json:"is_local"`
	InterfaceName     string    `json:"interface_name,omitempty"`
	ProbeMethod       string    `json:"probe_method,omitempty"`
	WebServer         string    `json:"web_server,omitempty"`
	HTTPTitle         string    `json:"http_title,omitempty"`
	SSHBanner         string    `json:"ssh_banner,omitempty"`
	RDPHostname       string    `json:"rdp_hostname,omitempty"`
	DiscoverySources  []string  `json:"discovery_sources,omitempty"`
	ConfidenceScore   int       `json:"confidence_score,omitempty"`
}

type Result struct {
	Subnet         string        `json:"subnet"`
	HostCount      int           `json:"host_count"`
	ReachableCount int           `json:"reachable_count"`
	ScannedCount   int           `json:"scanned_count"`
	Duration       time.Duration `json:"duration"`
	Devices        []DeviceInfo  `json:"devices"`
}

type Options struct {
	Timeout            time.Duration
	Retries            int
	Workers            int
	MaxHosts           int
	IncludeUnreachable bool
	Ports              []int
	FilterByPorts      bool
}

type InterfaceInfo struct {
	Name    string `json:"name"`
	IP      string `json:"ip"`
	CIDR    string `json:"cidr"`
	Default bool   `json:"default"`
}

const (
	defaultOUIFile         = "data/oui.txt"
	ouiFileEnv             = "SYSTRACK_OUI_FILE"
	ouiUpdateURLEnv        = "SYSTRACK_OUI_SOURCE_URL"
	ouiLegacyURLEnv        = "SYSTRACK_OUI_SOURCE"
	ouiAutoUpdateEnv       = "SYSTRACK_OUI_AUTO_UPDATE"
	defaultOUIURL          = "https://standards-oui.ieee.org/oui/oui.txt"
	fallbackOUIURL         = "https://linuxnet.ca/ieee/oui.txt"
	ouiUpdateInterval      = 7 * 24 * time.Hour
	ouiUpdateCheckInterval = 24 * time.Hour
	localDeviceVendorEnv   = "SYSTRACK_LOCAL_DEVICE_VENDOR"
	defaultLocalVendorName = "SysTrack Ltd"
)

func localDeviceVendorName() string {
	v := strings.TrimSpace(os.Getenv(localDeviceVendorEnv))
	if v == "" {
		return defaultLocalVendorName
	}
	if strings.EqualFold(v, "off") || v == "0" {
		return ""
	}
	return v
}

func ScanSubnet(ctx context.Context, subnet string, opts Options) (*Result, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(subnet))
	if err != nil {
		return nil, fmt.Errorf("invalid subnet: %w", err)
	}

	if !prefix.Addr().Is4() {
		return nil, errors.New("only IPv4 subnets are supported")
	}

	hosts, err := enumerateHosts(prefix.Masked(), opts.MaxHosts)
	if err != nil {
		return nil, err
	}

	return scanHosts(ctx, hosts, prefix.Masked().String(), opts)
}

func ScanRange(ctx context.Context, start, end netip.Addr, opts Options) (*Result, error) {
	if !start.Is4() || !end.Is4() {
		return nil, errors.New("only IPv4 ranges are supported")
	}

	hosts, err := enumerateRange(start, end, opts.MaxHosts)
	if err != nil {
		return nil, err
	}

	label := fmt.Sprintf("%s-%s", start.String(), end.String())
	return scanHosts(ctx, hosts, label, opts)
}

func ListInterfaces() ([]InterfaceInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	defaultIface := detectDefaultRouteInterface()

	var infos []InterfaceInfo
	for _, iface := range ifaces {
		if !isUsableInterface(iface) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}

			mask, bits := ipNet.Mask.Size()
			if mask == 0 && bits == 0 {
				continue
			}

			network := ip.Mask(ipNet.Mask)
			info := InterfaceInfo{
				Name: iface.Name,
				IP:   ip.String(),
				CIDR: fmt.Sprintf("%s/%d", network.String(), mask),
			}

			if iface.Name == defaultIface {
				info.Default = true
			}

			infos = append(infos, info)
		}
	}

	if len(infos) == 0 {
		return nil, errors.New("no active IPv4 interfaces detected")
	}

	if !markPreferredInterface(infos) {
		infos[0].Default = true
	}

	return infos, nil
}

func DetectDefaultSubnet() (*InterfaceInfo, error) {
	interfaces, err := ListInterfaces()
	if err != nil {
		return nil, err
	}

	for i := range interfaces {
		if interfaces[i].Default {
			return &interfaces[i], nil
		}
	}

	return &interfaces[0], nil
}

func isUsableInterface(iface net.Interface) bool {
	if (iface.Flags & net.FlagUp) == 0 {
		return false
	}
	if (iface.Flags & net.FlagLoopback) != 0 {
		return false
	}
	if shouldIgnoreInterface(iface.Name) {
		return false
	}
	return true
}

func shouldIgnoreInterface(name string) bool {
	lower := strings.ToLower(name)
	if lower == "" {
		return true
	}

	ignoredPrefixes := []string{"docker", "br-", "veth", "lo", "cni", "virbr", "tun", "tap", "wg", "zt", "qvb", "qvo"}
	for _, prefix := range ignoredPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	return false
}

func detectDefaultRouteInterface() string {
	if runtime.GOOS != "linux" {
		return ""
	}

	file, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return ""
	}

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 11 {
			continue
		}

		iface := fields[0]
		if shouldIgnoreInterface(iface) {
			continue
		}

		dest := fields[1]
		if dest != "00000000" {
			continue
		}

		flags, err := strconv.ParseInt(fields[3], 16, 32)
		if err != nil {
			continue
		}
		const rtfGateway = 0x2
		if flags&rtfGateway == 0 {
			continue
		}

		return iface
	}

	return ""
}

func markPreferredInterface(infos []InterfaceInfo) bool {
	if len(infos) == 0 {
		return false
	}

	bestIdx := -1
	bestScore := -1
	for idx := range infos {
		info := &infos[idx]
		score := interfaceScore(*info)
		if info.Default {
			score += 100
		}
		if score > bestScore {
			bestScore = score
			bestIdx = idx
		}
		info.Default = false
	}

	if bestIdx >= 0 {
		infos[bestIdx].Default = true
		return true
	}
	return false
}

func interfaceScore(info InterfaceInfo) int {
	ip := net.ParseIP(strings.Split(info.IP, "%")[0])
	if ip == nil {
		return 0
	}

	switch {
	case ip[0] == 192 && ip[1] == 168:
		return 30
	case ip[0] == 10:
		return 25
	case ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31:
		return 10
	default:
		if isPrivateIPv4(ip) {
			return 5
		}
		return 1
	}
}

func (o *Options) normalize() {
	if o.Timeout <= 0 {
		o.Timeout = 800 * time.Millisecond
	} else if o.Timeout > 5*time.Second {
		o.Timeout = 5 * time.Second
	}

	if o.Retries <= 0 {
		o.Retries = 1
	} else if o.Retries > 3 {
		o.Retries = 3
	}

	if o.Workers <= 0 {
		o.Workers = runtime.NumCPU()
	} else if o.Workers > 128 {
		o.Workers = 128
	}

	if o.MaxHosts <= 0 {
		o.MaxHosts = 512
	} else if o.MaxHosts > 4096 {
		o.MaxHosts = 4096
	}
}

func enumerateHosts(prefix netip.Prefix, limit int) ([]netip.Addr, error) {
	ones := prefix.Bits()
	if ones <= 0 || ones >= 32 {
		return nil, fmt.Errorf("no usable addresses in subnet %s", prefix.String())
	}

	total := uint32(1) << uint32(32-ones)
	if total <= 2 {
		return nil, fmt.Errorf("no usable addresses in subnet %s", prefix.String())
	}

	usable := int(total) - 2
	if usable > limit {
		return nil, fmt.Errorf("Subnet %d adres içeriyor fakat limit %d (CIDR aralığını azaltın)", usable, limit)
	}

	start := prefix.Masked().Addr().Next()
	hosts := make([]netip.Addr, 0, usable)
	for i := 0; i < usable; i++ {
		hosts = append(hosts, start)
		start = start.Next()
	}

	return hosts, nil
}

func mapLocalIPv4() map[string]string {
	result := make(map[string]string)

	ifaces, err := net.Interfaces()
	if err != nil {
		return result
	}

	for _, iface := range ifaces {
		if (iface.Flags & net.FlagUp) == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}

			result[ip.String()] = iface.Name
		}
	}

	return result
}

// LocalInterfaceInfo contains information about a local network interface
type LocalInterfaceInfo struct {
	IP            string
	MAC           string
	InterfaceName string
	Hostname      string
}

// mapLocalIPv4WithMAC maps local IPv4 addresses to their MAC addresses and info
func mapLocalIPv4WithMAC() map[string]LocalInterfaceInfo {
	result := make(map[string]LocalInterfaceInfo)

	ifaces, err := net.Interfaces()
	if err != nil {
		return result
	}

	// Get local hostname once
	localHostname, _ := os.Hostname()

	for _, iface := range ifaces {
		// Skip down interfaces
		if (iface.Flags & net.FlagUp) == 0 {
			continue
		}

		// Skip loopback
		if (iface.Flags & net.FlagLoopback) != 0 {
			continue
		}

		// Get MAC address
		macAddr := iface.HardwareAddr.String()
		if macAddr == "" || len(macAddr) < 17 {
			continue
		}

		// Normalize MAC format to XX:XX:XX:XX:XX:XX
		normalizedMAC := strings.ToUpper(strings.ReplaceAll(macAddr, ":", ":"))
		if isZeroMAC(normalizedMAC) {
			continue
		}

		// Get IP addresses for this interface
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}

			result[ip.String()] = LocalInterfaceInfo{
				IP:            ip.String(),
				MAC:           normalizedMAC,
				InterfaceName: iface.Name,
				Hostname:      localHostname,
			}
		}
	}

	return result
}

func snapshotARPTable() map[string]string {
	entries := make(map[string]string)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("arp", "-a")
	} else {
		cmd = exec.Command("arp", "-n")
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return entries
	}

	for ip, mac := range parseARPTable(string(output)) {
		entries[ip] = mac
	}

	return entries
}

func SnapshotARPTable() map[string]string {
	return snapshotARPTable()
}

func probeAddress(ctx context.Context, addr netip.Addr, opts Options, locals map[string]LocalInterfaceInfo, initialMAC string) DeviceInfo {
	device := DeviceInfo{
		IP:        addr.String(),
		CheckedAt: time.Now().UTC(),
		Reachable: false,
	}

	// Check if local interface - PRIORITY: Use local interface info
	if localInfo, ok := locals[device.IP]; ok {
		device.IsLocal = true
		device.InterfaceName = localInfo.InterfaceName
		if !opts.FilterByPorts {
			device.Reachable = true
			device.ProbeMethod = "local"
		}

		// Use MAC from local interface (most reliable for local machine)
		if localInfo.MAC != "" && !isZeroMAC(localInfo.MAC) {
			device.MACAddress = localInfo.MAC
			device.Vendor = getVendorFromMAC(localInfo.MAC)
			device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "local-interface")
		}

		// Use hostname from local machine
		if localInfo.Hostname != "" {
			device.Hostname = localInfo.Hostname
			device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "local-hostname")
		}
	}

	// If MAC not set from local interface, try initial MAC from ARP snapshot
	if device.MACAddress == "" {
		if mac := normalizeMACAddress(initialMAC); mac != "" && !isZeroMAC(mac) {
			device.MACAddress = mac
			device.Vendor = getVendorFromMAC(mac)
			device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "arp")
		}
	}

	// If not reachable yet, try probing
	if !device.Reachable {
		for attempt := 0; attempt < opts.Retries; attempt++ {
			if len(opts.Ports) > 0 && opts.FilterByPorts {
				reachable, latency, port := runPortsProbe(ctx, device.IP, opts.Timeout, opts.Ports)
				if reachable {
					device.Reachable = true
					device.LatencyMs = latency.Milliseconds()
					device.ProbeMethod = fmt.Sprintf("tcp:%d", port)
					break
				}
			} else {
				reachable, latency, method := runPingProbe(ctx, device.IP, opts.Timeout)
				if reachable {
					device.Reachable = true
					device.LatencyMs = latency.Milliseconds()
					device.ProbeMethod = method
					break
				}
			}
		}
	}

	shouldCollectDetails := device.Reachable || opts.IncludeUnreachable || device.MACAddress != ""

	if shouldCollectDetails {
		if device.Reachable {
			// Short delay for ARP cache to populate when target responded
			time.Sleep(50 * time.Millisecond)
		}

		detailCtx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
		defer cancel()

		macChan := make(chan struct{ mac, vendor string }, 1)
		hostnameChan := make(chan struct {
			name             string
			mac              string
			fqdn             string
			domain           string
			discoverySources []string
			smb              SMBInfo
		}, 1)
		serviceChan := make(chan struct {
			server string
			title  string
		}, 1)
		snmpChan := make(chan SNMPInfo, 1)
		tlsChan := make(chan []string, 1)

		// MAC/vendor lookup
		go func() {
			mac := getMACFromARPAggressive(detailCtx, device.IP)
			vendor := getVendorFromMAC(mac)
			select {
			case macChan <- struct{ mac, vendor string }{mac, vendor}:
			case <-detailCtx.Done():
			}
		}()

		// FQDN and hostname resolution pipeline
		go func() {
			result := struct {
				name             string
				mac              string
				fqdn             string
				domain           string
				discoverySources []string
				smb              SMBInfo
			}{}

			// Use comprehensive FQDN discovery
			fqdnResult := discoverFQDN(detailCtx, device.IP)

			if fqdnResult.FQDN != "" {
				result.fqdn = fqdnResult.FQDN
				result.name = fqdnResult.Hostname
				result.domain = fqdnResult.Domain
				result.discoverySources = fqdnResult.DiscoveryMethods
				if fqdnResult.SMBInfo.IsAvailable {
					result.smb = fqdnResult.SMBInfo
				}
				// Also try to get MAC from NetBIOS if available
				if info := netbiosLookup(detailCtx, device.IP); info.MAC != "" {
					result.mac = info.MAC
				}
			} else {
				// Fallback to original method if FQDN discovery fails
				if info := netbiosLookup(detailCtx, device.IP); info.Hostname != "" || info.MAC != "" {
					result.name = info.Hostname
					result.mac = info.MAC
					result.discoverySources = append(result.discoverySources, "netbios")
				}
			}

			select {
			case hostnameChan <- result:
			case <-detailCtx.Done():
			}
		}()

		// Lightweight HTTP probe for banner/title info
		go func() {
			httpTimeout := opts.Timeout * 2
			if httpTimeout < 800*time.Millisecond {
				httpTimeout = 800 * time.Millisecond
			}
			if httpTimeout > 3*time.Second {
				httpTimeout = 3 * time.Second
			}
			server, title := probeHTTPServer(detailCtx, device.IP, httpTimeout)
			select {
			case serviceChan <- struct {
				server string
				title  string
			}{server: server, title: title}:
			case <-detailCtx.Done():
			}
		}()

		// SNMP/LLDP discovery
		go func() {
			info := probeSNMP(detailCtx, device.IP)
			select {
			case snmpChan <- info:
			case <-detailCtx.Done():
			}
		}()

		// TLS certificate names
		go func() {
			names := probeTLSNames(detailCtx, device.IP)
			select {
			case tlsChan <- names:
			case <-detailCtx.Done():
			}
		}()

		// SSH Banner probe (for Linux/Unix systems)
		sshChan := make(chan string, 1)
		go func() {
			sshTimeout := opts.Timeout
			if sshTimeout < 800*time.Millisecond {
				sshTimeout = 800 * time.Millisecond
			}
			if sshTimeout > 2*time.Second {
				sshTimeout = 2 * time.Second
			}
			banner := probeSSHBanner(detailCtx, device.IP, sshTimeout)
			select {
			case sshChan <- banner:
			case <-detailCtx.Done():
			}
		}()

		// RDP hostname probe (for Windows systems)
		rdpChan := make(chan string, 1)
		go func() {
			rdpTimeout := opts.Timeout * 2
			if rdpTimeout < 1*time.Second {
				rdpTimeout = 1 * time.Second
			}
			if rdpTimeout > 3*time.Second {
				rdpTimeout = 3 * time.Second
			}
			hostname := probeRDPHostname(detailCtx, device.IP, rdpTimeout)
			select {
			case rdpChan <- hostname:
			case <-detailCtx.Done():
			}
		}()

		pending := 7
		for pending > 0 {
			select {
			case macResult := <-macChan:
				if macResult.mac != "" && !isZeroMAC(macResult.mac) {
					device.MACAddress = macResult.mac
					if macResult.vendor != "" {
						device.Vendor = macResult.vendor
					} else {
						device.Vendor = getVendorFromMAC(macResult.mac)
					}
					device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "arp")
				}
				pending--
			case hostname := <-hostnameChan:
				if hostname.fqdn != "" {
					if device.FQDN == "" {
						device.FQDN = hostname.fqdn
					}
					// hostname.name is already split by discoverFQDN -> splitFQDN
					if hostname.name != "" && device.Hostname == "" {
						device.Hostname = hostname.name
					}
				} else if device.Hostname == "" && hostname.name != "" {
					// hostname.name from netbiosLookup is already clean
					device.Hostname = hostname.name
				}
				// Set domain (stored internally but not shown in JSON by default)
				if device.Domain == "" && hostname.domain != "" {
					device.Domain = hostname.domain
				}
				// Set discovery sources
				if len(hostname.discoverySources) > 0 {
					for _, src := range hostname.discoverySources {
						device.DiscoverySources = appendIfNotExists(device.DiscoverySources, src)
					}
				}
				// Set MAC from NetBIOS if available
				if hostname.mac != "" && !isZeroMAC(hostname.mac) {
					device.MACAddress = hostname.mac
					device.Vendor = getVendorFromMAC(hostname.mac)
				}
				if hostname.smb.IsAvailable {
					device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "smb")
					if hostname.smb.Hostname != "" && device.Hostname == "" {
						device.Hostname = hostname.smb.Hostname
					}
					if hostname.smb.OSVersion != "" && device.OperatingSystem == "" {
						device.OperatingSystem = hostname.smb.OSVersion
					}
					if hostname.smb.Domain != "" && device.Domain == "" {
						device.Domain = hostname.smb.Domain
					}
				}
				pending--
			case service := <-serviceChan:
				if device.WebServer == "" && service.server != "" {
					device.WebServer = service.server
				}
				if device.HTTPTitle == "" && service.title != "" {
					device.HTTPTitle = service.title
				}
				pending--
			case snmpInfo := <-snmpChan:
				if snmpInfo.Available {
					device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "snmp")
					if snmpInfo.SysName != "" {
						device.SystemName = snmpInfo.SysName
						if device.Hostname == "" {
							if display := extractHostnameOnly(snmpInfo.SysName); display != "" {
								device.Hostname = display
							}
						}
					}
					if snmpInfo.SysDescr != "" {
						device.SystemDescription = snmpInfo.SysDescr
						if device.OperatingSystem == "" {
							device.OperatingSystem = snmpInfo.SysDescr
						}
					}
					if snmpInfo.SysObjectID != "" {
						device.SystemObjectID = snmpInfo.SysObjectID
					}
					if snmpInfo.LLDPSystemName != "" {
						device.LLDPSystemName = snmpInfo.LLDPSystemName
						device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "lldp")
						if device.Hostname == "" {
							if display := extractHostnameOnly(snmpInfo.LLDPSystemName); display != "" {
								device.Hostname = display
							}
						}
					}
					if snmpInfo.LLDPPortDescription != "" {
						device.LLDPPortDesc = snmpInfo.LLDPPortDescription
					}
				}
				pending--
			case tlsNames := <-tlsChan:
				if len(tlsNames) > 0 {
					device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "tls")
					if device.FQDN == "" {
						device.FQDN = tlsNames[0]
					}
					if device.Hostname == "" {
						if display := extractHostnameOnly(tlsNames[0]); display != "" {
							device.Hostname = display
						}
					}
				}
				pending--
			case sshBanner := <-sshChan:
				if sshBanner != "" {
					device.SSHBanner = sshBanner
					device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "ssh")

					// Extract OS info from SSH banner
					// Example: "SSH-2.0-OpenSSH_8.2p1 Ubuntu-4ubuntu0.5"
					if device.OperatingSystem == "" {
						if strings.Contains(sshBanner, "Ubuntu") {
							device.OperatingSystem = "Ubuntu Linux"
						} else if strings.Contains(sshBanner, "Debian") {
							device.OperatingSystem = "Debian Linux"
						} else if strings.Contains(sshBanner, "CentOS") {
							device.OperatingSystem = "CentOS Linux"
						} else if strings.Contains(sshBanner, "OpenSSH") {
							device.OperatingSystem = "Linux/Unix"
						}
					}
				}
				pending--
			case rdpHostname := <-rdpChan:
				if rdpHostname != "" {
					device.RDPHostname = rdpHostname
					device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "rdp")

					// RDP hostname is high priority for Windows systems
					if device.Hostname == "" {
						device.Hostname = rdpHostname
					}

					// Set OS to Windows if RDP is available
					if device.OperatingSystem == "" {
						device.OperatingSystem = "Windows"
					}
				}
				pending--
			case <-detailCtx.Done():
				pending = 0
			}
		}

	}

	// Fallback: If no hostname found, use IP address
	// Advanced IP Scanner shows IP when hostname unavailable (not MAC-based fake names)
	if device.Hostname == "" {
		device.Hostname = device.IP
	}

	if !device.Reachable {
		device.ProbeMethod = "timeout"
		device.LatencyMs = opts.Timeout.Milliseconds()
	}

	// Add additional discovery sources
	if device.MACAddress != "" {
		device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "arp")
	}
	if device.WebServer != "" || device.HTTPTitle != "" {
		device.DiscoverySources = appendIfNotExists(device.DiscoverySources, "http")
	}

	if device.IsLocal {
		if vendor := localDeviceVendorName(); vendor != "" {
			device.Vendor = vendor
		}
	}

	// Calculate confidence score
	device.ConfidenceScore = calculateConfidenceScore(&device)

	return device
}

func runPingProbe(ctx context.Context, ip string, timeout time.Duration) (bool, time.Duration, string) {
	args := buildPingArgs(ip, timeout)

	pingCtx, cancel := context.WithTimeout(ctx, timeout+250*time.Millisecond)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(pingCtx, "ping", args...)
	if err := cmd.Run(); err == nil {
		return true, time.Since(start), "ping"
	} else if errors.Is(err, exec.ErrNotFound) {
		return runTCPProbe(ctx, ip, timeout)
	}

	if pingCtx.Err() == context.DeadlineExceeded {
		return false, timeout, "ping"
	}

	return runTCPProbe(ctx, ip, timeout)
}

func runPortsProbe(ctx context.Context, ip string, timeout time.Duration, ports []int) (bool, time.Duration, int) {
	if len(ports) == 0 {
		return false, timeout, 0
	}

	for _, port := range ports {
		select {
		case <-ctx.Done():
			return false, timeout, 0
		default:
		}

		dialer := &net.Dialer{Timeout: timeout}
		address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
		start := time.Now()
		conn, err := dialer.DialContext(ctx, "tcp", address)
		elapsed := time.Since(start)
		if err == nil {
			conn.Close()
			return true, elapsed, port
		}

		if isConnRefused(err) {
			return true, elapsed, port
		}
	}

	return false, timeout, 0
}

func buildPingArgs(ip string, timeout time.Duration) []string {
	timeoutMs := int(timeout.Milliseconds())
	if timeoutMs < 200 {
		timeoutMs = 200
	}

	switch runtime.GOOS {
	case "windows":
		return []string{"-n", "1", "-w", fmt.Sprintf("%d", timeoutMs), ip}
	case "darwin":
		return []string{"-c", "1", "-W", fmt.Sprintf("%d", timeoutMs), "-n", ip}
	default:
		seconds := int(math.Ceil(timeout.Seconds()))
		if seconds < 1 {
			seconds = 1
		}
		return []string{"-c", "1", "-W", fmt.Sprintf("%d", seconds), "-n", ip}
	}
}

func runTCPProbe(ctx context.Context, ip string, timeout time.Duration) (bool, time.Duration, string) {
	// Aggressiveport list for maximum detection
	ports := []int{
		445, 139, 135, // Windows SMB/NetBIOS/RPC (most common for workstations)
		80, 443, 8080, 8443, // Web servers
		22, 3389, // SSH, RDP
		21, 23, // FTP, Telnet
		3306, 5432, 1433, // MySQL, PostgreSQL, MSSQL
		25, 110, 143, // Email
		53,         // DNS
		161,        // SNMP
		5900,       // VNC
		8000, 8888, // Alternative HTTP
	}

	// Try ports in parallel for speed
	resultChan := make(chan struct {
		success bool
		elapsed time.Duration
		port    int
	}, len(ports))

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for _, port := range ports {
		go func(p int) {
			dialer := &net.Dialer{Timeout: timeout}
			address := net.JoinHostPort(ip, fmt.Sprintf("%d", p))
			start := time.Now()
			conn, err := dialer.DialContext(probeCtx, "tcp", address)
			elapsed := time.Since(start)

			if err == nil {
				conn.Close()
				resultChan <- struct {
					success bool
					elapsed time.Duration
					port    int
				}{true, elapsed, p}
				return
			}

			// Connection refused means host is UP but port closed
			if isConnRefused(err) {
				resultChan <- struct {
					success bool
					elapsed time.Duration
					port    int
				}{true, elapsed, p}
				return
			}

			resultChan <- struct {
				success bool
				elapsed time.Duration
				port    int
			}{false, elapsed, p}
		}(port)
	}

	// Wait for first successful response or all to complete
	successCount := 0
	for i := 0; i < len(ports); i++ {
		select {
		case result := <-resultChan:
			if result.success {
				return true, result.elapsed, fmt.Sprintf("tcp:%d", result.port)
			}
			successCount++
		case <-probeCtx.Done():
			return false, timeout, "tcp"
		}
	}

	return false, timeout, "tcp"
}

func isConnRefused(err error) bool {
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return strings.Contains(strings.ToLower(err.Error()), "refused")
	}

	if errors.Is(opErr.Err, syscall.ECONNREFUSED) {
		return true
	}

	var sysErr *os.SyscallError
	if errors.As(opErr.Err, &sysErr) {
		if errno, ok := sysErr.Err.(syscall.Errno); ok {
			return errno == syscall.ECONNREFUSED
		}
	}

	if errno, ok := opErr.Err.(syscall.Errno); ok {
		return errno == syscall.ECONNREFUSED
	}

	return strings.Contains(strings.ToLower(opErr.Err.Error()), "refused")
}

func reverseLookup(ctx context.Context, ip string, timeout time.Duration) string {
	lookupCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	names, err := net.DefaultResolver.LookupAddr(lookupCtx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}

	return strings.TrimSuffix(names[0], ".")
}

func resolveHostname(ctx context.Context, ip string) string {
	if name := reverseLookup(ctx, ip, 600*time.Millisecond); name != "" {
		return name
	}

	if info := netbiosLookup(ctx, ip); info.Hostname != "" {
		return info.Hostname
	}

	if name := nslookupHostname(ctx, ip); name != "" {
		return name
	}

	return ""
}

type netbiosResult struct {
	Hostname string
	MAC      string
}

func netbiosLookup(ctx context.Context, ip string) netbiosResult {
	var cmd string
	var args []string

	if runtime.GOOS == "windows" {
		cmd = "nbtstat"
		args = []string{"-A", ip}
	} else {
		cmd = "nmblookup"
		args = []string{"-A", ip}
	}

	output, err := runCommand(ctx, 750*time.Millisecond, cmd, args...)
	if err != nil {
		return netbiosResult{}
	}

	return parseNetbiosOutput(output)
}

func nslookupHostname(ctx context.Context, ip string) string {
	output, err := runCommand(ctx, 750*time.Millisecond, "nslookup", ip)
	if err != nil {
		return ""
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "name:") || strings.HasPrefix(strings.ToLower(line), "ad:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				host := cleanHostname(parts[1])
				if host != "" {
					return host
				}
			}
		}
		if strings.Contains(line, "name =") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				host := cleanHostname(parts[1])
				if host != "" {
					return host
				}
			}
		}
	}

	return ""
}

// tryMDNSLookup attempts to resolve hostname using mDNS (multicast DNS)
func tryMDNSLookup(ctx context.Context, ip string) string {
	// Try reverse lookup with .local domain
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return ""
	}

	// Convert IP to PTR format for mDNS
	octets := strings.Split(addr.String(), ".")
	if len(octets) != 4 {
		return ""
	}

	// Try standard mDNS query
	lookupCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	// Use net.LookupAddr which might find mDNS entries
	names, err := net.DefaultResolver.LookupAddr(lookupCtx, ip)
	if err == nil && len(names) > 0 {
		for _, name := range names {
			cleaned := cleanHostname(name)
			if cleaned != "" && !strings.Contains(cleaned, "in-addr.arpa") {
				return cleaned
			}
		}
	}

	return ""
}

// probeLLMNRName attempts to resolve hostname using LLMNR (UDP 5355)
func probeLLMNRName(ctx context.Context, ip string) string {
	reversePtr, ok := ipv4PtrName(ip)
	if !ok {
		return ""
	}

	name, err := dnsmessage.NewName(reversePtr)
	if err != nil {
		return ""
	}

	header := dnsmessage.Header{
		ID:               uint16(rand.Uint32()),
		RecursionDesired: true,
	}

	builder := dnsmessage.NewBuilder(nil, header)
	builder.EnableCompression()
	if err := builder.StartQuestions(); err != nil {
		return ""
	}
	if err := builder.Question(dnsmessage.Question{
		Name:  name,
		Type:  dnsmessage.TypePTR,
		Class: dnsmessage.ClassINET,
	}); err != nil {
		return ""
	}

	msg, err := builder.Finish()
	if err != nil {
		return ""
	}

	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.ParseIP("224.0.0.252"),
		Port: 5355,
	})
	if err != nil {
		return ""
	}
	defer conn.Close()

	deadline := time.Now().Add(1200 * time.Millisecond)
	conn.SetDeadline(deadline)
	if _, err := conn.Write(msg); err != nil {
		return ""
	}

	buf := make([]byte, 1500)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil || n == 0 {
		return ""
	}

	var parser dnsmessage.Parser
	if _, err := parser.Start(buf[:n]); err != nil {
		return ""
	}
	if err := parser.SkipAllQuestions(); err != nil {
		return ""
	}

	for {
		header, err := parser.AnswerHeader()
		if errors.Is(err, dnsmessage.ErrSectionDone) {
			break
		}
		if err != nil {
			return ""
		}

		if header.Type != dnsmessage.TypePTR {
			if err := parser.SkipAnswer(); err != nil {
				return ""
			}
			continue
		}

		resource, err := parser.PTRResource()
		if err != nil {
			return ""
		}

		ptr := resource.PTR.String()
		ptr = strings.TrimSuffix(ptr, ".")
		if ptr != "" && !strings.Contains(ptr, "in-addr.arpa") {
			return ptr
		}
	}

	return ""
}

func parseNetbiosOutput(output string) netbiosResult {
	result := netbiosResult{}
	lines := strings.Split(output, "\n")
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		upper := strings.ToUpper(line)

		if result.Hostname == "" {
			if strings.Contains(line, "<00>") && strings.Contains(upper, "UNIQUE") {
				fields := strings.Fields(line)
				if len(fields) > 0 && !strings.HasPrefix(fields[0], "__") {
					if host := cleanHostname(fields[0]); host != "" {
						result.Hostname = host
					}
				}
			}

			if result.Hostname == "" && strings.HasPrefix(strings.ToLower(line), "host name") {
				parts := strings.Split(line, "=")
				if len(parts) == 2 {
					if host := cleanHostname(parts[1]); host != "" {
						result.Hostname = host
					}
				}
			}
		}

		if result.MAC == "" && strings.Contains(upper, "MAC ADDRESS") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				mac := strings.TrimSpace(parts[1])
				if isMACAddress(mac) {
					normalized := normalizeMACAddress(mac)
					if !isZeroMAC(normalized) {
						result.MAC = normalized
					}
				}
			}
		}

		if result.Hostname != "" && result.MAC != "" {
			break
		}
	}

	return result
}

func runCommand(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := exec.CommandContext(cmdCtx, name, args...).CombinedOutput()
	if err != nil {
		return "", err
	}

	return string(out), nil
}

func cleanHostname(raw string) string {
	host := strings.TrimSpace(raw)
	host = strings.Trim(host, ".")
	if host == "" || strings.EqualFold(host, "(unknown)") || strings.EqualFold(host, "unknown") {
		return ""
	}
	return host
}

func scanHosts(ctx context.Context, hosts []netip.Addr, label string, opts Options) (*Result, error) {
	if len(hosts) == 0 {
		return nil, errors.New("no hosts to scan")
	}

	opts.normalize()
	localIPs := mapLocalIPv4WithMAC()
	initialARP := snapshotARPTable()
	start := time.Now()

	jobs := make(chan netip.Addr)
	var wg sync.WaitGroup
	var mu sync.Mutex
	devices := make([]DeviceInfo, 0, len(hosts))
	reachable := 0

	worker := func() {
		defer wg.Done()
		for addr := range jobs {
			device := probeAddress(ctx, addr, opts, localIPs, initialARP[addr.String()])
			// Include device if:
			// 1. IncludeUnreachable is true, OR
			// 2. Device is reachable, OR
			// 3. Device has MAC address (exists in ARP table)
			if !opts.IncludeUnreachable && !device.Reachable && device.MACAddress == "" {
				continue
			}
			mu.Lock()
			devices = append(devices, device)
			if device.Reachable {
				reachable++
			}
			mu.Unlock()
		}
	}

	for i := 0; i < opts.Workers; i++ {
		wg.Add(1)
		go worker()
	}

loop:
	for _, ip := range hosts {
		select {
		case <-ctx.Done():
			break loop
		case jobs <- ip:
		}
	}
	close(jobs)
	wg.Wait()

	sort.Slice(devices, func(i, j int) bool {
		left, _ := netip.ParseAddr(devices[i].IP)
		right, _ := netip.ParseAddr(devices[j].IP)
		return left.Less(right)
	})

	return &Result{
		Subnet:         label,
		HostCount:      len(hosts),
		ReachableCount: reachable,
		ScannedCount:   len(devices),
		Duration:       time.Since(start),
		Devices:        devices,
	}, nil
}

func enumerateRange(start, end netip.Addr, limit int) ([]netip.Addr, error) {
	startInt := addrToUint32(start)
	endInt := addrToUint32(end)
	if endInt < startInt {
		return nil, errors.New("range end must be greater than or equal to start")
	}

	count := int(endInt-startInt) + 1
	if limit > 0 && count > limit {
		return nil, fmt.Errorf("range contains %d addresses, limit %d", count, limit)
	}

	hosts := make([]netip.Addr, 0, count)
	for cur := startInt; cur <= endInt; cur++ {
		hosts = append(hosts, uint32ToAddr(cur))
	}
	return hosts, nil
}

func addrToUint32(addr netip.Addr) uint32 {
	bytes := addr.As4()
	return uint32(bytes[0])<<24 | uint32(bytes[1])<<16 | uint32(bytes[2])<<8 | uint32(bytes[3])
}

func uint32ToAddr(v uint32) netip.Addr {
	b := [4]byte{
		byte(v >> 24),
		byte(v >> 16),
		byte(v >> 8),
		byte(v),
	}
	return netip.AddrFrom4(b)
}

func isPrivateIPv4(ip net.IP) bool {
	if len(ip) < 2 {
		return false
	}

	switch {
	case ip[0] == 10:
		return true
	case ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31:
		return true
	case ip[0] == 192 && ip[1] == 168:
		return true
	default:
		return false
	}
}

// getMACFromARP tries to get MAC address from ARP table with retry logic
func getMACFromARP(ctx context.Context, ip string) string {
	// Try up to 3 times with small delays to allow ARP cache to populate
	maxAttempts := 3
	delayBetweenAttempts := 50 * time.Millisecond

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			// Wait a bit for ARP cache to populate after ping
			select {
			case <-ctx.Done():
				return ""
			case <-time.After(delayBetweenAttempts):
			}
		}

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "arp", "-a", ip)
		} else {
			cmd = exec.CommandContext(ctx, "arp", "-n", ip)
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			continue
		}

		mac := parseARPOutput(string(output), ip)
		if mac != "" {
			return mac
		}

		// On first attempt failure, try to populate ARP cache
		if attempt == 0 {
			triggerARPRequest(ctx, ip)
		}
	}

	return ""
}

// getMACFromARPAggressive is more aggressive version with multiple trigger attempts
func getMACFromARPAggressive(ctx context.Context, ip string) string {
	// First, try to trigger ARP immediately
	triggerARPMultipleMethods(ctx, ip)

	// Wait a bit for ARP to populate
	time.Sleep(80 * time.Millisecond)

	// Try up to 4 times with increasing delays
	maxAttempts := 4
	delays := []time.Duration{0, 50 * time.Millisecond, 80 * time.Millisecond, 120 * time.Millisecond}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 && attempt < len(delays) {
			select {
			case <-ctx.Done():
				return ""
			case <-time.After(delays[attempt]):
			}
		}

		// Read FULL ARP table (not filtered by IP)
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(ctx, "arp", "-a")
		} else {
			cmd = exec.CommandContext(ctx, "arp", "-n")
		}

		output, err := cmd.CombinedOutput()
		if err != nil {
			continue
		}

		// Parse and get ALL MACs for this IP (there might be multiple interfaces)
		mac := parseARPOutput(string(output), ip)
		if mac != "" {
			return mac
		}

		// On middle attempts, try more triggers
		if attempt == 1 || attempt == 2 {
			triggerARPRequest(ctx, ip)
		}
	}

	return ""
}

// triggerARPRequest sends a packet to trigger ARP cache population
func triggerARPRequest(ctx context.Context, ip string) {
	// Try a quick TCP connection on common ports to trigger ARP
	ports := []int{445, 139, 135, 80, 443}

	for _, port := range ports {
		select {
		case <-ctx.Done():
			return
		default:
		}

		dialer := &net.Dialer{Timeout: 100 * time.Millisecond}
		address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
		conn, err := dialer.DialContext(ctx, "tcp", address)
		if err == nil {
			conn.Close()
			return // Success, ARP should be populated now
		}
	}
}

// triggerARPMultipleMethods uses multiple techniques to populate ARP cache
// Advanced IP Scanner methodology - prioritize ports by OS type
func triggerARPMultipleMethods(ctx context.Context, ip string) {
	// Priority ports matching Advanced IP Scanner's methodology
	priorityPorts := []int{
		// Apple/macOS specific (HIGHEST PRIORITY for Apple devices)
		62078, // iOS/macOS AirPlay
		548,   // AFP (Apple Filing Protocol)
		5353,  // mDNS/Bonjour (critical for Apple)
		5900,  // VNC (Screen Sharing - common on Mac)
		88,    // Kerberos (macOS Server)

		// Windows (HIGH PRIORITY)
		445, 139, 137, // SMB/NetBIOS
		135,  // RPC
		3389, // RDP

		// Universal/Web (MEDIUM PRIORITY)
		80, 443, 8080, 8443, // HTTP/HTTPS

		// Linux/Unix (MEDIUM PRIORITY)
		22, // SSH

		// Network Devices (LOWER PRIORITY)
		23,   // Telnet
		161,  // SNMP
		8291, // MikroTik
		9100, // HP JetDirect (printers)
	}

	// Try priority ports first with very short timeout
	done := make(chan bool, 1)

	for _, port := range priorityPorts {
		select {
		case <-ctx.Done():
			return
		case <-done:
			return
		default:
		}

		go func(p int) {
			dialer := &net.Dialer{Timeout: 80 * time.Millisecond}
			address := net.JoinHostPort(ip, fmt.Sprintf("%d", p))
			if conn, err := dialer.DialContext(ctx, "tcp", address); err == nil {
				conn.Close()
				select {
				case done <- true:
				default:
				}
			}
		}(port)
	}

	// Wait max 250ms for any port to respond
	select {
	case <-done:
		return
	case <-time.After(250 * time.Millisecond):
	case <-ctx.Done():
		return
	}

	// If no port responded, try ICMP ping
	if runtime.GOOS == "windows" || runtime.GOOS == "linux" {
		pingCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
		defer cancel()
		args := buildPingArgs(ip, 200*time.Millisecond)
		exec.CommandContext(pingCtx, "ping", args...).Run()
	}
}

// parseARPOutput parses ARP command output to extract MAC address
func parseARPOutput(output, targetIP string) string {
	lines := strings.Split(output, "\n")
	var candidates []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, targetIP) {
			continue
		}

		// Windows format: IP address       Physical Address      Type
		// Linux format:   Address          HWtype  HWaddress         Flags Mask Iface

		// Match MAC address patterns: XX:XX:XX:XX:XX:XX or XX-XX-XX-XX-XX-XX
		fields := strings.Fields(line)
		for _, field := range fields {
			// Clean and normalize MAC address
			mac := strings.ToUpper(strings.TrimSpace(field))

			// Check if it matches MAC address pattern
			if isMACAddress(mac) {
				normalized := normalizeMACAddress(mac)
				if isZeroMAC(normalized) || isVirtualMAC(normalized) {
					continue
				}
				candidates = append(candidates, normalized)
			}
		}
	}

	// Prefer non-virtual MAC addresses
	// If we have multiple candidates, pick the first non-virtual one
	for _, mac := range candidates {
		if !isVirtualMAC(mac) {
			return mac
		}
	}

	// If all are virtual, return first one
	if len(candidates) > 0 {
		return candidates[0]
	}

	return ""
}

// isVirtualMAC checks if a MAC address belongs to a virtual/emulated adapter
func isVirtualMAC(mac string) bool {
	if len(mac) < 8 {
		return false
	}

	// Get OUI (first 3 octets)
	oui := strings.ToUpper(mac[:8])

	// Common virtual adapter OUI prefixes
	virtualOUIs := []string{
		"00:05:69", // VMware
		"00:0C:29", // VMware
		"00:1C:14", // VMware
		"00:50:56", // VMware
		"08:00:27", // VirtualBox
		"52:54:00", // QEMU/KVM
		"00:1C:42", // Parallels
		"00:15:5D", // Hyper-V
		"00:16:3E", // Xen
		"00:03:FF", // Microsoft Virtual PC
		"02:00:4C", // QEMU user mode
		"00:21:F6", // Virtual Iron
		"00:14:4F", // Virtual Iron
	}

	for _, virtualOUI := range virtualOUIs {
		if oui == virtualOUI {
			return true
		}
	}

	// Check for locally administered addresses (LAA)
	// Second character (nibble) bit 1 set indicates LAA
	// This catches many virtual/random MACs
	if len(mac) >= 2 {
		secondChar := mac[1:2]
		// Check if second hex digit has bit 1 set (2, 3, 6, 7, A, B, E, F)
		if strings.ContainsAny(secondChar, "236AEae") || strings.ContainsAny(secondChar, "7BFbf") {
			// This might be LAA, but also could be legitimate
			// Only filter if it's a known virtual OUI pattern
			return false // Don't filter LAA by default, too many false positives
		}
	}

	return false
}

func parseARPTable(output string) map[string]string {
	lines := strings.Split(output, "\n")
	result := make(map[string]string)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		ip := ""
		mac := ""
		for _, field := range fields {
			if ip == "" && net.ParseIP(strings.TrimSpace(field)) != nil {
				ip = strings.TrimSpace(field)
				continue
			}
			clean := normalizeMACAddress(field)
			if isMACAddress(clean) && !isZeroMAC(clean) {
				mac = clean
			}
		}

		if ip != "" && mac != "" {
			result[ip] = mac
		}
	}

	return result
}

// isMACAddress checks if string is a valid MAC address format
func isMACAddress(s string) bool {
	// XX:XX:XX:XX:XX:XX or XX-XX-XX-XX-XX-XX
	if len(s) != 17 {
		return false
	}

	// Check pattern
	for i, c := range s {
		if i%3 == 2 {
			if c != ':' && c != '-' {
				return false
			}
		} else {
			if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')) {
				return false
			}
		}
	}

	return true
}

// normalizeMACAddress converts MAC to standard format (XX:XX:XX:XX:XX:XX)
func normalizeMACAddress(mac string) string {
	mac = strings.ReplaceAll(mac, "-", ":")
	return strings.ToUpper(mac)
}

func isZeroMAC(mac string) bool {
	cleaned := strings.ToUpper(strings.TrimSpace(mac))
	cleaned = strings.ReplaceAll(cleaned, "-", ":")
	return cleaned == "00:00:00:00:00:00"
}

var macPrefixCleaner = strings.NewReplacer(":", "", "-", "", ".", "", " ", "")

var (
	localVendorMu     sync.RWMutex
	localVendorMap    map[[3]byte]string
	localVendorLoaded bool
	vendorUpdateMu    sync.Mutex
)

func init() {
	go vendorAutoUpdater()
}

func lookupVendorFromEmbedded(mac string) string {
	clean := macPrefixCleaner.Replace(strings.TrimSpace(mac))
	if len(clean) < 6 {
		return ""
	}

	var prefix [3]byte
	for i := 0; i < 3; i++ {
		chunk := clean[i*2 : i*2+2]
		value, err := strconv.ParseUint(chunk, 16, 8)
		if err != nil {
			return ""
		}
		prefix[i] = byte(value)
	}

	if vendor, ok := macs.ValidMACPrefixMap[prefix]; ok {
		return strings.TrimSpace(vendor)
	}
	return ""
}

func lookupVendorFromGenerated(mac string) string {
	clean := macPrefixCleaner.Replace(strings.TrimSpace(mac))
	if len(clean) < 6 || generatedOUIData == nil {
		return ""
	}

	var prefix [3]byte
	for i := 0; i < 3; i++ {
		chunk := clean[i*2 : i*2+2]
		value, err := strconv.ParseUint(chunk, 16, 8)
		if err != nil {
			return ""
		}
		prefix[i] = byte(value)
	}

	if vendor, ok := generatedOUIData[prefix]; ok {
		return strings.TrimSpace(vendor)
	}
	return ""
}

func lookupVendorFromLocalFile(mac string) string {
	db := ensureLocalVendorMap()
	if db == nil {
		return ""
	}

	clean := macPrefixCleaner.Replace(strings.TrimSpace(mac))
	if len(clean) < 6 {
		return ""
	}

	var prefix [3]byte
	for i := 0; i < 3; i++ {
		chunk := clean[i*2 : i*2+2]
		value, err := strconv.ParseUint(chunk, 16, 8)
		if err != nil {
			return ""
		}
		prefix[i] = byte(value)
	}

	if vendor, ok := db[prefix]; ok {
		return vendor
	}
	return ""
}

func ensureLocalVendorMap() map[[3]byte]string {
	localVendorMu.RLock()
	loaded := localVendorLoaded
	m := localVendorMap
	localVendorMu.RUnlock()
	if loaded {
		return m
	}

	localVendorMu.Lock()
	defer localVendorMu.Unlock()
	if localVendorLoaded {
		return localVendorMap
	}

	paths := vendorLookupPaths()
	for _, path := range paths {
		if m, err := loadVendorMapFrom(path); err == nil {
			localVendorMap = m
			localVendorLoaded = true
			return localVendorMap
		}
	}

	localVendorLoaded = true
	return localVendorMap
}

func vendorLookupPaths() []string {
	var paths []string

	// Priority 1: Embedded data/oui.txt (for Docker/production)
	paths = append(paths, defaultOUIFile)

	// Priority 2: User cache directory (for development/updates)
	if path, _ := preferredOUIPath(); path != "" {
		paths = append(paths, path)
	}

	return paths
}

func loadVendorMapFrom(path string) (map[[3]byte]string, error) {
	if path == "" {
		return nil, errors.New("empty path")
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, err
	}

	db, err := oui.OpenStaticFile(path)
	if err != nil {
		return nil, err
	}
	return convertOUIDBToMap(db)
}

func buildVendorMapFromBytes(data []byte) (map[[3]byte]string, error) {
	db, err := oui.OpenStatic(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return convertOUIDBToMap(db)
}

func convertOUIDBToMap(db oui.StaticDB) (map[[3]byte]string, error) {
	rawGetter, ok := db.(oui.RawGetter)
	if !ok {
		return nil, errors.New("oui db does not expose raw entries")
	}

	raw := rawGetter.RawDB()
	result := make(map[[3]byte]string, len(raw))
	for prefix, entry := range raw {
		vendor := strings.TrimSpace(entry.Manufacturer)
		if vendor == "" && len(entry.Address) > 0 {
			vendor = strings.TrimSpace(entry.Address[0])
		}
		if vendor == "" {
			vendor = strings.TrimSpace(entry.Country)
		}
		if vendor == "" {
			continue
		}
		result[prefix] = vendor
	}
	return result, nil
}

func setLocalVendorMap(m map[[3]byte]string) {
	localVendorMu.Lock()
	localVendorMap = m
	localVendorLoaded = true
	localVendorMu.Unlock()
}

func vendorAutoUpdater() {
	ensureLocalVendorMap()
	if !autoUpdateEnabled() {
		return
	}

	updateVendorCacheIfNeeded(true)

	ticker := time.NewTicker(ouiUpdateCheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		updateVendorCacheIfNeeded(false)
	}
}

func autoUpdateEnabled() bool {
	val := strings.TrimSpace(os.Getenv(ouiAutoUpdateEnv))
	if val != "" {
		switch strings.ToLower(val) {
		case "0", "false", "no":
			return false
		case "1", "true", "yes":
			return true
		}
	}
	_, manageable := preferredOUIPath()
	return manageable
}

func updateVendorCacheIfNeeded(force bool) {
	path, manageable := preferredOUIPath()
	if path == "" || !manageable {
		return
	}

	vendorUpdateMu.Lock()
	defer vendorUpdateMu.Unlock()

	needUpdate := force
	if !needUpdate {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			needUpdate = true
		} else if time.Since(info.ModTime()) >= ouiUpdateInterval {
			needUpdate = true
		}
	}

	if !needUpdate {
		if !localVendorLoaded {
			if m, err := loadVendorMapFrom(path); err == nil {
				setLocalVendorMap(m)
			}
		}
		return
	}

	data, err := downloadOUIData()
	if err != nil {
		log.Printf("ip-scanner: OUI download failed: %v", err)
		return
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("ip-scanner: unable to create OUI cache directory: %v", err)
		return
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("ip-scanner: unable to write OUI cache temp file: %v", err)
		return
	}

	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		log.Printf("ip-scanner: unable to move OUI cache file: %v", err)
		return
	}

	if m, err := buildVendorMapFromBytes(data); err == nil {
		setLocalVendorMap(m)
		log.Printf("ip-scanner: refreshed OUI database (%d prefixes)", len(m))
	}
}

func downloadOUIData() ([]byte, error) {
	// Try primary URL first, then fallback URL if it fails
	urls := []string{vendorDownloadURL(), fallbackOUIURL}

	client := &http.Client{Timeout: 60 * time.Second}
	var lastErr error

	for i, url := range urls {
		// Create request with proper User-Agent header
		// IEEE servers may block requests without valid User-Agent (HTTP 418)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", "Systrack-Network-Scanner/1.0 (+https://github.com/yourusername/systrack)")
		req.Header.Set("Accept", "text/plain, */*")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if i < len(urls)-1 {
				log.Printf("ip-scanner: OUI download from %s failed: %v, trying fallback...", url, err)
			}
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("unexpected status %d %s", resp.StatusCode, resp.Status)
			if i < len(urls)-1 {
				log.Printf("ip-scanner: OUI download from %s failed: %v, trying fallback...", url, lastErr)
			}
			continue
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			if i < len(urls)-1 {
				log.Printf("ip-scanner: OUI download from %s failed: %v, trying fallback...", url, err)
			}
			continue
		}

		// Success
		if i > 0 {
			log.Printf("ip-scanner: OUI downloaded successfully from fallback source")
		}
		return data, nil
	}

	return nil, fmt.Errorf("all OUI download sources failed, last error: %w", lastErr)
}

func vendorDownloadURL() string {
	if url := strings.TrimSpace(os.Getenv(ouiUpdateURLEnv)); url != "" {
		return url
	}
	legacy := strings.TrimSpace(os.Getenv(ouiLegacyURLEnv))
	if legacy != "" {
		lower := strings.ToLower(legacy)
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			return legacy
		}
	}
	return defaultOUIURL
}

func preferredOUIPath() (string, bool) {
	override := strings.TrimSpace(os.Getenv(ouiFileEnv))
	if override != "" {
		return override, false
	}

	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	if dir == "" {
		return "", false
	}
	return filepath.Join(dir, "systrack", "oui.txt"), true
}

// getVendorFromMAC looks up vendor name from MAC address OUI prefix
func getVendorFromMAC(macAddress string) string {
	if macAddress == "" {
		return ""
	}

	if vendor := lookupVendorFromLocalFile(macAddress); vendor != "" {
		return vendor
	}
	if vendor := lookupVendorFromGenerated(macAddress); vendor != "" {
		return vendor
	}

	if vendor := lookupVendorFromEmbedded(macAddress); vendor != "" {
		return vendor
	}

	return ""
}

// probeHTTPServer attempts to detect web server and extract information
func probeHTTPServer(ctx context.Context, ip string, timeout time.Duration) (webServer string, title string) {
	// Quick parallel check of common ports
	type portResult struct {
		server string
		title  string
	}

	resultChan := make(chan portResult, 1)
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Try HTTPS (443) and HTTP (80) in parallel
	go func() {
		if server, pageTitle := tryHTTPSProbe(checkCtx, ip, 443, timeout/2); server != "" || pageTitle != "" {
			select {
			case resultChan <- portResult{server, pageTitle}:
			default:
			}
		}
	}()

	go func() {
		if server, pageTitle := tryHTTPProbe(checkCtx, ip, 80, timeout/2); server != "" || pageTitle != "" {
			select {
			case resultChan <- portResult{server, pageTitle}:
			default:
			}
		}
	}()

	// Wait for first result or timeout
	select {
	case result := <-resultChan:
		return result.server, result.title
	case <-checkCtx.Done():
		return "", ""
	}
}

// tryHTTPSProbe tries HTTPS connection
func tryHTTPSProbe(ctx context.Context, ip string, port int, timeout time.Duration) (string, string) {
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // Skip cert verification for scanning
			},
			DisableKeepAlives: true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	url := fmt.Sprintf("https://%s:%d/", ip, port)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", ""
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; NetworkScanner/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	server := resp.Header.Get("Server")
	title := extractHTMLTitle(resp.Body)

	return server, title
}

// tryHTTPProbe tries HTTP connection
func tryHTTPProbe(ctx context.Context, ip string, port int, timeout time.Duration) (string, string) {
	client := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DisableKeepAlives: true,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	url := fmt.Sprintf("http://%s:%d/", ip, port)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", ""
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; NetworkScanner/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()

	server := resp.Header.Get("Server")
	title := extractHTMLTitle(resp.Body)

	return server, title
}

// probeTLSNames inspects TLS certificates to extract potential hostnames/FQDNs
func probeTLSNames(ctx context.Context, ip string) []string {
	ports := []int{443, 8443, 9443}
	seen := make(map[string]struct{})
	var names []string

	addName := func(raw string) {
		if raw == "" {
			return
		}
		name := extractHostnameOnly(raw)
		if name == "" {
			return
		}
		lower := strings.ToLower(name)
		if _, exists := seen[lower]; exists {
			return
		}
		seen[lower] = struct{}{}
		names = append(names, name)
	}

	for _, port := range ports {
		select {
		case <-ctx.Done():
			return names
		default:
		}

		dialer := &net.Dialer{Timeout: 900 * time.Millisecond}
		address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
		conn, err := tls.DialWithDialer(dialer, "tcp", address, &tls.Config{
			InsecureSkipVerify: true,
		})
		if err != nil {
			continue
		}

		state := conn.ConnectionState()
		conn.Close()

		if len(state.PeerCertificates) == 0 {
			continue
		}

		for _, cert := range state.PeerCertificates {
			addName(cert.Subject.CommonName)
			for _, dnsName := range cert.DNSNames {
				addName(dnsName)
			}
		}

		if len(names) > 0 {
			break
		}
	}

	return names
}

// extractHTMLTitle extracts the <title> tag content from HTML
func extractHTMLTitle(body io.Reader) string {
	// Read first 8KB (title should be in header)
	buf := make([]byte, 8192)
	n, _ := body.Read(buf)
	if n == 0 {
		return ""
	}

	html := string(buf[:n])
	html = strings.ToLower(html)

	// Find <title> tag
	titleStart := strings.Index(html, "<title")
	if titleStart == -1 {
		return ""
	}

	// Find closing > of opening tag
	contentStart := strings.Index(html[titleStart:], ">")
	if contentStart == -1 {
		return ""
	}
	contentStart += titleStart + 1

	// Find </title>
	titleEnd := strings.Index(html[contentStart:], "</title>")
	if titleEnd == -1 {
		return ""
	}

	// Extract and clean title
	title := html[contentStart : contentStart+titleEnd]
	title = strings.TrimSpace(title)

	// Decode HTML entities and clean up
	title = strings.ReplaceAll(title, "&nbsp;", " ")
	title = strings.ReplaceAll(title, "&amp;", "&")
	title = strings.ReplaceAll(title, "&lt;", "<")
	title = strings.ReplaceAll(title, "&gt;", ">")
	title = strings.ReplaceAll(title, "&quot;", "\"")

	// Limit length
	if len(title) > 100 {
		title = title[:100] + "..."
	}

	return title
}

// ============================================================================
// SMB/CIFS Discovery Functions
// ============================================================================

// SMBInfo contains information discovered via SMB/CIFS protocol
type SMBInfo struct {
	Hostname    string
	Workgroup   string
	Domain      string
	OSVersion   string
	IsAvailable bool
}

// probeSMB attempts to discover information via SMB/CIFS protocol
func probeSMB(ctx context.Context, ip string) SMBInfo {
	result := SMBInfo{}

	// Try SMB ports in order of preference
	ports := []int{445, 139}

	for _, port := range ports {
		select {
		case <-ctx.Done():
			return result
		default:
		}

		// Try to establish SMB connection
		address := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
		dialer := &net.Dialer{Timeout: 800 * time.Millisecond}

		conn, err := dialer.DialContext(ctx, "tcp", address)
		if err != nil {
			continue
		}

		// Set deadline for the entire operation
		conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))

		// Try to get NetBIOS name via SMB
		if info := probeSMBConnection(conn, ip); info.Hostname != "" {
			result = info
			result.IsAvailable = true
			conn.Close()
			break
		}

		conn.Close()
	}

	return result
}

// probeSMBConnection attempts to extract information from an established SMB connection
func probeSMBConnection(conn net.Conn, ip string) SMBInfo {
	result := SMBInfo{}

	// SMB1 Negotiate Protocol Request
	// This is a minimal SMB1 negotiation to trigger a response with server info
	negotiateRequest := []byte{
		0x00, 0x00, 0x00, 0x85, // NetBIOS Session Service header
		0xff, 0x53, 0x4d, 0x42, // SMB magic "\xffSMB"
		0x72,                   // Negotiate Protocol
		0x00, 0x00, 0x00, 0x00, // Status
		0x18,       // Flags
		0x53, 0xc8, // Flags2
		0x00, 0x00, // PID High
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Signature
		0x00, 0x00, // Reserved
		0x00, 0x00, // TID
		0x2f, 0x4b, // PID
		0x00, 0x00, // UID
		0x00, 0xc0, // MID
		0x00,       // Word Count
		0x62, 0x00, // Byte Count
		// Dialect strings
		0x02, 0x50, 0x43, 0x20, 0x4e, 0x45, 0x54, 0x57, 0x4f, 0x52, 0x4b, 0x20, 0x50, 0x52, 0x4f, 0x47,
		0x52, 0x41, 0x4d, 0x20, 0x31, 0x2e, 0x30, 0x00,
		0x02, 0x4c, 0x41, 0x4e, 0x4d, 0x41, 0x4e, 0x31, 0x2e, 0x30, 0x00,
		0x02, 0x57, 0x69, 0x6e, 0x64, 0x6f, 0x77, 0x73, 0x20, 0x66, 0x6f, 0x72, 0x20, 0x57, 0x6f, 0x72,
		0x6b, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x73, 0x20, 0x33, 0x2e, 0x31, 0x61, 0x00,
		0x02, 0x4c, 0x4d, 0x31, 0x2e, 0x32, 0x58, 0x30, 0x30, 0x32, 0x00,
		0x02, 0x4c, 0x41, 0x4e, 0x4d, 0x41, 0x4e, 0x32, 0x2e, 0x31, 0x00,
		0x02, 0x4e, 0x54, 0x20, 0x4c, 0x4d, 0x20, 0x30, 0x2e, 0x31, 0x32, 0x00,
	}

	// Send negotiate request
	_, err := conn.Write(negotiateRequest)
	if err != nil {
		return result
	}

	// Read response
	response := make([]byte, 4096)
	n, err := conn.Read(response)
	if err != nil || n < 40 {
		return result
	}

	// Parse SMB response for server information
	// Look for NetBIOS name in the response
	if n > 40 && response[4] == 0xff && string(response[5:8]) == "SMB" {
		// Try to extract server name from various fields
		result.Hostname = extractSMBServerName(response[:n])
	}

	// If we didn't get hostname from SMB, try NetBIOS query as fallback
	if result.Hostname == "" {
		// Use existing netbiosLookup
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()
		if info := netbiosLookup(ctx, ip); info.Hostname != "" {
			result.Hostname = info.Hostname
		}
	}

	return result
}

// extractSMBServerName tries to extract server name from SMB response
func extractSMBServerName(response []byte) string {
	// This is a simplified parser - in production, you'd want more robust parsing

	// Look for Unicode strings in the response that might be the server name
	// Typically after byte offset 70-80 in negotiate response
	if len(response) < 80 {
		return ""
	}

	// Try to find printable ASCII/Unicode sequences that look like hostnames
	for i := 70; i < len(response)-20; i++ {
		if response[i] >= 'A' && response[i] <= 'Z' || response[i] >= 'a' && response[i] <= 'z' {
			// Found start of potential hostname
			hostname := extractPrintableString(response[i:])
			if len(hostname) >= 3 && len(hostname) <= 15 && isValidHostname(hostname) {
				return hostname
			}
		}
	}

	return ""
}

// extractPrintableString extracts a printable ASCII string
func extractPrintableString(data []byte) string {
	result := ""
	for i := 0; i < len(data) && i < 20; i++ {
		c := data[i]
		// Allow alphanumeric, hyphen, underscore
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result += string(c)
		} else if c == 0x00 {
			// Null terminator
			break
		} else if len(result) > 0 {
			// Non-printable character after we started collecting
			break
		}
	}
	return result
}

// isValidHostname checks if a string looks like a valid hostname
func isValidHostname(s string) bool {
	if len(s) < 1 || len(s) > 15 {
		return false
	}

	// Should not start or end with hyphen
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}

	// Should have at least one letter
	hasLetter := false
	for _, c := range s {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			hasLetter = true
			break
		}
	}

	return hasLetter
}

// probeNetBIOSName performs a direct NetBIOS Name Query (UDP port 137)
func probeNetBIOSName(ctx context.Context, ip string) string {
	// Create NetBIOS Name Query packet for wildcard query
	queryPacket := []byte{
		0x82, 0x28, // Transaction ID
		0x00, 0x00, // Flags (Standard query)
		0x00, 0x01, // Questions: 1
		0x00, 0x00, // Answer RRs: 0
		0x00, 0x00, // Authority RRs: 0
		0x00, 0x00, // Additional RRs: 0
		// Query for "*" (wildcard)
		0x20, // Length of encoded name
		0x43, 0x4b, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41,
		0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41, 0x41,
		0x00,       // Null terminator
		0x00, 0x21, // Type: NB (NetBIOS Name Service)
		0x00, 0x01, // Class: IN (Internet)
	}

	address := net.JoinHostPort(ip, "137")

	// Set up UDP connection with timeout
	dialer := &net.Dialer{Timeout: 1000 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "udp", address)
	if err != nil {
		return ""
	}
	defer conn.Close()

	// Set deadline
	conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))

	// Send query
	_, err = conn.Write(queryPacket)
	if err != nil {
		return ""
	}

	// Read response
	response := make([]byte, 512)
	n, err := conn.Read(response)
	if err != nil || n < 50 {
		return ""
	}

	// Parse NetBIOS response
	return parseNetBIOSNameQueryResponse(response[:n])
}

// parseNetBIOSNameQueryResponse parses NetBIOS Name Query response
func parseNetBIOSNameQueryResponse(response []byte) string {
	// Check if this is a valid response
	if len(response) < 56 {
		return ""
	}

	// Check response flags (should be 0x8400 or similar for positive response)
	flags := uint16(response[2])<<8 | uint16(response[3])
	if flags&0x8000 == 0 { // Not a response
		return ""
	}

	// Look for answer section
	// Skip header (12 bytes) and question section
	offset := 12

	// Skip question name
	for offset < len(response) && response[offset] != 0 {
		offset++
	}
	offset += 5 // Skip null terminator, type, and class

	if offset+10 > len(response) {
		return ""
	}

	// Now we're at the answer section
	// Skip answer name (usually compressed pointer)
	if offset < len(response) && (response[offset]&0xc0) == 0xc0 {
		offset += 2 // Skip compression pointer
	} else {
		// Skip regular name
		for offset < len(response) && response[offset] != 0 {
			offset++
		}
		offset++
	}

	// Skip type, class, TTL (10 bytes total)
	offset += 10

	if offset+2 > len(response) {
		return ""
	}

	// Read data length
	dataLen := uint16(response[offset])<<8 | uint16(response[offset+1])
	offset += 2

	if offset+int(dataLen) > len(response) {
		return ""
	}

	// Parse node names from response data
	numNames := int(response[offset])
	offset++

	for i := 0; i < numNames && offset+18 <= len(response); i++ {
		// Each entry is 18 bytes: 15 bytes name + 1 byte type + 2 bytes flags
		nameBytes := response[offset : offset+15]
		nameType := response[offset+15]

		// Type 0x00 = Workstation/Computer name
		// Type 0x20 = File Server service
		if nameType == 0x00 || nameType == 0x20 {
			name := strings.TrimSpace(string(nameBytes))
			name = strings.TrimRight(name, "\x00")
			if name != "" && !strings.HasPrefix(name, "__MSBROWSE__") && !strings.HasPrefix(name, "~") {
				return name
			}
		}

		offset += 18
	}

	return ""
}

// ============================================================================
// SNMP / LLDP Discovery Functions
// ============================================================================

type SNMPInfo struct {
	Available           bool
	Community           string
	SysName             string
	SysDescr            string
	SysObjectID         string
	LLDPSystemName      string
	LLDPPortDescription string
}

const (
	sysDescrOID      = ".1.3.6.1.2.1.1.1.0"
	sysObjectIDOID   = ".1.3.6.1.2.1.1.2.0"
	sysNameOID       = ".1.3.6.1.2.1.1.5.0"
	lldpSysNameOID   = ".1.0.8802.1.1.2.1.4.1.1.9"
	lldpPortDescOID  = ".1.0.8802.1.1.2.1.4.1.1.8"
	snmpCommunityEnv = "SYSTRACK_SNMP_COMMUNITIES"
	defaultCommunity = "public"
)

func probeSNMP(ctx context.Context, ip string) SNMPInfo {
	communities := getSNMPCommunities()
	if len(communities) == 0 {
		return SNMPInfo{}
	}

	timeout := 1200 * time.Millisecond

	for _, community := range communities {
		select {
		case <-ctx.Done():
			return SNMPInfo{}
		default:
		}

		if info, ok := attemptSNMPQuery(ip, community, timeout); ok {
			return info
		}
	}

	return SNMPInfo{}
}

func getSNMPCommunities() []string {
	envValue := strings.TrimSpace(os.Getenv(snmpCommunityEnv))
	if envValue == "-" {
		return nil
	}
	if envValue == "" {
		return []string{defaultCommunity}
	}

	parts := strings.Split(envValue, ",")
	var communities []string
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			communities = append(communities, value)
		}
	}

	return communities
}

func attemptSNMPQuery(ip, community string, timeout time.Duration) (SNMPInfo, bool) {
	client := &gosnmp.GoSNMP{
		Target:    ip,
		Port:      161,
		Community: community,
		Version:   gosnmp.Version2c,
		Timeout:   timeout,
		Retries:   0,
		MaxOids:   5,
	}

	if err := client.Connect(); err != nil {
		return SNMPInfo{}, false
	}
	defer func() {
		if client.Conn != nil {
			client.Conn.Close()
		}
	}()

	oids := []string{sysNameOID, sysDescrOID, sysObjectIDOID}
	packet, err := client.Get(oids)
	if err != nil || packet == nil || packet.Error != gosnmp.NoError {
		return SNMPInfo{}, false
	}

	info := SNMPInfo{
		Available: true,
		Community: community,
	}

	for _, variable := range packet.Variables {
		switch variable.Name {
		case sysNameOID:
			info.SysName = snmpPDUToString(variable)
		case sysDescrOID:
			info.SysDescr = snmpPDUToString(variable)
		case sysObjectIDOID:
			info.SysObjectID = snmpPDUToString(variable)
		}
	}

	if info.SysName == "" && info.SysDescr == "" {
		// Try SNMPv1 as fallback when v2 fails silently
		client.Version = gosnmp.Version1
		if packetV1, err := client.Get(oids); err == nil && packetV1 != nil && packetV1.Error == gosnmp.NoError {
			for _, variable := range packetV1.Variables {
				switch variable.Name {
				case sysNameOID:
					info.SysName = snmpPDUToString(variable)
				case sysDescrOID:
					info.SysDescr = snmpPDUToString(variable)
				case sysObjectIDOID:
					info.SysObjectID = snmpPDUToString(variable)
				}
			}
		}
	}

	// LLDP (optional)
	if name, port := queryLLDPInfo(client); name != "" || port != "" {
		info.LLDPSystemName = name
		info.LLDPPortDescription = port
	}

	if info.SysName == "" && info.SysDescr == "" && info.LLDPSystemName == "" && info.LLDPPortDescription == "" {
		return SNMPInfo{}, false
	}

	return info, true
}

func queryLLDPInfo(client *gosnmp.GoSNMP) (string, string) {
	if client == nil {
		return "", ""
	}

	sysName := walkFirstString(client, lldpSysNameOID)
	portDesc := walkFirstString(client, lldpPortDescOID)
	return sysName, portDesc
}

func walkFirstString(client *gosnmp.GoSNMP, oid string) string {
	if client == nil {
		return ""
	}

	vars, err := client.BulkWalkAll(oid)
	if err != nil {
		return ""
	}

	for _, variable := range vars {
		if value := snmpPDUToString(variable); value != "" {
			return value
		}
	}

	return ""
}

func snmpPDUToString(pdu gosnmp.SnmpPDU) string {
	switch pdu.Type {
	case gosnmp.OctetString:
		if data, ok := pdu.Value.([]byte); ok {
			return strings.TrimSpace(string(data))
		}
	case gosnmp.ObjectIdentifier:
		if oid, ok := pdu.Value.(string); ok {
			return strings.TrimSpace(oid)
		}
	case gosnmp.IPAddress:
		if ip, ok := pdu.Value.(string); ok {
			return strings.TrimSpace(ip)
		}
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", pdu.Value))
	}

	return ""
}

// ============================================================================
// FQDN Discovery Functions
// ============================================================================

// FQDNResult contains comprehensive DNS information about a device
type FQDNResult struct {
	FQDN             string
	Hostname         string
	Domain           string
	IsValidated      bool
	DiscoveryMethods []string
	SMBInfo          SMBInfo
}

// fqdnCandidate represents a potential FQDN discovered through a specific method
type fqdnCandidate struct {
	fqdn   string
	method string
}

// discoverFQDN performs comprehensive FQDN discovery for an IP address
func discoverFQDN(ctx context.Context, ip string) FQDNResult {
	result := FQDNResult{
		DiscoveryMethods: make([]string, 0),
	}

	// Channel for collecting results from different methods
	candidates := make(chan fqdnCandidate, 10)
	smbInfoChan := make(chan SMBInfo, 1)

	var wg sync.WaitGroup

	// 1. PTR Record Lookup (Reverse DNS)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if fqdn := lookupPTRRecord(ctx, ip); fqdn != "" {
			select {
			case candidates <- fqdnCandidate{fqdn, "ptr"}:
			case <-ctx.Done():
			}
		}
	}()

	// 2. DNS Server Query (System default only for local names)
	// Public DNS servers (Google, Cloudflare) don't know local hostnames
	// like "DC.zsistem.local", so we only use system default DNS
	dnsServers := []string{
		"", // System default (uses /etc/resolv.conf or Windows DNS settings)
	}

	for _, server := range dnsServers {
		wg.Add(1)
		go func(srv string) {
			defer wg.Done()
			if fqdn := queryDNSServer(ctx, ip, srv); fqdn != "" {
				method := "dns"
				if srv != "" {
					method = "dns:" + strings.Split(srv, ":")[0]
				}
				select {
				case candidates <- fqdnCandidate{fqdn, method}:
				case <-ctx.Done():
				}
			}
		}(server)
	}

	// 3. SMB/CIFS probe (most reliable for Windows)
	wg.Add(1)
	go func() {
		defer wg.Done()
		info := probeSMB(ctx, ip)
		if info.Hostname != "" {
			select {
			case candidates <- fqdnCandidate{info.Hostname, "smb"}:
			case <-ctx.Done():
			}
		}
		if info.IsAvailable {
			select {
			case smbInfoChan <- info:
			default:
			}
		}
	}()

	// 4. Direct NetBIOS Name Query (UDP 137)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if name := probeNetBIOSName(ctx, ip); name != "" {
			select {
			case candidates <- fqdnCandidate{name, "netbios-udp"}:
			case <-ctx.Done():
			}
		}
	}()

	// 5. NetBIOS lookup (command-line fallback)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if info := netbiosLookup(ctx, ip); info.Hostname != "" {
			select {
			case candidates <- fqdnCandidate{info.Hostname, "netbios"}:
			case <-ctx.Done():
			}
		}
	}()

	// 6. mDNS lookup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if name := tryMDNSLookup(ctx, ip); name != "" {
			select {
			case candidates <- fqdnCandidate{name, "mdns"}:
			case <-ctx.Done():
			}
		}
	}()

	// 7. LLMNR lookup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if name := probeLLMNRName(ctx, ip); name != "" {
			select {
			case candidates <- fqdnCandidate{name, "llmnr"}:
			case <-ctx.Done():
			}
		}
	}()

	// Close candidates channel when all goroutines complete
	go func() {
		wg.Wait()
		close(candidates)
	}()

	// Collect all candidates
	seen := make(map[string]bool)
	var allCandidates []fqdnCandidate

	for candidate := range candidates {
		normalized := normalizeFQDN(candidate.fqdn)
		if normalized == "" {
			continue
		}

		key := strings.ToLower(normalized)
		if seen[key] {
			continue
		}

		seen[key] = true
		allCandidates = append(allCandidates, fqdnCandidate{normalized, candidate.method})
		result.DiscoveryMethods = append(result.DiscoveryMethods, candidate.method)
	}

	// Select best FQDN from candidates
	if len(allCandidates) > 0 {
		best := selectBestFQDN(allCandidates)
		result.FQDN = best.fqdn
		// Use full FQDN as hostname (e.g., "www.loginme.net" not "www")
		result.Hostname = best.fqdn
		// Extract domain if FQDN contains dots
		if idx := strings.IndexByte(best.fqdn, '.'); idx > 0 {
			result.Domain = best.fqdn[idx+1:]
		}

		// Validate FQDN with forward lookup
		if validateFQDNForwardLookup(ctx, best.fqdn, ip) {
			result.IsValidated = true
		}
	}

	select {
	case smbInfo := <-smbInfoChan:
		result.SMBInfo = smbInfo
	default:
	}

	return result
}

// lookupPTRRecord performs reverse DNS lookup for PTR records
func lookupPTRRecord(ctx context.Context, ip string) string {
	lookupCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()

	names, err := net.DefaultResolver.LookupAddr(lookupCtx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}

	// Return first non-empty PTR record
	for _, name := range names {
		cleaned := strings.TrimSuffix(name, ".")
		if cleaned != "" && !strings.Contains(cleaned, "in-addr.arpa") {
			return cleaned
		}
	}

	return ""
}

// queryDNSServer queries a specific DNS server for reverse lookup
func queryDNSServer(ctx context.Context, ip string, server string) string {
	lookupCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()

	var resolver *net.Resolver
	if server != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 1000 * time.Millisecond}
				return d.DialContext(ctx, "udp", server)
			},
		}
	} else {
		resolver = net.DefaultResolver
	}

	names, err := resolver.LookupAddr(lookupCtx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}

	for _, name := range names {
		cleaned := strings.TrimSuffix(name, ".")
		if cleaned != "" && !strings.Contains(cleaned, "in-addr.arpa") {
			return cleaned
		}
	}

	return ""
}

// validateFQDNForwardLookup validates FQDN by performing forward lookup
func validateFQDNForwardLookup(ctx context.Context, fqdn string, expectedIP string) bool {
	lookupCtx, cancel := context.WithTimeout(ctx, 1000*time.Millisecond)
	defer cancel()

	ips, err := net.DefaultResolver.LookupHost(lookupCtx, fqdn)
	if err != nil || len(ips) == 0 {
		return false
	}

	// Check if expected IP is in the resolved IPs
	for _, ip := range ips {
		if ip == expectedIP {
			return true
		}
	}

	return false
}

// normalizeFQDN cleans and normalizes FQDN
func normalizeFQDN(fqdn string) string {
	fqdn = strings.TrimSpace(fqdn)
	if fqdn == "" {
		return ""
	}

	fqdn = strings.TrimSuffix(fqdn, ".")

	// Remove .local suffix if present (mDNS)
	if strings.HasSuffix(strings.ToLower(fqdn), ".local") {
		fqdn = fqdn[:len(fqdn)-len(".local")]
	}

	fqdn = strings.TrimSpace(fqdn)

	// Basic validation
	if fqdn == "" || strings.EqualFold(fqdn, "unknown") || strings.EqualFold(fqdn, "(unknown)") {
		return ""
	}

	// Filter out reverse DNS artifacts
	if strings.Contains(strings.ToLower(fqdn), "in-addr.arpa") {
		return ""
	}

	return fqdn
}

// splitFQDN splits FQDN into hostname and domain
func splitFQDN(fqdn string) (hostname string, domain string) {
	parts := strings.SplitN(fqdn, ".", 2)
	hostname = parts[0]

	if len(parts) > 1 {
		domain = parts[1]
	}

	return hostname, domain
}

// probeSSHBanner attempts to connect to SSH and read the banner
func probeSSHBanner(ctx context.Context, ip string, timeout time.Duration) string {
	address := net.JoinHostPort(ip, "22")

	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return ""
	}
	defer conn.Close()

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(timeout))

	// Read SSH banner (first line)
	// Format: SSH-2.0-OpenSSH_8.2p1 Ubuntu-4ubuntu0.5
	buffer := make([]byte, 256)
	n, err := conn.Read(buffer)
	if err != nil {
		return ""
	}

	banner := strings.TrimSpace(string(buffer[:n]))

	// Validate SSH banner format
	if strings.HasPrefix(banner, "SSH-") {
		return banner
	}

	return ""
}

// probeRDPHostname attempts to extract hostname from RDP certificate
func probeRDPHostname(ctx context.Context, ip string, timeout time.Duration) string {
	address := net.JoinHostPort(ip, "3389")

	dialer := &net.Dialer{Timeout: timeout}

	// Connect to RDP port
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return ""
	}
	defer conn.Close()

	// Set read/write deadline
	deadline := time.Now().Add(timeout)
	conn.SetDeadline(deadline)

	// Try TLS handshake (RDP over TLS)
	tlsConn := tls.Client(conn, &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS10,
	})

	// Perform handshake
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		// RDP might not use TLS, try basic connection
		return ""
	}
	defer tlsConn.Close()

	// Extract hostname from certificate
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return ""
	}

	// Get CN (Common Name) from certificate
	cert := state.PeerCertificates[0]
	if cert.Subject.CommonName != "" {
		// CN often contains hostname like "DESKTOP-ABC123" or "SERVER01.domain.local"
		return extractHostnameOnly(cert.Subject.CommonName)
	}

	// Try DNS names from SAN
	if len(cert.DNSNames) > 0 {
		return extractHostnameOnly(cert.DNSNames[0])
	}

	return ""
}

// extractHostnameOnly normalizes hostname/FQDN strings for display
// Just removes trailing dot, KEEPS full FQDN
// Example: "www.loginme.net." -> "www.loginme.net"
func extractHostnameOnly(fullname string) string {
	fullname = strings.TrimSpace(fullname)
	if fullname == "" {
		return ""
	}

	// Remove trailing dot only
	fullname = strings.TrimSuffix(fullname, ".")

	return fullname
}

func ipv4PtrName(ip string) (string, bool) {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return "", false
	}

	for _, part := range parts {
		if part == "" {
			return "", false
		}
	}

	return fmt.Sprintf("%s.%s.%s.%s.in-addr.arpa.", parts[3], parts[2], parts[1], parts[0]), true
}

// selectBestFQDN selects the most reliable FQDN from candidates
func selectBestFQDN(candidates []fqdnCandidate) fqdnCandidate {
	if len(candidates) == 0 {
		return fqdnCandidate{}
	}

	// Priority scoring - Advanced IP Scanner methodology
	// Based on research: NetBIOS > DNS > mDNS
	methodScores := map[string]int{
		// Windows-specific (HIGHEST PRIORITY - AIP uses NetBIOS first)
		"netbios":     100, // NetBIOS command-line - HIGHEST (AIP priority)
		"netbios-udp": 99,  // NetBIOS UDP 137 - very reliable
		"smb":         98,  // SMB/CIFS - Windows hostname

		// DNS (SECOND PRIORITY - AIP fallback)
		"ptr":                95, // PTR records (reverse DNS)
		"dns":                92, // System DNS
		"dns:8.8.8.8":        90, // Google DNS
		"dns:1.1.1.1":        90, // Cloudflare DNS
		"dns:208.67.222.222": 88, // OpenDNS

		// macOS/iOS specific (mDNS for Apple devices)
		"mdns": 85, // mDNS/Bonjour - for .local domains

		// Windows Link-Local
		"llmnr": 80, // LLMNR - Windows Link-Local
	}

	best := candidates[0]
	bestScore := methodScores[best.method]

	for _, candidate := range candidates[1:] {
		score := methodScores[candidate.method]

		// Prefer FQDNs with domains over just hostnames
		if strings.Contains(candidate.fqdn, ".") && !strings.Contains(best.fqdn, ".") {
			score += 50
		}

		// Prefer longer, more specific names
		if len(candidate.fqdn) > len(best.fqdn)+5 {
			score += 10
		}

		// Apple device heuristic: if mDNS returns .local domain, boost priority
		if candidate.method == "mdns" && strings.HasSuffix(candidate.fqdn, ".local") {
			score += 30 // Strong indicator of Apple/Linux device
		}

		// Windows device heuristic: if SMB/NetBIOS returns name, boost priority
		if (candidate.method == "smb" || candidate.method == "netbios-udp") && candidate.fqdn != "" {
			score += 25 // Strong indicator of Windows device
		}

		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}

	return best
}

// appendIfNotExists appends item to slice if it doesn't already exist
func appendIfNotExists(slice []string, item string) []string {
	for _, existing := range slice {
		if existing == item {
			return slice
		}
	}
	return append(slice, item)
}

// calculateConfidenceScore calculates device confidence score based on discovery methods
func calculateConfidenceScore(device *DeviceInfo) int {
	score := 0

	// Base score from reachability
	if device.Reachable {
		score += 20
	}

	// MAC address found
	if device.MACAddress != "" && !isZeroMAC(device.MACAddress) {
		score += 15
	}

	// Vendor identified
	if device.Vendor != "" {
		score += 10
	}

	// Hostname found
	if device.Hostname != "" {
		score += 15
	}

	// FQDN validated
	if device.FQDN != "" {
		score += 20
		// Extra points if FQDN has domain
		if device.Domain != "" {
			score += 10
		}
	}

	// Discovery sources bonus (more sources = higher confidence)
	sourceBonus := len(device.DiscoverySources) * 2
	if sourceBonus > 15 {
		sourceBonus = 15
	}
	score += sourceBonus

	// Web server detected
	if device.WebServer != "" || device.HTTPTitle != "" {
		score += 5
	}

	// SNMP/LLDP/OS signals
	if device.SystemName != "" || device.SystemDescription != "" {
		score += 10
	}
	if device.OperatingSystem != "" {
		score += 10
	}
	if device.LLDPSystemName != "" {
		score += 5
	}

	// Cap at 100
	if score > 100 {
		score = 100
	}

	return score
}

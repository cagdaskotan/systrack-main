package services

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

// Standard SNMP OIDs
const (
	OIDSysDescr    = ".1.3.6.1.2.1.1.1.0"    // System description
	OIDSysObjectID = ".1.3.6.1.2.1.1.2.0"    // System object ID
	OIDSysUpTime   = ".1.3.6.1.2.1.1.3.0"    // System uptime
	OIDSysContact  = ".1.3.6.1.2.1.1.4.0"    // System contact
	OIDSysName     = ".1.3.6.1.2.1.1.5.0"    // System name (hostname)
	OIDSysLocation = ".1.3.6.1.2.1.1.6.0"    // System location
	OIDSysServices = ".1.3.6.1.2.1.1.7.0"    // System services

	// Interface table
	OIDIfNumber    = ".1.3.6.1.2.1.2.1.0"    // Number of interfaces
	OIDIfDescr     = ".1.3.6.1.2.1.2.2.1.2"  // Interface description
	OIDIfType      = ".1.3.6.1.2.1.2.2.1.3"  // Interface type
	OIDIfSpeed     = ".1.3.6.1.2.1.2.2.1.5"  // Interface speed
	OIDIfPhysAddr  = ".1.3.6.1.2.1.2.2.1.6"  // Interface MAC address
	OIDIfOperStatus = ".1.3.6.1.2.1.2.2.1.8" // Interface operational status

	// Printer MIB (RFC 3805)
	OIDPrinterStatus = ".1.3.6.1.2.1.25.3.5.1.1" // Printer status
)

// SNMP Error Categories
const (
	SNMPErrCategoryTimeout     = "timeout"
	SNMPErrCategoryUnreachable = "unreachable"
	SNMPErrCategoryAuthFailed  = "auth_failed"
	SNMPErrCategoryNoResponse  = "no_response"
	SNMPErrCategoryUnknown     = "unknown_error"
)

// SNMPError represents a categorized SNMP error
type SNMPError struct {
	Category string `json:"category"`
	Message  string `json:"message"`
	Detail   string `json:"detail,omitempty"`
}

// ClassifySNMPError categorizes an SNMP error
func ClassifySNMPError(err error) SNMPError {
	if err == nil {
		return SNMPError{}
	}

	errStr := strings.ToLower(err.Error())

	switch {
	case strings.Contains(errStr, "timeout"):
		return SNMPError{
			Category: SNMPErrCategoryTimeout,
			Message:  "SNMP zaman aşımı",
			Detail:   err.Error(),
		}
	case strings.Contains(errStr, "no route") || strings.Contains(errStr, "unreachable") || strings.Contains(errStr, "connection refused"):
		return SNMPError{
			Category: SNMPErrCategoryUnreachable,
			Message:  "Hedef erişilemiyor",
			Detail:   err.Error(),
		}
	case strings.Contains(errStr, "no response") || strings.Contains(errStr, "request failed"):
		return SNMPError{
			Category: SNMPErrCategoryNoResponse,
			Message:  "SNMP yanıt yok (community string yanlış olabilir)",
			Detail:   err.Error(),
		}
	default:
		return SNMPError{
			Category: SNMPErrCategoryUnknown,
			Message:  "SNMP hatası",
			Detail:   err.Error(),
		}
	}
}

// SNMPClient handles SNMP connections to network devices
type SNMPClient struct {
	Version          gosnmp.SnmpVersion
	CommunityStrings []string
	Port             int
	TimeoutSeconds   int
	RetryCount       int
	ConcurrentLimit  int
}

// SNMPDeviceInfo represents collected SNMP information from a device
type SNMPDeviceInfo struct {
	// System MIB
	SysDescr      string `json:"sys_descr"`
	SysObjectID   string `json:"sys_object_id"`
	SysUpTime     string `json:"sys_uptime"`
	SysContact    string `json:"sys_contact"`
	SysName       string `json:"sys_name"`
	SysLocation   string `json:"sys_location"`

	// Derived info
	DeviceType    string `json:"device_type"`
	Vendor        string `json:"vendor"`
	Model         string `json:"model,omitempty"`

	// Interface summary
	InterfaceCount int              `json:"interface_count"`
	Interfaces     []SNMPInterface  `json:"interfaces,omitempty"`

	// Connection info - used for caching
	CommunityUsed  string    `json:"community_used,omitempty"`
	CommunityIndex int       `json:"community_index,omitempty"` // Index in community list for faster retry
	CollectedAt    time.Time `json:"collected_at"`
	CollectionMs   int64     `json:"collection_ms"`
	Errors         []string  `json:"errors,omitempty"`
}

// SNMPInterface represents a network interface
type SNMPInterface struct {
	Index       int    `json:"index"`
	Description string `json:"description"`
	Type        int    `json:"type"`
	TypeName    string `json:"type_name"`
	Speed       int64  `json:"speed_mbps"`
	MACAddress  string `json:"mac_address,omitempty"`
	OperStatus  string `json:"oper_status"`
}

// NewSNMPClient creates a new SNMP client
func NewSNMPClient(version string, communityStrings []string, port, timeoutSeconds, retryCount, concurrentLimit int) *SNMPClient {
	var snmpVersion gosnmp.SnmpVersion
	switch version {
	case "v1":
		snmpVersion = gosnmp.Version1
	case "v2c":
		snmpVersion = gosnmp.Version2c
	default:
		snmpVersion = gosnmp.Version2c
	}

	if port == 0 {
		port = 161
	}
	if timeoutSeconds == 0 {
		timeoutSeconds = 5
	}
	if retryCount == 0 {
		retryCount = 2
	}
	if concurrentLimit == 0 {
		concurrentLimit = 10
	}

	return &SNMPClient{
		Version:          snmpVersion,
		CommunityStrings: communityStrings,
		Port:             port,
		TimeoutSeconds:   timeoutSeconds,
		RetryCount:       retryCount,
		ConcurrentLimit:  concurrentLimit,
	}
}

// TestConnection tests SNMP connectivity with all community strings
func (c *SNMPClient) TestConnection(targetIP string) (*SNMPDeviceInfo, error) {
	startTime := time.Now()

	for _, community := range c.CommunityStrings {
		snmp := &gosnmp.GoSNMP{
			Target:    targetIP,
			Port:      uint16(c.Port),
			Community: community,
			Version:   c.Version,
			Timeout:   time.Duration(c.TimeoutSeconds) * time.Second,
			Retries:   c.RetryCount,
		}

		err := snmp.Connect()
		if err != nil {
			continue
		}
		defer snmp.Conn.Close()

		// Try to get sysDescr
		result, err := snmp.Get([]string{OIDSysDescr})
		if err != nil {
			snmp.Conn.Close()
			continue
		}

		if len(result.Variables) > 0 && result.Variables[0].Type != gosnmp.NoSuchObject {
			sysDescr := ""
			if result.Variables[0].Type == gosnmp.OctetString {
				sysDescr = string(result.Variables[0].Value.([]byte))
			}

			info := &SNMPDeviceInfo{
				SysDescr:     sysDescr,
				CommunityUsed: community,
				CollectedAt:  time.Now(),
				CollectionMs: time.Since(startTime).Milliseconds(),
			}

			// Detect device type from sysDescr
			info.DeviceType, info.Vendor = detectDeviceType(sysDescr)

			return info, nil
		}
		snmp.Conn.Close()
	}

	return nil, fmt.Errorf("SNMP bağlantısı başarısız - tüm community string'ler denendi")
}

// CollectDeviceInfo collects comprehensive SNMP information from a device
func (c *SNMPClient) CollectDeviceInfo(targetIP string) (*SNMPDeviceInfo, error) {
	return c.CollectDeviceInfoWithHint(targetIP, "", -1)
}

// CollectDeviceInfoWithHint collects SNMP info, trying the hinted community first
func (c *SNMPClient) CollectDeviceInfoWithHint(targetIP string, hintCommunity string, hintIndex int) (*SNMPDeviceInfo, error) {
	startTime := time.Now()
	info := &SNMPDeviceInfo{
		CollectedAt: time.Now(),
		Errors:      []string{},
	}

	var workingCommunity string
	var workingIndex int
	var snmp *gosnmp.GoSNMP

	// Build community list with hint first (if provided)
	communitiesToTry := make([]string, 0, len(c.CommunityStrings)+1)
	communityIndices := make([]int, 0, len(c.CommunityStrings)+1)

	// If we have a hint, try it first
	if hintCommunity != "" && hintIndex >= 0 {
		communitiesToTry = append(communitiesToTry, hintCommunity)
		communityIndices = append(communityIndices, hintIndex)
	}

	// Add remaining communities (skip the hinted one if already added)
	for i, community := range c.CommunityStrings {
		if community != hintCommunity {
			communitiesToTry = append(communitiesToTry, community)
			communityIndices = append(communityIndices, i)
		}
	}

	// Find working community string
	for idx, community := range communitiesToTry {
		testSnmp := &gosnmp.GoSNMP{
			Target:    targetIP,
			Port:      uint16(c.Port),
			Community: community,
			Version:   c.Version,
			Timeout:   time.Duration(c.TimeoutSeconds) * time.Second,
			Retries:   c.RetryCount,
		}

		if err := testSnmp.Connect(); err != nil {
			continue
		}

		result, err := testSnmp.Get([]string{OIDSysDescr})
		if err != nil {
			testSnmp.Conn.Close()
			continue
		}

		if len(result.Variables) > 0 && result.Variables[0].Type != gosnmp.NoSuchObject {
			workingCommunity = community
			workingIndex = communityIndices[idx]
			snmp = testSnmp
			break
		}
		testSnmp.Conn.Close()
	}

	if snmp == nil {
		return nil, fmt.Errorf("SNMP bağlantısı başarısız - community string bulunamadı")
	}
	defer snmp.Conn.Close()

	info.CommunityUsed = workingCommunity
	info.CommunityIndex = workingIndex

	// Collect system MIB info
	systemOIDs := []string{
		OIDSysDescr,
		OIDSysObjectID,
		OIDSysUpTime,
		OIDSysContact,
		OIDSysName,
		OIDSysLocation,
	}

	result, err := snmp.Get(systemOIDs)
	if err != nil {
		info.Errors = append(info.Errors, fmt.Sprintf("System MIB hatası: %v", err))
	} else {
		for _, variable := range result.Variables {
			switch variable.Name {
			case OIDSysDescr:
				if variable.Type == gosnmp.OctetString {
					info.SysDescr = string(variable.Value.([]byte))
				}
			case OIDSysObjectID:
				if variable.Type == gosnmp.ObjectIdentifier {
					info.SysObjectID = variable.Value.(string)
				}
			case OIDSysUpTime:
				if variable.Type == gosnmp.TimeTicks {
					ticks := variable.Value.(uint32)
					info.SysUpTime = formatUptime(ticks)
				}
			case OIDSysContact:
				if variable.Type == gosnmp.OctetString {
					info.SysContact = string(variable.Value.([]byte))
				}
			case OIDSysName:
				if variable.Type == gosnmp.OctetString {
					info.SysName = string(variable.Value.([]byte))
				}
			case OIDSysLocation:
				if variable.Type == gosnmp.OctetString {
					info.SysLocation = string(variable.Value.([]byte))
				}
			}
		}
	}

	// Detect device type and vendor
	info.DeviceType, info.Vendor = detectDeviceType(info.SysDescr)
	if info.SysObjectID != "" {
		// Override with OID-based detection if available
		deviceType, vendor := detectFromOID(info.SysObjectID)
		if deviceType != "" {
			info.DeviceType = deviceType
		}
		if vendor != "" {
			info.Vendor = vendor
		}
	}

	// Get interface count
	ifNumResult, err := snmp.Get([]string{OIDIfNumber})
	if err == nil && len(ifNumResult.Variables) > 0 {
		if ifNumResult.Variables[0].Type == gosnmp.Integer {
			info.InterfaceCount = int(ifNumResult.Variables[0].Value.(int))
		}
	}

	// Collect interface info (limit to first 10 for performance)
	if info.InterfaceCount > 0 {
		maxInterfaces := info.InterfaceCount
		if maxInterfaces > 10 {
			maxInterfaces = 10
		}

		for i := 1; i <= maxInterfaces; i++ {
			iface := SNMPInterface{Index: i}

			// Get interface details
			ifOIDs := []string{
				fmt.Sprintf("%s.%d", OIDIfDescr, i),
				fmt.Sprintf("%s.%d", OIDIfType, i),
				fmt.Sprintf("%s.%d", OIDIfSpeed, i),
				fmt.Sprintf("%s.%d", OIDIfPhysAddr, i),
				fmt.Sprintf("%s.%d", OIDIfOperStatus, i),
			}

			ifResult, err := snmp.Get(ifOIDs)
			if err != nil {
				continue
			}

			for _, v := range ifResult.Variables {
				oid := v.Name
				switch {
				case strings.HasPrefix(oid, OIDIfDescr):
					if v.Type == gosnmp.OctetString {
						iface.Description = string(v.Value.([]byte))
					}
				case strings.HasPrefix(oid, OIDIfType):
					if v.Type == gosnmp.Integer {
						iface.Type = int(v.Value.(int))
						iface.TypeName = getInterfaceTypeName(iface.Type)
					}
				case strings.HasPrefix(oid, OIDIfSpeed):
					if v.Type == gosnmp.Gauge32 {
						iface.Speed = int64(v.Value.(uint)) / 1000000 // Convert to Mbps
					}
				case strings.HasPrefix(oid, OIDIfPhysAddr):
					if v.Type == gosnmp.OctetString {
						mac := v.Value.([]byte)
						if len(mac) == 6 {
							iface.MACAddress = fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
								mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
						}
					}
				case strings.HasPrefix(oid, OIDIfOperStatus):
					if v.Type == gosnmp.Integer {
						status := int(v.Value.(int))
						iface.OperStatus = getOperStatusName(status)
					}
				}
			}

			if iface.Description != "" {
				info.Interfaces = append(info.Interfaces, iface)
			}
		}
	}

	info.CollectionMs = time.Since(startTime).Milliseconds()
	return info, nil
}

// SNMPEnrichmentResult contains the result of a single SNMP enrichment
type SNMPEnrichmentResult struct {
	IP           string          `json:"ip"`
	Success      bool            `json:"success"`
	DeviceInfo   *SNMPDeviceInfo `json:"device_info,omitempty"`
	Error        *SNMPError      `json:"error,omitempty"`
	DurationMs   int64           `json:"duration_ms"`
}

// BatchSNMPResult contains overall batch SNMP results
type BatchSNMPResult struct {
	TotalTargets    int                              `json:"total_targets"`
	SuccessCount    int                              `json:"success_count"`
	ErrorCount      int                              `json:"error_count"`
	Results         map[string]*SNMPEnrichmentResult `json:"results"`
	TotalDurationMs int64                            `json:"total_duration_ms"`
}

// EnrichInventoryItems collects SNMP info from multiple targets concurrently
func (c *SNMPClient) EnrichInventoryItems(targetIPs []string) *BatchSNMPResult {
	startTime := time.Now()

	result := &BatchSNMPResult{
		TotalTargets: len(targetIPs),
		Results:      make(map[string]*SNMPEnrichmentResult),
	}

	if len(targetIPs) == 0 {
		return result
	}

	concurrentLimit := c.ConcurrentLimit
	if concurrentLimit <= 0 {
		concurrentLimit = 10
	}

	semaphore := make(chan struct{}, concurrentLimit)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, ip := range targetIPs {
		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			itemStart := time.Now()
			info, err := c.CollectDeviceInfo(targetIP)
			duration := time.Since(itemStart).Milliseconds()

			enrichResult := &SNMPEnrichmentResult{
				IP:         targetIP,
				DurationMs: duration,
			}

			if err != nil {
				snmpErr := ClassifySNMPError(err)
				enrichResult.Success = false
				enrichResult.Error = &snmpErr
			} else {
				enrichResult.Success = true
				enrichResult.DeviceInfo = info
			}

			mu.Lock()
			result.Results[targetIP] = enrichResult
			if enrichResult.Success {
				result.SuccessCount++
			} else {
				result.ErrorCount++
			}
			mu.Unlock()
		}(ip)
	}

	wg.Wait()
	result.TotalDurationMs = time.Since(startTime).Milliseconds()

	return result
}

// CommunityHint contains cached community info for an IP
type CommunityHint struct {
	Community string
	Index     int
}

// EnrichInventoryItemsWithHints collects SNMP info using cached community hints
func (c *SNMPClient) EnrichInventoryItemsWithHints(targetIPs []string, hints map[string]CommunityHint) *BatchSNMPResult {
	startTime := time.Now()

	result := &BatchSNMPResult{
		TotalTargets: len(targetIPs),
		Results:      make(map[string]*SNMPEnrichmentResult),
	}

	if len(targetIPs) == 0 {
		return result
	}

	concurrentLimit := c.ConcurrentLimit
	if concurrentLimit <= 0 {
		concurrentLimit = 10
	}

	semaphore := make(chan struct{}, concurrentLimit)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, ip := range targetIPs {
		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			itemStart := time.Now()

			// Check if we have a hint for this IP
			var info *SNMPDeviceInfo
			var err error
			if hint, ok := hints[targetIP]; ok && hint.Community != "" && hint.Index >= 0 {
				// Use hint - try cached community first
				info, err = c.CollectDeviceInfoWithHint(targetIP, hint.Community, hint.Index)
			} else {
				// No hint - try all communities
				info, err = c.CollectDeviceInfo(targetIP)
			}

			duration := time.Since(itemStart).Milliseconds()

			enrichResult := &SNMPEnrichmentResult{
				IP:         targetIP,
				DurationMs: duration,
			}

			if err != nil {
				snmpErr := ClassifySNMPError(err)
				enrichResult.Success = false
				enrichResult.Error = &snmpErr
			} else {
				enrichResult.Success = true
				enrichResult.DeviceInfo = info
			}

			mu.Lock()
			result.Results[targetIP] = enrichResult
			if enrichResult.Success {
				result.SuccessCount++
			} else {
				result.ErrorCount++
			}
			mu.Unlock()
		}(ip)
	}

	wg.Wait()
	result.TotalDurationMs = time.Since(startTime).Milliseconds()

	return result
}

// CheckSNMPReachable performs a quick UDP port check
func (c *SNMPClient) CheckSNMPReachable(targetIP string) error {
	address := fmt.Sprintf("%s:%d", targetIP, c.Port)
	conn, err := net.DialTimeout("udp", address, time.Duration(2)*time.Second)
	if err != nil {
		return fmt.Errorf("SNMP portu erişilemiyor (%d): %v", c.Port, err)
	}
	conn.Close()
	return nil
}

// ToJSON converts SNMPDeviceInfo to JSON string
func (s *SNMPDeviceInfo) ToJSON() string {
	data, err := json.Marshal(s)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// GetSummary returns a human-readable summary
func (s *SNMPDeviceInfo) GetSummary() string {
	return fmt.Sprintf("%s | %s | %s | %d interfaces",
		s.Vendor, s.DeviceType, s.SysName, s.InterfaceCount)
}

// Helper functions

func detectDeviceType(sysDescr string) (deviceType, vendor string) {
	lower := strings.ToLower(sysDescr)

	// Detect vendor
	switch {
	case strings.Contains(lower, "cisco"):
		vendor = "Cisco"
	case strings.Contains(lower, "hp") || strings.Contains(lower, "hewlett"):
		vendor = "HP"
	case strings.Contains(lower, "juniper"):
		vendor = "Juniper"
	case strings.Contains(lower, "aruba"):
		vendor = "Aruba"
	case strings.Contains(lower, "mikrotik"):
		vendor = "MikroTik"
	case strings.Contains(lower, "ubiquiti") || strings.Contains(lower, "unifi"):
		vendor = "Ubiquiti"
	case strings.Contains(lower, "fortinet") || strings.Contains(lower, "fortigate"):
		vendor = "Fortinet"
	case strings.Contains(lower, "dell"):
		vendor = "Dell"
	case strings.Contains(lower, "netgear"):
		vendor = "Netgear"
	case strings.Contains(lower, "d-link") || strings.Contains(lower, "dlink"):
		vendor = "D-Link"
	case strings.Contains(lower, "tp-link") || strings.Contains(lower, "tplink"):
		vendor = "TP-Link"
	case strings.Contains(lower, "brother"):
		vendor = "Brother"
	case strings.Contains(lower, "canon"):
		vendor = "Canon"
	case strings.Contains(lower, "epson"):
		vendor = "Epson"
	case strings.Contains(lower, "xerox"):
		vendor = "Xerox"
	case strings.Contains(lower, "ricoh"):
		vendor = "Ricoh"
	case strings.Contains(lower, "kyocera"):
		vendor = "Kyocera"
	case strings.Contains(lower, "lexmark"):
		vendor = "Lexmark"
	case strings.Contains(lower, "samsung"):
		vendor = "Samsung"
	case strings.Contains(lower, "synology"):
		vendor = "Synology"
	case strings.Contains(lower, "qnap"):
		vendor = "QNAP"
	case strings.Contains(lower, "linux"):
		vendor = "Linux"
	case strings.Contains(lower, "windows"):
		vendor = "Microsoft"
	default:
		vendor = "Unknown"
	}

	// Detect device type
	switch {
	case strings.Contains(lower, "switch"):
		deviceType = "Switch"
	case strings.Contains(lower, "router") || strings.Contains(lower, "gateway"):
		deviceType = "Router"
	case strings.Contains(lower, "firewall"):
		deviceType = "Firewall"
	case strings.Contains(lower, "access point") || strings.Contains(lower, "wireless"):
		deviceType = "Access Point"
	case strings.Contains(lower, "printer") || strings.Contains(lower, "mfp") || strings.Contains(lower, "laserjet") || strings.Contains(lower, "inkjet"):
		deviceType = "Printer"
	case strings.Contains(lower, "nas") || strings.Contains(lower, "storage"):
		deviceType = "NAS"
	case strings.Contains(lower, "ups"):
		deviceType = "UPS"
	case strings.Contains(lower, "camera") || strings.Contains(lower, "dvr") || strings.Contains(lower, "nvr"):
		deviceType = "Camera/DVR"
	case strings.Contains(lower, "phone") || strings.Contains(lower, "voip"):
		deviceType = "VoIP Phone"
	case strings.Contains(lower, "server"):
		deviceType = "Server"
	default:
		deviceType = "Network Device"
	}

	return deviceType, vendor
}

func detectFromOID(oid string) (deviceType, vendor string) {
	// Common enterprise OIDs
	switch {
	case strings.HasPrefix(oid, ".1.3.6.1.4.1.9"):
		vendor = "Cisco"
	case strings.HasPrefix(oid, ".1.3.6.1.4.1.11"):
		vendor = "HP"
	case strings.HasPrefix(oid, ".1.3.6.1.4.1.2636"):
		vendor = "Juniper"
	case strings.HasPrefix(oid, ".1.3.6.1.4.1.14988"):
		vendor = "MikroTik"
	case strings.HasPrefix(oid, ".1.3.6.1.4.1.41112"):
		vendor = "Ubiquiti"
	case strings.HasPrefix(oid, ".1.3.6.1.4.1.12356"):
		vendor = "Fortinet"
	}
	return deviceType, vendor
}

func formatUptime(ticks uint32) string {
	seconds := ticks / 100
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	minutes := (seconds % 3600) / 60

	if days > 0 {
		return fmt.Sprintf("%d gün %d saat %d dakika", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%d saat %d dakika", hours, minutes)
	}
	return fmt.Sprintf("%d dakika", minutes)
}

func getInterfaceTypeName(ifType int) string {
	types := map[int]string{
		1:   "other",
		6:   "ethernetCsmacd",
		24:  "softwareLoopback",
		53:  "propVirtual",
		71:  "ieee80211",
		117: "gigabitEthernet",
		131: "tunnel",
		135: "l2vlan",
		136: "l3ipvlan",
		161: "ieee8023adLag",
	}
	if name, ok := types[ifType]; ok {
		return name
	}
	return fmt.Sprintf("type-%d", ifType)
}

func getOperStatusName(status int) string {
	switch status {
	case 1:
		return "up"
	case 2:
		return "down"
	case 3:
		return "testing"
	case 4:
		return "unknown"
	case 5:
		return "dormant"
	case 6:
		return "notPresent"
	case 7:
		return "lowerLayerDown"
	default:
		return "unknown"
	}
}

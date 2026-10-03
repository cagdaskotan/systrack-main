package handlers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"systrack/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/go-ldap/ldap/v3"
	"github.com/google/uuid"
	"github.com/gosnmp/gosnmp"
	"github.com/masterzen/winrm"
	"golang.org/x/crypto/ssh"
)

// DiscoveryRequest - Kullanıcıdan gelen keşif isteği
type DiscoveryRequest struct {
	Methods               []string               `json:"methods"`                 // ["ldap", "dns", "snmp", "winrm", "ssh"]
	ADSettings            map[string]interface{} `json:"ad_settings"`             // LDAP ayarları
	SNMPSettings          map[string]interface{} `json:"snmp_settings"`           // SNMP ayarları (opsiyonel)
	WinRMSettings         map[string]interface{} `json:"winrm_settings"`          // WinRM ayarları (opsiyonel)
	SSHSettings           map[string]interface{} `json:"ssh_settings"`            // SSH ayarları (opsiyonel, Linux/macOS)
	SNMPTargets           []string               `json:"snmp_targets"`            // SNMP tarama hedefleri (CIDR, aralık, IP)
	SSHTargets            []string               `json:"ssh_targets"`             // SSH tarama hedefleri (Linux/Unix cihazlar için IP listesi)
	CrackDetectionEnabled bool                   `json:"crack_detection_enabled"` // Lisans/crack uyumluluk taraması
}

// DiscoveredItem - Keşfedilen ancak henüz kaydedilmemiş varlık
type DiscoveredItem struct {
	TempID     string `json:"temp_id"`     // Geçici UUID
	Name       string `json:"name"`        // Varlık adı
	Hostname   string `json:"hostname"`    // DNS hostname
	IPAddress  string `json:"ip_address"`  // IP (DNS'den veya SNMP'den)
	MACAddress string `json:"mac_address"` // MAC (SNMP'den veya WinRM'den)

	// AD/LDAP Bilgileri
	ADData *ADDiscoveryData `json:"ad_data,omitempty"`

	// DNS Bilgileri
	DNSResolved bool   `json:"dns_resolved"`
	DNSError    string `json:"dns_error,omitempty"`

	// SNMP Bilgileri
	SNMPData    map[string]interface{} `json:"snmp_data,omitempty"`
	SNMPSuccess bool                   `json:"snmp_success"`
	SNMPError   string                 `json:"snmp_error,omitempty"`

	// WinRM Bilgileri
	WinRMData    map[string]interface{} `json:"winrm_data,omitempty"`
	WinRMSuccess bool                   `json:"winrm_success"`
	WinRMError   string                 `json:"winrm_error,omitempty"`

	// SSH Bilgileri (Linux/macOS)
	SSHData    map[string]interface{} `json:"ssh_data,omitempty"`
	SSHSuccess bool                   `json:"ssh_success"`
	SSHError   string                 `json:"ssh_error,omitempty"`

	// Meta
	MethodsUsed   []string `json:"methods_used"`          // Hangi metodlar kullanıldı
	AlreadyExists bool     `json:"already_exists"`        // Envanterde zaten var mı?
	ExistingID    *int     `json:"existing_id,omitempty"` // Varsa ID'si
	AssetType     string   `json:"asset_type"`            // pc, server, printer, switch, etc.

	// Hesaplanan / çıkarılan alanlar
	PowerEstimateWatts int    `json:"power_estimate_watts,omitempty"` // Tahmini güç tüketimi (W)
	LastBootTime       string `json:"last_boot_time,omitempty"`       // Son açılış zamanı

	// Crack / Lisans Uyumluluk (sadece discovery sırasında, DB'ye yazılmaz)
	CrackFindings []CrackFinding `json:"crack_findings,omitempty"` // Bulgular
}

// ADDiscoveryData - AD'den gelen bilgiler
type ADDiscoveryData struct {
	ObjectGUID            string     `json:"object_guid"`
	DistinguishedName     string     `json:"distinguished_name"`
	ComputerName          string     `json:"computer_name"`
	CNName                string     `json:"cn_name"`
	DNSHostName           string     `json:"dns_hostname"`
	Description           string     `json:"description"`
	Comment               string     `json:"comment"`
	Location              string     `json:"location"`
	ManagedBy             string     `json:"managed_by"`
	ManagedByDisplayName  string     `json:"managed_by_display_name"`
	ManagedByMail         string     `json:"managed_by_mail"`
	ManagedByDepartment   string     `json:"managed_by_department"`
	ManagedByTitle        string     `json:"managed_by_title"`
	ManagedBySamAccount   string     `json:"managed_by_sam_account"`
	ManagedByUPN          string     `json:"managed_by_upn"`
	OUPath                string     `json:"ou_path"`
	OSName                string     `json:"os_name"`
	OSVersion             string     `json:"os_version"`
	OSServicePack         string     `json:"os_service_pack"`
	Model                 string     `json:"model"`         // Hardware model
	SerialNumber          string     `json:"serial_number"` // Hardware serial
	Vendor                string     `json:"vendor"`        // Hardware vendor (from model)
	LastLogon             *time.Time `json:"last_logon"`
	WhenCreated           *time.Time `json:"when_created"`
	WhenChanged           *time.Time `json:"when_changed"`
	PwdLastSet            *time.Time `json:"pwd_last_set"`
	Enabled               bool       `json:"enabled"`
	ServicePrincipalNames []string   `json:"service_principal_names"`
}

// DiscoveryResponse - Tarama sonuçları
type DiscoveryResponse struct {
	Total    int              `json:"total"`
	NewItems int              `json:"new_items"`
	Existing int              `json:"existing"`
	Items    []DiscoveredItem `json:"items"`
	Duration float64          `json:"duration_seconds"`
	Summary  map[string]int   `json:"summary"` // {"ldap": 45, "dns": 40, "snmp": 10, "winrm": 5}
}

// PreviewDiscovery - Envantere eklemeden önce keşif yap ve sonuçları göster
func PreviewDiscovery(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()

		var req DiscoveryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek: " + err.Error()})
			return
		}

		// Metodları validate et
		validMethods := map[string]bool{"ldap": true, "dns": true, "snmp": true, "winrm": true, "ssh": true}
		for _, method := range req.Methods {
			if !validMethods[method] {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Geçersiz metod: %s", method)})
				return
			}
		}

		var discoveredItems []DiscoveredItem
		summary := make(map[string]int)

		// Handle password: if placeholder or empty, get real password from DB
		if bindPwd, ok := req.ADSettings["bind_password"].(string); !ok || bindPwd == "" || bindPwd == "********" {
			var encryptedPwd string
			err := db.QueryRow(`SELECT bind_password_encrypted FROM ad_settings WHERE id = 1`).Scan(&encryptedPwd)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "AD şifresi okunamadı: " + err.Error()})
				return
			}
			if encryptedPwd != "" {
				// Decrypt password
				decryptedPwd, err := auth.DecryptPassword(encryptedPwd)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Şifre çözülemedi: " + err.Error()})
					return
				}
				req.ADSettings["bind_password"] = decryptedPwd
			}
		}

		// 1. LDAP Discovery (zorunlu temel)
		if stringSliceContains(req.Methods, "ldap") {
			items, err := performLDAPDiscovery(req.ADSettings)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "LDAP keşfi başarısız: " + err.Error()})
				return
			}
			discoveredItems = items

			// Otomatik asset type belirleme (OS'e göre)
			autoDetectAssetTypes(discoveredItems)

			summary["ldap"] = len(items)
		}

		// 2. DNS Lookup (IP resolution)
		if stringSliceContains(req.Methods, "dns") {
			// Use Domain Controller as DNS server
			dcIP, _ := req.ADSettings["domain_controller"].(string)
			performDNSLookup(discoveredItems, dcIP)
			dnsSuccess := 0
			for _, item := range discoveredItems {
				if item.DNSResolved {
					dnsSuccess++
				}
			}
			summary["dns"] = dnsSuccess

			// 2.5. ARP Lookup (MAC address resolution)
			// Automatically run ARP lookup after DNS to get MAC addresses
			performARPLookup(discoveredItems)
			arpSuccess := 0
			for _, item := range discoveredItems {
				if item.MACAddress != "" {
					arpSuccess++
				}
			}
			if arpSuccess > 0 {
				summary["arp"] = arpSuccess
			}
		}

		// 3. SNMP Discovery (opsiyonel - port scanning + sadece başarılı bağlantılar)
		if stringSliceContains(req.Methods, "snmp") && req.SNMPSettings != nil {
			snmpTargets, err := extractSNMPTargets(req)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "SNMP tarama hedefleri geçersiz: " + err.Error()})
				return
			}

			if len(snmpTargets) > 0 {
				performSNMPDiscoveryWithTargets(&discoveredItems, req.SNMPSettings, snmpTargets)
			}

			// Mevcut cihazlarda da SNMP dene (LDAP'tan gelenler)
			performSNMPDiscovery(discoveredItems, req.SNMPSettings)

			snmpSuccess := 0
			for _, item := range discoveredItems {
				if item.SNMPSuccess {
					snmpSuccess++
				}
			}
			summary["snmp"] = snmpSuccess
		}

		// 5. WinRM Enrichment (Windows cihazlar)
		if stringSliceContains(req.Methods, "winrm") {
			winrmSettings := buildWinRMSettings(req)
			if winrmSettings != nil {
				performWinRMEnrichment(discoveredItems, winrmSettings, req.CrackDetectionEnabled)
			}
			winrmSuccess := 0
			for _, item := range discoveredItems {
				if item.WinRMSuccess {
					winrmSuccess++
				}
			}
			summary["winrm"] = winrmSuccess
		}

		// 6. SSH Discovery (Linux/Unix - port scanning + sadece başarılı bağlantılar)
		if stringSliceContains(req.Methods, "ssh") && req.SSHSettings != nil {
			// Handle SSH password: if placeholder or empty, get real password from DB
			if sshPwd, ok := req.SSHSettings["password"].(string); !ok || sshPwd == "" || sshPwd == "********" {
				var encryptedPwd string
				err := db.QueryRow(`SELECT password_encrypted FROM ssh_settings WHERE id = 1`).Scan(&encryptedPwd)
				if err != nil && err != sql.ErrNoRows {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "SSH şifresi okunamadı: " + err.Error()})
					return
				}
				if encryptedPwd != "" {
					// Decrypt password
					decryptedPwd, err := auth.DecryptPassword(encryptedPwd)
					if err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "SSH şifresi çözülemedi: " + err.Error()})
						return
					}
					req.SSHSettings["password"] = decryptedPwd
				}
			}

			sshTargets, err := extractSSHTargets(req)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "SSH tarama hedefleri geçersiz: " + err.Error()})
				return
			}

			if len(sshTargets) > 0 {
				performSSHDiscoveryWithTargets(&discoveredItems, req.SSHSettings, sshTargets)
			}

			// Mevcut cihazlarda da SSH dene (LDAP'tan gelenler)
			performSSHEnrichment(discoveredItems, req.SSHSettings)

			sshSuccess := 0
			for _, item := range discoveredItems {
				if item.SSHSuccess {
					sshSuccess++
				}
			}
			summary["ssh"] = sshSuccess
		}

		// 6b. Standalone WinRM Hosts — domain dışı Windows cihazlar
		{
			rows, err := db.Query(`
				SELECT id, ip_address, COALESCE(label,''), username, password_encrypted, port, use_ssl
				FROM winrm_standalone_hosts WHERE is_enabled = 1
			`)
			if err == nil {
				type standaloneEntry struct {
					id       int
					ip       string
					label    string
					username string
					password string
					port     int
					useSSL   bool
				}
				var standalones []standaloneEntry
				for rows.Next() {
					var e standaloneEntry
					var encPass string
					if rows.Scan(&e.id, &e.ip, &e.label, &e.username, &encPass, &e.port, &e.useSSL) == nil {
						if pw, err := auth.DecryptPassword(encPass); err == nil {
							e.password = pw
							standalones = append(standalones, e)
						}
					}
				}
				rows.Close()

				standaloneSuccess := 0

				// Standalone host'ları paralel işle — her birinin kendi credentials'ı var,
				// bu yüzden performWinRMEnrichment'a toplu gönderilemez; ayrı bir worker
				// pool ile aynı parallelism sağlanır.
				type standaloneResult struct {
					item  DiscoveredItem
					id    int
					label string
				}
				var standaloneResults []standaloneResult
				var standaloneMu sync.Mutex
				var standaloneWg sync.WaitGroup
				standaloneSem := make(chan struct{}, 10)

				for _, e := range standalones {
					standaloneWg.Add(1)
					eCopy := e
					go func() {
						defer standaloneWg.Done()
						standaloneSem <- struct{}{}
						defer func() { <-standaloneSem }()

						winrmCfg := map[string]interface{}{
							"username": eCopy.username,
							"password": eCopy.password,
							"port":     eCopy.port,
							"use_ssl":  eCopy.useSSL,
						}
						item := DiscoveredItem{
							TempID:    fmt.Sprintf("standalone-%d", eCopy.id),
							IPAddress: eCopy.ip,
							Name:      eCopy.ip,
						}
						if eCopy.label != "" {
							item.Name = eCopy.label
						}
						enrichSingleWinRM(&item, winrmCfg, req.CrackDetectionEnabled)

						standaloneMu.Lock()
						standaloneResults = append(standaloneResults, standaloneResult{item, eCopy.id, eCopy.label})
						standaloneMu.Unlock()
					}()
				}
				standaloneWg.Wait()

				for _, r := range standaloneResults {
					item := r.item
					if item.WinRMSuccess {
						standaloneSuccess++
						if item.Hostname != "" && r.label == "" {
							item.Name = item.Hostname
						}
						db.Exec(`UPDATE winrm_standalone_hosts SET last_seen_at = NOW() WHERE id = ?`, r.id)
					}
					discoveredItems = append(discoveredItems, item)
				}
				if standaloneSuccess > 0 {
					summary["standalone"] = standaloneSuccess
				}
			}
		}

		// 7. Güç tahmini + son açılış zamanı hesapla (tüm enrichment'lar bittikten sonra)
		for i := range discoveredItems {
			discoveredItems[i].PowerEstimateWatts = estimatePowerWatts(&discoveredItems[i])
			// SSH last_boot_time zaten set edildi; WinRM için çıkarım da goroutine içinde yapıldı.
			// SNMP-only cihazlarda last_boot_time yoktur.
		}

		// 8. Envanterde var mı kontrol et
		checkExistingItems(db, discoveredItems)

		// İstatistikler
		newItems := 0
		existing := 0
		for _, item := range discoveredItems {
			if item.AlreadyExists {
				existing++
			} else {
				newItems++
			}
		}

		duration := time.Since(startTime).Seconds()

		c.JSON(http.StatusOK, DiscoveryResponse{
			Total:    len(discoveredItems),
			NewItems: newItems,
			Existing: existing,
			Items:    discoveredItems,
			Duration: duration,
			Summary:  summary,
		})
	}
}

// performLDAPDiscovery - LDAP'den bilgisayarları keşfet
func performLDAPDiscovery(settings map[string]interface{}) ([]DiscoveredItem, error) {
	// AD bağlantısını kur
	dc, _ := settings["domain_controller"].(string)
	port, _ := settings["port"].(float64)
	useSSL, _ := settings["use_ssl"].(bool)
	bindUsername, _ := settings["bind_username"].(string)
	bindPassword, _ := settings["bind_password"].(string)
	baseDN, _ := settings["base_dn"].(string)

	if dc == "" || bindUsername == "" {
		return nil, fmt.Errorf("domain controller ve bind username zorunludur")
	}

	// LDAP bağlantısı
	var conn *ldap.Conn
	var err error

	address := fmt.Sprintf("%s:%d", dc, int(port))
	if useSSL {
		conn, err = ldap.DialTLS("tcp", address, nil)
	} else {
		conn, err = ldap.Dial("tcp", address)
	}
	if err != nil {
		return nil, fmt.Errorf("LDAP bağlantı hatası: %w", err)
	}
	defer conn.Close()

	// Bind (kimlik doğrulama)
	if err := conn.Bind(bindUsername, bindPassword); err != nil {
		return nil, fmt.Errorf("LDAP bind hatası: %w", err)
	}

	// Base DN otomatik tespit
	if baseDN == "" {
		baseDN, err = autoDetectBaseDN(conn)
		if err != nil {
			return nil, fmt.Errorf("Base DN otomatik tespit edilemedi: %w", err)
		}
	}

	// LDAP search - tüm bilgisayarları bul
	searchRequest := ldap.NewSearchRequest(
		baseDN,
		ldap.ScopeWholeSubtree,
		ldap.NeverDerefAliases,
		0, 0, false,
		"(&(objectClass=computer))",
		[]string{
			"objectGUID", "distinguishedName", "sAMAccountName", "cn", "dNSHostName",
			"operatingSystem", "operatingSystemVersion", "operatingSystemServicePack",
			"description", "comment", "location", "managedBy",
			"whenCreated", "whenChanged", "lastLogonTimestamp", "pwdLastSet",
			"userAccountControl", "servicePrincipalName",
			"serialNumber", "model", "physicalDeliveryOfficeName", // Hardware info
			"networkAddress", // IP/MAC if available (rarely populated)
		},
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("LDAP arama hatası: %w", err)
	}

	// Sonuçları parse et
	var items []DiscoveredItem
	locationFromOU, _ := settings["location_from_ou"].(bool)
	managedByCache := make(map[string]managedByInfo)

	for _, entry := range result.Entries {
		adData := parseADEntry(entry)
		if adData.ManagedBy != "" {
			enrichManagedByDetails(conn, adData, managedByCache)
		}

		// Eğer location boş ve location_from_ou aktifse, OU path'den çıkar
		if adData.Location == "" && locationFromOU && adData.OUPath != "" {
			// OU Path'den ilk OU adını al
			// Örnek: "OU=Computers,OU=Ankara,OU=Turkey,DC=test,DC=local" → "Ankara"
			ouParts := strings.Split(adData.OUPath, ",")
			for _, part := range ouParts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(strings.ToUpper(part), "OU=") {
					// İlk OU'yu bul (Computers değil, onun üstündeki)
					ouName := strings.TrimPrefix(part, "OU=")
					ouName = strings.TrimPrefix(ouName, "ou=")
					if ouName != "" && strings.ToLower(ouName) != "computers" {
						adData.Location = ouName
						break
					}
				}
			}
		}

		item := DiscoveredItem{
			TempID:      uuid.New().String(),
			Name:        adData.ComputerName,
			Hostname:    adData.DNSHostName,
			ADData:      adData,
			MethodsUsed: []string{"ldap"},
			AssetType:   determineAssetType(adData.OSName),
		}

		items = append(items, item)
	}

	return items, nil
}

// performDNSLookup - DNS ile hostname'den IP çöz
func performDNSLookup(items []DiscoveredItem, dnsServer string) {
	// Create custom DNS resolver using DC's DNS server
	var resolver *net.Resolver
	if dnsServer != "" {
		resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 3 * time.Second}
				return d.DialContext(ctx, network, net.JoinHostPort(dnsServer, "53"))
			},
		}
	} else {
		resolver = net.DefaultResolver
	}

	var wg sync.WaitGroup
	for i := range items {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			hostname := items[idx].Hostname
			if hostname == "" {
				items[idx].DNSError = "Hostname boş"
				return
			}

			// DNS lookup with timeout (using custom resolver if provided)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			ips, err := resolver.LookupIP(ctx, "ip4", hostname)
			if err != nil {
				items[idx].DNSError = err.Error()
				return
			}

			// İlk IPv4 adresini al
			for _, ip := range ips {
				if ipv4 := ip.To4(); ipv4 != nil {
					items[idx].IPAddress = ipv4.String()
					items[idx].DNSResolved = true
					items[idx].MethodsUsed = append(items[idx].MethodsUsed, "dns")
					break
				}
			}

			if items[idx].IPAddress == "" {
				items[idx].DNSError = "IPv4 adresi bulunamadı"
			}
		}(i)
	}
	wg.Wait()
}

// performARPLookup - ARP table'dan IP'ye karşılık MAC adresini bul
func performARPLookup(items []DiscoveredItem) {
	var wg sync.WaitGroup
	for i := range items {
		// Sadece IP'si olan itemlar için ARP lookup yap
		if items[i].IPAddress == "" {
			continue
		}

		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			ip := items[idx].IPAddress
			var cmd *exec.Cmd

			// OS'e göre ARP komutu
			if runtime.GOOS == "windows" {
				cmd = exec.Command("arp", "-a", ip)
			} else {
				// Linux/Unix
				cmd = exec.Command("arp", "-n", ip)
			}

			output, err := cmd.CombinedOutput()
			if err != nil {
				// ARP başarısız - normal, her zaman çalışmayabilir
				return
			}

			// MAC adresini parse et (XX:XX:XX:XX:XX:XX veya XX-XX-XX-XX-XX-XX formatı)
			macRegex := regexp.MustCompile(`([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})`)
			matches := macRegex.FindStringSubmatch(string(output))
			if len(matches) > 0 {
				// MAC'i normalize et: tire'leri colon'a çevir ve uppercase yap
				mac := strings.ToUpper(matches[0])
				mac = strings.ReplaceAll(mac, "-", ":")
				items[idx].MACAddress = mac
				// MAC bulunduğunda methods_used'a ekle
				if !stringSliceContains(items[idx].MethodsUsed, "arp") {
					items[idx].MethodsUsed = append(items[idx].MethodsUsed, "arp")
				}
			}
		}(i)
	}
	wg.Wait()
}

// performSNMPDiscovery - SNMP ile cihaz bilgilerini çek (network devices için - PARALEL)
func performSNMPDiscovery(items []DiscoveredItem, settings map[string]interface{}) {
	// SNMP ayarlarını çıkar
	communityStringsRaw, _ := settings["community_strings"].(string)
	communityStrings := []string{"public"}
	if communityStringsRaw != "" {
		// JSON array parse et: ["public", "private"]
		var parsed []string
		if err := json.Unmarshal([]byte(communityStringsRaw), &parsed); err == nil && len(parsed) > 0 {
			communityStrings = parsed
		}
	}

	portFloat, ok := settings["port"].(float64)
	if !ok {
		portFloat = 161
	}
	port := uint16(portFloat)

	timeoutFloat, ok := settings["timeout_seconds"].(float64)
	if !ok {
		timeoutFloat = 3 // 5'ten 3'e düşürdük
	}
	timeout := time.Duration(timeoutFloat) * time.Second

	// Paralel işlem
	var wg sync.WaitGroup
	maxConcurrent := 10 // SNMP hızlı, daha fazla paralel
	semaphore := make(chan struct{}, maxConcurrent)

	// Her cihaz için SNMP dene (PARALEL)
	for i := range items {
		if items[i].IPAddress == "" {
			continue
		}

		wg.Add(1)
		go func(idx int, commStrs []string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Her community string ile dene
			var snmpSuccess bool
			var lastErr error

			for _, community := range commStrs {
				params := &gosnmp.GoSNMP{
					Target:    items[idx].IPAddress,
					Port:      port,
					Community: community,
					Version:   gosnmp.Version2c,
					Timeout:   timeout,
					Retries:   1,
				}

				err := params.Connect()
				if err != nil {
					lastErr = err
					continue
				}

				snmpData := make(map[string]interface{})

				// 1. sysDescr - Device description
				if result, err := params.Get([]string{"1.3.6.1.2.1.1.1.0"}); err == nil && len(result.Variables) > 0 {
					if sysDescr := getSNMPValue(result.Variables[0]); sysDescr != "" {
						snmpData["sysDescr"] = sysDescr
					}
				}

				// 2. sysName - Device name
				if result, err := params.Get([]string{"1.3.6.1.2.1.1.5.0"}); err == nil && len(result.Variables) > 0 {
					if sysName := getSNMPValue(result.Variables[0]); sysName != "" {
						snmpData["sysName"] = sysName
						if items[idx].Hostname == "" {
							items[idx].Hostname = sysName
						}
					}
				}

				// 3. sysLocation
				if result, err := params.Get([]string{"1.3.6.1.2.1.1.6.0"}); err == nil && len(result.Variables) > 0 {
					if sysLocation := getSNMPValue(result.Variables[0]); sysLocation != "" {
						snmpData["sysLocation"] = sysLocation
					}
				}

				// 4. sysContact
				if result, err := params.Get([]string{"1.3.6.1.2.1.1.4.0"}); err == nil && len(result.Variables) > 0 {
					if sysContact := getSNMPValue(result.Variables[0]); sysContact != "" {
						snmpData["sysContact"] = sysContact
					}
				}

				// 5. sysUpTime
				if result, err := params.Get([]string{"1.3.6.1.2.1.1.3.0"}); err == nil && len(result.Variables) > 0 {
					if sysUpTime := getSNMPValue(result.Variables[0]); sysUpTime != "" {
						snmpData["sysUpTime"] = sysUpTime
					}
				}

				// 6. ifNumber
				if result, err := params.Get([]string{"1.3.6.1.2.1.2.1.0"}); err == nil && len(result.Variables) > 0 {
					if ifNumber := getSNMPValue(result.Variables[0]); ifNumber != "" {
						snmpData["ifNumber"] = ifNumber
					}
				}

				// 7. Interface MAC addresses
				if results, err := params.BulkWalkAll("1.3.6.1.2.1.2.2.1.6"); err == nil && len(results) > 0 {
					for _, result := range results {
						if macBytes, ok := result.Value.([]byte); ok && len(macBytes) == 6 {
							mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
								macBytes[0], macBytes[1], macBytes[2],
								macBytes[3], macBytes[4], macBytes[5])
							if items[idx].MACAddress == "" {
								items[idx].MACAddress = mac
							}
							break
						}
					}
				}

				// 8. SNMP ile donanım/zenginleştirme (CPU, RAM, Disk)
				enrichSNMPHardwareData(params, snmpData)

				snmpData["community_used"] = community
				items[idx].SNMPData = snmpData
				items[idx].SNMPSuccess = true
				snmpSuccess = true
				// methods_used listesine ekle
				if !stringSliceContains(items[idx].MethodsUsed, "snmp") {
					items[idx].MethodsUsed = append(items[idx].MethodsUsed, "snmp")
				}

				params.Conn.Close()
				break
			}

			if !snmpSuccess && lastErr != nil {
				items[idx].SNMPSuccess = false
				items[idx].SNMPError = fmt.Sprintf("SNMP bağlantı hatası: %v", lastErr)
			}
		}(i, communityStrings)
	}

	wg.Wait()
}

// getSNMPValue - SNMP variable'dan string değer çıkar
func getSNMPValue(variable gosnmp.SnmpPDU) string {
	switch variable.Type {
	case gosnmp.OctetString:
		if bytes, ok := variable.Value.([]byte); ok {
			return string(bytes)
		}
	case gosnmp.Integer, gosnmp.Counter32, gosnmp.Gauge32, gosnmp.TimeTicks, gosnmp.Counter64:
		return fmt.Sprintf("%v", variable.Value)
	}
	return ""
}

func enrichSNMPHardwareData(params *gosnmp.GoSNMP, snmpData map[string]interface{}) {
	if params == nil || snmpData == nil {
		return
	}

	// HOST-RESOURCES-MIB: toplam bellek (KB)
	if result, err := params.Get([]string{"1.3.6.1.2.1.25.2.2.0"}); err == nil && len(result.Variables) > 0 {
		memKB := parseNumericValue(getSNMPValue(result.Variables[0]))
		if memKB > 0 {
			memGB := memKB / 1024 / 1024
			snmpData["memory_kb"] = memKB
			snmpData["total_ram_gb"] = fmt.Sprintf("%.2f GB", memGB)
		}
	}

	// HOST-RESOURCES-MIB: CPU yük tablosu (hrProcessorLoad)
	if results, err := params.BulkWalkAll("1.3.6.1.2.1.25.3.3.1.2"); err == nil && len(results) > 0 {
		var cpuLoads []float64
		for _, result := range results {
			load := parseNumericValue(getSNMPValue(result))
			if load >= 0 {
				cpuLoads = append(cpuLoads, load)
			}
		}
		if len(cpuLoads) > 0 {
			total := 0.0
			for _, v := range cpuLoads {
				total += v
			}
			avg := total / float64(len(cpuLoads))
			snmpData["cpu_core_count"] = len(cpuLoads)
			snmpData["cpu_load_avg_percent"] = fmt.Sprintf("%.1f%%", avg)
		}
	}

	// HOST-RESOURCES-MIB: disk/storage
	descrMap := make(map[string]string)
	unitMap := make(map[string]float64)
	sizeMap := make(map[string]float64)
	usedMap := make(map[string]float64)

	if results, err := params.BulkWalkAll("1.3.6.1.2.1.25.2.3.1.3"); err == nil {
		for _, result := range results {
			idx := strings.TrimPrefix(result.Name, ".1.3.6.1.2.1.25.2.3.1.3.")
			if idx == result.Name || idx == "" {
				continue
			}
			if descr := strings.TrimSpace(getSNMPValue(result)); descr != "" {
				descrMap[idx] = descr
			}
		}
	}
	if results, err := params.BulkWalkAll("1.3.6.1.2.1.25.2.3.1.4"); err == nil {
		for _, result := range results {
			idx := strings.TrimPrefix(result.Name, ".1.3.6.1.2.1.25.2.3.1.4.")
			if idx == result.Name || idx == "" {
				continue
			}
			unitMap[idx] = parseNumericValue(getSNMPValue(result))
		}
	}
	if results, err := params.BulkWalkAll("1.3.6.1.2.1.25.2.3.1.5"); err == nil {
		for _, result := range results {
			idx := strings.TrimPrefix(result.Name, ".1.3.6.1.2.1.25.2.3.1.5.")
			if idx == result.Name || idx == "" {
				continue
			}
			sizeMap[idx] = parseNumericValue(getSNMPValue(result))
		}
	}
	if results, err := params.BulkWalkAll("1.3.6.1.2.1.25.2.3.1.6"); err == nil {
		for _, result := range results {
			idx := strings.TrimPrefix(result.Name, ".1.3.6.1.2.1.25.2.3.1.6.")
			if idx == result.Name || idx == "" {
				continue
			}
			usedMap[idx] = parseNumericValue(getSNMPValue(result))
		}
	}

	if len(descrMap) > 0 {
		var storages []map[string]interface{}
		totalBytes := 0.0
		usedBytes := 0.0
		for idx, descr := range descrMap {
			allocUnit := unitMap[idx]
			size := sizeMap[idx]
			used := usedMap[idx]
			if allocUnit <= 0 || size <= 0 {
				continue
			}

			lowerDescr := strings.ToLower(descr)
			isDiskLike := strings.Contains(lowerDescr, "/") ||
				strings.Contains(lowerDescr, "disk") ||
				strings.Contains(lowerDescr, "c:") ||
				strings.Contains(lowerDescr, "d:")
			if !isDiskLike {
				continue
			}

			entryTotal := allocUnit * size
			entryUsed := allocUnit * used
			totalBytes += entryTotal
			usedBytes += entryUsed

			storages = append(storages, map[string]interface{}{
				"description": descr,
				"total_gb":    fmt.Sprintf("%.2f GB", entryTotal/1024/1024/1024),
				"used_gb":     fmt.Sprintf("%.2f GB", entryUsed/1024/1024/1024),
			})
		}

		if len(storages) > 0 {
			snmpData["storage"] = storages
			snmpData["disk_total_gb"] = fmt.Sprintf("%.2f GB", totalBytes/1024/1024/1024)
			snmpData["disk_used_gb"] = fmt.Sprintf("%.2f GB", usedBytes/1024/1024/1024)
		}
	}
}

// trySNMPEnrichmentOnItem - Try SNMP connection on single item, returns true if successful
// Used by performSNMPDiscoveryWithTargets to only add items where SNMP connection works
func trySNMPEnrichmentOnItem(item *DiscoveredItem, settings map[string]interface{}) bool {
	if item == nil || item.IPAddress == "" {
		return false
	}

	// SNMP ayarlarını çıkar
	communityStringsRaw, _ := settings["community_strings"].(string)
	communityStrings := []string{"public"}
	if communityStringsRaw != "" {
		// JSON array parse et: ["public", "private"]
		var parsed []string
		if err := json.Unmarshal([]byte(communityStringsRaw), &parsed); err == nil && len(parsed) > 0 {
			communityStrings = parsed
		}
	}

	portFloat, ok := settings["port"].(float64)
	if !ok {
		portFloat = 161
	}
	port := uint16(portFloat)

	timeoutFloat, ok := settings["timeout_seconds"].(float64)
	if !ok {
		timeoutFloat = 1 // Changed from 3 to 1 second for faster discovery
	}
	timeout := time.Duration(timeoutFloat) * time.Second

	// Her community string ile dene
	var snmpSuccess bool
	var lastErr error

	for _, community := range communityStrings {
		params := &gosnmp.GoSNMP{
			Target:    item.IPAddress,
			Port:      port,
			Community: community,
			Version:   gosnmp.Version2c,
			Timeout:   timeout,
			Retries:   0,  // Changed from 1 to 0 - no retries for faster discovery
			MaxOids:   60, // Enable bulk queries for speed (like Server Status page)
		}

		err := params.Connect()
		if err != nil {
			lastErr = err
			continue
		}

		snmpData := make(map[string]interface{})
		hadData := false

		// BULK QUERY: Get all system info in ONE request (much faster!)
		oids := []string{
			"1.3.6.1.2.1.1.1.0", // sysDescr
			"1.3.6.1.2.1.1.5.0", // sysName
			"1.3.6.1.2.1.1.6.0", // sysLocation
			"1.3.6.1.2.1.1.4.0", // sysContact
			"1.3.6.1.2.1.1.3.0", // sysUpTime
			"1.3.6.1.2.1.2.1.0", // ifNumber
		}

		result, err := params.Get(oids)
		if err != nil {
			lastErr = err
			params.Conn.Close()
			continue
		}

		// Parse bulk response (all OIDs in one request!)
		if len(result.Variables) > 0 {
			if sysDescr := getSNMPValue(result.Variables[0]); sysDescr != "" {
				snmpData["sysDescr"] = sysDescr
				hadData = true
			}
		}
		if len(result.Variables) > 1 {
			if sysName := getSNMPValue(result.Variables[1]); sysName != "" {
				snmpData["sysName"] = sysName
				hadData = true
				if item.Hostname == "" {
					item.Hostname = sysName
				}
			}
		}
		if len(result.Variables) > 2 {
			if sysLocation := getSNMPValue(result.Variables[2]); sysLocation != "" {
				snmpData["sysLocation"] = sysLocation
				hadData = true
			}
		}
		if len(result.Variables) > 3 {
			if sysContact := getSNMPValue(result.Variables[3]); sysContact != "" {
				snmpData["sysContact"] = sysContact
				hadData = true
			}
		}
		if len(result.Variables) > 4 {
			if sysUpTime := getSNMPValue(result.Variables[4]); sysUpTime != "" {
				snmpData["sysUpTime"] = sysUpTime
				hadData = true
			}
		}
		if len(result.Variables) > 5 {
			if ifNumber := getSNMPValue(result.Variables[5]); ifNumber != "" {
				snmpData["ifNumber"] = ifNumber
				hadData = true
			}
		}

		// 7. Interface MAC addresses
		if results, err := params.BulkWalkAll("1.3.6.1.2.1.2.2.1.6"); err == nil && len(results) > 0 {
			for _, result := range results {
				if macBytes, ok := result.Value.([]byte); ok && len(macBytes) == 6 {
					mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
						macBytes[0], macBytes[1], macBytes[2],
						macBytes[3], macBytes[4], macBytes[5])
					if item.MACAddress == "" {
						item.MACAddress = mac
					}
					hadData = true
					break
				}
			}
		}

		// 8. SNMP ile donanım/zenginleştirme (CPU, RAM, Disk)
		enrichSNMPHardwareData(params, snmpData)
		if _, ok := snmpData["total_ram_gb"]; ok {
			hadData = true
		}
		if _, ok := snmpData["disk_total_gb"]; ok {
			hadData = true
		}
		if _, ok := snmpData["cpu_load_avg_percent"]; ok {
			hadData = true
		}

		params.Conn.Close()

		// Check if we collected any data
		if !hadData {
			lastErr = fmt.Errorf("SNMP bağlandı ama veri alınamadı")
			continue // Try next community string
		}

		// SUCCESS: SNMP connection worked AND data collected
		snmpData["community_used"] = community
		item.SNMPData = snmpData
		item.SNMPSuccess = true
		snmpSuccess = true
		if !stringSliceContains(item.MethodsUsed, "snmp") {
			item.MethodsUsed = append(item.MethodsUsed, "snmp")
		}
		return true
	}

	// All community strings failed
	if !snmpSuccess && lastErr != nil {
		item.SNMPSuccess = false
		item.SNMPError = fmt.Sprintf("SNMP bağlantı hatası: %v", lastErr)
	}
	return false // SNMP failed - DO NOT add to results
}

// performWinRMEnrichment - WinRM ile Windows cihazlardan donanım bilgilerini çek (PARALEL)
// enrichSingleWinRM, tek bir DiscoveredItem'ı verilen WinRM ayarlarıyla zenginleştirir.
// Standalone host'lar için kullanılır; performWinRMEnrichment ile aynı mantığı izler.
func enrichSingleWinRM(item *DiscoveredItem, settings map[string]interface{}, crackEnabled bool) {
	items := []DiscoveredItem{*item}
	// shouldTryWinRM, ADData.OSName'e bakar — standalone host'larda ADData yok.
	// Sahte bir ADData oluşturarak kontrolü geçiyoruz.
	items[0].ADData = &ADDiscoveryData{OSName: "Windows"}
	items[0].AssetType = "pc"
	performWinRMEnrichment(items, settings, crackEnabled)
	*item = items[0]
}

func performWinRMEnrichment(items []DiscoveredItem, settings map[string]interface{}, crackEnabled bool) {
	// WinRM ayarlarını çıkar
	username, _ := settings["username"].(string)
	password, _ := settings["password"].(string)

	port := toInt(settings["port"], 5985)
	timeout := time.Duration(toInt(settings["timeout"], 3)) * time.Second
	useSSL, _ := settings["use_ssl"].(bool)

	if username == "" || password == "" {
		for i := range items {
			if shouldTryWinRM(&items[i]) {
				addMethod(&items[i], "winrm")
				items[i].WinRMSuccess = false
				items[i].WinRMError = "WinRM kullanıcı adı veya parolası belirtilmedi"
			}
		}
		return
	}

	// Paralel işlem için goroutine + sync
	var wg sync.WaitGroup
	maxConcurrent := 10 // Aynı anda max 10 bağlantı
	semaphore := make(chan struct{}, maxConcurrent)

	// Her Windows cihaz için WinRM bağlantısı dene (PARALEL)
	for i := range items {
		if !shouldTryWinRM(&items[i]) {
			continue
		}

		if items[i].IPAddress == "" {
			addMethod(&items[i], "winrm")
			items[i].WinRMSuccess = false
			items[i].WinRMError = "IP adresi bulunamadı"
			continue
		}

		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			settingsCopy := map[string]interface{}{
				"username":     username,
				"password":     password,
				"port":         port,
				"use_ssl":      useSSL,
				"timeout":      timeout.Seconds(),
				"username_raw": settings["username_raw"],
			}

			// Hostname varsa tercih et (DNS FQDN), yoksa IP kullan
			// tryWinRMClient hem hostname hem IP'yi, hem 5985 hem 5986'yı dener
			target := items[idx].IPAddress
			ipFallback := ""
			if items[idx].ADData != nil && items[idx].ADData.DNSHostName != "" {
				target = items[idx].ADData.DNSHostName
				ipFallback = items[idx].IPAddress // Hostname başarısız olursa IP dene
			}
			client, err := tryWinRMClient(target, ipFallback, settingsCopy)
			if err != nil {
				addMethod(&items[idx], "winrm")
				items[idx].WinRMSuccess = false
				// Debug: tam hata + kullanılan kullanıcı adını göster
				items[idx].WinRMError = fmt.Sprintf("[user: %s] %v", username, err)
				return
			}

			winrmData := make(map[string]interface{})
			hadData := false

			// Batch A: Hızlı WMI sorguları (CS, BIOS, CPU, diskler, ağ, OS, bellek,
			// GPU, monitör) — kritik donanım verisi, tipik cihazda ~5-10 saniye.
			// Win32_PnPEntity sorguları (klavye/fare/BT) kasıtlı olarak dışarıda:
			// tüm PnP aygıtlarını sayımlamaları nedeniyle 15-30 saniye çekebilir ve
			// kritik veri toplamayı + crack scan'i bloklamamalı.
			psBatchA := `` +
				`$r=@{` +
				`CS=(Get-WmiObject Win32_ComputerSystem|Select-Object Name,Manufacturer,Model,TotalPhysicalMemory,Domain);` +
				`BIOS=(Get-WmiObject Win32_BIOS|Select-Object SerialNumber,Manufacturer,SMBIOSBIOSVersion);` +
				`CPU=(Get-WmiObject Win32_Processor|Select-Object -First 1 Name,NumberOfCores,NumberOfLogicalProcessors,MaxClockSpeed,LoadPercentage);` +
				`LD=@(Get-WmiObject Win32_LogicalDisk -Filter 'DriveType=3'|Select-Object DeviceID,Size,FreeSpace);` +
				`NET=@(Get-WmiObject Win32_NetworkAdapterConfiguration -Filter 'IPEnabled=True'|Select-Object Description,MACAddress,IPAddress);` +
				`OS=(Get-WmiObject Win32_OperatingSystem|Select-Object Caption,Version,BuildNumber,LastBootUpTime);` +
				`MEM=@(Get-WmiObject Win32_PhysicalMemory|Select-Object Manufacturer,Capacity,Speed,PartNumber,SerialNumber,BankLabel);` +
				`DD=@(Get-WmiObject Win32_DiskDrive|Select-Object Model,InterfaceType,MediaType,Size,SerialNumber);` +
				`VID=@(Get-WmiObject Win32_VideoController|Select-Object Name,AdapterRAM,DriverVersion);` +
				`MON=@(try{Get-WmiObject -Namespace root\wmi -Class WmiMonitorID|ForEach-Object{[PSCustomObject]@{` +
				`Manufacturer=([System.Text.Encoding]::ASCII.GetString($_.ManufacturerName)).Trim([char]0);` +
				`Model=([System.Text.Encoding]::ASCII.GetString($_.UserFriendlyName)).Trim([char]0);` +
				`Serial=([System.Text.Encoding]::ASCII.GetString($_.SerialNumberID)).Trim([char]0)}}}catch{@()});` +
				`DM=@(Get-WmiObject Win32_DesktopMonitor|Select-Object Name,MonitorManufacturer,PNPDeviceID,ScreenWidth,ScreenHeight,Status)` +
				`};ConvertTo-Json -InputObject $r -Depth 5 -Compress`

			if output, err := runWinRMCommand(client, psBatchA); err == nil && output != "" {
				var batch map[string]json.RawMessage
				if json.Unmarshal([]byte(output), &batch) == nil {
					hadData = true

					// CS – bilgisayar sistemi
					if raw, ok := batch["CS"]; ok {
						var sysInfo map[string]interface{}
						if json.Unmarshal(raw, &sysInfo) == nil {
							winrmData["computer_system"] = sysInfo
							if name, ok := sysInfo["Name"].(string); ok && name != "" {
								if items[idx].Hostname == "" {
									items[idx].Hostname = name
								}
								if items[idx].ADData != nil && items[idx].ADData.ComputerName == "" {
									items[idx].ADData.ComputerName = name
								}
							}
							if mfr, ok := sysInfo["Manufacturer"].(string); ok && mfr != "" {
								if items[idx].ADData != nil && items[idx].ADData.Vendor == "" {
									items[idx].ADData.Vendor = mfr
								}
							}
							if model, ok := sysInfo["Model"].(string); ok && model != "" {
								if items[idx].ADData != nil && items[idx].ADData.Model == "" {
									items[idx].ADData.Model = model
								}
							}
							if ramBytes, ok := sysInfo["TotalPhysicalMemory"].(float64); ok {
								winrmData["total_ram_gb"] = fmt.Sprintf("%.2f GB", ramBytes/1024/1024/1024)
							}
						}
					}
					// BIOS
					if raw, ok := batch["BIOS"]; ok {
						var biosInfo map[string]interface{}
						if json.Unmarshal(raw, &biosInfo) == nil {
							winrmData["bios"] = biosInfo
							if serial, ok := biosInfo["SerialNumber"].(string); ok && serial != "" {
								if items[idx].ADData != nil && items[idx].ADData.SerialNumber == "" {
									items[idx].ADData.SerialNumber = serial
								}
							}
						}
					}
					// CPU
					if raw, ok := batch["CPU"]; ok {
						var cpuInfo map[string]interface{}
						if json.Unmarshal(raw, &cpuInfo) == nil {
							winrmData["cpu"] = cpuInfo
						}
					}
					// LD – mantıksal diskler
					if raw, ok := batch["LD"]; ok {
						var disks interface{}
						if json.Unmarshal(raw, &disks) == nil {
							winrmData["disks"] = disks
							winrmData["disks_raw"] = string(raw)
						}
					}
					// NET – ağ adaptörleri
					if raw, ok := batch["NET"]; ok {
						var adapters interface{}
						if json.Unmarshal(raw, &adapters) == nil {
							winrmData["network_adapters"] = adapters
							winrmData["network_adapters_raw"] = string(raw)
							if items[idx].MACAddress == "" {
								if adapterList, ok := adapters.([]interface{}); ok && len(adapterList) > 0 {
									if adapterMap, ok := adapterList[0].(map[string]interface{}); ok {
										if mac, ok := adapterMap["MACAddress"].(string); ok && mac != "" {
											items[idx].MACAddress = strings.ToUpper(strings.ReplaceAll(mac, "-", ":"))
										}
									}
								}
							}
						}
					}
					// OS
					if raw, ok := batch["OS"]; ok {
						var osInfo map[string]interface{}
						if json.Unmarshal(raw, &osInfo) == nil {
							winrmData["os"] = osInfo
							if caption, ok := osInfo["Caption"].(string); ok && caption != "" {
								if items[idx].ADData != nil {
									items[idx].ADData.OSName = caption
								}
							}
							if version, ok := osInfo["Version"].(string); ok && version != "" {
								if items[idx].ADData != nil && items[idx].ADData.OSVersion == "" {
									items[idx].ADData.OSVersion = version
								}
							}
						}
					}
					// MEM – fiziksel bellek modülleri
					if raw, ok := batch["MEM"]; ok {
						var memModules interface{}
						if json.Unmarshal(raw, &memModules) == nil {
							winrmData["physical_memory_modules"] = memModules
						}
					}
					// DD – fiziksel diskler
					if raw, ok := batch["DD"]; ok {
						var physDisks interface{}
						if json.Unmarshal(raw, &physDisks) == nil {
							winrmData["physical_disks"] = physDisks
							if gb := sumDiskSizeGB(physDisks); gb > 0 {
								winrmData["disk_total_gb"] = gb
							}
						}
					}
					// VID – ekran kartları
					if raw, ok := batch["VID"]; ok {
						var gpus interface{}
						if json.Unmarshal(raw, &gpus) == nil {
							winrmData["video_controllers"] = gpus
						}
					}
					// MON – monitör EDID
					if raw, ok := batch["MON"]; ok {
						var monitors interface{}
						if json.Unmarshal(raw, &monitors) == nil {
							winrmData["monitors"] = monitors
						}
					}
					// DM – desktop monitor
					if raw, ok := batch["DM"]; ok {
						var monDevices interface{}
						if json.Unmarshal(raw, &monDevices) == nil {
							winrmData["monitor_devices"] = monDevices
						}
					}
				}
			}

			// Yazılım listesi ayrı çalışır: büyük payload ve bağımsız — batch'e
			// eklenirse JSON boyutu öngörülemeyen büyüklüğe ulaşabilir.
			psSoftware := `$a=@(Get-ItemProperty 'HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\*' -EA SilentlyContinue)+@(Get-ItemProperty 'HKLM:\Software\Wow6432Node\Microsoft\Windows\CurrentVersion\Uninstall\*' -EA SilentlyContinue);$r=$a|Where-Object{$_.DisplayName}|Select-Object DisplayName,DisplayVersion,Publisher,InstallDate|Sort-Object DisplayName;ConvertTo-Json -InputObject @($r) -Compress`
			if output, err := runWinRMCommand(client, psSoftware); err == nil && output != "" {
				var softwareRaw interface{}
				if json.Unmarshal([]byte(output), &softwareRaw) == nil {
					switch s := softwareRaw.(type) {
					case []interface{}:
						winrmData["software_list"] = s
					case map[string]interface{}:
						winrmData["software_list"] = []interface{}{s}
					}
				}
			}

			// LastBootTime – OS verisinden çıkar
			if osData, ok := winrmData["os"].(map[string]interface{}); ok {
				if lastBoot, ok := osData["LastBootUpTime"].(string); ok && lastBoot != "" {
					if parsed := parseWMIDatetime(lastBoot); parsed != "" {
						items[idx].LastBootTime = parsed
					}
				}
			}

			if hadData {
				items[idx].WinRMData = winrmData
				items[idx].WinRMSuccess = true
				addMethod(&items[idx], "winrm")

				if crackEnabled {
					_, findings, _ := PerformCrackScan(client)
					items[idx].CrackFindings = findings
				}

				// Batch B: Yavaş Win32_PnPEntity sorguları (klavye, fare, Bluetooth).
				// Crack scan ve kritik donanım verisi zaten toplandıktan sonra çalışır —
				// böylece PnP yavaşlığı (15-30 sn) crack tespitini bloklamaz.
				psBatchB := `` +
					`$r=@{` +
					`KB=@(Get-WmiObject Win32_PnPEntity|Where-Object{$_.PNPClass -eq 'Keyboard'}|Select-Object Name,Manufacturer,Status,PNPDeviceID);` +
					`MS=@(Get-WmiObject Win32_PnPEntity|Where-Object{$_.PNPClass -eq 'Mouse'-or($_.PNPClass -eq 'HIDClass'-and($_.Name -match 'Mouse|Touchpad|Pointing'))}|Select-Object Name,Manufacturer,Status,PNPDeviceID);` +
					`BT=@(Get-WmiObject Win32_PnPEntity|Where-Object{$_.PNPClass -eq 'Bluetooth'}|Select-Object Name,Manufacturer,Status,PNPDeviceID)` +
					`};ConvertTo-Json -InputObject $r -Depth 5 -Compress`
				if outB, errB := runWinRMCommand(client, psBatchB); errB == nil && outB != "" {
					var batchB map[string]json.RawMessage
					if json.Unmarshal([]byte(outB), &batchB) == nil {
						if raw, ok := batchB["KB"]; ok {
							var keyboards interface{}
							if json.Unmarshal(raw, &keyboards) == nil {
								winrmData["keyboards"] = keyboards
							}
						}
						if raw, ok := batchB["MS"]; ok {
							var mice interface{}
							if json.Unmarshal(raw, &mice) == nil {
								winrmData["mice"] = mice
							}
						}
						if raw, ok := batchB["BT"]; ok {
							var btDevices interface{}
							if json.Unmarshal(raw, &btDevices) == nil {
								winrmData["bluetooth_devices"] = btDevices
							}
						}
						// WinRMData'yı peripheral verilerle güncelle
						items[idx].WinRMData = winrmData
					}
				}

				return
			}

			items[idx].WinRMSuccess = false
			addMethod(&items[idx], "winrm")
			items[idx].WinRMError = "WinRM bağlandı ama WMI batch sorgusu veri döndürmedi"
		}(i) // goroutine'e index gönder
	}

	wg.Wait() // Tüm WinRM bağlantılarının bitmesini bekle
}

// shouldTryWinRM - Bu cihaz için WinRM denemeli miyiz?
func shouldTryWinRM(item *DiscoveredItem) bool {
	if item.ADData == nil || item.ADData.OSName == "" {
		return false
	}

	osLower := strings.ToLower(item.ADData.OSName)
	return strings.Contains(osLower, "windows")
}

// runWinRMCommand - WinRM üzerinden PowerShell komutu çalıştır.
// WinRM (WinRS_SKIP_CMD_SHELL=FALSE) komutu cmd.exe üzerinden çalıştırır ve cmd.exe satır
// bazlıdır: komutun içine gömülü gerçek newline karakterleri sessizce komutu keser/bozar
// (çok satırlı PS script'leri hiçbir hata vermeden boş sonuç döndürür). Bunu önlemek için
// script UTF-16LE + base64'e çevrilir (encodePowerShellCommand) - bu hem newline sorununu
// hem de gömülü " karakterleri için tırnak escaping sorununu ortadan kaldırır. Bu payload
// komut satırına DEĞİL, STDIN'e gönderilir (sabit/kısa bir decode+çalıştır önyükleyicisiyle):
// -EncodedCommand'ı doğrudan komut satırında kullanmak script büyüdükçe Windows'un ~8191
// karakterlik komut satırı sınırına çarpabilir ve CreateProcess hiçbir hata vermeden sessizce
// başarısız olur - STDIN'e taşımak script boyutundan bağımsız hale getirir.
func runWinRMCommand(client *winrm.Client, command string) (string, error) {
	// WinRM'in kendi protokol-seviyesi "receive" polling'i, uzaktaki komut gerçekten
	// asılı kalırsa (örn. büyük bir event log sorgusu) süresiz bekleyebilir; client/endpoint
	// timeout'u sadece bağlantı/probe içindir, komut çalışma süresini sınırlamaz. Context
	// timeout'u tek bir komutun tüm taramayı sonsuza kadar kilitlemesini önler.
	ctx, cancel := context.WithTimeout(context.Background(), winRMCommandTimeout)
	defer cancel()

	// "Turkish I" sorunu: .NET'in -imatch/-match için kullandığı culture-aware case
	// katlama, Türkçe (tr-TR) yerelinde büyük 'I' harfini noktasız 'ı'ya çevirir, normal
	// 'i'ye değil - bu yüzden örn. 'HWIDGEN.exe' -imatch 'hwidgen' Türkçe Windows'ta False
	// döner. Her komutun başına invariant culture set'i eklemek bunu TÜM -imatch/-match
	// kullanımları için tek noktadan düzeltir (her PS script'ini ayrı ayrı değiştirmek yerine).
	const cultureFix = "[System.Threading.Thread]::CurrentThread.CurrentCulture = [System.Globalization.CultureInfo]::InvariantCulture; [System.Threading.Thread]::CurrentThread.CurrentUICulture = [System.Globalization.CultureInfo]::InvariantCulture; "
	encoded := encodePowerShellCommand(cultureFix + command)

	// -EncodedCommand payload'u komut satırına gömülür; script büyüdükçe (örn. kurulu her
	// programı dolaşan kontroller) Windows'un ~8191 karakterlik komut satırı sınırına
	// yaklaşır/aşar - bu durumda CreateProcess HİÇBİR HATA VERMEDEN sessizce başarısız olur,
	// sadece boş çıktı döner (bu yüzden fark edilmesi çok zordu). Payload'u komut satırı
	// argümanı yerine STDIN üzerinden göndermek (sabit, kısa bir decode+çalıştır
	// önyükleyicisiyle) script boyutundan tamamen bağımsız hale getirir - artık ne kadar
	// uzun olursa olsun bu sınıra hiç çarpmaz.
	const bootstrap = `$b64=[Console]::In.ReadToEnd();$bytes=[Convert]::FromBase64String($b64);$s=[System.Text.Encoding]::Unicode.GetString($bytes);Invoke-Expression $s`
	psCommand := `powershell -NoProfile -NonInteractive -Command "` + bootstrap + `"`
	stdout, stderr, _, err := client.RunWithContextWithString(ctx, psCommand, encoded)
	if err != nil {
		return "", fmt.Errorf("command error: %v, stderr: %s", err, stderr)
	}
	return strings.TrimSpace(stdout), nil
}

// winRMCommandTimeout, tek bir uzak PowerShell komutuna izin verilen üst süre sınırıdır.
const winRMCommandTimeout = 90 * time.Second

// encodePowerShellCommand, -EncodedCommand'ın beklediği formata (UTF-16LE bayt dizisinin
// base64'ü) script'i dönüştürür.
func encodePowerShellCommand(script string) string {
	units := utf16.Encode([]rune(script))
	buf := make([]byte, len(units)*2)
	for i, u := range units {
		binary.LittleEndian.PutUint16(buf[i*2:], u)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func sumDiskSizeGB(v interface{}) float64 {
	entries := interfaceToSlice(v)
	if len(entries) == 0 {
		return 0
	}

	totalBytes := float64(0)
	for _, entry := range entries {
		obj, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}

		sizeBytes := parseNumericValue(obj["Size"])
		if sizeBytes > 0 {
			totalBytes += sizeBytes
		}
	}

	if totalBytes <= 0 {
		return 0
	}
	return totalBytes / 1024 / 1024 / 1024
}

func interfaceToSlice(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]interface{}); ok {
		return arr
	}
	return []interface{}{v}
}

func parseNumericValue(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case uint:
		return float64(n)
	case uint64:
		return float64(n)
	case uint32:
		return float64(n)
	case uint16:
		return float64(n)
	case int16:
		return float64(n)
	case string:
		if n == "" {
			return 0
		}
		parsed, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0
		}
		return parsed
	default:
		return 0
	}
}

// performSSHEnrichment - SSH ile Linux/macOS cihazlardan detaylı bilgi topla (PARALEL)
func performSSHEnrichment(items []DiscoveredItem, settings map[string]interface{}) {
	// SSH ayarlarını çıkar
	username, _ := settings["username"].(string)
	password, _ := settings["password"].(string)
	portFloat, ok := settings["port"].(float64)
	if !ok {
		portFloat = 22
	}
	port := int(portFloat)

	timeoutFloat, ok := settings["timeout"].(float64)
	if !ok {
		timeoutFloat = 5 // 10 saniye çok uzun! 5 saniyeye düşürdük
	}
	timeout := time.Duration(timeoutFloat) * time.Second

	if username == "" || password == "" {
		// Kullanıcı adı/parola yok - SSH hatası gösterme (gereksiz)
		return
	}

	// 1. Port 22 taraması yap - sadece açık olanları SSH ile dene (1 sn timeout)
	var targetsForSSH []string
	for i := range items {
		if shouldTrySSH(&items[i]) && items[i].IPAddress != "" {
			targetsForSSH = append(targetsForSSH, items[i].IPAddress)
		}
	}

	if len(targetsForSSH) == 0 {
		return // SSH deneyecek cihaz yok
	}

	// Port 22 açık olanları bul (hızlı TCP port scan)
	openSSHHosts := checkPortOpen(targetsForSSH, port, 1*time.Second)
	if len(openSSHHosts) == 0 {
		return // Hiçbir cihazda port 22 açık değil - hata göstermeye gerek yok
	}

	// Port 22 açık olanları bir map'te tut
	openHostsMap := make(map[string]bool)
	for _, ip := range openSSHHosts {
		openHostsMap[ip] = true
	}

	// SSH config
	sshConfig := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Production'da düzelt!
		Timeout:         timeout,
	}

	// Paralel işlem için goroutine + sync
	var wg sync.WaitGroup
	maxConcurrent := 10 // Aynı anda max 10 bağlantı
	semaphore := make(chan struct{}, maxConcurrent)

	// Her cihaz için SSH bağlantısı dene (PARALEL) - SADECE PORT 22 AÇIK OLANLARA
	for i := range items {
		// Sadece Linux/macOS cihazlare bağlan
		if !shouldTrySSH(&items[i]) {
			continue
		}

		if items[i].IPAddress == "" {
			continue // IP yok - hata gösterme
		}

		// Port 22 kapalıysa SSH deneme - hata gösterme (SNMP-only cihazlar için)
		if !openHostsMap[items[i].IPAddress] {
			continue
		}

		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			// SSH bağlantısı kur
			address := fmt.Sprintf("%s:%d", items[idx].IPAddress, port)
			client, err := ssh.Dial("tcp", address, sshConfig)
			if err != nil {
				items[idx].SSHSuccess = false
				items[idx].SSHError = fmt.Sprintf("Bağlantı hatası: %v", err)
				return // goroutine içinde continue yerine return
			}
			defer client.Close()

			// Bilgi topla
			sshData := make(map[string]interface{})

			// 1. OS Detayları (uname -a)
			if output, err := runSSHCommand(client, "uname -a"); err == nil {
				sshData["uname"] = strings.TrimSpace(output)

				// OS bilgisini parse et
				parts := strings.Fields(output)
				if len(parts) >= 3 {
					sshData["kernel_name"] = parts[0]    // Linux / Darwin
					sshData["kernel_version"] = parts[2] // 5.15.0-76-generic
				}
			}

			// 2. Hostname
			if output, err := runSSHCommand(client, "hostname"); err == nil {
				hostname := strings.TrimSpace(output)
				sshData["hostname"] = hostname

				// Eğer hostname boşsa, SSH'tan gelen ile güncelle
				if items[idx].Hostname == "" {
					items[idx].Hostname = hostname
				}
			}

			// 3. OS Detaylı Bilgi (/etc/os-release)
			if output, err := runSSHCommand(client, "cat /etc/os-release 2>/dev/null || cat /etc/lsb-release 2>/dev/null || sw_vers"); err == nil {
				osInfo := parseOSRelease(output)
				if len(osInfo) > 0 {
					sshData["os_release"] = osInfo

					// Pretty name varsa kullan
					if prettyName, ok := osInfo["PRETTY_NAME"]; ok {
						sshData["os_pretty_name"] = prettyName
					}

					// macOS için
					if productName, ok := osInfo["ProductName"]; ok {
						sshData["os_pretty_name"] = fmt.Sprintf("%s %s", productName, osInfo["ProductVersion"])
					}
				}
			}

			// 4. Network Interfaces
			netCmd := "ip addr show 2>/dev/null || ifconfig"
			if output, err := runSSHCommand(client, netCmd); err == nil {
				interfaces := parseNetworkInterfaces(output)
				sshData["network_interfaces"] = interfaces

				// MAC adreslerini güncelle
				if items[idx].MACAddress == "" && len(interfaces) > 0 {
					// İlk fiziksel interface'in MAC'ini al (lo hariç)
					for _, iface := range interfaces {
						if ifaceMap, ok := iface.(map[string]interface{}); ok {
							if name, _ := ifaceMap["name"].(string); name != "lo" && name != "lo0" {
								if mac, ok := ifaceMap["mac"].(string); ok && mac != "" {
									items[idx].MACAddress = mac
									break
								}
							}
						}
					}
				}
			}

			// 5. Current User
			if output, err := runSSHCommand(client, "whoami"); err == nil {
				sshData["current_user"] = strings.TrimSpace(output)
			}

			// 6. Uptime
			if output, err := runSSHCommand(client, "uptime"); err == nil {
				sshData["uptime"] = strings.TrimSpace(output)
			}

			// 7. Memory Info
			if output, err := runSSHCommand(client, "free -m 2>/dev/null || vm_stat"); err == nil {
				sshData["memory_info"] = strings.TrimSpace(output)
			}

			// 8. Disk Usage
			if output, err := runSSHCommand(client, "df -h / 2>/dev/null"); err == nil {
				sshData["disk_usage"] = strings.TrimSpace(output)
			}

			// 9. CPU Info
			cpuCmd := "lscpu 2>/dev/null || sysctl -n machdep.cpu.brand_string 2>/dev/null || cat /proc/cpuinfo | grep 'model name' | head -1"
			if output, err := runSSHCommand(client, cpuCmd); err == nil {
				cpuInfo := strings.TrimSpace(output)
				if cpuInfo != "" {
					sshData["cpu_info"] = cpuInfo

					// Model name'i extract et
					if strings.Contains(cpuInfo, "model name") {
						parts := strings.Split(cpuInfo, ":")
						if len(parts) > 1 {
							sshData["cpu_model"] = strings.TrimSpace(parts[1])
						}
					} else {
						sshData["cpu_model"] = cpuInfo
					}
				}
			}

			// 10. Yapılandırılmış SSH donanım verileri (RAM, disk, son yeniden başlatma)
			enrichSSHHardwareData(client, sshData)

			// 11. Serial Number (DMI/macOS)
			serialCmd := "sudo dmidecode -s system-serial-number 2>/dev/null || cat /sys/class/dmi/id/product_serial 2>/dev/null"
			if output, err := runSSHCommand(client, serialCmd); err == nil {
				serial := strings.TrimSpace(output)
				if serial != "" && !isSSHErrorOutput(serial) {
					sshData["serial_number"] = serial
					if items[idx].ADData != nil && items[idx].ADData.SerialNumber == "" {
						items[idx].ADData.SerialNumber = serial
					}
				}
			}

			// 12. Kurulu yazılımlar (dpkg/rpm)
			softwareCmd := `dpkg -l 2>/dev/null | grep '^ii' | awk '{print $2"\t"$3"\t"$4}' | head -300`
			if output, err := runSSHCommand(client, softwareCmd); err == nil && output != "" && !isSSHErrorOutput(output) {
				if software := parseSSHSoftwareList(output); len(software) > 0 {
					sshData["software_list"] = software
				}
			} else {
				// RPM tabanlı sistemler (RHEL, CentOS, Fedora)
				rpmCmd := `rpm -qa --queryformat '%{NAME}\t%{VERSION}\t%{VENDOR}\n' 2>/dev/null | sort | head -300`
				if output, err := runSSHCommand(client, rpmCmd); err == nil && output != "" && !isSSHErrorOutput(output) {
					if software := parseSSHSoftwareList(output); len(software) > 0 {
						sshData["software_list"] = software
					}
				}
			}

			// Son açılış zamanını sshData'dan çıkar (enrichSSHHardwareData zaten dolduruyor)
			if lbt, ok := sshData["last_boot_time"].(string); ok && lbt != "" {
				items[idx].LastBootTime = lbt
			}

			// 13. Manufacturer/Model (DMI)
			if output, err := runSSHCommand(client, "sudo dmidecode -s system-manufacturer 2>/dev/null || cat /sys/class/dmi/id/sys_vendor 2>/dev/null"); err == nil {
				manufacturer := strings.TrimSpace(output)
				if manufacturer != "" && !isSSHErrorOutput(manufacturer) {
					sshData["manufacturer"] = manufacturer
					if items[idx].ADData != nil && items[idx].ADData.Vendor == "" {
						items[idx].ADData.Vendor = manufacturer
					}
				}
			}

			if output, err := runSSHCommand(client, "sudo dmidecode -s system-product-name 2>/dev/null || cat /sys/class/dmi/id/product_name 2>/dev/null"); err == nil {
				model := strings.TrimSpace(output)
				if model != "" && !isSSHErrorOutput(model) {
					sshData["model"] = model
					if items[idx].ADData != nil && items[idx].ADData.Model == "" {
						items[idx].ADData.Model = model
					}
				}
			}

			// Başarılı
			items[idx].SSHData = sshData
			items[idx].SSHSuccess = true
			// methods_used listesine ekle
			if !stringSliceContains(items[idx].MethodsUsed, "ssh") {
				items[idx].MethodsUsed = append(items[idx].MethodsUsed, "ssh")
			}
		}(i) // goroutine'e index gönder
	}

	wg.Wait() // Tüm SSH bağlantılarının bitmesini bekle
}

// shouldTrySSH - Bu cihaz için SSH denemeli miyiz?
func shouldTrySSH(item *DiscoveredItem) bool {
	// WinRM başarılı olduysa, bu bir Windows cihaz - SSH deneme
	if item.WinRMSuccess {
		return false
	}

	// AD data varsa OS name'e bak
	if item.ADData != nil && item.ADData.OSName != "" {
		osLower := strings.ToLower(item.ADData.OSName)

		// Windows ise SSH deneme
		if strings.Contains(osLower, "windows") || strings.Contains(osLower, "server") {
			return false
		}

		// Linux dağıtımları
		linuxKeywords := []string{"linux", "ubuntu", "debian", "centos", "rhel", "fedora", "suse", "arch", "mint", "kali"}
		for _, keyword := range linuxKeywords {
			if strings.Contains(osLower, keyword) {
				return true
			}
		}

		// macOS
		if strings.Contains(osLower, "mac") || strings.Contains(osLower, "darwin") {
			return true
		}
	}

	// AD data yoksa da dene (non-AD Linux/Unix cihazlar için)
	// IP adresi varsa SSH bağlantısını dene
	return item.IPAddress != ""
}

// trySSHEnrichmentOnItem - Try SSH connection on single item, returns true if successful
// Used by performSSHDiscoveryWithTargets to only add items where SSH login works
func trySSHEnrichmentOnItem(item *DiscoveredItem, settings map[string]interface{}) bool {
	if item == nil || item.IPAddress == "" {
		return false
	}

	// SSH ayarlarını çıkar
	username, _ := settings["username"].(string)
	password, _ := settings["password"].(string)
	portFloat, ok := settings["port"].(float64)
	if !ok {
		portFloat = 22
	}
	port := int(portFloat)

	timeoutFloat, ok := settings["timeout"].(float64)
	if !ok {
		timeoutFloat = 5
	}
	timeout := time.Duration(timeoutFloat) * time.Second

	if username == "" || password == "" {
		item.SSHSuccess = false
		item.SSHError = "SSH kullanıcı adı veya parolası belirtilmedi"
		return false
	}

	// SSH config
	sshConfig := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	// SSH bağlantısı kur
	address := fmt.Sprintf("%s:%d", item.IPAddress, port)
	client, err := ssh.Dial("tcp", address, sshConfig)
	if err != nil {
		item.SSHSuccess = false
		item.SSHError = fmt.Sprintf("Bağlantı hatası: %v", err)
		return false // SSH login failed - DO NOT add to results
	}
	defer client.Close()

	// Initialize ADData if needed (for storing hardware info)
	if item.ADData == nil {
		item.ADData = &ADDiscoveryData{}
	}

	// Bilgi topla
	sshData := make(map[string]interface{})
	hadData := false

	// 1. OS Detayları (uname -a)
	if output, err := runSSHCommand(client, "uname -a"); err == nil && output != "" {
		sshData["uname"] = strings.TrimSpace(output)
		hadData = true

		// OS bilgisini parse et
		parts := strings.Fields(output)
		if len(parts) >= 3 {
			sshData["kernel_name"] = parts[0]    // Linux / Darwin
			sshData["kernel_version"] = parts[2] // 5.15.0-76-generic
		}
	}

	// 2. Hostname
	if output, err := runSSHCommand(client, "hostname"); err == nil && output != "" {
		hostname := strings.TrimSpace(output)
		sshData["hostname"] = hostname
		hadData = true

		// Update both Hostname and Name fields
		if item.Hostname == "" {
			item.Hostname = hostname
		}
		// Update Name field if empty or same as IP
		if item.Name == "" || item.Name == item.IPAddress {
			item.Name = hostname
		}
	}

	// 3. OS Detaylı Bilgi (/etc/os-release)
	if output, err := runSSHCommand(client, "cat /etc/os-release 2>/dev/null || cat /etc/lsb-release 2>/dev/null || sw_vers"); err == nil && output != "" {
		osInfo := parseOSRelease(output)
		if len(osInfo) > 0 {
			sshData["os_release"] = osInfo
			hadData = true

			// Pretty name varsa kullan
			if prettyName, ok := osInfo["PRETTY_NAME"]; ok {
				sshData["os_pretty_name"] = prettyName
			}

			// macOS için
			if productName, ok := osInfo["ProductName"]; ok {
				sshData["os_pretty_name"] = fmt.Sprintf("%s %s", productName, osInfo["ProductVersion"])
			}
		}
	}

	// 4. Network Interfaces
	netCmd := "ip addr show 2>/dev/null || ifconfig"
	if output, err := runSSHCommand(client, netCmd); err == nil && output != "" {
		interfaces := parseNetworkInterfaces(output)
		if len(interfaces) > 0 {
			sshData["network_interfaces"] = interfaces
			hadData = true

			// MAC adreslerini güncelle
			if item.MACAddress == "" {
				// İlk fiziksel interface'in MAC'ini al (lo hariç)
				for _, iface := range interfaces {
					if ifaceMap, ok := iface.(map[string]interface{}); ok {
						if name, _ := ifaceMap["name"].(string); name != "lo" && name != "lo0" {
							if mac, ok := ifaceMap["mac"].(string); ok && mac != "" {
								item.MACAddress = mac
								break
							}
						}
					}
				}
			}
		}
	}

	// 5. Current User
	if output, err := runSSHCommand(client, "whoami"); err == nil && output != "" {
		sshData["current_user"] = strings.TrimSpace(output)
		hadData = true
	}

	// 6. Uptime
	if output, err := runSSHCommand(client, "uptime"); err == nil && output != "" {
		sshData["uptime"] = strings.TrimSpace(output)
		hadData = true
	}

	// 7. Memory Info
	if output, err := runSSHCommand(client, "free -m 2>/dev/null || vm_stat"); err == nil && output != "" {
		sshData["memory_info"] = strings.TrimSpace(output)
		hadData = true
	}

	// 8. Disk Usage
	if output, err := runSSHCommand(client, "df -h / 2>/dev/null"); err == nil && output != "" {
		sshData["disk_usage"] = strings.TrimSpace(output)
		hadData = true
	}

	// 9. CPU Info
	cpuCmd := "lscpu 2>/dev/null || sysctl -n machdep.cpu.brand_string 2>/dev/null || cat /proc/cpuinfo | grep 'model name' | head -1"
	if output, err := runSSHCommand(client, cpuCmd); err == nil && output != "" {
		cpuInfo := strings.TrimSpace(output)
		if cpuInfo != "" {
			sshData["cpu_info"] = cpuInfo
			hadData = true

			// Model name'i extract et
			if strings.Contains(cpuInfo, "model name") {
				parts := strings.Split(cpuInfo, ":")
				if len(parts) > 1 {
					sshData["cpu_model"] = strings.TrimSpace(parts[1])
				}
			} else {
				sshData["cpu_model"] = cpuInfo
			}
		}
	}

	// 10. Yapılandırılmış SSH donanım verileri (RAM, disk, son yeniden başlatma)
	if enrichSSHHardwareData(client, sshData) {
		hadData = true
	}

	// 10. Hardware Info (x86/ARM compatible)
	// First, try to detect architecture
	archCmd := "uname -m"
	var isARM bool
	if output, err := runSSHCommand(client, archCmd); err == nil {
		arch := strings.TrimSpace(strings.ToLower(output))
		isARM = strings.Contains(arch, "arm") || strings.Contains(arch, "aarch")
		sshData["architecture"] = arch
		hadData = true
	}

	// 10a. Serial Number
	var serialFound bool
	if isARM {
		// ARM devices (Raspberry Pi, etc.) - use /proc/cpuinfo
		serialCmd := "cat /proc/cpuinfo | grep Serial | awk '{print $3}'"
		if output, err := runSSHCommand(client, serialCmd); err == nil {
			serial := strings.TrimSpace(output)
			if serial != "" && serial != "0000000000000000" {
				sshData["serial_number"] = serial
				serialFound = true
				hadData = true
				if item.ADData == nil {
					item.ADData = &ADDiscoveryData{}
				}
				if item.ADData.SerialNumber == "" {
					item.ADData.SerialNumber = serial
				}
			}
		}
	}

	if !serialFound {
		// x86/x64 devices - try dmidecode and fallbacks
		serialCmd := "sudo dmidecode -s system-serial-number 2>/dev/null || dmidecode -s system-serial-number 2>/dev/null || cat /sys/class/dmi/id/product_serial 2>/dev/null || ioreg -l | grep IOPlatformSerialNumber | awk '{print $4}' | tr -d '\"'"
		if output, err := runSSHCommand(client, serialCmd); err == nil {
			serial := strings.TrimSpace(output)
			// Filter out error messages and invalid values
			if serial != "" &&
				!strings.Contains(serial, "sudo") &&
				!strings.Contains(serial, "Permission denied") &&
				!strings.Contains(serial, "command not found") &&
				!strings.Contains(serial, "No such file") &&
				!strings.Contains(serial, "bash:") {
				sshData["serial_number"] = serial
				hadData = true
				if item.ADData == nil {
					item.ADData = &ADDiscoveryData{}
				}
				if item.ADData.SerialNumber == "" {
					item.ADData.SerialNumber = serial
				}
			}
		}
	}

	// 10b. Manufacturer
	var manufacturerFound bool
	if isARM {
		// ARM devices - try /proc/device-tree/model first, then /proc/cpuinfo
		modelCmd := "cat /proc/device-tree/model 2>/dev/null || cat /proc/cpuinfo | grep Model | cut -d: -f2"
		if output, err := runSSHCommand(client, modelCmd); err == nil {
			model := strings.TrimSpace(output)
			if model != "" {
				// Extract manufacturer from model string (e.g., "Raspberry Pi 5" -> "Raspberry Pi Foundation")
				manufacturer := "Unknown"
				if strings.Contains(strings.ToLower(model), "raspberry pi") {
					manufacturer = "Raspberry Pi Foundation"
				} else if strings.Contains(strings.ToLower(model), "nvidia") {
					manufacturer = "NVIDIA"
				} else {
					// Use first word as manufacturer
					parts := strings.Fields(model)
					if len(parts) > 0 {
						manufacturer = parts[0]
					}
				}

				sshData["manufacturer"] = manufacturer
				sshData["full_model"] = model
				manufacturerFound = true
				hadData = true
				if item.ADData == nil {
					item.ADData = &ADDiscoveryData{}
				}
				if item.ADData.Vendor == "" {
					item.ADData.Vendor = manufacturer
				}
			}
		}
	}

	if !manufacturerFound {
		// x86/x64 devices - try dmidecode and fallbacks
		manufacturerCmd := "sudo dmidecode -s system-manufacturer 2>/dev/null || dmidecode -s system-manufacturer 2>/dev/null || cat /sys/class/dmi/id/sys_vendor 2>/dev/null"
		if output, err := runSSHCommand(client, manufacturerCmd); err == nil {
			manufacturer := strings.TrimSpace(output)
			// Filter out error messages and invalid values
			if manufacturer != "" &&
				!strings.Contains(manufacturer, "sudo") &&
				!strings.Contains(manufacturer, "Permission denied") &&
				!strings.Contains(manufacturer, "command not found") &&
				!strings.Contains(manufacturer, "No such file") &&
				!strings.Contains(manufacturer, "bash:") {
				sshData["manufacturer"] = manufacturer
				hadData = true
				if item.ADData == nil {
					item.ADData = &ADDiscoveryData{}
				}
				if item.ADData.Vendor == "" {
					item.ADData.Vendor = manufacturer
				}
			}
		}
	}

	// 10c. Model
	var modelFound bool
	if isARM {
		// Already extracted in manufacturer section for ARM
		if fullModel, ok := sshData["full_model"].(string); ok && fullModel != "" {
			// Extract model without manufacturer prefix
			model := fullModel
			// For "Raspberry Pi 5 Model B Rev 1.1", extract "Pi 5 Model B Rev 1.1" or keep as is
			sshData["model"] = model
			modelFound = true
			if item.ADData == nil {
				item.ADData = &ADDiscoveryData{}
			}
			if item.ADData.Model == "" {
				item.ADData.Model = model
			}
		}
	}

	if !modelFound {
		// x86/x64 devices - try dmidecode and fallbacks
		modelCmd := "sudo dmidecode -s system-product-name 2>/dev/null || dmidecode -s system-product-name 2>/dev/null || cat /sys/class/dmi/id/product_name 2>/dev/null"
		if output, err := runSSHCommand(client, modelCmd); err == nil {
			model := strings.TrimSpace(output)
			// Filter out error messages and invalid values
			if model != "" &&
				!strings.Contains(model, "sudo") &&
				!strings.Contains(model, "Permission denied") &&
				!strings.Contains(model, "command not found") &&
				!strings.Contains(model, "No such file") &&
				!strings.Contains(model, "bash:") {
				sshData["model"] = model
				hadData = true
				if item.ADData == nil {
					item.ADData = &ADDiscoveryData{}
				}
				if item.ADData.Model == "" {
					item.ADData.Model = model
				}
			}
		}
	}

	// 10d. Revision (ARM specific - useful for Raspberry Pi)
	if isARM {
		revCmd := "cat /proc/cpuinfo | grep Revision | awk '{print $3}'"
		if output, err := runSSHCommand(client, revCmd); err == nil {
			revision := strings.TrimSpace(output)
			if revision != "" {
				sshData["revision"] = revision
				hadData = true
			}
		}
	}

	// Check if we collected any data
	if !hadData {
		item.SSHSuccess = false
		item.SSHError = "SSH bağlandı ama veri alınamadı"
		return false // Connected but no data - DO NOT add to results
	}

	// SUCCESS: SSH login worked AND data collected
	item.SSHData = sshData
	item.SSHSuccess = true
	if !stringSliceContains(item.MethodsUsed, "ssh") {
		item.MethodsUsed = append(item.MethodsUsed, "ssh")
	}
	return true
}

// runSSHCommand - SSH üzerinden komut çalıştır
func runSSHCommand(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		return string(output), err
	}

	return string(output), nil
}

func enrichSSHHardwareData(client *ssh.Client, sshData map[string]interface{}) bool {
	if client == nil || sshData == nil {
		return false
	}

	hadData := false

	// Son yeniden başlatma zamanı
	if output, err := runSSHCommand(client, "uptime -s 2>/dev/null || who -b 2>/dev/null | awk '{print $3\" \"$4}'"); err == nil {
		lastBoot := strings.TrimSpace(output)
		if lastBoot != "" && !isSSHErrorOutput(lastBoot) {
			sshData["last_boot_time"] = lastBoot
			hadData = true
		}
	}

	// Toplam RAM (bytes)
	if output, err := runSSHCommand(client, "free -b 2>/dev/null | awk '/Mem:/ {print $2}' || sysctl -n hw.memsize 2>/dev/null"); err == nil {
		totalRAMBytes := parseNumericValue(strings.TrimSpace(output))
		if totalRAMBytes > 0 {
			sshData["total_ram_bytes"] = totalRAMBytes
			sshData["total_ram_gb"] = fmt.Sprintf("%.2f GB", totalRAMBytes/1024/1024/1024)
			hadData = true
		}
	}

	// Fiziksel diskler (lsblk JSON)
	if output, err := runSSHCommand(client, "lsblk -b -J -o NAME,MODEL,ROTA,SIZE,TYPE 2>/dev/null"); err == nil {
		disks, totalGB := parseSSHPhysicalDisks(output)
		if len(disks) > 0 {
			sshData["physical_disks"] = disks
			sshData["disk_total_gb"] = totalGB
			hadData = true
		}
	}

	// CPU çekirdek sayısı
	if output, err := runSSHCommand(client, "nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null"); err == nil {
		coreCount := parseNumericValue(strings.TrimSpace(output))
		if coreCount > 0 {
			sshData["cpu_core_count"] = int(coreCount)
			hadData = true
		}
	}

	return hadData
}

func parseSSHPhysicalDisks(output string) ([]map[string]interface{}, float64) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, 0
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, 0
	}

	var disks []map[string]interface{}
	blockDevices, ok := raw["blockdevices"].([]interface{})
	if !ok {
		return nil, 0
	}

	for _, node := range blockDevices {
		collectSSHDiskNodes(node, &disks)
	}

	totalGB := 0.0
	for _, disk := range disks {
		totalGB += parseNumericValue(disk["Size"]) / 1024 / 1024 / 1024
	}

	return disks, totalGB
}

func collectSSHDiskNodes(node interface{}, disks *[]map[string]interface{}) {
	obj, ok := node.(map[string]interface{})
	if !ok {
		return
	}

	nodeType, _ := obj["type"].(string)
	if strings.EqualFold(strings.TrimSpace(nodeType), "disk") {
		model := strings.TrimSpace(fmt.Sprintf("%v", obj["model"]))
		if model == "<nil>" {
			model = ""
		}
		rota := strings.TrimSpace(fmt.Sprintf("%v", obj["rota"]))
		diskType := ""
		if rota == "0" {
			diskType = "SSD"
		} else if rota == "1" {
			diskType = "HDD"
		}

		*disks = append(*disks, map[string]interface{}{
			"Model": model,
			"Type":  diskType,
			"Size":  obj["size"],
		})
	}

	children, ok := obj["children"].([]interface{})
	if !ok {
		return
	}
	for _, child := range children {
		collectSSHDiskNodes(child, disks)
	}
}

// parseOSRelease - /etc/os-release veya sw_vers çıktısını parse et
func parseOSRelease(output string) map[string]string {
	result := make(map[string]string)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// KEY=VALUE veya KEY: VALUE formatı
		var key, value string
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key = strings.TrimSpace(parts[0])
				value = strings.Trim(strings.TrimSpace(parts[1]), "\"")
			}
		} else if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				key = strings.TrimSpace(parts[0])
				value = strings.TrimSpace(parts[1])
			}
		}

		if key != "" && value != "" {
			result[key] = value
		}
	}

	return result
}

// parseNetworkInterfaces - ip addr veya ifconfig çıktısını parse et
func parseNetworkInterfaces(output string) []interface{} {
	var interfaces []interface{}

	// Basit parsing - her interface için bir map
	lines := strings.Split(output, "\n")
	var currentInterface map[string]interface{}

	macRegex := regexp.MustCompile(`(?i)(?:ether|hwaddr|lladdr)\s+([0-9a-f]{2}[:\-][0-9a-f]{2}[:\-][0-9a-f]{2}[:\-][0-9a-f]{2}[:\-][0-9a-f]{2}[:\-][0-9a-f]{2})`)
	ipv4Regex := regexp.MustCompile(`(?i)inet\s+(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" {
			continue
		}

		// Yeni interface başlangıcı: indent yok (orijinal satıra bakıyoruz, trimlenmiş değil!)
		isIndented := strings.HasPrefix(rawLine, " ") || strings.HasPrefix(rawLine, "\t")
		if !isIndented && (strings.Contains(trimmed, ": <") || strings.Contains(trimmed, "flags=")) {
			if currentInterface != nil {
				interfaces = append(interfaces, currentInterface)
			}

			// Interface adını çıkar (örn: "2: eth0:" → "eth0", veya "eth0: flags=..." → "eth0")
			fields := strings.Fields(trimmed)
			ifName := fields[0]
			ifName = strings.TrimSuffix(ifName, ":")
			// "2:" gibi numara ise, sonraki field'ı al
			if len(fields) > 1 && strings.HasSuffix(fields[0], ":") {
				num := strings.TrimSuffix(fields[0], ":")
				if _, err := fmt.Sscanf(num, "%d", new(int)); err == nil {
					ifName = strings.TrimSuffix(fields[1], ":")
				}
			}

			currentInterface = map[string]interface{}{
				"name": ifName,
			}
			continue
		}

		if currentInterface == nil {
			continue
		}

		// MAC address
		if matches := macRegex.FindStringSubmatch(trimmed); len(matches) > 1 {
			mac := strings.ToUpper(matches[1])
			mac = strings.ReplaceAll(mac, "-", ":")
			currentInterface["mac"] = mac
		}

		// IPv4 address
		if matches := ipv4Regex.FindStringSubmatch(trimmed); len(matches) > 1 {
			currentInterface["ipv4"] = matches[1]
		}
	}

	// Son interface'i ekle
	if currentInterface != nil {
		interfaces = append(interfaces, currentInterface)
	}

	return interfaces
}

// autoDetectAssetTypes - OS bilgisine göre asset type otomatik belirle
func autoDetectAssetTypes(items []DiscoveredItem) {
	for i := range items {
		if items[i].AssetType != "" {
			continue // Zaten belirlenmişse değiştirme
		}

		if items[i].ADData == nil || items[i].ADData.OSName == "" {
			items[i].AssetType = "other"
			continue
		}

		osName := strings.ToLower(items[i].ADData.OSName)

		// Windows Server → Server
		if strings.Contains(osName, "server") {
			items[i].AssetType = "server"
		} else if strings.Contains(osName, "windows 10") || strings.Contains(osName, "windows 11") {
			// Windows 10/11 → PC (varsayılan)
			items[i].AssetType = "pc"
		} else if strings.Contains(osName, "windows 7") || strings.Contains(osName, "windows 8") {
			items[i].AssetType = "pc"
		} else if strings.Contains(osName, "linux") || strings.Contains(osName, "ubuntu") || strings.Contains(osName, "centos") {
			items[i].AssetType = "server" // Linux genelde server
		} else if strings.Contains(osName, "mac") || strings.Contains(osName, "darwin") {
			items[i].AssetType = "laptop" // Mac genelde laptop
		} else {
			items[i].AssetType = "other"
		}
	}
}

// checkExistingItems - Envanterde zaten var mı kontrol et
func checkExistingItems(db *sql.DB, items []DiscoveredItem) {
	for i := range items {
		// AD GUID'ye göre kontrol et
		if items[i].ADData != nil && items[i].ADData.ObjectGUID != "" {
			var existingID int
			err := db.QueryRow("SELECT id FROM inventory WHERE ad_object_guid = ?", items[i].ADData.ObjectGUID).Scan(&existingID)
			if err == nil {
				items[i].AlreadyExists = true
				items[i].ExistingID = &existingID
				continue
			}
		}

		// Hostname'e göre kontrol et
		if items[i].Hostname != "" {
			var existingID int
			err := db.QueryRow("SELECT id FROM inventory WHERE hostname = ?", items[i].Hostname).Scan(&existingID)
			if err == nil {
				items[i].AlreadyExists = true
				items[i].ExistingID = &existingID
				continue
			}
		}

		// IP'ye göre kontrol et
		if items[i].IPAddress != "" {
			var existingID int
			err := db.QueryRow("SELECT id FROM inventory WHERE ip_address = ?", items[i].IPAddress).Scan(&existingID)
			if err == nil {
				items[i].AlreadyExists = true
				items[i].ExistingID = &existingID
			}
		}
	}
}

// parseADEntry - LDAP entry'den AD bilgilerini çıkar
type managedByInfo struct {
	displayName string
	mail        string
	department  string
	title       string
	samAccount  string
	upn         string
}

func enrichManagedByDetails(conn *ldap.Conn, adData *ADDiscoveryData, cache map[string]managedByInfo) {
	dn := strings.TrimSpace(adData.ManagedBy)
	if dn == "" || !strings.Contains(strings.ToUpper(dn), "CN=") {
		return
	}

	if cached, ok := cache[dn]; ok {
		applyManagedByInfo(adData, cached)
		return
	}

	searchRequest := ldap.NewSearchRequest(
		dn,
		ldap.ScopeBaseObject,
		ldap.NeverDerefAliases,
		1, 5, false,
		"(objectClass=*)",
		[]string{"displayName", "mail", "department", "title", "sAMAccountName", "userPrincipalName", "cn"},
		nil,
	)

	result, err := conn.Search(searchRequest)
	if err != nil || len(result.Entries) == 0 {
		cache[dn] = managedByInfo{}
		return
	}

	entry := result.Entries[0]
	info := managedByInfo{
		displayName: strings.TrimSpace(firstNonEmpty(
			entry.GetAttributeValue("displayName"),
			entry.GetAttributeValue("cn"),
		)),
		mail:       strings.TrimSpace(entry.GetAttributeValue("mail")),
		department: strings.TrimSpace(entry.GetAttributeValue("department")),
		title:      strings.TrimSpace(entry.GetAttributeValue("title")),
		samAccount: strings.TrimSpace(entry.GetAttributeValue("sAMAccountName")),
		upn:        strings.TrimSpace(entry.GetAttributeValue("userPrincipalName")),
	}

	cache[dn] = info
	applyManagedByInfo(adData, info)
}

func applyManagedByInfo(adData *ADDiscoveryData, info managedByInfo) {
	adData.ManagedByDisplayName = info.displayName
	adData.ManagedByMail = info.mail
	adData.ManagedByDepartment = info.department
	adData.ManagedByTitle = info.title
	adData.ManagedBySamAccount = info.samAccount
	adData.ManagedByUPN = info.upn
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func parseADEntry(entry *ldap.Entry) *ADDiscoveryData {
	data := &ADDiscoveryData{}

	// Basic fields
	// objectGUID binary gelir; metin gibi okunursa bozuk karakter olur.
	guidBytes := entry.GetRawAttributeValue("objectGUID")
	if len(guidBytes) == 16 {
		data.ObjectGUID = fmt.Sprintf("%x-%x-%x-%x-%x",
			guidBytes[0:4], guidBytes[4:6], guidBytes[6:8], guidBytes[8:10], guidBytes[10:16])
	}
	data.DistinguishedName = entry.GetAttributeValue("distinguishedName")
	data.ComputerName = strings.TrimSuffix(entry.GetAttributeValue("sAMAccountName"), "$")
	data.CNName = entry.GetAttributeValue("cn")
	data.DNSHostName = entry.GetAttributeValue("dNSHostName")
	data.Description = entry.GetAttributeValue("description")
	data.Comment = entry.GetAttributeValue("comment")
	data.Location = entry.GetAttributeValue("location")
	data.ManagedBy = entry.GetAttributeValue("managedBy")
	data.OSName = entry.GetAttributeValue("operatingSystem")
	data.OSVersion = entry.GetAttributeValue("operatingSystemVersion")
	data.OSServicePack = entry.GetAttributeValue("operatingSystemServicePack")

	// Hardware info
	data.Model = entry.GetAttributeValue("model")
	if data.Model == "" {
		data.Model = entry.GetAttributeValue("physicalDeliveryOfficeName") // Alternative attribute
	}
	data.SerialNumber = entry.GetAttributeValue("serialNumber")

	// Extract vendor from model (e.g., "Dell OptiPlex 7090" -> "Dell")
	if data.Model != "" {
		if parts := strings.Fields(data.Model); len(parts) > 0 {
			data.Vendor = parts[0]
		}
	}

	// OU Path
	dn := data.DistinguishedName
	if idx := strings.Index(dn, ",OU="); idx != -1 {
		data.OUPath = dn[idx+1:]
	} else if idx := strings.Index(dn, ",CN="); idx != -1 {
		data.OUPath = dn[idx+1:]
	}

	// Service Principal Names
	data.ServicePrincipalNames = entry.GetAttributeValues("servicePrincipalName")

	// Timestamps
	if ts := entry.GetAttributeValue("whenCreated"); ts != "" {
		if t, err := time.Parse("20060102150405.0Z", ts); err == nil {
			data.WhenCreated = &t
		}
	}
	if ts := entry.GetAttributeValue("whenChanged"); ts != "" {
		if t, err := time.Parse("20060102150405.0Z", ts); err == nil {
			data.WhenChanged = &t
		}
	}

	// User Account Control (enabled/disabled)
	if uacStr := entry.GetAttributeValue("userAccountControl"); uacStr != "" {
		var uac int
		fmt.Sscanf(uacStr, "%d", &uac)
		data.Enabled = (uac & 0x0002) == 0 // ACCOUNTDISABLE flag
	}

	return data
}

// checkPortOpen - TCP port kontrolü (paralel, hızlı)
func checkPortOpen(targets []string, port int, timeout time.Duration) []string {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var openHosts []string

	maxConcurrent := 50 // 50 paralel port check
	semaphore := make(chan struct{}, maxConcurrent)

	for _, target := range targets {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// TCP port check
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), timeout)
			if err == nil {
				conn.Close()
				mu.Lock()
				openHosts = append(openHosts, ip)
				mu.Unlock()
			}
		}(target)
	}

	wg.Wait()
	return openHosts
}

// performSSHDiscoveryWithTargets - SSH discovery with port scanning optimization
// FIXED: Only adds devices where SSH login succeeds and data is collected
func performSSHDiscoveryWithTargets(items *[]DiscoveredItem, settings map[string]interface{}, targets []string) {
	if len(targets) == 0 {
		return
	}

	// 1. Port 22 taraması yap (1 saniye timeout, paralel)
	openSSHHosts := checkPortOpen(targets, 22, 1*time.Second)

	if len(openSSHHosts) == 0 {
		return // Hiçbir IP'de port 22 açık değil
	}

	// 2. Check which IPs already exist in items
	existing := make(map[string]bool)
	for _, item := range *items {
		if item.IPAddress != "" {
			existing[item.IPAddress] = true
		}
	}

	// 3. For each open port, try SSH connection and collect data (PARALLEL)
	// ONLY add items where SSH login succeeds AND data is collected
	var wg sync.WaitGroup
	var mu sync.Mutex
	successfulItems := []DiscoveredItem{}
	maxConcurrent := 10 // 10 parallel SSH connections (SSH is slower than SNMP)
	semaphore := make(chan struct{}, maxConcurrent)

	for _, ip := range openSSHHosts {
		if existing[ip] {
			continue // Already in items list (from LDAP/other source)
		}

		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Create temporary item for SSH attempt
			item := DiscoveredItem{
				TempID:      uuid.New().String(),
				Name:        targetIP,
				Hostname:    targetIP,
				IPAddress:   targetIP,
				MethodsUsed: []string{},
				AssetType:   "server", // Linux/Unix servers
			}

			// Try SSH connection and data collection
			if trySSHEnrichmentOnItem(&item, settings) {
				// SUCCESS: SSH login worked AND data collected
				mu.Lock()
				successfulItems = append(successfulItems, item)
				mu.Unlock()
			}
		}(ip)
	}

	wg.Wait()

	// 4. Add only successful SSH items to main list
	*items = append(*items, successfulItems...)
}

// performSNMPDiscoveryWithTargets - SNMP discovery with port scanning optimization
// FIXED: Only adds devices where SNMP connection succeeds and data is collected
func performSNMPDiscoveryWithTargets(items *[]DiscoveredItem, settings map[string]interface{}, targets []string) {
	if len(targets) == 0 {
		fmt.Println("[SNMP Discovery] No targets provided")
		return
	}

	fmt.Printf("[SNMP Discovery] Starting with %d targets: %v\n", len(targets), targets)

	// NOTE: Port 161 is UDP, TCP port scanning doesn't work for SNMP!
	// Instead, we directly try SNMP connection on all targets (fast with 3s timeout)
	fmt.Println("[SNMP Discovery] Skipping port scan (SNMP is UDP), trying direct SNMP connections")

	// 1. Check which IPs already exist in items
	existing := make(map[string]bool)
	for _, item := range *items {
		if item.IPAddress != "" {
			existing[item.IPAddress] = true
		}
	}

	// 2. For each target, try SNMP connection and collect data (PARALLEL)
	// ONLY add items where SNMP connection succeeds AND data is collected
	var wg sync.WaitGroup
	var mu sync.Mutex
	successfulItems := []DiscoveredItem{}
	maxConcurrent := 20 // 20 parallel SNMP connections
	semaphore := make(chan struct{}, maxConcurrent)

	for _, ip := range targets {
		if existing[ip] {
			continue // Already in items list (from LDAP/other source)
		}

		wg.Add(1)
		go func(targetIP string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Create temporary item for SNMP attempt
			item := DiscoveredItem{
				TempID:      uuid.New().String(),
				Name:        targetIP,
				IPAddress:   targetIP,
				MethodsUsed: []string{},
				AssetType:   "other", // Network devices
			}

			// Try SNMP connection and data collection
			fmt.Printf("[SNMP Discovery] Trying SNMP on %s...\n", targetIP)
			if trySNMPEnrichmentOnItem(&item, settings) {
				// SUCCESS: SNMP connection worked AND data collected
				fmt.Printf("[SNMP Discovery] ✅ SUCCESS on %s\n", targetIP)
				mu.Lock()
				successfulItems = append(successfulItems, item)
				mu.Unlock()
			} else {
				fmt.Printf("[SNMP Discovery] ❌ FAILED on %s\n", targetIP)
			}
		}(ip)
	}

	wg.Wait()

	fmt.Printf("[SNMP Discovery] Added %d successful items to results\n", len(successfulItems))

	// 4. Add only successful SNMP items to main list
	*items = append(*items, successfulItems...)
}

// determineAssetType - OS'e göre varlık tipini belirle
func determineAssetType(osName string) string {
	osLower := strings.ToLower(osName)

	if strings.Contains(osLower, "server") {
		return "server"
	}
	if strings.Contains(osLower, "windows") {
		return "pc"
	}

	return "other"
}

// stringSliceContains - String slice'da değer var mı kontrol et
func stringSliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func addMethod(item *DiscoveredItem, method string) {
	if method == "" {
		return
	}
	if !stringSliceContains(item.MethodsUsed, method) {
		item.MethodsUsed = append(item.MethodsUsed, method)
	}
}

func normalizeWinRMSettings(settings map[string]interface{}) map[string]interface{} {
	if settings == nil {
		return nil
	}
	if _, ok := settings["timeout"]; !ok {
		if v, ok := settings["timeout_seconds"]; ok {
			settings["timeout"] = v
		}
	}
	return settings
}

func buildWinRMSettings(req DiscoveryRequest) map[string]interface{} {
	// Prefer explicit WinRM settings, otherwise fall back to AD bind credentials.
	settings := normalizeWinRMSettings(req.WinRMSettings)
	if settings == nil {
		settings = map[string]interface{}{}
	}
	username, _ := settings["username"].(string)
	password, _ := settings["password"].(string)
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		adUser, _ := req.ADSettings["bind_username"].(string)
		adPass, _ := req.ADSettings["bind_password"].(string)
		if strings.TrimSpace(adUser) != "" && strings.TrimSpace(adPass) != "" {
			settings["username_raw"] = adUser
			settings["username"] = normalizeWinRMUsername(adUser, req.ADSettings)
			settings["password"] = adPass
		}
	}
	if _, ok := settings["port"]; !ok {
		settings["port"] = 5985
	}
	if _, ok := settings["use_ssl"]; !ok {
		settings["use_ssl"] = false
	}
	if _, ok := settings["timeout"]; !ok {
		settings["timeout"] = 5
	}
	return settings
}

func normalizeWinRMUsername(username string, adSettings map[string]interface{}) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return username
	}

	// Zaten DOMAIN\user formatındaysa dokunma
	if strings.Contains(username, "\\") {
		return username
	}

	// user@domain.com formatını DOMAIN\user'a çevir (NTLM uyumluluğu)
	if strings.Contains(username, "@") {
		parts := strings.SplitN(username, "@", 2)
		user := parts[0]
		domain := parts[1]
		netbios := domainToNetBIOS(domain)
		if netbios == "" {
			netbios = strings.ToUpper(strings.Split(domain, ".")[0])
		}
		if user != "" && netbios != "" {
			return netbios + `\` + user
		}
	}

	// Düz kullanıcı adı varsa (ne @ ne \), AD ayarlarından domain al
	if adSettings != nil {
		if server, ok := adSettings["server"].(string); ok && server != "" {
			// server genelde "dc.domain.local" formatında
			parts := strings.SplitN(server, ".", 2)
			if len(parts) > 1 {
				netbios := domainToNetBIOS(parts[1])
				if netbios != "" {
					return netbios + `\` + username
				}
			}
		}
		if baseDN, ok := adSettings["base_dn"].(string); ok && baseDN != "" {
			// base_dn: "DC=systrack,DC=local" → SYSTRACK
			dnParts := strings.Split(strings.ToUpper(baseDN), ",")
			for _, part := range dnParts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "DC=") {
					netbios := strings.TrimPrefix(part, "DC=")
					if netbios != "" && netbios != "LOCAL" && netbios != "COM" && netbios != "NET" && netbios != "ORG" {
						return netbios + `\` + username
					}
				}
			}
		}
	}

	return username
}

func domainToNetBIOS(domain string) string {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return ""
	}
	upper := strings.ToUpper(domain)
	if strings.Contains(upper, ".") {
		return strings.ToUpper(strings.Split(upper, ".")[0])
	}
	return upper
}

func tryWinRMClient(target string, ipFallback string, settings map[string]interface{}) (*winrm.Client, error) {
	if settings == nil {
		return nil, fmt.Errorf("WinRM ayarları eksik")
	}
	username, _ := settings["username"].(string)
	password, _ := settings["password"].(string)
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return nil, fmt.Errorf("WinRM kullanıcı adı veya parolası belirtilmedi")
	}

	timeout := time.Duration(toInt(settings["timeout"], 5)) * time.Second

	// Deneme stratejisi: port/host/auth kombinasyonları
	// Öncelik sırası: HTTPS > HTTP, NTLM > Basic
	type attempt struct {
		host     string
		port     int
		useSSL   bool
		authType string // "ntlm" or "basic"
		label    string
	}

	var attempts []attempt

	// Caller'ın belirttiği port/SSL ayarını al (standalone hosts bunu açıkça verir).
	// Bu ayarlara uyan kombinasyonları listenin BAŞINA koy — TCP dial başarısız olursa
	// zaten 1s'de atlanır ama doğru kombinasyon başta olduğunda başarılı durumda
	// gereksiz HTTPS deneme maliyeti (1s × 2 per host) önlenir.
	preferredPort, _ := settings["port"].(int)
	useSSLHint, _ := settings["use_ssl"].(bool)
	preferHTTP := preferredPort == 5985 || (!useSSLHint && preferredPort != 5986)

	seenHosts := map[string]bool{}
	for _, host := range []string{target, ipFallback} {
		host = strings.TrimSpace(host)
		hostKey := strings.ToLower(host)
		if host == "" || seenHosts[hostKey] {
			continue
		}
		seenHosts[hostKey] = true

		if preferHTTP {
			// HTTP önce (standalone hosts, plain WinRM kurulumu)
			attempts = append(attempts,
				attempt{host, 5985, false, "ntlm", fmt.Sprintf("%s:5985/HTTP/NTLM", host)},
				attempt{host, 5985, false, "basic", fmt.Sprintf("%s:5985/HTTP/Basic", host)},
				attempt{host, 5986, true, "ntlm", fmt.Sprintf("%s:5986/HTTPS/NTLM", host)},
				attempt{host, 5986, true, "basic", fmt.Sprintf("%s:5986/HTTPS/Basic", host)},
			)
		} else {
			// HTTPS önce (AD/domain ortamları, sertifikalı kurulumlar)
			attempts = append(attempts,
				attempt{host, 5986, true, "ntlm", fmt.Sprintf("%s:5986/HTTPS/NTLM", host)},
				attempt{host, 5986, true, "basic", fmt.Sprintf("%s:5986/HTTPS/Basic", host)},
				attempt{host, 5985, false, "ntlm", fmt.Sprintf("%s:5985/HTTP/NTLM", host)},
				attempt{host, 5985, false, "basic", fmt.Sprintf("%s:5985/HTTP/Basic", host)},
			)
		}
	}

	var errors []string

	for _, a := range attempts {
		// TCP port check (hızlı - 1 saniye)
		tcpAddr := net.JoinHostPort(a.host, strconv.Itoa(a.port))
		conn, err := net.DialTimeout("tcp", tcpAddr, 1000*time.Millisecond)
		if err != nil {
			// Port kapalı, bir sonraki denemeye geç
			continue
		}
		conn.Close()

		// Port açık! WinRM client oluştur
		endpoint := winrm.NewEndpoint(a.host, a.port, a.useSSL, true, nil, nil, nil, timeout)

		var client *winrm.Client
		if a.authType == "ntlm" {
			// NTLM authentication (kurumsal ortamlar için)
			params := winrm.DefaultParameters
			params.TransportDecorator = func() winrm.Transporter {
				return &winrm.ClientNTLM{}
			}
			client, err = winrm.NewClientWithParameters(endpoint, username, password, params)
		} else {
			// Basic authentication (eski sistemler/test ortamları için)
			client, err = winrm.NewClient(endpoint, username, password)
		}

		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: client error: %v", a.label, err))
			continue
		}

		// Test komutu - bağlantıyı doğrula
		stdout, stderr, _, cmdErr := client.RunWithString("hostname", "")
		if cmdErr != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", a.label, cmdErr))
			continue
		}

		// Başarılı bağlantı!
		hostname := strings.TrimSpace(stdout)
		if hostname != "" {
			// Log: hangi yöntemle bağlandık (debug için)
			fmt.Printf("[WinRM] ✓ Connected to %s via %s (hostname: %s)\n", a.host, a.label, hostname)
			return client, nil
		}

		// Hostname boş ama hata yok - stderr kontrol et
		if stderr != "" && !strings.Contains(stderr, "CLIXML") {
			errors = append(errors, fmt.Sprintf("%s: stderr: %s", a.label, stderr))
		}
	}

	// Tüm denemeler başarısız
	if len(errors) == 0 {
		return nil, fmt.Errorf("WinRM: Hiçbir port açık değil (5985/5986)")
	}
	return nil, fmt.Errorf("WinRM bağlantı başarısız (toplam %d deneme): %s", len(errors), strings.Join(errors, " | "))
}

// toInt - interface{}'den int'e güvenli dönüşüm (float64 veya int)
func toInt(v interface{}, defaultVal int) int {
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	}
	return defaultVal
}

func extractSNMPTargets(req DiscoveryRequest) ([]string, error) {
	if len(req.SNMPTargets) > 0 {
		fmt.Printf("[extractSNMPTargets] Using SNMPTargets array: %v\n", req.SNMPTargets)
		return normalizeTargets(req.SNMPTargets), nil
	}
	if req.SNMPSettings == nil {
		fmt.Println("[extractSNMPTargets] SNMPSettings is nil")
		return nil, nil
	}
	if raw, ok := req.SNMPSettings["scan_targets"].(string); ok && strings.TrimSpace(raw) != "" {
		fmt.Printf("[extractSNMPTargets] Parsing scan_targets: %s\n", raw)
		targets, err := parseTargetString(raw, 2048)
		fmt.Printf("[extractSNMPTargets] Parsed %d targets\n", len(targets))
		return targets, err
	}
	fmt.Println("[extractSNMPTargets] No scan_targets found in SNMPSettings")
	return nil, nil
}

func extractSSHTargets(req DiscoveryRequest) ([]string, error) {
	if len(req.SSHTargets) > 0 {
		return normalizeTargets(req.SSHTargets), nil
	}
	if req.SSHSettings == nil {
		return nil, nil
	}
	if raw, ok := req.SSHSettings["scan_targets"].(string); ok && strings.TrimSpace(raw) != "" {
		return parseTargetString(raw, 1024)
	}
	return nil, nil
}

func appendSNMPTargets(items *[]DiscoveredItem, targets []string) {
	if len(targets) == 0 {
		return
	}
	existing := make(map[string]bool, len(*items))
	for _, item := range *items {
		if item.IPAddress != "" {
			existing[item.IPAddress] = true
		}
	}
	for _, ip := range targets {
		if ip == "" || existing[ip] {
			continue
		}
		existing[ip] = true
		*items = append(*items, DiscoveredItem{
			TempID:      uuid.New().String(),
			Name:        ip,
			IPAddress:   ip,
			MethodsUsed: []string{"snmp"},
			AssetType:   "other",
		})
	}
}

func appendSSHTargets(items *[]DiscoveredItem, targets []string) {
	if len(targets) == 0 {
		return
	}
	existing := make(map[string]bool, len(*items))
	for _, item := range *items {
		if item.IPAddress != "" {
			existing[item.IPAddress] = true
		}
	}
	for _, ip := range targets {
		if ip == "" || existing[ip] {
			continue
		}
		existing[ip] = true
		*items = append(*items, DiscoveredItem{
			TempID:      uuid.New().String(),
			Name:        ip,
			IPAddress:   ip,
			MethodsUsed: []string{"ssh"},
			AssetType:   "server", // Assume Linux/Unix servers
		})
	}
}

func getLinuxTargetsFromDB(db *sql.DB) ([]string, error) {
	// IP tarayıcısından (targets tablosundan) aktif olan ve muhtemelen Linux olan cihazları çek
	// Kriter: type = 'linux' veya service içinde ssh/linux keywords varsa
	query := `
		SELECT DISTINCT ip
		FROM targets
		WHERE is_enabled = 1
		AND ip IS NOT NULL
		AND ip != ''
		AND (
			type = 'linux'
			OR type = 'server'
			OR os_name LIKE '%linux%'
			OR os_name LIKE '%ubuntu%'
			OR os_name LIKE '%debian%'
			OR os_name LIKE '%centos%'
			OR os_name LIKE '%unix%'
		)
		LIMIT 500
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ips []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err == nil && ip != "" {
			ips = append(ips, ip)
		}
	}

	return ips, nil
}

func getSNMPTargetsFromDB(db *sql.DB) ([]string, error) {
	// IP tarayıcısından ve server status'tan SNMP destekleyen cihazları çek
	// Kriter: type = 'switch', 'router', 'printer' veya service içinde snmp varsa
	query := `
		SELECT DISTINCT ip
		FROM targets
		WHERE is_enabled = 1
		AND ip IS NOT NULL
		AND ip != ''
		AND (
			type IN ('switch', 'router', 'printer', 'nas', 'firewall', 'access_point')
			OR service LIKE '%snmp%'
			OR os_name LIKE '%cisco%'
			OR os_name LIKE '%mikrotik%'
		)
		LIMIT 1000
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ips []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err == nil && ip != "" {
			ips = append(ips, ip)
		}
	}

	return ips, nil
}

func parseTargetString(raw string, max int) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';' || r == ' ' || r == '\t'
	})
	var targets []string
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		switch {
		case strings.Contains(token, "/"):
			ipList, err := expandCIDR(token, max-len(targets))
			if err != nil {
				return nil, err
			}
			targets = append(targets, ipList...)
		case strings.Contains(token, "-"):
			ipList, err := expandRange(token, max-len(targets))
			if err != nil {
				return nil, err
			}
			targets = append(targets, ipList...)
		default:
			ip := net.ParseIP(token)
			if ip == nil || ip.To4() == nil {
				return nil, fmt.Errorf("geçersiz IP: %s", token)
			}
			targets = append(targets, ip.String())
		}
		if max > 0 && len(targets) > max {
			return nil, fmt.Errorf("hedef sayısı çok fazla (maksimum %d)", max)
		}
	}
	return normalizeTargets(targets), nil
}

func normalizeTargets(targets []string) []string {
	seen := make(map[string]bool, len(targets))
	var out []string
	for _, t := range targets {
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

func expandCIDR(cidr string, remaining int) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("geçersiz CIDR: %s", cidr)
	}
	ip = ip.To4()
	if ip == nil {
		return nil, fmt.Errorf("yalnızca IPv4 desteklenir: %s", cidr)
	}
	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); ip = incIP(ip) {
		ips = append(ips, ip.String())
		if remaining > 0 && len(ips) > remaining {
			return nil, fmt.Errorf("CIDR hedefi çok geniş: %s", cidr)
		}
	}
	return ips, nil
}

func expandRange(token string, remaining int) ([]string, error) {
	parts := strings.SplitN(token, "-", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("geçersiz aralık: %s", token)
	}
	start := net.ParseIP(strings.TrimSpace(parts[0])).To4()
	end := net.ParseIP(strings.TrimSpace(parts[1])).To4()
	if start == nil || end == nil {
		return nil, fmt.Errorf("geçersiz IP aralığı: %s", token)
	}
	startInt := ipToUint32(start)
	endInt := ipToUint32(end)
	if startInt > endInt {
		return nil, fmt.Errorf("IP aralığı ters: %s", token)
	}
	var ips []string
	for i := startInt; i <= endInt; i++ {
		ips = append(ips, uint32ToIP(i).String())
		if remaining > 0 && len(ips) > remaining {
			return nil, fmt.Errorf("IP aralığı çok geniş: %s", token)
		}
		if i == ^uint32(0) {
			break
		}
	}
	return ips, nil
}

func ipToUint32(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip.To4())
}

func uint32ToIP(n uint32) net.IP {
	ip := make([]byte, 4)
	binary.BigEndian.PutUint32(ip, n)
	return net.IP(ip)
}

func incIP(ip net.IP) net.IP {
	ip = append(net.IP(nil), ip...)
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
	return ip
}

// ImportScannedItems - Tarama sonuçlarından seçilenleri envantere ekle veya güncelle
func ImportScannedItems(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Items []DiscoveredItem `json:"items"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Geçersiz istek: " + err.Error()})
			return
		}

		imported := 0
		updated := 0
		skipped := 0
		errors := []string{}

		fmt.Printf("[IMPORT] Received %d items\n", len(req.Items))

		for idx, item := range req.Items {
			fmt.Printf("[IMPORT] Item %d: name=%q ip=%q already_exists=%v existing_id=%v methods=%v\n",
				idx, item.Name, item.IPAddress, item.AlreadyExists, item.ExistingID, item.MethodsUsed)

			if item.AlreadyExists && item.ExistingID != nil {
				// Mevcut kaydı güncelle
				if err := updateExistingInventoryItem(db, item); err != nil {
					errors = append(errors, fmt.Sprintf("%s: %v", item.Name, err))
					fmt.Printf("[IMPORT] Item %d: UPDATE ERROR: %v\n", idx, err)
				} else {
					updated++
					fmt.Printf("[IMPORT] Item %d: UPDATED (id=%d)\n", idx, *item.ExistingID)
				}
			} else if !item.AlreadyExists {
				// Yeni kayıt ekle
				if err := insertInventoryItem(db, item); err != nil {
					errors = append(errors, fmt.Sprintf("%s: %v", item.Name, err))
					fmt.Printf("[IMPORT] Item %d: INSERT ERROR: %v\n", idx, err)
				} else {
					imported++
					fmt.Printf("[IMPORT] Item %d: INSERTED\n", idx)
				}
			} else {
				skipped++
				fmt.Printf("[IMPORT] Item %d: SKIPPED (already exists, no ID)\n", idx)
			}
		}

		fmt.Printf("[IMPORT] Result: imported=%d updated=%d skipped=%d errors=%d\n", imported, updated, skipped, len(errors))

		c.JSON(http.StatusOK, gin.H{
			"imported":       imported,
			"updated":        updated,
			"skipped":        skipped,
			"errors":         errors,
			"total_received": len(req.Items),
		})
	}
}

// insertInventoryItem - Keşfedilen varlığı envantere ekle
func insertInventoryItem(db *sql.DB, item DiscoveredItem) error {
	// ADData nil ise VEYA LDAP ile keşfedilmediyse → Non-AD insert kullan
	// (SSH enrichment ADData oluşturabiliyor ama bu LDAP cihazı değil)
	if item.ADData == nil || !stringSliceContains(item.MethodsUsed, "ldap") {
		return insertNonADInventoryItem(db, item)
	}

	ad := item.ADData

	// Service Principal Names JSON olarak serialize et
	spnJSON, _ := json.Marshal(ad.ServicePrincipalNames)

	// SSH data JSON olarak serialize et
	sshJSON := ""
	if item.SSHData != nil {
		if b, err := json.Marshal(item.SSHData); err == nil {
			sshJSON = string(b)
		}
	}

	// SNMP data JSON olarak serialize et
	snmpJSON := ""
	if item.SNMPData != nil {
		if b, err := json.Marshal(item.SNMPData); err == nil {
			snmpJSON = string(b)
		}
	}

	// Discovery methods JSON olarak serialize et
	methodsJSON := ""
	if len(item.MethodsUsed) > 0 {
		if b, err := json.Marshal(item.MethodsUsed); err == nil {
			methodsJSON = string(b)
		}
	}

	winrmJSON := marshalJSON(item.WinRMData)
	ownerJSON := marshalJSON(buildOwnerPayload(ad))
	lastWinRMEnrichAt := nullableNow(winrmJSON != "")
	assignedTo := nullableString(firstNonEmpty(ad.ManagedByDisplayName, ad.ManagedBySamAccount))
	department := nullableString(ad.ManagedByDepartment)

	softwareJSON := extractSoftwareJSON(item)
	softwareScanAt := nullableNow(softwareJSON != "")
	crackFindingsJSON := marshalCrackFindings(item.CrackFindings)
	crackRiskLevel := ""
	if crackFindingsJSON != "" {
		crackRiskLevel = calcRiskLevel(item.CrackFindings)
	}
	lastBootTime := nullableString(item.LastBootTime)
	var powerEstimate interface{}
	if item.PowerEstimateWatts > 0 {
		powerEstimate = item.PowerEstimateWatts
	}

	columns := []string{
		"asset_name", "hostname", "ip_address", "mac_address", "asset_type",
		"vendor", "brand", "model", "serial_number", "location", "department", "assigned_to",
		"ad_object_guid", "ad_distinguished_name", "ad_computer_name", "ad_cn_name",
		"ad_description", "ad_comment", "ad_location", "ad_managed_by",
		"ad_ou_path",
		"ad_os_name", "ad_os_version", "ad_os_service_pack",
		"ad_last_logon", "ad_when_created", "ad_when_changed", "ad_pwd_last_set",
		"ad_service_principal_names", "owner_json",
		"snmp_json", "ssh_json", "winrm_json", "discovery_methods",
		"discovered_at", "last_ad_sync_at", "last_winrm_enrich_at", "last_snmp_enrich_at", "last_ssh_enrich_at",
		"source", "discovery_source",
		"software_json", "software_scan_at", "last_boot_time", "power_estimate_watts",
		"crack_scan_at", "crack_risk_level", "crack_findings_json",
	}
	args := []interface{}{
		item.Name, item.Hostname, item.IPAddress, item.MACAddress, item.AssetType,
		ad.Vendor, ad.Vendor, ad.Model, ad.SerialNumber, ad.Location, department, assignedTo,
		ad.ObjectGUID, ad.DistinguishedName, ad.ComputerName, ad.CNName,
		ad.Description, ad.Comment, ad.Location, ad.ManagedBy,
		ad.OUPath,
		ad.OSName, ad.OSVersion, ad.OSServicePack,
		ad.LastLogon, ad.WhenCreated, ad.WhenChanged, ad.PwdLastSet,
		string(spnJSON), nullableString(ownerJSON),
		nullableString(snmpJSON), nullableString(sshJSON), nullableString(winrmJSON), nullableString(methodsJSON),
		time.Now(), time.Now(), lastWinRMEnrichAt, time.Now(), time.Now(),
		"ad_ldap", "ad_ldap",
		nullableString(softwareJSON), softwareScanAt, lastBootTime, powerEstimate,
		nullableNow(crackFindingsJSON != ""), nullableString(crackRiskLevel), nullableString(crackFindingsJSON),
	}
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	query := fmt.Sprintf("INSERT INTO inventory (%s) VALUES (%s)", strings.Join(columns, ", "), strings.Join(placeholders, ", "))
	_, err := db.Exec(query, args...)

	return err
}

func insertNonADInventoryItem(db *sql.DB, item DiscoveredItem) error {
	assetName := strings.TrimSpace(item.Name)
	if assetName == "" {
		if item.Hostname != "" {
			assetName = item.Hostname
		} else if item.IPAddress != "" {
			assetName = item.IPAddress
		} else {
			return fmt.Errorf("varlık adı oluşturulamadı")
		}
	}

	assetType := item.AssetType
	if assetType == "" {
		assetType = "other"
	}

	var vendor, model, serial, location, department, assignedTo string

	// SNMP'den bilgi al
	if item.SNMPData != nil {
		if v, ok := item.SNMPData["vendor"].(string); ok && v != "" {
			vendor = v
		}
		if m, ok := item.SNMPData["model"].(string); ok && m != "" {
			model = m
		}
		if m, ok := item.SNMPData["sysDescr"].(string); ok && model == "" {
			model = m
		}
		if loc, ok := item.SNMPData["sysLocation"].(string); ok && loc != "" {
			location = loc
		}
		if name, ok := item.SNMPData["sysName"].(string); ok && item.Hostname == "" {
			item.Hostname = name
		}
	}

	// SSH'den bilgi al (SNMP'den alınamayanları tamamla)
	if item.SSHData != nil {
		if vendor == "" {
			if v, ok := item.SSHData["manufacturer"].(string); ok {
				vendor = cleanSSHValue(v)
			}
		}
		if model == "" {
			if m, ok := item.SSHData["model"].(string); ok {
				model = cleanSSHValue(m)
			}
		}
		if serial == "" {
			if s, ok := item.SSHData["serial_number"].(string); ok {
				serial = cleanSSHValue(s)
			}
		}
	}

	// ADData'dan da bilgi al (SSH enrichment ADData oluşturmuş olabilir)
	if item.ADData != nil {
		if vendor == "" {
			vendor = cleanSSHValue(item.ADData.Vendor)
		}
		if model == "" {
			model = cleanSSHValue(item.ADData.Model)
		}
		if serial == "" {
			serial = cleanSSHValue(item.ADData.SerialNumber)
		}
		if department == "" {
			department = strings.TrimSpace(item.ADData.ManagedByDepartment)
		}
		if assignedTo == "" {
			assignedTo = strings.TrimSpace(firstNonEmpty(item.ADData.ManagedByDisplayName, item.ADData.ManagedBySamAccount))
		}
	}

	snmpJSON := ""
	if item.SNMPData != nil {
		if b, err := json.Marshal(item.SNMPData); err == nil {
			snmpJSON = string(b)
		}
	}

	// SSH data JSON olarak serialize et
	sshJSON := ""
	if item.SSHData != nil {
		if b, err := json.Marshal(item.SSHData); err == nil {
			sshJSON = string(b)
		}
	}

	// Discovery methods JSON olarak serialize et
	methodsJSON := ""
	if len(item.MethodsUsed) > 0 {
		if b, err := json.Marshal(item.MethodsUsed); err == nil {
			methodsJSON = string(b)
		}
	}

	winrmJSON := marshalJSON(item.WinRMData)
	ownerJSON := ""
	if item.ADData != nil {
		ownerJSON = marshalJSON(buildOwnerPayload(item.ADData))
	}
	lastWinRMEnrichAt := nullableNow(winrmJSON != "")

	// Yazılım listesini çıkar (WinRM veya SSH'dan)
	softwareJSON := extractSoftwareJSON(item)
	softwareScanAt := nullableNow(softwareJSON != "")
	crackFindingsJSON := marshalCrackFindings(item.CrackFindings)
	crackRiskLevel := ""
	if crackFindingsJSON != "" {
		crackRiskLevel = calcRiskLevel(item.CrackFindings)
	}

	// Son açılış zamanı
	lastBootTime := nullableString(item.LastBootTime)

	// Tahmini güç tüketimi
	var powerEstimate interface{}
	if item.PowerEstimateWatts > 0 {
		powerEstimate = item.PowerEstimateWatts
	}

	// Discovery source'u belirle (primary method)
	discoverySource := "manual"
	if len(item.MethodsUsed) > 0 {
		discoverySource = item.MethodsUsed[0]
	}

	columns := []string{
		"asset_name", "hostname", "ip_address", "mac_address", "asset_type",
		"vendor", "brand", "model", "serial_number", "location", "department", "assigned_to",
		"source", "discovery_source", "discovery_methods", "discovered_at",
		"snmp_json", "ssh_json", "winrm_json", "owner_json",
		"last_winrm_enrich_at", "last_snmp_enrich_at", "last_ssh_enrich_at",
		"software_json", "software_scan_at", "last_boot_time", "power_estimate_watts",
		"crack_scan_at", "crack_risk_level", "crack_findings_json",
	}
	args := []interface{}{
		assetName, item.Hostname, item.IPAddress, item.MACAddress, assetType,
		nullableString(vendor), nullableString(vendor), nullableString(model), nullableString(serial), nullableString(location), nullableString(department), nullableString(assignedTo),
		discoverySource, discoverySource, nullableString(methodsJSON), time.Now(),
		nullableString(snmpJSON), nullableString(sshJSON), nullableString(winrmJSON), nullableString(ownerJSON),
		lastWinRMEnrichAt, time.Now(), time.Now(),
		nullableString(softwareJSON), softwareScanAt, lastBootTime, powerEstimate,
		nullableNow(crackFindingsJSON != ""), nullableString(crackRiskLevel), nullableString(crackFindingsJSON),
	}
	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	query := fmt.Sprintf("INSERT INTO inventory (%s) VALUES (%s)", strings.Join(columns, ", "), strings.Join(placeholders, ", "))
	_, err := db.Exec(query, args...)
	return err
}

func nullableString(value string) interface{} {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullableNow(enabled bool) interface{} {
	if enabled {
		return time.Now()
	}
	return nil
}

func marshalJSON(value interface{}) string {
	if value == nil {
		return ""
	}
	b, err := json.Marshal(value)
	if err != nil || string(b) == "null" {
		return ""
	}
	return string(b)
}

func marshalCrackFindings(findings []CrackFinding) string {
	if len(findings) == 0 {
		return ""
	}
	b, err := json.Marshal(findings)
	if err != nil {
		return ""
	}
	return string(b)
}

func buildOwnerPayload(ad *ADDiscoveryData) map[string]string {
	if ad == nil {
		return nil
	}

	payload := map[string]string{
		"display_name":  strings.TrimSpace(ad.ManagedByDisplayName),
		"mail":          strings.TrimSpace(ad.ManagedByMail),
		"department":    strings.TrimSpace(ad.ManagedByDepartment),
		"title":         strings.TrimSpace(ad.ManagedByTitle),
		"sam_account":   strings.TrimSpace(ad.ManagedBySamAccount),
		"upn":           strings.TrimSpace(ad.ManagedByUPN),
		"managed_by_dn": strings.TrimSpace(ad.ManagedBy),
	}

	hasValue := false
	for _, v := range payload {
		if v != "" {
			hasValue = true
			break
		}
	}
	if !hasValue {
		return nil
	}
	return payload
}

// updateExistingInventoryItem - Mevcut envanter kaydını yeni keşif verileriyle güncelle
func updateExistingInventoryItem(db *sql.DB, item DiscoveredItem) error {
	if item.ExistingID == nil {
		return fmt.Errorf("existing ID is nil")
	}

	isLDAP := item.ADData != nil && stringSliceContains(item.MethodsUsed, "ldap")

	// Temel alanları hazırla
	setClauses := []string{}
	args := []interface{}{}

	// Hostname güncelle (boş değilse)
	if item.Hostname != "" {
		setClauses = append(setClauses, "hostname = ?")
		args = append(args, item.Hostname)
	}

	// IP güncelle (boş değilse)
	if item.IPAddress != "" {
		setClauses = append(setClauses, "ip_address = ?")
		args = append(args, item.IPAddress)
	}

	// MAC güncelle (boş değilse)
	if item.MACAddress != "" {
		setClauses = append(setClauses, "mac_address = ?")
		args = append(args, item.MACAddress)
	}

	// Asset type güncelle (boş değilse)
	if item.AssetType != "" && item.AssetType != "other" {
		setClauses = append(setClauses, "asset_type = ?")
		args = append(args, item.AssetType)
	}

	// Vendor, model, serial - kaynaklardan topla
	var vendor, model, serial, location string

	if item.SNMPData != nil {
		if v, ok := item.SNMPData["vendor"].(string); ok && v != "" {
			vendor = v
		}
		if m, ok := item.SNMPData["model"].(string); ok && m != "" {
			model = m
		}
		if m, ok := item.SNMPData["sysDescr"].(string); ok && model == "" {
			model = m
		}
		if loc, ok := item.SNMPData["sysLocation"].(string); ok && loc != "" {
			location = loc
		}
	}

	if item.SSHData != nil {
		if vendor == "" {
			if v, ok := item.SSHData["manufacturer"].(string); ok {
				vendor = cleanSSHValue(v)
			}
		}
		if model == "" {
			if m, ok := item.SSHData["model"].(string); ok {
				model = cleanSSHValue(m)
			}
		}
		if serial == "" {
			if s, ok := item.SSHData["serial_number"].(string); ok {
				serial = cleanSSHValue(s)
			}
		}
	}

	if item.ADData != nil {
		if vendor == "" {
			vendor = cleanSSHValue(item.ADData.Vendor)
		}
		if model == "" {
			model = cleanSSHValue(item.ADData.Model)
		}
		if serial == "" {
			serial = cleanSSHValue(item.ADData.SerialNumber)
		}
		if location == "" {
			location = item.ADData.Location
		}
	}

	if vendor != "" {
		setClauses = append(setClauses, "vendor = ?", "brand = ?")
		args = append(args, vendor, vendor)
	}
	if model != "" {
		setClauses = append(setClauses, "model = ?")
		args = append(args, model)
	}
	if serial != "" {
		setClauses = append(setClauses, "serial_number = ?")
		args = append(args, serial)
	}
	if location != "" {
		setClauses = append(setClauses, "location = ?")
		args = append(args, location)
	}

	// SNMP JSON
	if item.SNMPData != nil {
		if b, err := json.Marshal(item.SNMPData); err == nil {
			setClauses = append(setClauses, "snmp_json = ?", "last_snmp_enrich_at = NOW()")
			args = append(args, string(b))
		}
	}

	// SSH JSON
	if item.SSHData != nil {
		if b, err := json.Marshal(item.SSHData); err == nil {
			setClauses = append(setClauses, "ssh_json = ?", "last_ssh_enrich_at = NOW()")
			args = append(args, string(b))
		}
	}

	// WinRM JSON
	if item.WinRMData != nil {
		if b, err := json.Marshal(item.WinRMData); err == nil {
			setClauses = append(setClauses, "winrm_json = ?", "last_winrm_enrich_at = NOW()")
			args = append(args, string(b))
		}
	}

	// Discovery methods
	if len(item.MethodsUsed) > 0 {
		if b, err := json.Marshal(item.MethodsUsed); err == nil {
			setClauses = append(setClauses, "discovery_methods = ?")
			args = append(args, string(b))
		}
	}

	// LDAP/AD alanları
	if isLDAP {
		ad := item.ADData
		setClauses = append(setClauses,
			"ad_object_guid = ?", "ad_distinguished_name = ?", "ad_computer_name = ?", "ad_cn_name = ?",
			"ad_description = ?", "ad_comment = ?", "ad_location = ?", "ad_managed_by = ?", "ad_ou_path = ?",
			"ad_os_name = ?", "ad_os_version = ?", "ad_os_service_pack = ?",
			"ad_last_logon = ?", "ad_when_created = ?", "ad_when_changed = ?", "ad_pwd_last_set = ?",
			"last_ad_sync_at = NOW()",
		)
		spnJSON, _ := json.Marshal(ad.ServicePrincipalNames)
		args = append(args,
			ad.ObjectGUID, ad.DistinguishedName, ad.ComputerName, ad.CNName,
			ad.Description, ad.Comment, ad.Location, ad.ManagedBy, ad.OUPath,
			ad.OSName, ad.OSVersion, ad.OSServicePack,
			ad.LastLogon, ad.WhenCreated, ad.WhenChanged, ad.PwdLastSet,
		)
		setClauses = append(setClauses, "ad_service_principal_names = ?")
		args = append(args, string(spnJSON))

		ownerJSON := marshalJSON(buildOwnerPayload(ad))
		if ownerJSON != "" {
			setClauses = append(setClauses, "owner_json = ?")
			args = append(args, ownerJSON)
		}
		if ad.ManagedByDepartment != "" {
			setClauses = append(setClauses, "department = ?")
			args = append(args, ad.ManagedByDepartment)
		}
		if ad.ManagedByDisplayName != "" || ad.ManagedBySamAccount != "" {
			setClauses = append(setClauses, "assigned_to = ?")
			args = append(args, firstNonEmpty(ad.ManagedByDisplayName, ad.ManagedBySamAccount))
		}
	}

	// Yazılım listesi — değişimler varsa history'ye kaydet
	if sw := extractSoftwareJSON(item); sw != "" {
		var prevSoftwareJSON sql.NullString
		db.QueryRow(`SELECT software_json FROM inventory WHERE id = ?`, *item.ExistingID).Scan(&prevSoftwareJSON)
		go RecordSoftwareChanges(db, *item.ExistingID, sw, prevSoftwareJSON.String) //nolint
		setClauses = append(setClauses, "software_json = ?", "software_scan_at = NOW()")
		args = append(args, sw)
	}

	// Son açılış zamanı
	if item.LastBootTime != "" {
		setClauses = append(setClauses, "last_boot_time = ?")
		args = append(args, item.LastBootTime)
	}

	// Güç tahmini
	if item.PowerEstimateWatts > 0 {
		setClauses = append(setClauses, "power_estimate_watts = ?")
		args = append(args, item.PowerEstimateWatts)
	}

	if crackFindingsJSON := marshalCrackFindings(item.CrackFindings); crackFindingsJSON != "" {
		setClauses = append(setClauses, "crack_scan_at = NOW()", "crack_risk_level = ?", "crack_findings_json = ?")
		args = append(args, calcRiskLevel(item.CrackFindings), crackFindingsJSON)
	}

	if len(setClauses) == 0 {
		return fmt.Errorf("güncellenecek alan bulunamadı")
	}

	// WHERE id = ?
	args = append(args, *item.ExistingID)

	query := fmt.Sprintf("UPDATE inventory SET %s WHERE id = ?", strings.Join(setClauses, ", "))

	_, err := db.Exec(query, args...)
	return err
}

// extractSoftwareJSON - WinRM veya SSH verisinden yazılım listesini JSON olarak döndür
func extractSoftwareJSON(item DiscoveredItem) string {
	// WinRM'den software_list (Windows)
	if item.WinRMData != nil {
		if sw, ok := item.WinRMData["software_list"]; ok {
			if b, err := json.Marshal(sw); err == nil {
				return string(b)
			}
		}
	}
	// SSH'dan software_list (Linux)
	if item.SSHData != nil {
		if sw, ok := item.SSHData["software_list"]; ok {
			if b, err := json.Marshal(sw); err == nil {
				return string(b)
			}
		}
	}
	return ""
}

// isSSHErrorOutput - SSH komut çıktısının hata mesajı olup olmadığını kontrol et
func isSSHErrorOutput(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "command not found") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "no such file") ||
		strings.Contains(lower, "bash:") ||
		strings.Contains(lower, "sudo:") ||
		strings.Contains(lower, "not found") ||
		strings.Contains(lower, "error:")
}

// cleanSSHValue - SSH'den gelen değeri temizle, hata mesajı ise boş döndür
func cleanSSHValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || isSSHErrorOutput(value) {
		return ""
	}
	return value
}

// parseWMIDatetime - WMI tarih formatını okunabilir stringe çevirir
// WMI format: "20240115143022.000000-000" → "2024-01-15 14:30:22"
func parseWMIDatetime(wmiDate string) string {
	wmiDate = strings.TrimSpace(wmiDate)
	if len(wmiDate) < 14 {
		return ""
	}
	// Sadece rakam içeren kısımları al
	year := wmiDate[0:4]
	month := wmiDate[4:6]
	day := wmiDate[6:8]
	hour := wmiDate[8:10]
	min := wmiDate[10:12]
	sec := wmiDate[12:14]
	return fmt.Sprintf("%s-%s-%s %s:%s:%s", year, month, day, hour, min, sec)
}

// parseSSHSoftwareList - dpkg/rpm çıktısını yazılım listesine çevirir
// Format (tab-separated): name\tversion\tpublisher
func parseSSHSoftwareList(output string) []interface{} {
	var software []interface{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}
		entry := map[string]interface{}{
			"name":      name,
			"version":   "",
			"publisher": "",
		}
		if len(parts) > 1 {
			entry["version"] = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			entry["publisher"] = strings.TrimSpace(parts[2])
		}
		software = append(software, entry)
	}
	return software
}

// estimateCPUTDP - CPU adından TDP watt tahmini (Intel/AMD veri tabanına göre)
func estimateCPUTDP(cpuName, assetType string) int {
	lower := strings.ToLower(cpuName)

	if lower == "" {
		switch assetType {
		case "laptop":
			return 15
		case "server":
			return 150
		case "switch", "router", "access_point":
			return 0
		default:
			return 65
		}
	}

	// Ultra-low power mobile (U/Y serisi)
	if strings.HasSuffix(lower, " u") || strings.Contains(lower, " u ") ||
		strings.HasSuffix(lower, "y") || strings.Contains(lower, "ultra low") {
		return 15
	}

	// Sunucu işlemcileri
	if strings.Contains(lower, "xeon") {
		return 150
	}
	if strings.Contains(lower, "epyc") {
		return 200
	}
	if strings.Contains(lower, "threadripper") {
		return 180
	}

	// Intel Core
	if strings.Contains(lower, "core") {
		if strings.Contains(lower, "i9") {
			return 125
		}
		if strings.Contains(lower, "i7") {
			return 95
		}
		if strings.Contains(lower, "i5") {
			return 65
		}
		if strings.Contains(lower, "i3") {
			return 55
		}
		if strings.Contains(lower, "2 duo") || strings.Contains(lower, "2 quad") {
			return 65
		}
	}
	// Intel diğer
	if strings.Contains(lower, "pentium") {
		return 54
	}
	if strings.Contains(lower, "celeron") {
		return 35
	}
	if strings.Contains(lower, "atom") {
		return 10
	}

	// AMD Ryzen
	if strings.Contains(lower, "ryzen") {
		if strings.Contains(lower, "9 ") {
			return 105
		}
		if strings.Contains(lower, "7 ") {
			return 95
		}
		if strings.Contains(lower, "5 ") {
			return 65
		}
		if strings.Contains(lower, "3 ") {
			return 55
		}
	}
	if strings.Contains(lower, "athlon") {
		return 45
	}

	// ARM (Raspberry Pi vb.)
	if strings.Contains(lower, "arm") || strings.Contains(lower, "cortex") {
		return 5
	}

	return 65 // varsayılan masaüstü CPU
}

// estimatePowerWatts - Cihaz verilerinden tahmini güç tüketimi hesaplar (Watt)
// CPU yük oranı biliniyorsa TDP'yi ölçekler: aktualCPU = TDP × (0.30 + 0.70 × yük%)
// Yük bilinmiyorsa tipik ofis yükü %30 varsayılır (tam TDP yerine).
// Sanal makinelerde (VMware, Hyper-V, VirtualBox) fiziksel PSU/fan/anakart yükü olmadığından
// base watt uygulanmaz ve CPU watt %50 düşürülür.
func estimatePowerWatts(item *DiscoveredItem) int {
	var cpuName string
	var ramGB float64
	var diskCount int
	var hasSSD bool
	cpuLoadPercent := -1.0 // -1 = bilinmiyor
	isVirtualMachine := false

	// WinRM verilerinden bilgi al
	if item.WinRMData != nil {
		if cpu, ok := item.WinRMData["cpu"].(map[string]interface{}); ok {
			cpuName, _ = cpu["Name"].(string)
			// LoadPercentage: WMI float64 ya da string olarak gelebilir
			switch v := cpu["LoadPercentage"].(type) {
			case float64:
				cpuLoadPercent = v
			case string:
				var f float64
				if _, err := fmt.Sscanf(v, "%f", &f); err == nil {
					cpuLoadPercent = f
				}
			}
		}
		if cs, ok := item.WinRMData["computer_system"].(map[string]interface{}); ok {
			if totalMem, ok := cs["TotalPhysicalMemory"].(float64); ok {
				ramGB = totalMem / 1024 / 1024 / 1024
			}
			// Sanal makine tespiti: Model veya Manufacturer alanından
			model, _ := cs["Model"].(string)
			manufacturer, _ := cs["Manufacturer"].(string)
			modelL := strings.ToLower(model)
			mfrL := strings.ToLower(manufacturer)
			if strings.Contains(modelL, "vmware") || strings.Contains(mfrL, "vmware") ||
				strings.Contains(modelL, "virtualbox") || strings.Contains(mfrL, "virtualbox") ||
				strings.Contains(modelL, "virtual machine") || strings.Contains(modelL, "kvm") ||
				(strings.Contains(mfrL, "microsoft") && strings.Contains(modelL, "virtual")) {
				isVirtualMachine = true
			}
		}
		if disks := interfaceToSlice(item.WinRMData["physical_disks"]); len(disks) > 0 {
			diskCount = len(disks)
			for _, d := range disks {
				if dm, ok := d.(map[string]interface{}); ok {
					mt, _ := dm["MediaType"].(string)
					if strings.Contains(strings.ToLower(mt), "ssd") || strings.Contains(strings.ToLower(mt), "solid") {
						hasSSD = true
					}
				}
			}
		}
	}

	// SSH verilerinden bilgi al (WinRM yoksa)
	if item.SSHData != nil {
		if cpuName == "" {
			cpuName, _ = item.SSHData["cpu_model"].(string)
		}
		if ramGB == 0 {
			if rb, ok := item.SSHData["total_ram_bytes"].(float64); ok {
				ramGB = rb / 1024 / 1024 / 1024
			}
		}
		if diskCount == 0 {
			if disks := interfaceToSlice(item.SSHData["physical_disks"]); len(disks) > 0 {
				diskCount = len(disks)
				for _, d := range disks {
					if dm, ok := d.(map[string]interface{}); ok {
						rotational, _ := dm["rotational"].(bool)
						if !rotational {
							hasSSD = true
						}
					}
				}
			}
		}
	}

	// SNMP CPU yük verisi (WinRM/SSH yoksa)
	if cpuLoadPercent < 0 && item.SNMPData != nil {
		if s, ok := item.SNMPData["cpu_load_avg_percent"].(string); ok {
			var f float64
			if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
				cpuLoadPercent = f
			}
		}
	}

	// CPU TDP tahmini
	tdp := estimateCPUTDP(cpuName, item.AssetType)

	// CPU yük oranına göre ölçekle:
	//   Boşta (%0 yük): TDP'nin ~%30'u (idle power)
	//   Tam yük (%100):  TDP'nin %100'ü
	//   Bilinmiyor:      tipik ofis yükü %30 varsayımıyla hesapla
	var loadFactor float64
	if cpuLoadPercent >= 0 {
		loadFactor = 0.30 + 0.70*(cpuLoadPercent/100.0)
	} else {
		loadFactor = 0.45 // bilinmiyor → %30 yük tahmini ile dengeli değer
	}
	cpuWatts := int(float64(tdp) * loadFactor)
	if cpuWatts < 3 {
		cpuWatts = 3
	}

	// RAM güç tüketimi: ~3W per 8GB
	ramWatts := 2
	if ramGB > 0 {
		ramWatts = int(ramGB/8*3) + 2
	}

	// Disk güç tüketimi
	diskWattsEach := 3 // SSD varsayılan
	if !hasSSD {
		diskWattsEach = 7 // HDD
	}
	if diskCount == 0 {
		diskCount = 1
	}
	diskWatts := diskCount * diskWattsEach

	// Cihaz tipine göre temel sistem yükü (anakart, fan, güç kaynağı verimsizliği)
	baseWatts := 50
	switch item.AssetType {
	case "laptop":
		baseWatts = 15
	case "server":
		baseWatts = 80
	case "switch":
		baseWatts = 20
	case "router":
		baseWatts = 15
	case "access_point":
		baseWatts = 10
	case "printer":
		baseWatts = 20
	case "phone", "tablet":
		baseWatts = 5
	}

	// SNMP-only ağ cihazları için sadece temel güç
	if item.SNMPData != nil && item.WinRMData == nil && item.SSHData == nil {
		return baseWatts
	}

	// Sanal makine düzeltmesi:
	// VM'de fiziksel PSU/fan/anakart yükü yoktur → base sıfırlanır.
	// CPU watt: host CPU TDP yerine VM'nin hissesini temsil etsin → %50 indirim.
	// Disk: VMDK dosyası olduğundan fiziksel disk spin yok → sıfırlanır.
	if isVirtualMachine {
		baseWatts = 0
		diskWatts = 0
		cpuWatts = cpuWatts / 2
	}

	total := cpuWatts + ramWatts + diskWatts + baseWatts
	if total < 3 {
		total = 3
	}
	if total > 600 {
		total = 600
	}
	return total
}

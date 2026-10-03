package services

import (
	"fmt"
	"log"
	"time"

	"github.com/gosnmp/gosnmp"
)

// SNMPTestResult represents the result of a single SNMP test
type SNMPTestResult struct {
	TargetID   int       `json:"target_id"`
	Name       string    `json:"name"`
	IP         string    `json:"ip"`
	Accessible bool      `json:"accessible"`
	SysDescr   string    `json:"sys_descr"`
	Version    string    `json:"version"`
	Community  string    `json:"community"`
	ErrorMsg   string    `json:"error_msg,omitempty"`
	TestedAt   time.Time `json:"tested_at"`
}

// SNMPTester provides SNMP connectivity testing functionality
type SNMPTester struct{}

// NewSNMPTester creates a new SNMP tester instance
func NewSNMPTester() *SNMPTester {
	return &SNMPTester{}
}

// TestTarget performs a quick SNMP connectivity test on a single target
func (st *SNMPTester) TestTarget(targetID int, name, ip, community, version string) SNMPTestResult {
	result := SNMPTestResult{
		TargetID:   targetID,
		Name:       name,
		IP:         ip,
		Accessible: false,
		Community:  community,
		Version:    version,
		TestedAt:   time.Now(),
	}

	// Convert version string to gosnmp.SnmpVersion
	snmpVersion := gosnmp.Version2c
	switch version {
	case "v1":
		snmpVersion = gosnmp.Version1
	case "v2c":
		snmpVersion = gosnmp.Version2c
	case "v3":
		snmpVersion = gosnmp.Version3
	default:
		snmpVersion = gosnmp.Version2c
	}

	// Create SNMP client with fast timeout for testing
	client := &gosnmp.GoSNMP{
		Target:    ip,
		Port:      161,
		Community: community,
		Version:   snmpVersion,
		Timeout:   5 * time.Second, // Fast timeout for testing
		Retries:   2,
		MaxOids:   10,
	}

	log.Printf("[SNMP Test] Testing %s (%s) with community '%s' version %s", name, ip, community, version)

	// Attempt connection
	if err := client.Connect(); err != nil {
		result.ErrorMsg = fmt.Sprintf("Connection failed: %v", err)
		log.Printf("[SNMP Test] ❌ %s (%s) - Connection FAILED: %v", name, ip, err)
		return result
	}
	defer client.Conn.Close()

	// Try to get sysDescr (.1.3.6.1.2.1.1.1.0)
	oid := ".1.3.6.1.2.1.1.1.0"
	resp, err := client.Get([]string{oid})
	if err != nil {
		// If sysDescr fails, try sysUpTime as fallback
		oid = ".1.3.6.1.2.1.1.3.0"
		resp, err = client.Get([]string{oid})
		if err != nil {
			result.ErrorMsg = fmt.Sprintf("SNMP query failed: %v", err)
			log.Printf("[SNMP Test] ❌ %s (%s) - Query FAILED: %v", name, ip, err)
			return result
		}
	}

	// Successfully got SNMP response
	result.Accessible = true

	// Extract sysDescr value if available
	if len(resp.Variables) > 0 {
		switch resp.Variables[0].Type {
		case gosnmp.OctetString:
			result.SysDescr = string(resp.Variables[0].Value.([]byte))
		default:
			result.SysDescr = fmt.Sprintf("%v", resp.Variables[0].Value)
		}
	}

	log.Printf("[SNMP Test] ✅ %s (%s) - ACCESSIBLE: %s", name, ip, result.SysDescr)
	return result
}

// TestTargetWithFallback tries multiple SNMP versions if the specified version fails
func (st *SNMPTester) TestTargetWithFallback(targetID int, name, ip, community string) SNMPTestResult {
	// Try v2c first (most common)
	result := st.TestTarget(targetID, name, ip, community, "v2c")
	if result.Accessible {
		return result
	}

	// If v2c fails, try v1
	log.Printf("[SNMP Test] Trying fallback to v1 for %s (%s)", name, ip)
	result = st.TestTarget(targetID, name, ip, community, "v1")
	if result.Accessible {
		return result
	}

	// Both failed - return the v2c result with error
	result.Version = "v2c"
	result.ErrorMsg = fmt.Sprintf("SNMP not accessible with v2c or v1 using community '%s'", community)
	log.Printf("[SNMP Test] ❌ %s (%s) - All versions FAILED", name, ip)
	return result
}

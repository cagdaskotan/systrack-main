package handlers

import (
	"database/sql"
	"log"
	"net/http"
	"sync"

	"systrack/internal/services"

	"github.com/gin-gonic/gin"
)

// SNMPTestRequest represents the request body for SNMP testing
type SNMPTestRequest struct {
	TargetIDs []int             `json:"target_ids"`
	Devices   []DeviceToTest    `json:"devices"`
	Community string            `json:"community"`
	Version   string            `json:"version"`
}

// DeviceToTest represents a device to test (without target ID)
type DeviceToTest struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}

// SNMPTestResponse represents the response for SNMP testing
type SNMPTestResponse struct {
	TestedCount     int                       `json:"tested_count"`
	AccessibleCount int                       `json:"accessible_count"`
	Results         []services.SNMPTestResult `json:"results"`
}

// TestSNMP performs SNMP connectivity tests on multiple targets
func TestSNMP(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req SNMPTestRequest

		// Parse request body
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		// Validate that either target_ids or devices is provided
		if len(req.TargetIDs) == 0 && len(req.Devices) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Either target_ids or devices must be provided"})
			return
		}

		// Limit to 50 targets to prevent resource exhaustion
		totalTests := len(req.TargetIDs) + len(req.Devices)
		if totalTests > 50 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Cannot test more than 50 devices at once"})
			return
		}

		// Set defaults
		if req.Community == "" {
			req.Community = "public"
		}
		if req.Version == "" {
			req.Version = "v2c"
		}

		log.Printf("[SNMP Test API] Testing %d devices with community '%s' version %s", totalTests, req.Community, req.Version)

		// Create SNMP tester
		tester := services.NewSNMPTester()

		// Test all devices in parallel with goroutines
		var wg sync.WaitGroup
		results := make([]services.SNMPTestResult, totalTests)

		// Test existing targets from database
		if len(req.TargetIDs) > 0 {
			targets, err := fetchTargetsByIDs(db, req.TargetIDs)
			if err != nil {
				log.Printf("[SNMP Test API] Error fetching targets: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch targets"})
				return
			}

			for i, target := range targets {
				wg.Add(1)
				go func(idx int, tgt targetInfo) {
					defer wg.Done()
					results[idx] = tester.TestTarget(tgt.ID, tgt.Name, tgt.Address, req.Community, req.Version)
				}(i, target)
			}
		}

		// Test devices without target IDs (directly from IP scanner)
		if len(req.Devices) > 0 {
			offset := len(req.TargetIDs)
			for i, device := range req.Devices {
				wg.Add(1)
				go func(idx int, dev DeviceToTest) {
					defer wg.Done()
					// Use negative IDs for devices without target IDs
					results[offset+idx] = tester.TestTarget(-(idx + 1), dev.Hostname, dev.IP, req.Community, req.Version)
				}(i, device)
			}
		}

		// Wait for all tests to complete
		wg.Wait()

		// Count accessible targets
		accessibleCount := 0
		for _, result := range results {
			if result.Accessible {
				accessibleCount++
			}
		}

		log.Printf("[SNMP Test API] Completed: %d tested, %d accessible", len(results), accessibleCount)

		// Return results
		response := SNMPTestResponse{
			TestedCount:     len(results),
			AccessibleCount: accessibleCount,
			Results:         results,
		}

		c.JSON(http.StatusOK, response)
	}
}

// targetInfo holds basic target information for SNMP testing
type targetInfo struct {
	ID      int
	Name    string
	Address string
}

// fetchTargetsByIDs retrieves target information by IDs
func fetchTargetsByIDs(db *sql.DB, ids []int) ([]targetInfo, error) {
	if len(ids) == 0 {
		return []targetInfo{}, nil
	}

	// Build placeholders for SQL IN clause
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := `
		SELECT id, name, address
		FROM targets
		WHERE id IN (` + joinStrings(placeholders, ",") + `)
	`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var targets []targetInfo
	for rows.Next() {
		var t targetInfo
		if err := rows.Scan(&t.ID, &t.Name, &t.Address); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}

	return targets, rows.Err()
}

// joinStrings joins a slice of strings with a separator
func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}

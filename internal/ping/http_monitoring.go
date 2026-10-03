package ping

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// HTTPMonitoringConfig represents configuration for HTTP monitoring
type HTTPMonitoringConfig struct {
	Method            string
	Path              string
	Headers           map[string]string
	ExpectedStatus    int
	ExpectedContent   string
	SSLCheck          bool
	FollowRedirects   bool
	TimeoutSec        int
	UserAgent         string
	MaxRedirects      int
	ConnectTimeoutSec int
	ReadTimeoutSec    int
}

// HTTPMonitoringEngine handles HTTP/HTTPS monitoring
type HTTPMonitoringEngine struct {
	config HTTPMonitoringConfig
}

// NewHTTPMonitoringEngine creates a new HTTP monitoring engine
func NewHTTPMonitoringEngine(config HTTPMonitoringConfig) *HTTPMonitoringEngine {
	return &HTTPMonitoringEngine{
		config: config,
	}
}

// MonitorHTTP performs HTTP/HTTPS monitoring
func (h *HTTPMonitoringEngine) MonitorHTTP(target Target) (PingResult, error) {
	startTime := time.Now()

	// Build URL
	protocol := "http"
	if target.MonitoringType == "https" {
		protocol = "https"
	}

	port := ""
	if target.Port != nil && *target.Port != 80 && *target.Port != 443 {
		port = ":" + strconv.Itoa(*target.Port)
	}

	url := fmt.Sprintf("%s://%s%s%s", protocol, target.Address, port, target.HTTPPath)

	// Create HTTP client
	client := &http.Client{
		Timeout: time.Duration(h.config.TimeoutSec) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: !h.config.SSLCheck,
			},
		},
	}

	// Don't follow redirects if disabled
	if !h.config.FollowRedirects {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	// Create request
	req, err := http.NewRequest(h.config.Method, url, nil)
	if err != nil {
		return PingResult{
			Host:     target.Address,
			Success:  false,
			Duration: 0,
			Error:    fmt.Sprintf("Failed to create request: %v", err),
		}, nil
	}

	// Set headers
	req.Header.Set("User-Agent", h.config.UserAgent)
	for key, value := range h.config.Headers {
		req.Header.Set(key, value)
	}

	// Perform request
	resp, err := client.Do(req)
	duration := time.Since(startTime)

	if err != nil {
		return PingResult{
			Host:     target.Address,
			Success:  false,
			Duration: duration,
			Error:    fmt.Sprintf("Request failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return PingResult{
			Host:     target.Address,
			Success:  false,
			Duration: duration,
			Error:    fmt.Sprintf("Failed to read response: %v", err),
		}, nil
	}

	// Check status code
	success := resp.StatusCode == h.config.ExpectedStatus

	// Check expected content if specified
	if success && h.config.ExpectedContent != "" {
		if !strings.Contains(string(body), h.config.ExpectedContent) {
			success = false
		}
	}

	// Get SSL expiry date if HTTPS
	var sslExpiryDate *time.Time
	if target.MonitoringType == "https" && resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		cert := resp.TLS.PeerCertificates[0]
		expiryDate := cert.NotAfter
		sslExpiryDate = &expiryDate
	}

	// Convert headers to map
	responseHeaders := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			responseHeaders[key] = values[0]
		}
	}

	// Prepare result
	result := PingResult{
		Host:              target.Address,
		Success:           success,
		Duration:          duration,
		Error:             "",
		StatusCode:        resp.StatusCode,
		ResponseSizeBytes: len(body),
		SSLExpiryDate:     sslExpiryDate,
		ResponseHeaders:   responseHeaders,
	}

	// Add HTTP-specific data to result
	if !success {
		if resp.StatusCode != h.config.ExpectedStatus {
			result.Error = fmt.Sprintf("Expected status %d, got %d", h.config.ExpectedStatus, resp.StatusCode)
		} else if h.config.ExpectedContent != "" {
			result.Error = "Expected content not found in response"
		}
	}

	return result, nil
}

// ParseHTTPHeaders parses JSON string headers to map
func ParseHTTPHeaders(headersJSON string) (map[string]string, error) {
	if headersJSON == "" {
		return make(map[string]string), nil
	}

	var headers map[string]string
	err := json.Unmarshal([]byte(headersJSON), &headers)
	if err != nil {
		return nil, fmt.Errorf("failed to parse headers JSON: %v", err)
	}

	return headers, nil
}

package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	AbuseIPDBBaseURL = "https://api.abuseipdb.com/api/v2"
	AbuseIPDBAPIKey  = "2686e840e2691462324c443b266e7afadfc86d1f00daf4b75a36252eabaa6c8f5d41a485771aff23"
)

type AbuseIPDBService struct {
	client *http.Client
	apiKey string
}

type AbuseIPDBResponse struct {
	Data struct {
		IPAddress        string    `json:"ipAddress"`
		IsPublic         bool      `json:"isPublic"`
		IPVersion        int       `json:"ipVersion"`
		IsWhitelisted    bool      `json:"isWhitelisted"`
		AbuseConfidence  int       `json:"abuseConfidenceScore"`
		CountryCode      string    `json:"countryCode"`
		CountryName      string    `json:"countryName"`
		UsageType        string    `json:"usageType"`
		ISP              string    `json:"isp"`
		Domain           string    `json:"domain"`
		Hostnames        []string  `json:"hostnames"`
		IsTor            bool      `json:"isTor"`
		TotalReports     int       `json:"totalReports"`
		NumDistinctUsers int       `json:"numDistinctUsers"`
		LastReportedAt   time.Time `json:"lastReportedAt"`
		Reports          []Report  `json:"reports"`
	} `json:"data"`
}

type Report struct {
	ReportedAt          time.Time `json:"reportedAt"`
	Comment             string    `json:"comment"`
	Categories          []int     `json:"categories"`
	ReporterID          int       `json:"reporterId"`
	ReporterCountryCode string    `json:"reporterCountryCode"`
	ReporterCountryName string    `json:"reporterCountryName"`
}

type IPBlacklistRecord struct {
	ID              int       `json:"id"`
	IPAddress       string    `json:"ip_address"`
	AbuseConfidence int       `json:"abuse_confidence"`
	CountryCode     string    `json:"country_code"`
	ISP             string    `json:"isp"`
	Domain          string    `json:"domain"`
	TotalReports    int       `json:"total_reports"`
	LastReportedAt  time.Time `json:"last_reported_at"`
	IsWhitelisted   bool      `json:"is_whitelisted"`
	IsTor           bool      `json:"is_tor"`
	UsageType       string    `json:"usage_type"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func NewAbuseIPDBService() *AbuseIPDBService {
	return &AbuseIPDBService{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		apiKey: AbuseIPDBAPIKey,
	}
}

func (s *AbuseIPDBService) CheckIP(ipAddress string) (*AbuseIPDBResponse, error) {
	url := fmt.Sprintf("%s/check?ipAddress=%s&maxAgeInDays=90&verbose", AbuseIPDBBaseURL, ipAddress)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Key", s.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var response AbuseIPDBResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &response, nil
}

func (s *AbuseIPDBService) ConvertToBlacklistRecord(response *AbuseIPDBResponse) *IPBlacklistRecord {
	return &IPBlacklistRecord{
		IPAddress:       response.Data.IPAddress,
		AbuseConfidence: response.Data.AbuseConfidence,
		CountryCode:     response.Data.CountryCode,
		ISP:             response.Data.ISP,
		Domain:          response.Data.Domain,
		TotalReports:    response.Data.TotalReports,
		LastReportedAt:  response.Data.LastReportedAt,
		IsWhitelisted:   response.Data.IsWhitelisted,
		IsTor:           response.Data.IsTor,
		UsageType:       response.Data.UsageType,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

package license

import "time"

// License system configuration constants
const (
	// Heartbeat configuration
	HeartbeatIntervalSec = 600 // 10 minutes
	HeartbeatTimeoutSec  = 30  // HTTP request timeout

	// Grace period configuration
	GracePeriodHours    = 24 // 24 hours offline operation
	MaxFailedHeartbeats = 10 // After 10 failures, enter grace period

	// Trial configuration
	TrialPeriodDays = 30 // 30 day trial period

	// Management server URL (can be overridden by environment variable)
	DefaultManagementServerURL = "http://37.148.202.211:18080"

	// Encryption configuration
	EncryptionKeyVersion = 1

	// Software version (updated on each release)
	SoftwareVersion = "1.0.0"

	// License status codes
	StatusUninitialized = 0 // Not activated yet
	StatusTrial         = 1 // Trial period
	StatusActive        = 2 // Fully licensed
	StatusExpired       = 3 // License expired
	StatusBlocked       = 4 // Blocked by management server
	StatusGracePeriod   = 5 // Grace period (offline mode)
)

// Status names for logging
var StatusNames = map[int]string{
	StatusUninitialized: "uninitialized",
	StatusTrial:         "trial",
	StatusActive:        "active",
	StatusExpired:       "expired",
	StatusBlocked:       "blocked",
	StatusGracePeriod:   "grace_period",
}

// GetStatusName returns human-readable status name
func GetStatusName(status int) string {
	if name, ok := StatusNames[status]; ok {
		return name
	}
	return "unknown"
}

// GetTrialDuration returns trial period as time.Duration
func GetTrialDuration() time.Duration {
	return time.Duration(TrialPeriodDays) * 24 * time.Hour
}

// GetGracePeriodDuration returns grace period as time.Duration
func GetGracePeriodDuration() time.Duration {
	return time.Duration(GracePeriodHours) * time.Hour
}

// GetHeartbeatInterval returns heartbeat interval as time.Duration
func GetHeartbeatInterval() time.Duration {
	return time.Duration(HeartbeatIntervalSec) * time.Second
}

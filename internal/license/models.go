package license

import (
	"database/sql"
	"time"
)

// License represents a device license record from database
type License struct {
	ID              int
	SerialNumber    string
	MACAddress      string
	Status          int
	CustomerEmail   sql.NullString
	CustomerCompany sql.NullString
	ActivatedAt     sql.NullTime
	CreatedAt       time.Time
}

// GetStatusName returns human-readable status name for this license
func (l *License) GetStatusName() string {
	return GetStatusName(l.Status)
}

// IsValid checks if license is in a valid state for operation
func (l *License) IsValid() bool {
	switch l.Status {
	case StatusActive, StatusTrial:
		return true
	default:
		return false
	}
}

// IsActivated returns true if license has been activated
func (l *License) IsActivated() bool {
	return l.ActivatedAt.Valid && l.Status != StatusUninitialized
}

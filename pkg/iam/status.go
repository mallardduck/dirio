package iam

import "fmt"

// UserStatus represents the status of a user account
type UserStatus string

// Valid user status values
const (
	// UserStatusActive indicates an active user account (MinIO: "on")
	UserStatusActive UserStatus = "on"

	// UserStatusDisabled indicates a disabled user account (MinIO: "off")
	UserStatusDisabled UserStatus = "off"

	// Future statuses can be added here:
	// UserStatusSuspended UserStatus = "suspended"
	// UserStatusPendingVerification UserStatus = "pending"
	// UserStatusLocked UserStatus = "locked"
)

// Validate checks if the status is a valid value
func (s UserStatus) Validate() error {
	switch s {
	case UserStatusActive, UserStatusDisabled:
		return nil
	default:
		return fmt.Errorf("invalid user status: %q (must be %q or %q)", s, UserStatusActive, UserStatusDisabled)
	}
}

// IsActive returns true if the user status allows authentication
func (s UserStatus) IsActive() bool {
	return s == UserStatusActive
}

// String returns the string representation of the status
func (s UserStatus) String() string {
	return string(s)
}

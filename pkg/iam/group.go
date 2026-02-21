package iam

import (
	"fmt"
	"time"
)

// Metadata format version for groups
const GroupMetadataVersion = "1.0.0"

// GroupStatus represents the status of a group
type GroupStatus string

// Valid group status values
const (
	// GroupStatusActive indicates an active group (MinIO: "enabled")
	GroupStatusActive GroupStatus = "on"

	// GroupStatusDisabled indicates a disabled group (MinIO: "disabled")
	GroupStatusDisabled GroupStatus = "off"
)

// Validate checks if the status is a valid value
func (s GroupStatus) Validate() error {
	switch s {
	case GroupStatusActive, GroupStatusDisabled:
		return nil
	default:
		return fmt.Errorf("invalid group status: %q (must be %q or %q)", s, GroupStatusActive, GroupStatusDisabled)
	}
}

// IsActive returns true if the group status is active
func (s GroupStatus) IsActive() bool {
	return s == GroupStatusActive
}

// String returns the string representation of the status
func (s GroupStatus) String() string {
	return string(s)
}

// Group represents an IAM group with members and attached policies
type Group struct {
	Version          string      `json:"version"`                    // DirIO metadata version
	Name             string      `json:"name"`                       // Group name (immutable after creation)
	Members          []string    `json:"members,omitempty"`          // Access keys of member users
	AttachedPolicies []string    `json:"attachedPolicies,omitempty"` // Names of attached IAM policies
	Status           GroupStatus `json:"status"`                     // Group status (on/off)
	CreatedAt        time.Time   `json:"createdAt"`
	UpdatedAt        time.Time   `json:"updatedAt"`
}

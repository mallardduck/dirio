package iam

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Metadata format version for service accounts
const ServiceAccountMetadataVersion = "1.0.0"

// PolicyMode controls how a service account resolves IAM policies during authorization.
type PolicyMode string

const (
	// PolicyModeInherit (default) uses the parent user's attached policies.
	// An empty PolicyMode is treated as PolicyModeInherit for backward compatibility.
	PolicyModeInherit PolicyMode = "inherit"

	// PolicyModeOverride uses the service account's own attached policies instead of the parent's.
	PolicyModeOverride PolicyMode = "override"
)

// ServiceAcctStatus represents the status of a service account
type ServiceAcctStatus string

// Valid service account status values
const (
	// ServiceAcctStatusActive indicates an active service account
	ServiceAcctStatusActive ServiceAcctStatus = "on"

	// ServiceAcctStatusDisabled indicates a disabled service account
	ServiceAcctStatusDisabled ServiceAcctStatus = "off"
)

// Validate checks if the status is a valid value
func (s ServiceAcctStatus) Validate() error {
	switch s {
	case ServiceAcctStatusActive, ServiceAcctStatusDisabled:
		return nil
	default:
		return fmt.Errorf("invalid service account status: %q (must be %q or %q)", s, ServiceAcctStatusActive, ServiceAcctStatusDisabled)
	}
}

// IsActive returns true if the service account status allows authentication
func (s ServiceAcctStatus) IsActive() bool {
	return s == ServiceAcctStatusActive
}

// String returns the string representation of the status
func (s ServiceAcctStatus) String() string {
	return string(s)
}

// ServiceAccount represents a long-lived or temporary credential scoped to an application or service
type ServiceAccount struct {
	Version            string            `json:"version"`                      // DirIO metadata version
	UUID               uuid.UUID         `json:"uuid"`                         // Stable identifier
	AccessKey          string            `json:"accessKey"`                    // Credential access key
	SecretKey          string            `json:"secretKey"`                    // Credential secret key
	Username           string            `json:"username"`                     // Display name
	ParentUserUUID     *uuid.UUID        `json:"parentUserUUID,omitempty"`     // Optional parent user UUID (stable across key rotation)
	PolicyMode         PolicyMode        `json:"policyMode,omitempty"`         // "inherit" (default) or "override"
	Status             ServiceAcctStatus `json:"status"`                       // Account status (on/off)
	EmbeddedPolicyJSON string            `json:"embeddedPolicyJSON,omitempty"` // Raw IAM policy JSON; evaluated directly in override mode
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
	ExpiresAt          *time.Time        `json:"expiresAt,omitempty"` // Optional expiration time
}

func NewServiceAccount(
	uuid uuid.UUID,
	accessKey, secretKey, username string,
	parentUserUUID *uuid.UUID,
	policyMode PolicyMode,
	status ServiceAcctStatus,
	policyJSON string,
	expiresAt *time.Time,
) *ServiceAccount {
	return &ServiceAccount{
		Version:            ServiceAccountMetadataVersion,
		UUID:               uuid,
		AccessKey:          accessKey,
		SecretKey:          secretKey,
		Username:           username,
		ParentUserUUID:     parentUserUUID,
		PolicyMode:         policyMode,
		Status:             status,
		EmbeddedPolicyJSON: policyJSON,
		ExpiresAt:          expiresAt,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}
}

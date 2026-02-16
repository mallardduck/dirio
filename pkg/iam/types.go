package iam

import (
	"time"

	"github.com/google/uuid"
)

// Metadata format versions
const (
	UserMetadataVersion   = "1.0.0"
	PolicyMetadataVersion = "1.0.0"
)

// User represents a user with credentials
type User struct {
	Version          string     `json:"version"`                    // DirIO metadata version
	UUID             uuid.UUID  `json:"uuid"`                       // Stable user identifier (immutable, survives key rotation)
	Username         string     `json:"username"`                   // Display name (mutable)
	AccessKey        string     `json:"accessKey"`                  // Rotatable credential
	SecretKey        string     `json:"secretKey"`                  // Rotatable credential
	Status           UserStatus `json:"status"`                     // User account status (on/off)
	UpdatedAt        time.Time  `json:"updatedAt"`                  // Last modification time
	AttachedPolicies []string   `json:"attachedPolicies,omitempty"` // Names of attached IAM policies (supports multiple)
}

// PolicyDocument represents an AWS IAM Policy Document (used by both IAM policies and bucket policies)
// See: https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_elements.html
type PolicyDocument struct {
	Version   string      `json:"Version"`      // Policy language version (usually "2012-10-17")
	Id        string      `json:"Id,omitempty"` // Optional policy ID
	Statement []Statement `json:"Statement"`    // List of policy statements
}

// Statement represents a single statement in a policy document
type Statement struct {
	Sid          string                 `json:"Sid,omitempty"`          // Optional statement ID
	Effect       string                 `json:"Effect"`                 // "Allow" or "Deny"
	Principal    interface{}            `json:"Principal,omitempty"`    // Who (can be string, map, or array)
	NotPrincipal interface{}            `json:"NotPrincipal,omitempty"` // Inverse principal matching
	Action       interface{}            `json:"Action,omitempty"`       // What actions (string or []string)
	NotAction    interface{}            `json:"NotAction,omitempty"`    // Inverse action matching
	Resource     interface{}            `json:"Resource,omitempty"`     // What resources (string or []string)
	NotResource  interface{}            `json:"NotResource,omitempty"`  // Inverse resource matching
	Condition    map[string]interface{} `json:"Condition,omitempty"`    // Optional conditions
}

// Policy represents an IAM policy (attached to users/roles)
type Policy struct {
	Version        string          `json:"version"`        // DirIO metadata version
	Name           string          `json:"name"`           // Policy name
	PolicyDocument *PolicyDocument `json:"policyDocument"` // The actual IAM policy
	CreateDate     time.Time       `json:"createDate"`
	UpdateDate     time.Time       `json:"updateDate"`
}

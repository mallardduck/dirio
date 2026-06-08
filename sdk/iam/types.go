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

// Statement represents a single statement in a policy document.
//
// AWS IAM policy statements use dynamic JSON types, so the Principal, Action, and Resource
// fields must be interface{} to support multiple formats:
//
//   - Principal: "*", {"AWS": "*"}, {"AWS": "arn:..."}, or {"AWS": ["arn:...", "arn:..."]}
//   - Action: "s3:GetObject" or ["s3:GetObject", "s3:PutObject"]
//   - Resource: "arn:aws:s3:::bucket/*" or ["arn:aws:s3:::bucket1", "arn:aws:s3:::bucket2"]
//
// The actual types after JSON unmarshaling will be one of:
//   - string: single value
//   - []string: array of values (sometimes)
//   - []interface{}: array of values (from JSON unmarshaling)
//   - map[string]interface{}: structured value (for Principal only)
//
// Use the validation functions in internal/policy to validate these fields after unmarshaling.
type Statement struct {
	Sid          string         `json:"Sid,omitempty"`          // Optional statement ID for debugging
	Effect       string         `json:"Effect"`                 // "Allow" or "Deny"
	Principal    any            `json:"Principal,omitempty"`    // Who - see type documentation above
	NotPrincipal any            `json:"NotPrincipal,omitempty"` // Inverse principal matching
	Action       any            `json:"Action,omitempty"`       // What actions - string or array
	NotAction    any            `json:"NotAction,omitempty"`    // Inverse action matching
	Resource     any            `json:"Resource,omitempty"`     // What resources - string or array
	NotResource  any            `json:"NotResource,omitempty"`  // Inverse resource matching
	Condition    map[string]any `json:"Condition,omitempty"`    // Optional conditions map[operator]map[key]value
}

// Policy represents an IAM policy (attached to users/roles)
type Policy struct {
	Version        string          `json:"version"`        // DirIO metadata version
	Name           string          `json:"name"`           // Policy name
	PolicyDocument *PolicyDocument `json:"policyDocument"` // The actual IAM policy
	CreateDate     time.Time       `json:"createDate"`
	UpdateDate     time.Time       `json:"updateDate"`
	IsBuiltin      bool            `json:"isBuiltin,omitempty"` // true for system-defined policies
}

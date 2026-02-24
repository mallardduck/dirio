package validation

import (
	"fmt"

	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/pkg/iam"
)

const (
	MinPolicyNameLength = 1
	MaxPolicyNameLength = 128
	RequiredVersion     = "2012-10-17"
)

// ValidatePolicyName validates a policy name
func ValidatePolicyName(name string) error {
	if name == "" {
		return svcerrors.NewValidationError("Name", "policy name is required")
	}
	if !InRange(name, MinPolicyNameLength, MaxPolicyNameLength) {
		return svcerrors.NewValidationError("Name",
			fmt.Sprintf("policy name must be between %d and %d characters", MinPolicyNameLength, MaxPolicyNameLength))
	}
	if !IsAlphanumericWithHyphens(name) {
		return svcerrors.NewValidationError("Name", "policy name must contain only alphanumeric characters and hyphens")
	}
	return nil
}

// ValidatePolicyDocument validates a policy document
func ValidatePolicyDocument(doc *iam.PolicyDocument) error {
	if doc == nil {
		return svcerrors.NewValidationError("PolicyDocument", "policy document is required")
	}

	if doc.Version != RequiredVersion {
		return svcerrors.NewValidationError("PolicyDocument.Version",
			fmt.Sprintf("policy document version must be '%s'", RequiredVersion))
	}

	if len(doc.Statement) == 0 {
		return svcerrors.NewValidationError("PolicyDocument.Statement", "policy document must have at least one statement")
	}

	for i, stmt := range doc.Statement {
		if err := validateStatement(&stmt, i); err != nil {
			return err
		}
	}

	return nil
}

func validateStatement(stmt *iam.Statement, index int) error {
	prefix := fmt.Sprintf("PolicyDocument.Statement[%d]", index)

	if stmt.Effect != "Allow" && stmt.Effect != "Deny" {
		return svcerrors.NewValidationError(prefix+".Effect", "effect must be 'Allow' or 'Deny'")
	}

	// Action can be a string or an array of strings
	if stmt.Action == nil {
		return svcerrors.NewValidationError(prefix+".Action", "action is required")
	}

	// Validate Action is not empty (handle both string and []string)
	switch v := stmt.Action.(type) {
	case string:
		if v == "" {
			return svcerrors.NewValidationError(prefix+".Action", "action cannot be empty")
		}
	case []any:
		if len(v) == 0 {
			return svcerrors.NewValidationError(prefix+".Action", "action array cannot be empty")
		}
	case []string:
		if len(v) == 0 {
			return svcerrors.NewValidationError(prefix+".Action", "action array cannot be empty")
		}
	default:
		return svcerrors.NewValidationError(prefix+".Action", "action must be a string or array of strings")
	}

	return nil
}

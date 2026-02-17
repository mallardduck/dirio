package iam

import (
	"fmt"
)

// Validate checks if a Statement has valid structure and required fields.
// This performs basic structural validation - does not validate policy logic or semantics.
func (s *Statement) Validate() error {
	// Effect is required and must be Allow or Deny
	if s.Effect == "" {
		return fmt.Errorf("statement missing required Effect field")
	}
	if s.Effect != "Allow" && s.Effect != "Deny" {
		return fmt.Errorf("statement Effect must be 'Allow' or 'Deny', got %q", s.Effect)
	}

	// Must have at least one of: Action, NotAction
	hasAction := s.Action != nil || s.NotAction != nil
	if !hasAction {
		return fmt.Errorf("statement must have either Action or NotAction")
	}

	// Must have at least one of: Resource, NotResource
	hasResource := s.Resource != nil || s.NotResource != nil
	if !hasResource {
		return fmt.Errorf("statement must have either Resource or NotResource")
	}

	// Cannot have both Action and NotAction
	if s.Action != nil && s.NotAction != nil {
		return fmt.Errorf("statement cannot have both Action and NotAction")
	}

	// Cannot have both Resource and NotResource
	if s.Resource != nil && s.NotResource != nil {
		return fmt.Errorf("statement cannot have both Resource and NotResource")
	}

	// Cannot have both Principal and NotPrincipal
	if s.Principal != nil && s.NotPrincipal != nil {
		return fmt.Errorf("statement cannot have both Principal and NotPrincipal")
	}

	// Validate field types (basic check - detailed validation in policy package)
	if s.Principal != nil {
		if err := validatePolicyValue(s.Principal, "Principal"); err != nil {
			return err
		}
	}

	if s.NotPrincipal != nil {
		if err := validatePolicyValue(s.NotPrincipal, "NotPrincipal"); err != nil {
			return err
		}
	}

	if s.Action != nil {
		if err := validatePolicyValue(s.Action, "Action"); err != nil {
			return err
		}
	}

	if s.NotAction != nil {
		if err := validatePolicyValue(s.NotAction, "NotAction"); err != nil {
			return err
		}
	}

	if s.Resource != nil {
		if err := validatePolicyValue(s.Resource, "Resource"); err != nil {
			return err
		}
	}

	if s.NotResource != nil {
		if err := validatePolicyValue(s.NotResource, "NotResource"); err != nil {
			return err
		}
	}

	// Note: Condition validation is more complex and handled by the policy engine

	return nil
}

// validatePolicyValue performs basic type checking for policy value fields
func validatePolicyValue(v any, fieldName string) error {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case string:
		if val == "" {
			return fmt.Errorf("%s cannot be empty string", fieldName)
		}
		return nil

	case []string:
		if len(val) == 0 {
			return fmt.Errorf("%s array cannot be empty", fieldName)
		}
		for i, s := range val {
			if s == "" {
				return fmt.Errorf("%s[%d] cannot be empty string", fieldName, i)
			}
		}
		return nil

	case []any:
		if len(val) == 0 {
			return fmt.Errorf("%s array cannot be empty", fieldName)
		}
		// Check that all items are strings (for Action/Resource)
		// or valid types (for Principal)
		for i, item := range val {
			if s, ok := item.(string); ok {
				if s == "" {
					return fmt.Errorf("%s[%d] cannot be empty string", fieldName, i)
				}
			} else if fieldName != "Principal" && fieldName != "NotPrincipal" {
				// Action/Resource must be strings
				return fmt.Errorf("%s[%d] must be string, got %T", fieldName, i, item)
			}
		}
		return nil

	case map[string]any:
		// Only valid for Principal/NotPrincipal
		if fieldName != "Principal" && fieldName != "NotPrincipal" {
			return fmt.Errorf("%s cannot be a map (only Principal/NotPrincipal support maps)", fieldName)
		}
		if len(val) == 0 {
			return fmt.Errorf("%s map cannot be empty", fieldName)
		}
		return nil

	default:
		return fmt.Errorf("%s has unsupported type %T", fieldName, v)
	}
}

// Validate checks if a PolicyDocument has valid structure.
func (pd *PolicyDocument) Validate() error {
	if pd.Version == "" {
		return fmt.Errorf("policy document missing Version field")
	}

	if len(pd.Statement) == 0 {
		return fmt.Errorf("policy document must have at least one statement")
	}

	// Validate each statement
	for i, stmt := range pd.Statement {
		if err := stmt.Validate(); err != nil {
			return fmt.Errorf("statement %d (Sid=%q): %w", i, stmt.Sid, err)
		}
	}

	return nil
}

// Validate checks if a Policy has valid structure.
func (p *Policy) Validate() error {
	if p.Version == "" {
		return fmt.Errorf("policy missing version field")
	}

	if p.Name == "" {
		return fmt.Errorf("policy missing name field")
	}

	if p.PolicyDocument == nil {
		return fmt.Errorf("policy missing policy document")
	}

	return p.PolicyDocument.Validate()
}

package policy

import "fmt"

// Value represents a field value from an IAM policy statement that can be
// a string or array of strings. This type is used for Principal, Action, and Resource fields.
//
// Valid underlying types from JSON unmarshaling:
//   - string: single value (e.g., "s3:GetObject" or "*")
//   - []string: array of values (e.g., ["s3:GetObject", "s3:PutObject"])
//   - []any: array from JSON (e.g., ["s3:GetObject", "s3:PutObject"])
//   - map[string]any: structured value (e.g., {"AWS": "*"} for Principal)
//
// After JSON unmarshaling, use Validate* functions to ensure the value conforms
// to the expected schema for its field type.
type Value = any

// ConditionMap represents the Condition block in a policy statement.
// Structure: map[operator]map[key]value
//
// Example:
//
//	{
//	  "StringEquals": {"aws:username": "alice"},
//	  "IpAddress": {"aws:SourceIp": ["192.168.1.0/24", "10.0.0.0/8"]}
//	}
//
// Operators can be: StringEquals, NumericLessThan, DateGreaterThan, IpAddress, Bool, etc.
// Values can be: string, []string, []any, number, bool (depending on operator)
type ConditionMap = map[string]any

// ValidatePrincipal checks if a principal value has a valid structure.
// Valid structures:
//   - string: "*" (public access)
//   - map[string]any: {"AWS": "*"} or {"AWS": "arn:..."} or {"AWS": ["arn:...", ...]}
func ValidatePrincipal(v any) error {
	if v == nil {
		return nil // nil is valid (means no principal restriction)
	}

	switch val := v.(type) {
	case string:
		// Single string principal (typically "*")
		return nil

	case map[string]any:
		// Must have at least one recognized principal type key
		// Note: map[string]any and map[string]interface{} are the same type in Go 1.18+
		validKeys := []string{"AWS", "Service", "Federated", "CanonicalUser"}
		for _, key := range validKeys {
			if _, exists := val[key]; exists {
				// Validate the value is string or array
				return validatePrincipalValue(val[key])
			}
		}
		return fmt.Errorf("principal map must contain at least one of: AWS, Service, Federated, CanonicalUser")

	default:
		return fmt.Errorf("principal must be string or map[string]any, got %T", v)
	}
}

// validatePrincipalValue checks if a principal value (the value inside {"AWS": ...}) is valid
func validatePrincipalValue(v any) error {
	if v == nil {
		return fmt.Errorf("principal value cannot be nil")
	}

	switch val := v.(type) {
	case string:
		return nil
	case []string:
		if len(val) == 0 {
			return fmt.Errorf("principal array cannot be empty")
		}
		return nil
	case []any:
		// Note: []any and []interface{} are the same type in Go 1.18+
		if len(val) == 0 {
			return fmt.Errorf("principal array cannot be empty")
		}
		// Validate all items are strings
		for i, item := range val {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("principal array item %d must be string, got %T", i, item)
			}
		}
		return nil
	default:
		return fmt.Errorf("principal value must be string or array, got %T", v)
	}
}

// ValidateAction checks if an action value has a valid structure.
// Valid structures:
//   - string: single action (e.g., "s3:GetObject")
//   - []string: array of actions (e.g., ["s3:GetObject", "s3:PutObject"])
//   - []any: array from JSON (will be validated to contain only strings)
func ValidateAction(v any) error {
	if v == nil {
		return nil // nil is valid (means no action restriction, used with NotAction)
	}

	switch val := v.(type) {
	case string:
		if val == "" {
			return fmt.Errorf("action string cannot be empty")
		}
		return nil

	case []string:
		if len(val) == 0 {
			return fmt.Errorf("action array cannot be empty")
		}
		for i, action := range val {
			if action == "" {
				return fmt.Errorf("action array item %d cannot be empty", i)
			}
		}
		return nil

	case []any:
		if len(val) == 0 {
			return fmt.Errorf("action array cannot be empty")
		}
		for i, item := range val {
			if s, ok := item.(string); !ok {
				return fmt.Errorf("action array item %d must be string, got %T", i, item)
			} else if s == "" {
				return fmt.Errorf("action array item %d cannot be empty", i)
			}
		}
		return nil

	default:
		return fmt.Errorf("action must be string or array, got %T", v)
	}
}

// ValidateResource checks if a resource value has a valid structure.
// Valid structures:
//   - string: single resource ARN or "*" (e.g., "arn:aws:s3:::bucket/*")
//   - []string: array of resource ARNs
//   - []any: array from JSON (will be validated to contain only strings)
func ValidateResource(v any) error {
	if v == nil {
		return nil // nil is valid (means no resource restriction, used with NotResource)
	}

	switch val := v.(type) {
	case string:
		if val == "" {
			return fmt.Errorf("resource string cannot be empty")
		}
		return nil

	case []string:
		if len(val) == 0 {
			return fmt.Errorf("resource array cannot be empty")
		}
		for i, resource := range val {
			if resource == "" {
				return fmt.Errorf("resource array item %d cannot be empty", i)
			}
		}
		return nil

	case []any:
		if len(val) == 0 {
			return fmt.Errorf("resource array cannot be empty")
		}
		for i, item := range val {
			if s, ok := item.(string); !ok {
				return fmt.Errorf("resource array item %d must be string, got %T", i, item)
			} else if s == "" {
				return fmt.Errorf("resource array item %d cannot be empty", i)
			}
		}
		return nil

	default:
		return fmt.Errorf("resource must be string or array, got %T", v)
	}
}

// ValidateCondition checks if a condition map has a valid structure.
// Returns an error if the structure is invalid (helps catch malformed policies early).
func ValidateCondition(conditions ConditionMap) error {
	if conditions == nil {
		return nil // nil is valid (no conditions)
	}

	if len(conditions) == 0 {
		return nil // empty map is valid
	}

	// Check that each operator has a map[string]any value
	for operator, keyValues := range conditions {
		if operator == "" {
			return fmt.Errorf("condition operator cannot be empty")
		}

		// keyValues should be map[string]any
		kvMap, ok := keyValues.(map[string]any)
		if !ok {
			// Try interface{} variant
			kvMapInterface, ok := keyValues.(map[string]any)
			if !ok {
				return fmt.Errorf("condition operator %q must have map[string]any value, got %T", operator, keyValues)
			}
			// Convert to check structure
			if len(kvMapInterface) == 0 {
				return fmt.Errorf("condition operator %q cannot have empty map", operator)
			}
			continue
		}

		if len(kvMap) == 0 {
			return fmt.Errorf("condition operator %q cannot have empty map", operator)
		}

		// Check each key-value pair
		for key, value := range kvMap {
			if key == "" {
				return fmt.Errorf("condition key cannot be empty in operator %q", operator)
			}
			if value == nil {
				return fmt.Errorf("condition value cannot be nil for key %q in operator %q", key, operator)
			}
			// Value can be string, []string, []any, number, bool - all valid
		}
	}

	return nil
}

// NormalizeAction converts an action value to a consistent []string format.
// This is useful for code that wants to iterate over actions uniformly.
func NormalizeAction(v any) ([]string, error) {
	if v == nil {
		return nil, fmt.Errorf("action cannot be nil")
	}

	switch val := v.(type) {
	case string:
		return []string{val}, nil

	case []string:
		return val, nil

	case []any:
		result := make([]string, len(val))
		for i, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("action array item %d must be string, got %T", i, item)
			}
			result[i] = s
		}
		return result, nil

	default:
		return nil, fmt.Errorf("action must be string or array, got %T", v)
	}
}

// NormalizeResource converts a resource value to a consistent []string format.
// This is useful for code that wants to iterate over resources uniformly.
func NormalizeResource(v any) ([]string, error) {
	if v == nil {
		return nil, fmt.Errorf("resource cannot be nil")
	}

	switch val := v.(type) {
	case string:
		return []string{val}, nil

	case []string:
		return val, nil

	case []any:
		result := make([]string, len(val))
		for i, item := range val {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("resource array item %d must be string, got %T", i, item)
			}
			result[i] = s
		}
		return result, nil

	default:
		return nil, fmt.Errorf("resource must be string or array, got %T", v)
	}
}

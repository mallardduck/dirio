package minio

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PolicyList represents a list of policy names that can be unmarshaled from either:
// - A single string: "policy1"
// - A comma-separated string: "policy1,policy2"
// - An array: ["policy1", "policy2"]
type PolicyList []string

// UnmarshalJSON implements custom unmarshaling for PolicyList
func (p *PolicyList) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as array first
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*p = PolicyList(arr)
		return nil
	}

	// Try to unmarshal as string
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return fmt.Errorf("policy must be string or array of strings")
	}

	// Handle empty string
	if str == "" {
		*p = PolicyList{}
		return nil
	}

	// Split by comma for comma-separated policies
	policies := strings.Split(str, ",")
	for i, policy := range policies {
		policies[i] = strings.TrimSpace(policy)
	}

	*p = PolicyList(policies)
	return nil
}

// MarshalJSON implements custom marshaling for PolicyList
func (p PolicyList) MarshalJSON() ([]byte, error) {
	// Marshal as array
	return json.Marshal([]string(p))
}

// String returns a comma-separated string representation
func (p PolicyList) String() string {
	return strings.Join(p, ",")
}

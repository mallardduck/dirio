package policy_test

import (
	"encoding/json"
	"fmt"

	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/pkg/iam"
)

// Example showing how to validate a policy document after unmarshaling
func ExampleValidateAction() {
	// Simulate unmarshaling a policy from JSON
	var stmt iam.Statement
	policyJSON := `{
		"Effect": "Allow",
		"Action": ["s3:GetObject", "s3:PutObject"],
		"Resource": "arn:aws:s3:::my-bucket/*"
	}`
	json.Unmarshal([]byte(policyJSON), &stmt)

	// Validate the Action field
	if err := policy.ValidateAction(stmt.Action); err != nil {
		fmt.Printf("Invalid action: %v\n", err)
		return
	}

	fmt.Println("Action is valid")
	// Output: Action is valid
}

// Example showing how to normalize actions to a consistent []string format
func ExampleNormalizeAction() {
	// Actions can come in different formats from JSON
	examples := []any{
		"s3:GetObject",                      // Single string
		[]string{"s3:GetObject", "s3:Put*"}, // Array of strings
		[]any{"s3:ListBucket"},              // Array from JSON unmarshal
	}

	for i, action := range examples {
		normalized, err := policy.NormalizeAction(action)
		if err != nil {
			fmt.Printf("Error normalizing action %d: %v\n", i, err)
			continue
		}

		fmt.Printf("Example %d: %v -> %v\n", i+1, action, normalized)
	}
	// Output:
	// Example 1: s3:GetObject -> [s3:GetObject]
	// Example 2: [s3:GetObject s3:Put*] -> [s3:GetObject s3:Put*]
	// Example 3: [s3:ListBucket] -> [s3:ListBucket]
}

// Example showing comprehensive policy validation
func ExampleStatement_Validate() {
	// Create a valid statement
	validStmt := iam.Statement{
		Effect:   "Allow",
		Action:   "s3:GetObject",
		Resource: "arn:aws:s3:::my-bucket/*",
	}

	if err := validStmt.Validate(); err != nil {
		fmt.Printf("Valid statement failed: %v\n", err)
	} else {
		fmt.Println("Valid statement passed")
	}

	// Create an invalid statement (missing Resource)
	invalidStmt := iam.Statement{
		Effect: "Allow",
		Action: "s3:GetObject",
		// Missing Resource!
	}

	if err := invalidStmt.Validate(); err != nil {
		fmt.Println("Invalid statement correctly rejected")
	}

	// Output:
	// Valid statement passed
	// Invalid statement correctly rejected
}

// Example showing the difference between interface{} usage before and after improvements
func Example_beforeAndAfter() {
	stmt := iam.Statement{
		Action: []any{"s3:GetObject", "s3:PutObject"}, // From JSON unmarshal
	}

	// ❌ BEFORE: Manual type assertion (error-prone)
	fmt.Println("=== Before (manual type assertions) ===")
	var actionsBefore []string
	switch v := stmt.Action.(type) {
	case string:
		actionsBefore = []string{v}
	case []string:
		actionsBefore = v
	case []any:
		actionsBefore = make([]string, len(v))
		for i, item := range v {
			if s, ok := item.(string); ok {
				actionsBefore[i] = s
			}
		}
	}
	fmt.Printf("Actions: %v\n", actionsBefore)

	// ✅ AFTER: Use normalization function (clean, tested)
	fmt.Println("\n=== After (using NormalizeAction) ===")
	actionsAfter, err := policy.NormalizeAction(stmt.Action)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Actions: %v\n", actionsAfter)

	// Output:
	// === Before (manual type assertions) ===
	// Actions: [s3:GetObject s3:PutObject]
	//
	// === After (using NormalizeAction) ===
	// Actions: [s3:GetObject s3:PutObject]
}

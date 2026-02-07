package policy

import (
	"strings"

	"github.com/mallardduck/dirio/pkg/iam"
)

// matchPrincipal checks if the request principal matches the statement principal.
//
// Principal formats:
//   - "*" or {"AWS": "*"} - matches everyone (including anonymous)
//   - {"AWS": "arn:aws:iam::123456789012:user/username"} - specific user
//   - {"AWS": ["arn:...", "arn:..."]} - multiple users
//
// For Phase 3.1 MVP, we only handle:
//   - "*" for public access (anonymous allowed)
//   - Authenticated admin bypass (handled at engine level)
func matchPrincipal(stmtPrincipal interface{}, reqPrincipal *Principal) bool {
	if stmtPrincipal == nil {
		// No principal specified - this is unusual, treat as no match
		return false
	}

	// Handle string principal (e.g., "*")
	if s, ok := stmtPrincipal.(string); ok {
		if s == "*" {
			return true // Everyone matches, including anonymous
		}
		// For Phase 3.1, we don't handle specific user ARNs in string format
		return false
	}

	// Handle map principal (e.g., {"AWS": "*"} or {"AWS": ["arn:..."]})
	if m, ok := stmtPrincipal.(map[string]interface{}); ok {
		if aws, exists := m["AWS"]; exists {
			return matchAWSPrincipal(aws, reqPrincipal)
		}
		// Could also have "Service", "Federated", etc. - not supported in Phase 3.1
		return false
	}

	return false
}

// matchAWSPrincipal handles the AWS portion of a principal map
func matchAWSPrincipal(aws interface{}, reqPrincipal *Principal) bool {
	// Handle "*" for public access
	if s, ok := aws.(string); ok {
		if s == "*" {
			return true // Everyone matches
		}
		// Single ARN - check if it matches the user
		return matchUserARN(s, reqPrincipal)
	}

	// Handle array of ARNs
	if arr, ok := aws.([]interface{}); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				if s == "*" {
					return true
				}
				if matchUserARN(s, reqPrincipal) {
					return true
				}
			}
		}
	}

	// Handle array of strings
	if arr, ok := aws.([]string); ok {
		for _, s := range arr {
			if s == "*" {
				return true
			}
			if matchUserARN(s, reqPrincipal) {
				return true
			}
		}
	}

	return false
}

// matchUserARN checks if a user ARN matches the request principal
func matchUserARN(arn string, reqPrincipal *Principal) bool {
	if reqPrincipal.IsAnonymous || reqPrincipal.User == nil {
		return false // Anonymous requests don't match specific ARNs
	}

	// For Phase 3.1, we do simple username matching
	// ARN format: arn:aws:iam::123456789012:user/username
	// We extract username and compare with User.AccessKey or User.Username
	if strings.HasPrefix(arn, "arn:aws:iam::") && strings.Contains(arn, ":user/") {
		parts := strings.Split(arn, ":user/")
		if len(parts) == 2 {
			username := parts[1]
			if reqPrincipal.User.AccessKey == username {
				return true
			}
		}
	}

	return false
}

// matchAction checks if the request action matches the statement action.
//
// Action formats:
//   - "s3:GetObject" - specific action
//   - "s3:*" - all S3 actions
//   - "s3:Get*" - all S3 Get actions
//   - ["s3:GetObject", "s3:PutObject"] - multiple actions
func matchAction(stmtAction interface{}, reqAction string) bool {
	if stmtAction == nil {
		return false
	}

	// Handle single string action
	if s, ok := stmtAction.(string); ok {
		return matchSingleAction(s, reqAction)
	}

	// Handle array of actions
	if arr, ok := stmtAction.([]interface{}); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				if matchSingleAction(s, reqAction) {
					return true
				}
			}
		}
	}

	// Handle array of strings
	if arr, ok := stmtAction.([]string); ok {
		for _, s := range arr {
			if matchSingleAction(s, reqAction) {
				return true
			}
		}
	}

	return false
}

// matchSingleAction matches a single action pattern against a request action
func matchSingleAction(pattern, reqAction string) bool {
	// Exact match
	if pattern == reqAction {
		return true
	}

	// Wildcard "*" matches everything
	if pattern == "*" {
		return true
	}

	// Handle prefix wildcards like "s3:*" or "s3:Get*"
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(reqAction, prefix)
	}

	return false
}

// matchResource checks if the request resource matches the statement resource.
//
// Resource formats:
//   - "arn:aws:s3:::bucket" - specific bucket
//   - "arn:aws:s3:::bucket/*" - all objects in bucket
//   - "arn:aws:s3:::bucket/prefix/*" - objects with prefix
//   - "*" - all resources
//   - ["arn:...", "arn:..."] - multiple resources
func matchResource(stmtResource interface{}, reqResource *Resource) bool {
	if stmtResource == nil {
		return false
	}

	reqARN := reqResource.ARN()

	// Handle single string resource
	if s, ok := stmtResource.(string); ok {
		return matchSingleResource(s, reqARN)
	}

	// Handle array of resources
	if arr, ok := stmtResource.([]interface{}); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok {
				if matchSingleResource(s, reqARN) {
					return true
				}
			}
		}
	}

	// Handle array of strings
	if arr, ok := stmtResource.([]string); ok {
		for _, s := range arr {
			if matchSingleResource(s, reqARN) {
				return true
			}
		}
	}

	return false
}

// matchSingleResource matches a single resource pattern against a request ARN
func matchSingleResource(pattern, reqARN string) bool {
	// Exact match
	if pattern == reqARN {
		return true
	}

	// Wildcard "*" matches everything
	if pattern == "*" {
		return true
	}

	// Handle suffix wildcards like "arn:aws:s3:::bucket/*"
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(reqARN, prefix)
	}

	// Handle "?" single character wildcard (advanced - Phase 3.2)
	// For now, not supported

	return false
}

// evaluateStatement evaluates a single policy statement against a request.
// Returns the decision based on the statement's Effect if all conditions match.
func evaluateStatement(stmt *iam.Statement, req *RequestContext) Decision {
	// 1. Check if principal matches
	if !matchPrincipal(stmt.Principal, req.Principal) {
		return DecisionDeny // Statement doesn't apply
	}

	// 2. Check if action matches
	if !matchAction(stmt.Action, req.Action) {
		return DecisionDeny // Statement doesn't apply
	}

	// 3. Check if resource matches
	if !matchResource(stmt.Resource, req.Resource) {
		return DecisionDeny // Statement doesn't apply
	}

	// 4. Check conditions (Phase 3.2 - skip for MVP)
	// TODO: Implement condition evaluation

	// 5. Return effect
	if stmt.Effect == "Deny" {
		return DecisionExplicitDeny
	}
	return DecisionAllow
}

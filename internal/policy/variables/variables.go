package variables

import (
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Variable names supported by the policy engine
const (
	// AWS IAM variables
	VarUsername    = "aws:username"    // Current authenticated username
	VarUserID      = "aws:userid"      // Current authenticated user UUID
	VarSourceIP    = "aws:SourceIp"    // Request source IP address
	VarCurrentTime = "aws:CurrentTime" // Current timestamp (ISO 8601)
	VarUserAgent   = "aws:UserAgent"   // HTTP User-Agent header

	// S3-specific variables
	VarS3Prefix    = "s3:prefix"    // Object key prefix for ListObjects
	VarS3Delimiter = "s3:delimiter" // Delimiter for ListObjects
)

// Context holds values for variable substitution during policy evaluation
type Context struct {
	// User identity
	Username string
	UserID   uuid.UUID

	// Request metadata
	SourceIP    net.IP
	CurrentTime time.Time
	UserAgent   string

	// S3-specific context
	S3Prefix    string
	S3Delimiter string
}

// variablePattern matches ${variable} syntax
var variablePattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Substitute replaces all ${variable} patterns in the input string with their values from the context.
// Returns the substituted string and any error encountered.
//
// Examples:
//   - "arn:aws:s3:::bucket/${aws:username}/*" → "arn:aws:s3:::bucket/alice/*"
//   - "prefix/${aws:userid}/data" → "prefix/550e8400-e29b-41d4-a716-446655440000/data"
func (c *Context) Substitute(input string) (string, error) {
	var firstError error

	result := variablePattern.ReplaceAllStringFunc(input, func(match string) string {
		// Extract variable name (remove ${ and })
		varName := match[2 : len(match)-1]

		// Get the value for this variable
		value, err := c.getValue(varName)
		if err != nil {
			// Store first error but continue substitution
			if firstError == nil {
				firstError = err
			}
			// Return the original pattern if substitution fails
			return match
		}

		return value
	})

	return result, firstError
}

// getValue returns the value for a given variable name
func (c *Context) getValue(varName string) (string, error) {
	switch varName {
	case VarUsername:
		if c.Username == "" {
			return "", fmt.Errorf("variable %q not available: username not set", varName)
		}
		return c.Username, nil

	case VarUserID:
		if c.UserID == uuid.Nil {
			return "", fmt.Errorf("variable %q not available: user ID not set", varName)
		}
		return c.UserID.String(), nil

	case VarSourceIP:
		if c.SourceIP == nil {
			return "", fmt.Errorf("variable %q not available: source IP not set", varName)
		}
		return c.SourceIP.String(), nil

	case VarCurrentTime:
		if c.CurrentTime.IsZero() {
			return "", fmt.Errorf("variable %q not available: current time not set", varName)
		}
		return c.CurrentTime.Format(time.RFC3339), nil

	case VarUserAgent:
		if c.UserAgent == "" {
			return "", fmt.Errorf("variable %q not available: user agent not set", varName)
		}
		return c.UserAgent, nil

	case VarS3Prefix:
		// S3 prefix can be empty (list all objects)
		return c.S3Prefix, nil

	case VarS3Delimiter:
		// S3 delimiter can be empty (no grouping)
		return c.S3Delimiter, nil

	default:
		return "", fmt.Errorf("unknown variable: %q", varName)
	}
}

// HasVariables returns true if the input string contains any ${...} variable patterns
func HasVariables(input string) bool {
	return variablePattern.MatchString(input)
}

// ExtractVariables returns a list of all variable names found in the input string
func ExtractVariables(input string) []string {
	matches := variablePattern.FindAllStringSubmatch(input, -1)
	variables := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			variables = append(variables, match[1])
		}
	}
	return variables
}

// SubstituteSlice applies substitution to all strings in a slice
func (c *Context) SubstituteSlice(inputs []string) ([]string, error) {
	results := make([]string, len(inputs))
	var firstError error

	for i, input := range inputs {
		result, err := c.Substitute(input)
		if err != nil && firstError == nil {
			firstError = err
		}
		results[i] = result
	}

	return results, firstError
}

// SubstituteInterface handles substitution for interface{} values (string or []string)
// This is useful for Resource and Action fields in policy statements which can be either type
func (c *Context) SubstituteInterface(input interface{}) (interface{}, error) {
	switch v := input.(type) {
	case string:
		return c.Substitute(v)
	case []string:
		return c.SubstituteSlice(v)
	case []interface{}:
		// Convert []interface{} to []string, substitute, and return
		strs := make([]string, len(v))
		for i, item := range v {
			if str, ok := item.(string); ok {
				strs[i] = str
			} else {
				strs[i] = fmt.Sprintf("%v", item)
			}
		}
		return c.SubstituteSlice(strs)
	default:
		// For other types, return as-is
		return input, nil
	}
}

// Normalize ensures commonly-used variable names work with different casing
// AWS uses inconsistent casing (aws:SourceIp vs aws:username), so we normalize
func Normalize(varName string) string {
	// Already normalized
	if _, err := (&Context{}).getValue(varName); err == nil {
		return varName
	}

	// Try case-insensitive match for common variants
	lower := strings.ToLower(varName)
	switch {
	case strings.Contains(lower, "username"):
		return VarUsername
	case strings.Contains(lower, "userid"):
		return VarUserID
	case strings.Contains(lower, "sourceip"):
		return VarSourceIP
	case strings.Contains(lower, "currenttime"):
		return VarCurrentTime
	case strings.Contains(lower, "useragent"):
		return VarUserAgent
	case strings.Contains(lower, "prefix"):
		return VarS3Prefix
	case strings.Contains(lower, "delimiter"):
		return VarS3Delimiter
	}

	// Return original if no match
	return varName
}

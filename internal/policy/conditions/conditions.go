package conditions

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/mallardduck/dirio/internal/policy/variables"
)

// Operator represents a condition operator type
type Operator string

// String condition operators
const (
	OpStringEquals              Operator = "StringEquals"
	OpStringNotEquals           Operator = "StringNotEquals"
	OpStringEqualsIgnoreCase    Operator = "StringEqualsIgnoreCase"
	OpStringNotEqualsIgnoreCase Operator = "StringNotEqualsIgnoreCase"
	OpStringLike                Operator = "StringLike"
	OpStringNotLike             Operator = "StringNotLike"
)

// Numeric condition operators
const (
	OpNumericEquals            Operator = "NumericEquals"
	OpNumericNotEquals         Operator = "NumericNotEquals"
	OpNumericLessThan          Operator = "NumericLessThan"
	OpNumericLessThanEquals    Operator = "NumericLessThanEquals"
	OpNumericGreaterThan       Operator = "NumericGreaterThan"
	OpNumericGreaterThanEquals Operator = "NumericGreaterThanEquals"
)

// Date condition operators
const (
	OpDateEquals            Operator = "DateEquals"
	OpDateNotEquals         Operator = "DateNotEquals"
	OpDateLessThan          Operator = "DateLessThan"
	OpDateLessThanEquals    Operator = "DateLessThanEquals"
	OpDateGreaterThan       Operator = "DateGreaterThan"
	OpDateGreaterThanEquals Operator = "DateGreaterThanEquals"
)

// IP address condition operators
const (
	OpIpAddress    Operator = "IpAddress"
	OpNotIpAddress Operator = "NotIpAddress"
)

// Boolean condition operator
const (
	OpBool Operator = "Bool"
)

// Null condition operator
const (
	OpNull Operator = "Null"
)

// Context holds runtime values for condition evaluation
type Context struct {
	// From ConditionContext
	SourceIP        net.IP
	UserAgent       string
	SecureTransport bool
	CurrentTime     time.Time

	// From VarContext
	Username    string
	UserID      string
	S3Prefix    string
	S3Delimiter string

	// Additional fields
	ContentLength int64

	// Variable substitution context
	VarContext *variables.Context
}

// Evaluator evaluates policy conditions against a context
type Evaluator struct {
	ctx *Context
}

// NewEvaluator creates a new condition evaluator with the given context
func NewEvaluator(ctx *Context) *Evaluator {
	return &Evaluator{ctx: ctx}
}

// Evaluate evaluates a set of policy conditions
// Returns true if all conditions pass (AND logic across operators)
// Returns false if any condition fails or an error occurs (fail-closed)
func (e *Evaluator) Evaluate(conditions map[string]any) (bool, error) {
	if len(conditions) == 0 {
		return true, nil // No conditions means automatic pass
	}

	// Iterate over operators (AND logic)
	for opStr, keyValues := range conditions {
		op := Operator(opStr)

		// Evaluate this operator
		match, err := e.evaluateOperator(op, keyValues)
		if err != nil {
			return false, fmt.Errorf("operator %s: %w", op, err)
		}

		// Short-circuit: if any operator fails, entire condition fails
		if !match {
			return false, nil
		}
	}

	// All operators passed
	return true, nil
}

// evaluateOperator evaluates a single operator with its key-value pairs
// Returns true if all key-value pairs match (AND logic)
func (e *Evaluator) evaluateOperator(op Operator, keyValues any) (bool, error) {
	// keyValues should be a map[string]interface{}
	kvMap, ok := keyValues.(map[string]any)
	if !ok {
		return false, fmt.Errorf("invalid condition format: expected map, got %T", keyValues)
	}

	// Iterate over key-value pairs (AND logic)
	for key, value := range kvMap {
		match, err := e.evaluateSingleCondition(op, key, value)
		if err != nil {
			return false, fmt.Errorf("key %s: %w", key, err)
		}

		// Short-circuit: if any key-value fails, operator fails
		if !match {
			return false, nil
		}
	}

	// All key-value pairs passed
	return true, nil
}

// evaluateSingleCondition evaluates a single condition key-value pair
// Returns true if the condition matches
func (e *Evaluator) evaluateSingleCondition(op Operator, key string, value any) (bool, error) {
	// Get runtime value for this condition key
	contextValue, err := e.getContextValue(key)
	if err != nil {
		// Missing context value - fail closed
		return false, err
	}

	// Apply variable substitution to condition value
	substitutedValue := value
	if e.ctx.VarContext != nil {
		var subErr error
		substitutedValue, subErr = e.ctx.VarContext.SubstituteInterface(value)
		if subErr != nil {
			// Substitution failed - use original value and continue
			substitutedValue = value
		}
	}

	// Handle array values (OR logic - any match succeeds)
	if arr, ok := substitutedValue.([]any); ok {
		for _, item := range arr {
			match, err := e.evaluateConditionValue(op, key, contextValue, item)
			if err != nil {
				continue // Skip invalid values
			}
			if match {
				return true, nil // Any match succeeds
			}
		}
		return false, nil // No matches
	}

	if arr, ok := substitutedValue.([]string); ok {
		for _, item := range arr {
			match, err := e.evaluateConditionValue(op, key, contextValue, item)
			if err != nil {
				continue // Skip invalid values
			}
			if match {
				return true, nil // Any match succeeds
			}
		}
		return false, nil // No matches
	}

	// Single value
	return e.evaluateConditionValue(op, key, contextValue, substitutedValue)
}

// evaluateConditionValue evaluates a single value against the context
func (e *Evaluator) evaluateConditionValue(op Operator, _ string, contextValue, conditionValue any) (bool, error) {
	switch op {
	// String operators
	case OpStringEquals:
		return evaluateStringEquals(contextValue, conditionValue, false)
	case OpStringNotEquals:
		match, err := evaluateStringEquals(contextValue, conditionValue, false)
		return !match, err
	case OpStringEqualsIgnoreCase:
		return evaluateStringEquals(contextValue, conditionValue, true)
	case OpStringNotEqualsIgnoreCase:
		match, err := evaluateStringEquals(contextValue, conditionValue, true)
		return !match, err
	case OpStringLike:
		return evaluateStringLike(contextValue, conditionValue, false)
	case OpStringNotLike:
		return evaluateStringLike(contextValue, conditionValue, true)

	// Numeric operators
	case OpNumericEquals:
		return evaluateNumericEquals(contextValue, conditionValue)
	case OpNumericNotEquals:
		match, err := evaluateNumericEquals(contextValue, conditionValue)
		return !match, err
	case OpNumericLessThan:
		return evaluateNumericLessThan(contextValue, conditionValue, false)
	case OpNumericLessThanEquals:
		return evaluateNumericLessThan(contextValue, conditionValue, true)
	case OpNumericGreaterThan:
		return evaluateNumericGreaterThan(contextValue, conditionValue, false)
	case OpNumericGreaterThanEquals:
		return evaluateNumericGreaterThan(contextValue, conditionValue, true)

	// Date operators
	case OpDateEquals:
		return evaluateDateEquals(contextValue, conditionValue)
	case OpDateNotEquals:
		match, err := evaluateDateEquals(contextValue, conditionValue)
		return !match, err
	case OpDateLessThan:
		return evaluateDateLessThan(contextValue, conditionValue, false)
	case OpDateLessThanEquals:
		return evaluateDateLessThan(contextValue, conditionValue, true)
	case OpDateGreaterThan:
		return evaluateDateGreaterThan(contextValue, conditionValue, false)
	case OpDateGreaterThanEquals:
		return evaluateDateGreaterThan(contextValue, conditionValue, true)

	// IP operators
	case OpIpAddress:
		return evaluateIpAddress(contextValue, conditionValue, false)
	case OpNotIpAddress:
		return evaluateIpAddress(contextValue, conditionValue, true)

	// Boolean operator
	case OpBool:
		return evaluateBool(contextValue, conditionValue)

	// Null operator
	case OpNull:
		return evaluateNull(contextValue, conditionValue)

	default:
		return false, fmt.Errorf("unknown operator: %s", op)
	}
}

// getContextValue retrieves the runtime value for a condition key
func (e *Evaluator) getContextValue(key string) (any, error) {
	switch key {
	case "aws:SourceIp":
		if e.ctx.SourceIP == nil {
			return nil, fmt.Errorf("source IP not available")
		}
		return e.ctx.SourceIP, nil

	case "aws:UserAgent":
		if e.ctx.UserAgent == "" {
			return nil, fmt.Errorf("user agent not available")
		}
		return e.ctx.UserAgent, nil

	case "aws:SecureTransport":
		return e.ctx.SecureTransport, nil

	case "aws:CurrentTime":
		if e.ctx.CurrentTime.IsZero() {
			return nil, fmt.Errorf("current time not available")
		}
		return e.ctx.CurrentTime, nil

	case "aws:username":
		if e.ctx.Username == "" {
			return nil, fmt.Errorf("username not available")
		}
		return e.ctx.Username, nil

	case "aws:userid":
		if e.ctx.UserID == "" {
			return nil, fmt.Errorf("user ID not available")
		}
		return e.ctx.UserID, nil

	case "s3:prefix":
		// S3 prefix can be empty
		return e.ctx.S3Prefix, nil

	case "s3:delimiter":
		// S3 delimiter can be empty
		return e.ctx.S3Delimiter, nil

	case "s3:content-length":
		return e.ctx.ContentLength, nil

	default:
		return nil, fmt.Errorf("unknown condition key: %s", key)
	}
}

// Helper function to convert interface{} to string
func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Helper function to convert interface{} to float64
func toFloat64(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case float32:
		return float64(val), nil
	case int:
		return float64(val), nil
	case int32:
		return float64(val), nil
	case int64:
		return float64(val), nil
	case string:
		return strconv.ParseFloat(val, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float64", v)
	}
}

// Helper function to convert interface{} to time.Time
func toTime(v any) (time.Time, error) {
	switch val := v.(type) {
	case time.Time:
		return val, nil
	case string:
		// Try multiple formats
		formats := []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02T15:04:05Z",
			"2006-01-02",
		}
		for _, format := range formats {
			if t, err := time.Parse(format, val); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("invalid time format: %s", val)
	default:
		return time.Time{}, fmt.Errorf("cannot convert %T to time.Time", v)
	}
}

// Helper function to convert interface{} to net.IP
func toIP(v any) (net.IP, error) {
	switch val := v.(type) {
	case net.IP:
		return val, nil
	case string:
		ip := net.ParseIP(val)
		if ip == nil {
			return nil, fmt.Errorf("invalid IP address: %s", val)
		}
		return ip, nil
	default:
		return nil, fmt.Errorf("cannot convert %T to IP", v)
	}
}

// Helper function to convert interface{} to bool
func toBool(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		return strconv.ParseBool(val)
	default:
		return false, fmt.Errorf("cannot convert %T to bool", v)
	}
}

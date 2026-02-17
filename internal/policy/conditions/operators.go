package conditions

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// String Operators

// evaluateStringEquals performs exact string matching
func evaluateStringEquals(contextValue, conditionValue interface{}, ignoreCase bool) (bool, error) {
	ctxStr := toString(contextValue)
	condStr := toString(conditionValue)

	if ignoreCase {
		return strings.EqualFold(ctxStr, condStr), nil
	}
	return ctxStr == condStr, nil
}

// evaluateStringLike performs glob pattern matching
// Supports * (any characters) and ? (single character)
func evaluateStringLike(contextValue, conditionValue interface{}, negate bool) (bool, error) {
	ctxStr := toString(contextValue)
	pattern := toString(conditionValue)

	// Convert glob pattern to regex
	regexPattern := globToRegex(pattern)

	matched, err := regexp.MatchString(regexPattern, ctxStr)
	if err != nil {
		return false, fmt.Errorf("invalid pattern: %w", err)
	}

	if negate {
		return !matched, nil
	}
	return matched, nil
}

// globToRegex converts a glob pattern to a regular expression
func globToRegex(pattern string) string {
	var result strings.Builder
	result.WriteString("^")

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			result.WriteString(".*")
		case '?':
			result.WriteString(".")
		case '.', '+', '(', ')', '|', '[', ']', '{', '}', '^', '$', '\\':
			// Escape regex special characters
			result.WriteString("\\")
			result.WriteByte(ch)
		default:
			result.WriteByte(ch)
		}
	}

	result.WriteString("$")
	return result.String()
}

// Numeric Operators

// evaluateNumericEquals checks numeric equality
func evaluateNumericEquals(contextValue, conditionValue interface{}) (bool, error) {
	ctxNum, err := toFloat64(contextValue)
	if err != nil {
		return false, err
	}

	condNum, err := toFloat64(conditionValue)
	if err != nil {
		return false, err
	}

	return ctxNum == condNum, nil
}

// evaluateNumericLessThan checks if context value is less than condition value
func evaluateNumericLessThan(contextValue, conditionValue interface{}, orEqual bool) (bool, error) {
	ctxNum, err := toFloat64(contextValue)
	if err != nil {
		return false, err
	}

	condNum, err := toFloat64(conditionValue)
	if err != nil {
		return false, err
	}

	if orEqual {
		return ctxNum <= condNum, nil
	}
	return ctxNum < condNum, nil
}

// evaluateNumericGreaterThan checks if context value is greater than condition value
func evaluateNumericGreaterThan(contextValue, conditionValue interface{}, orEqual bool) (bool, error) {
	ctxNum, err := toFloat64(contextValue)
	if err != nil {
		return false, err
	}

	condNum, err := toFloat64(conditionValue)
	if err != nil {
		return false, err
	}

	if orEqual {
		return ctxNum >= condNum, nil
	}
	return ctxNum > condNum, nil
}

// Date Operators

// evaluateDateEquals checks date equality
func evaluateDateEquals(contextValue, conditionValue interface{}) (bool, error) {
	ctxTime, err := toTime(contextValue)
	if err != nil {
		return false, err
	}

	condTime, err := toTime(conditionValue)
	if err != nil {
		return false, err
	}

	return ctxTime.Equal(condTime), nil
}

// evaluateDateLessThan checks if context time is before condition time
func evaluateDateLessThan(contextValue, conditionValue interface{}, orEqual bool) (bool, error) {
	ctxTime, err := toTime(contextValue)
	if err != nil {
		return false, err
	}

	condTime, err := toTime(conditionValue)
	if err != nil {
		return false, err
	}

	if orEqual {
		return ctxTime.Before(condTime) || ctxTime.Equal(condTime), nil
	}
	return ctxTime.Before(condTime), nil
}

// evaluateDateGreaterThan checks if context time is after condition time
func evaluateDateGreaterThan(contextValue, conditionValue interface{}, orEqual bool) (bool, error) {
	ctxTime, err := toTime(contextValue)
	if err != nil {
		return false, err
	}

	condTime, err := toTime(conditionValue)
	if err != nil {
		return false, err
	}

	if orEqual {
		return ctxTime.After(condTime) || ctxTime.Equal(condTime), nil
	}
	return ctxTime.After(condTime), nil
}

// IP Address Operators

// evaluateIpAddress checks if IP is in CIDR range
func evaluateIpAddress(contextValue, conditionValue interface{}, negate bool) (bool, error) {
	ctxIP, err := toIP(contextValue)
	if err != nil {
		return false, err
	}

	cidrStr := toString(conditionValue)

	// Check if it's a CIDR or a plain IP
	if !strings.Contains(cidrStr, "/") {
		// Plain IP - add /32 for IPv4 or /128 for IPv6
		if strings.Contains(cidrStr, ":") {
			cidrStr += "/128"
		} else {
			cidrStr += "/32"
		}
	}

	_, ipNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return false, fmt.Errorf("invalid CIDR: %w", err)
	}

	contains := ipNet.Contains(ctxIP)
	if negate {
		return !contains, nil
	}
	return contains, nil
}

// Boolean Operator

// evaluateBool checks boolean value matching
func evaluateBool(contextValue, conditionValue interface{}) (bool, error) {
	ctxBool, err := toBool(contextValue)
	if err != nil {
		return false, err
	}

	condBool, err := toBool(conditionValue)
	if err != nil {
		return false, err
	}

	return ctxBool == condBool, nil
}

// Null Operator

// evaluateNull checks if a value exists or is null/empty
func evaluateNull(contextValue, conditionValue interface{}) (bool, error) {
	shouldBeNull, err := toBool(conditionValue)
	if err != nil {
		return false, err
	}

	isNull := contextValue == nil || contextValue == ""

	// If shouldBeNull is true, we want isNull to be true
	// If shouldBeNull is false, we want isNull to be false
	return isNull == shouldBeNull, nil
}

package validation

import "regexp"

// alphanumericRegex matches alphanumeric characters only
var alphanumericRegex = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

// alphanumericHyphenRegex matches alphanumeric characters and hyphens
var alphanumericHyphenRegex = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

// IsAlphanumeric checks if a string contains only alphanumeric characters
func IsAlphanumeric(s string) bool {
	return alphanumericRegex.MatchString(s)
}

// IsAlphanumericWithHyphens checks if a string contains only alphanumeric characters and hyphens
func IsAlphanumericWithHyphens(s string) bool {
	return alphanumericHyphenRegex.MatchString(s)
}

// InRange checks if a string length is within the specified range
func InRange(s string, minVal, maxVal int) bool {
	length := len(s)
	return length >= minVal && length <= maxVal
}

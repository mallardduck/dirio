package hostname

import (
	"regexp"
	"strings"
)

const maxLabelLen = 63

var labelRE = regexp.MustCompile(`[^a-z0-9-]`)

func sanitize(s string) string {
	s = strings.ToLower(s)
	s = labelRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")

	if len(s) > maxLabelLen {
		s = s[:maxLabelLen]
	}

	return s
}

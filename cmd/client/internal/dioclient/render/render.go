// Package render provides output formatting for dio CLI commands.
// It writes data rows to stdout and status/error messages to stderr.
package render

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// OutputMode controls how data is written to stdout.
type OutputMode int

const (
	// ModeTUI uses styled tables (lipgloss). Chosen automatically when stdout is a TTY.
	ModeTUI OutputMode = iota
	// ModePlain writes plain uncolored text. Safe for piping.
	ModePlain
	// ModeJSON writes newline-delimited JSON, one object per row.
	ModeJSON
)

// DetectMode returns the default OutputMode based on whether stdout is a TTY.
func DetectMode() OutputMode {
	if term.IsTerminal(os.Stdout.Fd()) {
		return ModeTUI
	}
	return ModePlain
}

// ParseMode converts a user-supplied string to an OutputMode.
// Returns ModeTUI and false if the value is not recognised.
func ParseMode(s string) (OutputMode, bool) {
	switch s {
	case "tui", "":
		return ModeTUI, true
	case "plain":
		return ModePlain, true
	case "json":
		return ModeJSON, true
	}
	return ModeTUI, false
}

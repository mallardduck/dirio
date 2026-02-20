// Package output provides styled terminal output helpers for DirIO CLI commands.
// It uses lipgloss for formatting and writes to stderr so that actual data
// (e.g. generated keys) can safely be piped via stdout.
package output

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	successStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("10")) // bright green
	warnStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")) // bright yellow
	hintStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))             // dark grey
	labelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")) // bright blue
	valueStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))            // white
	headerStyle  = lipgloss.NewStyle().Bold(true).Underline(true)
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// Success prints a bold green success message to stderr.
func Success(msg string) {
	fmt.Fprintln(os.Stderr, successStyle.Render("✓ "+msg))
}

// Warn prints a bold yellow warning message to stderr.
func Warn(msg string) {
	fmt.Fprintln(os.Stderr, warnStyle.Render("⚠ "+msg))
}

// Hint prints a dimmed hint/next-step line to stderr.
func Hint(msg string) {
	fmt.Fprintln(os.Stderr, hintStyle.Render("  "+msg))
}

// Header prints a bold underlined section header to stderr.
func Header(msg string) {
	fmt.Fprintln(os.Stderr, headerStyle.Render(msg))
}

// Field prints a labelled key-value pair to stderr.
func Field(label, value string) {
	fmt.Fprintf(os.Stderr, "  %s  %s\n", labelStyle.Render(label+":"), valueStyle.Render(value))
}

// Steps prints a numbered list of next-step instructions to stderr.
func Steps(header string, steps []string) {
	fmt.Fprintln(os.Stderr, dimStyle.Render(header))
	for i, s := range steps {
		fmt.Fprintln(os.Stderr, dimStyle.Render(fmt.Sprintf("  %d. %s", i+1, s)))
	}
}

// Blank prints a blank line to stderr.
func Blank() {
	fmt.Fprintln(os.Stderr)
}

// Divider prints a dim horizontal rule to stderr.
func Divider() {
	fmt.Fprintln(os.Stderr, dimStyle.Render(strings.Repeat("─", 60)))
}

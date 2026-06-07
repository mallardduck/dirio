package render

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	cellStyle   = lipgloss.NewStyle()
)

// Table writes a styled table to w. headers defines the column names; rows is
// a slice of string slices, one per data row. Column widths are auto-sized.
func Table(w io.Writer, headers []string, rows [][]string, mode OutputMode) {
	if len(rows) == 0 && mode != ModePlain {
		fmt.Fprintln(w, dimStyle.Render("  (no results)"))
		return
	}
	if len(rows) == 0 {
		fmt.Fprintln(w, "  (no results)")
		return
	}

	// Calculate column widths.
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) {
				if n := utf8.RuneCountInString(cell); n > widths[i] {
					widths[i] = n
				}
			}
		}
	}

	switch mode {
	case ModeTUI:
		writeTUITable(w, headers, rows, widths)
	case ModePlain, ModeJSON:
		writePlainTable(w, headers, rows, widths)
	}
}

func writeTUITable(w io.Writer, headers []string, rows [][]string, widths []int) {
	// Header row.
	parts := make([]string, len(headers))
	for i, h := range headers {
		parts[i] = headerStyle.Width(widths[i]).Render(h)
	}
	fmt.Fprintln(w, "  "+strings.Join(parts, "  "))

	// Divider.
	divParts := make([]string, len(headers))
	for i, w := range widths {
		divParts[i] = dimStyle.Render(strings.Repeat("─", w))
	}
	fmt.Fprintln(w, "  "+strings.Join(divParts, "  "))

	// Data rows.
	for _, row := range rows {
		cells := make([]string, len(headers))
		for i := range headers {
			val := ""
			if i < len(row) {
				val = row[i]
			}
			cells[i] = cellStyle.Width(widths[i]).Render(val)
		}
		fmt.Fprintln(w, "  "+strings.Join(cells, "  "))
	}
}

func writePlainTable(w io.Writer, headers []string, rows [][]string, widths []int) {
	fmt.Fprintln(w, formatRow(headers, widths))
	fmt.Fprintln(w, strings.Repeat("-", totalWidth(widths)))
	for _, row := range rows {
		fmt.Fprintln(w, formatRow(row, widths))
	}
}

func formatRow(cols []string, widths []int) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		val := ""
		if i < len(cols) {
			val = cols[i]
		}
		parts[i] = fmt.Sprintf("%-*s", w, val)
	}
	return strings.Join(parts, "  ")
}

func totalWidth(widths []int) int {
	total := 0
	for _, w := range widths {
		total += w + 2
	}
	if total > 0 {
		total -= 2
	}
	return total
}

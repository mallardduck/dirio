package logging

import (
	"io"
	"log"
	"os"
)

// DiscardLogger returns a standard library logger that discards all output.
// Use this for noisy third-party libraries that should be silenced.
func DiscardLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

// StdLogger returns a standard library *log.Logger for third-party libraries
// that don't support slog. The logger respects the global verbosity setting:
// - In quiet mode: discards all output
// - Otherwise: writes to stderr with a component prefix
func StdLogger(component string) *log.Logger {
	// TODO: revisit quite mode to explore if we can be more surgical. should only skip non-critical error logs
	return log.New(os.Stderr, "["+component+"] ", log.LstdFlags)
}

package logging

import "log/slog"

// Component returns a logger with a "component" attribute set to the given name.
// Use this to create component-specific loggers for structured logging.
func Component(name string) *slog.Logger {
	return Default().With("component", name)
}

// ShouldLogSuppressible returns true if suppressible messages should be logged.
// Suppressible messages are routine/noisy messages that can be hidden in quiet mode.
// Returns false only when verbosity is "quiet".
func ShouldLogSuppressible() bool {
	return globalVerbosity != VerbosityQuiet
}

// ShouldLogVerbose returns true if verbose messages should be logged.
// Verbose messages are extra diagnostic output only shown in verbose mode.
// Returns true only when verbosity is "verbose".
func ShouldLogVerbose() bool {
	return globalVerbosity == VerbosityVerbose
}

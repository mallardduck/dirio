package logging

// Verbosity represents the chattiness level of log output.
// This is separate from log levels (debug, info, warn, error) - verbosity
// controls how much detail components output at any given level.
type Verbosity int

const (
	// VerbosityQuiet suppresses suppressible messages (routine noise)
	VerbosityQuiet Verbosity = -1
	// VerbosityNormal is the default verbosity level
	VerbosityNormal Verbosity = 0
	// VerbosityVerbose enables extra diagnostic output
	VerbosityVerbose Verbosity = 1
)

// ParseVerbosity converts a string to a Verbosity level.
// Accepts "quiet", "normal", or "verbose". Defaults to VerbosityNormal.
func ParseVerbosity(s string) Verbosity {
	switch s {
	case "quiet":
		return VerbosityQuiet
	case "verbose":
		return VerbosityVerbose
	default:
		return VerbosityNormal
	}
}

// String returns the string representation of the verbosity level.
func (v Verbosity) String() string {
	switch v {
	case VerbosityQuiet:
		return "quiet"
	case VerbosityVerbose:
		return "verbose"
	default:
		return "normal"
	}
}

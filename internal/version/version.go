package version

import "runtime"

var (
	// Version is the current version of the application.
	Version = "dev"
	// Commit is the git commit hash of the application.
	Commit = "none"
	// BuildTime is the time when the application was built.
	BuildTime = "unknown"
	// BuiltBy is the entity that built the application.
	BuiltBy = "local"
	// GoVersion is the version of Go used to build the application.
	GoVersion = runtime.Version()
	// GitTreeState is the state of the git tree (clean or dirty).
	GitTreeState = "unknown"
)

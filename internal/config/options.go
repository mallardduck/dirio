package config

import (
	"github.com/mallardduck/dirio/internal/config/option"
)

// Server configuration options
var (
	// DataDir specifies the root directory for object storage
	DataDir = option.NewOption("data-dir", "/data")

	// Port specifies the HTTP server port
	Port = option.NewOption("port", 9000)

	// AccessKey is the root access key for authentication
	AccessKey = option.NewOption("access-key", "dirio-admin")

	// SecretKey is the root secret key for authentication
	SecretKey = option.NewOption("secret-key", "dirio-admin-secret")
)

// Logging configuration options
var (
	// LogLevel controls the application log level (debug, info, warn, error)
	LogLevel = option.NewOption("log-level", "info")

	// LogFormat controls the log output format (text, json)
	LogFormat = option.NewOption("log-format", "text")

	// Verbosity controls component chattiness (quiet, normal, verbose)
	Verbosity = option.NewOption("verbosity", "normal")

	// Debug enables debug mode (shortcut for log-level=debug)
	Debug = option.NewOption("debug", false)
)

// mDNS and networking options (for future use)
var (
	// MDNSEnabled controls whether mDNS service discovery is enabled
	MDNSEnabled = option.NewOption("mdns-enabled", false)

	// MDNSName is the mDNS service name to advertise
	MDNSName = option.NewOption("mdns-name", "dirio-s3")

	// MDNSHostname is the hostname component for mDNS (defaults to system hostname)
	// The advertised name will be: {mdns-name}.{mdns-hostname}.local
	MDNSHostname = option.NewOption("mdns-hostname", "")

	// MDNSMode controls mDNS responder mode detection
	// - "auto": Detect via port 5353 probe (default)
	// - "guest": Force Guest mode (PTR/SRV only, delegates A/AAAA to system)
	// - "master": Force Master mode (full A/AAAA + PTR/SRV stack)
	MDNSMode = option.NewOption("mdns-mode", "auto")

	// CanonicalDomain is the canonical domain for URL generation
	CanonicalDomain = option.NewOption("canonical-domain", "")
)

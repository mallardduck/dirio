package server

import (
	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/persistence/metadata"
)

var log = logging.Component("auth")

func Authenticator(metadataManager *metadata.Manager, config *Config) *auth.Authenticator {
	dataCredsConfigured := config.DataConfig != nil && config.DataConfig.Credentials.IsConfigured()

	switch {
	case dataCredsConfigured && config.CLICredentialsExplicitlySet:
		// Both data config credentials and explicit CLI credentials present — dual admin mode.
		log.Info("Configured dual admin access",
			"cli_admin", config.AccessKey,
			"data_admin", config.DataConfig.Credentials.AccessKey)
		authenticator := auth.New(metadataManager, config.AccessKey, config.SecretKey)
		return authenticator.WithAlternativeRoot(
			config.DataConfig.Credentials.AccessKey,
			config.DataConfig.Credentials.SecretKey,
		)
	case dataCredsConfigured:
		// Data config credentials configured, no explicit CLI override — data config admin only.
		log.Info("Using data config admin credentials",
			"data_admin", config.DataConfig.Credentials.AccessKey)
		return auth.New(metadataManager,
			config.DataConfig.Credentials.AccessKey,
			config.DataConfig.Credentials.SecretKey,
		)
	default:
		// No configured data credentials — fall back to CLI/env credentials.
		if !config.CLICredentialsExplicitlySet {
			// TODO: eventually we should stop doing this since default credentials are risky
			log.Warn("No admin credentials configured — using defaults. Run \"dirio init\" to set up admin credentials.",
				"admin", config.AccessKey)
		}
		return auth.New(metadataManager, config.AccessKey, config.SecretKey)
	}
}

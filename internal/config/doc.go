/*
Package config implements unified configuration handling for dirio.

This allows uniform definition and handling of environment variables, CLI flags,
and YAML config file values with consistent precedence rules.

# Source Priority

Configuration values are resolved with the following priority (highest to lowest):

 1. Environment variables (DIRIO_* prefix)
 2. CLI flags
 3. Config file (YAML via viper)
 4. Default values

# Usage

Options are defined in options.go using the Option[T] generic type:

	var Port = option.NewOption("port", 9000)

The ValueResolver retrieves values respecting priority:

	resolver := config.NewValueResolver(cmd.Flags(), nil)
	port := resolver.GetInt(config.Port)

For typical usage, call LoadConfig during startup:

	settings, err := config.LoadConfig(cmd.Flags(), nil)
	if err != nil {
		return err
	}

The global configuration can be accessed from anywhere:

	cfg := config.GetConfig()
	// or, if you expect it to be set:
	cfg := config.MustGetConfig()
*/
package config
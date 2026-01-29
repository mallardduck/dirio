package option

// AllOptions returns all registered options
func AllOptions() map[string]RegisteredOption {
	return options
}

// EnvVarsMap maps environment variable keys to their values
type EnvVarsMap map[string]string

// AllEnvValues returns a map of all env keys to their current values
func AllEnvValues() EnvVarsMap {
	envMap := make(EnvVarsMap)

	for _, registeredOption := range options {
		if registeredOption.AllowsEnv() {
			envMap[registeredOption.GetEnvKey()] = registeredOption.GetEnv()
		}
	}

	return envMap
}

// ConfiguredEnvValues returns only env values that are actually set (non-empty)
func ConfiguredEnvValues() EnvVarsMap {
	envMap := make(EnvVarsMap)

	for _, registeredOption := range options {
		if registeredOption.AllowsEnv() {
			if envVal := registeredOption.GetEnv(); envVal != "" {
				envMap[registeredOption.GetEnvKey()] = envVal
			}
		}
	}

	return envMap
}

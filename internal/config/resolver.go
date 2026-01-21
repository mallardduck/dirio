package config

import (
	"fmt"

	"github.com/mallardduck/dirio/internal/config/option"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// ValueResolver handles the resolution of configuration values from multiple sources.
// Priority order (highest to lowest): Environment -> CLI Flag -> Config File (Viper) -> Default
type ValueResolver struct {
	envVars option.EnvVarsMap
	flagSet *pflag.FlagSet
	viper   *viper.Viper
}

// NewValueResolver creates a new resolver with the given flag set and viper instance.
// If viper is nil, the default viper instance will be used.
func NewValueResolver(flagSet *pflag.FlagSet, v *viper.Viper) *ValueResolver {
	if v == nil {
		v = viper.GetViper()
	}
	return &ValueResolver{
		envVars: option.AllEnvValues(),
		flagSet: flagSet,
		viper:   v,
	}
}

// Get retrieves the value for an option, respecting the source priority.
// Returns the value as a string.
func (vr *ValueResolver) Get(o option.RegisteredOption) string {
	// 1. Check environment variable (highest priority)
	if o.AllowsEnv() {
		if val := vr.envVars[o.GetEnvKey()]; val != "" {
			return val
		}
	}

	// 2. Check CLI flags (if flag was explicitly set)
	if o.AllowsFlag() && vr.flagSet != nil {
		flag := vr.flagSet.Lookup(o.GetFlagKey())
		if flag != nil && flag.Changed {
			return flag.Value.String()
		}
	}

	// 3. Check viper (config file)
	if o.AllowsViper() && vr.viper != nil {
		if vr.viper.IsSet(o.GetViperKey()) {
			return vr.viper.GetString(o.GetViperKey())
		}
	}

	// 4. Return default
	return o.GetDefaultAsString()
}

// GetInt retrieves an integer value for an option
func (vr *ValueResolver) GetInt(o option.RegisteredOption) int {
	// 1. Check environment variable (highest priority)
	if o.AllowsEnv() {
		if val := vr.envVars[o.GetEnvKey()]; val != "" {
			if i := parseInt(val); i != 0 {
				return i
			}
		}
	}

	// 2. Check CLI flags
	if o.AllowsFlag() && vr.flagSet != nil {
		flag := vr.flagSet.Lookup(o.GetFlagKey())
		if flag != nil && flag.Changed {
			if i := parseInt(flag.Value.String()); i != 0 {
				return i
			}
		}
	}

	// 3. Check viper
	if o.AllowsViper() && vr.viper != nil {
		if vr.viper.IsSet(o.GetViperKey()) {
			return vr.viper.GetInt(o.GetViperKey())
		}
	}

	// 4. Return default
	return parseInt(o.GetDefaultAsString())
}

// GetBool retrieves a boolean value for an option
func (vr *ValueResolver) GetBool(o option.RegisteredOption) bool {
	// 1. Check environment variable (highest priority)
	if o.AllowsEnv() {
		if val := vr.envVars[o.GetEnvKey()]; val != "" {
			return parseBool(val)
		}
	}

	// 2. Check CLI flags
	if o.AllowsFlag() && vr.flagSet != nil {
		flag := vr.flagSet.Lookup(o.GetFlagKey())
		if flag != nil && flag.Changed {
			return parseBool(flag.Value.String())
		}
	}

	// 3. Check viper
	if o.AllowsViper() && vr.viper != nil {
		if vr.viper.IsSet(o.GetViperKey()) {
			return vr.viper.GetBool(o.GetViperKey())
		}
	}

	// 4. Return default
	return parseBool(o.GetDefaultAsString())
}

func parseInt(s string) int {
	var i int
	fmt.Sscanf(s, "%d", &i)
	return i
}

func parseBool(s string) bool {
	switch s {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}
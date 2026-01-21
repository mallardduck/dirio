package option

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// RegisteredOption defines the interface for all configuration options.
// This allows uniform handling of options regardless of their underlying type.
type RegisteredOption interface {
	GetName() string
	GetDefaultAsString() string

	SetEnvKey(string)
	GetEnvKey() string
	GetEnv() string
	SetAllowFromEnv(bool)
	AllowsEnv() bool

	SetFlagKey(string)
	GetFlagKey() string
	SetAllowFromFlag(bool)
	AllowsFlag() bool

	SetViperKey(string)
	GetViperKey() string
	SetAllowFromViper(bool)
	AllowsViper() bool

	Type() reflect.Type
}

// Option represents a configuration value that can be sourced from
// environment variables, CLI flags, or viper (config file).
// The name should be in kebab-case (e.g., "data-dir").
type Option[T any] struct {
	// Name gives the option an identifier and is the only required field
	Name string

	// Keys for different sources - automatically derived from Name if not set
	EnvKey   string
	FlagKey  string
	ViperKey string

	// Default represents the default value when unset from all other sources
	Default T

	// Source permissions
	AllowFromEnv   bool
	AllowFromFlag  bool
	AllowFromViper bool
}

func (o *Option[T]) GetName() string {
	return o.Name
}

func (o *Option[T]) GetDefaultAsString() string {
	switch v := any(o.Default).(type) {
	case int:
		return strconv.Itoa(v)
	case float64:
		return strconv.FormatFloat(v, 'f', 2, 64)
	case bool:
		return strconv.FormatBool(v)
	case string:
		return v
	default:
		return fmt.Sprintf("%v", o.Default)
	}
}

func (o *Option[T]) SetEnvKey(in string) {
	o.EnvKey = in
}

func (o *Option[T]) GetEnvKey() string {
	return o.EnvKey
}

func (o *Option[T]) GetEnv() string {
	if !o.AllowFromEnv {
		return ""
	}
	return os.Getenv(o.EnvKey)
}

func (o *Option[T]) SetAllowFromEnv(isAllowed bool) {
	o.AllowFromEnv = isAllowed
}

func (o *Option[T]) AllowsEnv() bool {
	return o.AllowFromEnv
}

func (o *Option[T]) SetFlagKey(in string) {
	o.FlagKey = in
}

func (o *Option[T]) GetFlagKey() string {
	return o.FlagKey
}

func (o *Option[T]) SetAllowFromFlag(isAllowed bool) {
	o.AllowFromFlag = isAllowed
}

func (o *Option[T]) AllowsFlag() bool {
	return o.AllowFromFlag
}

func (o *Option[T]) SetViperKey(in string) {
	o.ViperKey = in
}

func (o *Option[T]) GetViperKey() string {
	return o.ViperKey
}

func (o *Option[T]) SetAllowFromViper(isAllowed bool) {
	o.AllowFromViper = isAllowed
}

func (o *Option[T]) AllowsViper() bool {
	return o.AllowFromViper
}

func (o *Option[T]) Type() reflect.Type {
	var z T
	return reflect.TypeOf(z)
}

// Compile-time interface check
var _ RegisteredOption = &Option[string]{}

// Global registry of all options
var options = map[string]RegisteredOption{}

// OptionalValue is a functional option for configuring Option behavior
type OptionalValue func(RegisteredOption)

// WithEnvKey sets a custom environment variable key
func WithEnvKey(key string) OptionalValue {
	return func(o RegisteredOption) {
		o.SetEnvKey(key)
	}
}

// WithFlagKey sets a custom CLI flag key
func WithFlagKey(key string) OptionalValue {
	return func(o RegisteredOption) {
		o.SetFlagKey(key)
	}
}

// WithViperKey sets a custom viper key (for config file)
func WithViperKey(key string) OptionalValue {
	return func(o RegisteredOption) {
		o.SetViperKey(key)
	}
}

// WithoutEnv disables environment variable as a source
var WithoutEnv OptionalValue = func(o RegisteredOption) {
	o.SetEnvKey("")
	o.SetAllowFromEnv(false)
}

// WithoutFlag disables CLI flag as a source
var WithoutFlag OptionalValue = func(o RegisteredOption) {
	o.SetFlagKey("")
	o.SetAllowFromFlag(false)
}

// WithoutViper disables viper/config file as a source
var WithoutViper OptionalValue = func(o RegisteredOption) {
	o.SetViperKey("")
	o.SetAllowFromViper(false)
}

// NewOption creates and registers a new option.
// The name should be kebab-case (e.g., "data-dir").
// By default, options are allowed from all sources (env, flag, viper).
func NewOption[T any](name string, defaultValue T, opts ...OptionalValue) *Option[T] {
	o := &Option[T]{
		Name:           name,
		Default:        defaultValue,
		AllowFromEnv:   true,
		AllowFromFlag:  true,
		AllowFromViper: true,
	}

	for _, opt := range opts {
		opt(o)
	}

	prepareUnsetKeys(o)
	options[o.GetName()] = o

	return o
}

// prepareUnsetKeys derives default keys from the option name if not explicitly set
func prepareUnsetKeys[T any](o *Option[T]) {
	// ENV: kebab-case -> SCREAMING_SNAKE_CASE with DIRIO_ prefix
	if o.AllowFromEnv && o.EnvKey == "" {
		o.EnvKey = "DIRIO_" + strings.ToUpper(strings.ReplaceAll(o.Name, "-", "_"))
	}

	// Flag: use name as-is (kebab-case)
	if o.AllowFromFlag && o.FlagKey == "" {
		o.FlagKey = o.Name
	}

	// Viper: use snake_case (matches viper convention)
	if o.AllowFromViper && o.ViperKey == "" {
		o.ViperKey = strings.ReplaceAll(o.Name, "-", "_")
	}
}
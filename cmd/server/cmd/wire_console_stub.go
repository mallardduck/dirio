//go:build noconsole

package cmd

import "github.com/mallardduck/dirio/internal/http/server"

// setupConsole is a no-op when the noconsole build tag is set.
// Build without console: go build -tags noconsole ./...
func setupConsole(_ *server.Server, _ bool, _ int) {}

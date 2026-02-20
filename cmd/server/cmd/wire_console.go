//go:build !noconsole

package cmd

import (
	"github.com/mallardduck/dirio/console"
	consolewire "github.com/mallardduck/dirio/internal/console"
	"github.com/mallardduck/dirio/internal/http/server"
	"github.com/mallardduck/dirio/internal/service"
)

// setupConsole wires the admin console into the server when the noconsole build
// tag is NOT set (the default). It creates the adapter, builds the console
// handler, and registers it with the server.
func setupConsole(srv *server.Server, enabled bool, address string) {
	if !enabled {
		return
	}

	factory := service.NewServiceFactory(srv.Storage(), srv.Metadata(), srv.PolicyEngine())
	adapter := consolewire.NewAdapter(factory)
	handler := console.New(adapter)

	srv.SetConsole(handler, address)
}

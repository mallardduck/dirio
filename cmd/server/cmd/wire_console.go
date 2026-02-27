//go:build !noconsole

package cmd

import (
	"context"

	"github.com/mallardduck/dirio/console"
	consoleauth "github.com/mallardduck/dirio/console/auth"
	consolewire "github.com/mallardduck/dirio/internal/console"
	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/http/server"
	"github.com/mallardduck/dirio/internal/service"
	"github.com/mallardduck/dirio/pkg/iam"
)

// setupConsole wires the admin console into the server when the noconsole build
// tag is NOT set (the default). It creates the adapter, builds the console
// handler, and registers it with the server.
//
// When dedicatedPort is false the console is mounted at /dirio/ui/ on the main
// S3 port (single-port mode). When true it is served on its own listener at
// port (dual-port mode).
func setupConsole(srv *server.Server, enabled, dedicatedPort bool, port int) {
	if !enabled {
		return
	}

	factory := service.NewServiceFactory(srv.Storage(), srv.Metadata(), srv.PolicyEngine())
	adapter := consolewire.NewAdapter(factory)
	handler := console.New(adapter, srv.Router(), newConsoleAdminAuth(srv.Auth()))

	effectivePort := 0
	if dedicatedPort {
		effectivePort = port
	}
	srv.SetConsole(handler, effectivePort)
}

// consoleAdminAuth adapts internal/http/auth.Authenticator to the console's
// AdminAuth interface, ensuring only admin-UUID credentials are accepted.
type consoleAdminAuth struct {
	authenticator *auth.Authenticator
}

func newConsoleAdminAuth(a *auth.Authenticator) consoleauth.AdminAuth {
	return &consoleAdminAuth{authenticator: a}
}

// AuthenticateAdmin returns true only when the credentials are valid AND the
// resolved user carries the admin UUID.
func (a *consoleAdminAuth) AuthenticateAdmin(ctx context.Context, accessKey, secretKey string) bool {
	if !a.authenticator.ValidateCredentials(ctx, accessKey, secretKey) {
		return false
	}
	user, err := a.authenticator.GetUserForAccessKey(ctx, accessKey)
	if err != nil || user == nil {
		return false
	}
	return user.UUID == iam.AdminUserUUID
}

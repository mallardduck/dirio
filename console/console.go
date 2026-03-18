// Package console provides the embedded DirIO web admin console.
// It imports only consoleapi/ and the standard library — never internal/.
// Handlers render HTML server-side via templ components by calling consoleapi.API directly.
// This makes it straightforward to extract into its own module later if needed.
//
// UI tooling:
//   - go generate runs templ to recompile .templ → _templ.go
//   - make tailwind-build regenerates console/static/style.css via the Tailwind v4 standalone CLI
//   - make vendor-htmx re-downloads htmx.min.js to console/static/

//go:generate templ generate

package console

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/mallardduck/teapot-router/pkg/teapot"

	consoleauth "github.com/mallardduck/dirio/console/auth"
	"github.com/mallardduck/dirio/console/handlers"
	"github.com/mallardduck/dirio/console/middleware"
	"github.com/mallardduck/dirio/console/ui"
	"github.com/mallardduck/dirio/consoleapi"
)

//go:embed static
var staticFiles embed.FS

// New returns an http.Handler that serves the admin console.
// s3Router is used purely for URL generation via named routes; it is not invoked.
// adminAuth validates admin credentials at login time.
// When mounted at a path prefix, callers must strip that prefix before passing
// requests here (e.g. http.StripPrefix("/dirio/ui", New(api, s3Router, adminAuth))).
func New(api consoleapi.API, s3Router ui.S3Router, adminAuth consoleauth.AdminAuth, version string) *teapot.Router {
	ui.AppVersion = version
	sessions, err := consoleauth.NewSession()
	if err != nil {
		panic("console: failed to create session manager: " + err.Error())
	}

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("console: failed to create static sub-filesystem: " + err.Error())
	}

	h := handlers.New(api, s3Router, adminAuth, sessions)

	// UI Console-specific router
	consoleTeapot := teapot.New()

	// TODO: make this conditional and only included when on dedicated port
	consoleTeapot.GET("/.internal/routes", teapot.NewListRoutesHandler(consoleTeapot, nil)).Name("routes")

	// Public routes — accessible without a session (login page + static assets).
	consoleTeapot.Func().GET("/login", h.LoginPage).Name("login")
	consoleTeapot.Func().POST("/login", h.LoginSubmit).Name("login")
	consoleTeapot.Func().POST("/logout", h.Logout).Name("logout")
	consoleTeapot.Func().GET(
		"/static/{AssetUrl:.*}",
		func(rw http.ResponseWriter, r *http.Request) {
			prefix := strings.SplitAfter(r.URL.Path, "/static/")[0]
			http.StripPrefix(prefix, http.FileServer(http.FS(staticFS))).ServeHTTP(rw, r)
		},
	).Name("asset")

	// Protected routes — all require a valid session.
	consoleTeapot.MiddlewareGroup(func(r *teapot.Router) {
		r.Func().GET("/", h.Dashboard).Name("dashboard")
		r.Func().GET("/users", h.Users).Name("users")
		r.Func().POST("/users", h.UserCreate).Name("users.create")
		r.Func().POST("/users/{uuid}/delete", h.UserDelete).Name("users.delete")
		r.Func().POST("/users/{uuid}/status", h.UserSetStatus).Name("users.status")
		r.Func().POST("/users/{uuid}/secret", h.UserUpdateSecret).Name("users.secret.update")
		r.Func().GET("/users/{uuid}/secret", h.UserRevealSecret).Name("users.secret.reveal")
		r.Func().GET("/groups", h.Groups).Name("groups")
		r.Func().POST("/groups", h.GroupCreate).Name("groups.create")
		r.Func().GET("/groups/{group}", h.GroupDetail).Name("groups.detail")
		r.Func().POST("/groups/{group}/delete", h.GroupDelete).Name("groups.delete")
		r.Func().POST("/groups/{group}/members", h.GroupAddMember).Name("groups.members.add")
		r.Func().POST("/groups/{group}/members/remove", h.GroupRemoveMember).Name("groups.members.remove")
		r.Func().POST("/groups/{group}/policies", h.GroupAttachPolicy).Name("groups.policies.attach")
		r.Func().POST("/groups/{group}/policies/detach", h.GroupDetachPolicy).Name("groups.policies.detach")
		r.Func().POST("/groups/{group}/status", h.GroupSetStatus).Name("groups.status")
		r.Func().GET("/service-accounts", h.ServiceAccounts).Name("service-accounts")
		r.Func().POST("/service-accounts", h.ServiceAccountCreate).Name("service-accounts.create")
		r.Func().POST("/service-accounts/{uuid}/delete", h.ServiceAccountDelete).Name("service-accounts.delete")
		r.Func().POST("/service-accounts/{uuid}/status", h.ServiceAccountSetStatus).Name("service-accounts.status")
		r.Func().POST("/service-accounts/{uuid}/secret", h.ServiceAccountUpdateSecret).Name("service-accounts.secret.update")
		r.Func().GET("/service-accounts/{uuid}/secret", h.ServiceAccountRevealSecret).Name("service-accounts.secret.reveal")
		r.Func().GET("/policies", h.Policies).Name("policies")
		r.Func().GET("/buckets", h.Buckets).Name("buckets")
		r.Func().GET("/buckets/{bucket}", h.BucketDetail).Name("buckets.detail")
		r.Func().POST("/buckets/{bucket}/policy", h.BucketPolicySet).Name("buckets.policy.set")
		r.Func().POST("/buckets/{bucket}/ownership", h.BucketTransferOwnership).Name("buckets.ownership.transfer")
		r.Func().GET("/simulate", h.Simulate).Name("simulate")
		r.Func().POST("/simulate", h.Simulate).Name("simulate")
		r.Func().GET("/toasts", h.Toasts).Name("toasts")
	}, middleware.RequireAdminSession(sessions))

	return consoleTeapot
}

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

	consoleauth "github.com/mallardduck/dirio/console/auth"
	"github.com/mallardduck/dirio/console/handlers"
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
func New(api consoleapi.API, s3Router ui.S3Router, adminAuth consoleauth.AdminAuth) http.Handler {
	sessions, err := consoleauth.NewSession()
	if err != nil {
		panic("console: failed to create session manager: " + err.Error())
	}

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("console: failed to create static sub-filesystem: " + err.Error())
	}

	h := handlers.New(api, s3Router, adminAuth, sessions)

	// Public routes — accessible without a session (login page + static assets).
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", h.LoginPage)
	mux.HandleFunc("POST /login", h.LoginSubmit)
	mux.HandleFunc("POST /logout", h.Logout)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Protected routes — all require a valid session.
	protected := http.NewServeMux()
	protected.HandleFunc("GET /{$}", h.Dashboard)
	protected.HandleFunc("GET /users", h.Users)
	protected.HandleFunc("GET /policies", h.Policies)
	protected.HandleFunc("GET /buckets", h.Buckets)

	mux.Handle("/", requireSession(sessions, protected))

	return mux
}

// requireSession is middleware that redirects unauthenticated requests to the
// login page. It wraps a handler so only requests with a valid session cookie
// are forwarded.
func requireSession(sessions *consoleauth.Session, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := sessions.Validate(r); !ok {
			http.Redirect(w, r, string(ui.LoginURL()), http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

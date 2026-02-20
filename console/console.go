// Package console provides the embedded DirIO web admin console.
// It imports only consoleapi/ and the standard library — never internal/.
// Handlers render HTML server-side by calling consoleapi.API directly.
// This makes it straightforward to extract into its own module later if needed.
package console

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/mallardduck/dirio/console/handlers"
	"github.com/mallardduck/dirio/consoleapi"
)

//go:embed static
var staticFiles embed.FS

// New returns an http.Handler that serves the admin console.
// When mounted at a path prefix, callers must strip that prefix before passing
// requests here (e.g. http.StripPrefix("/dirio/ui", New(api))).
func New(api consoleapi.API) http.Handler {
	mux := http.NewServeMux()

	// Static assets (CSS, images, etc. in the future)
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic("console: failed to create static sub-filesystem: " + err.Error())
	}
	// Server-side rendered HTML pages (registered before the catch-all file server).
	h := handlers.New(api)
	mux.HandleFunc("/users", h.Users)
	mux.HandleFunc("/policies", h.Policies)
	mux.HandleFunc("/buckets", h.Buckets)

	// Catch-all: serve static files (index.html, dirio_logo.svg, etc.).
	// http.FileServer handles directory requests by looking for index.html automatically.
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	return mux
}

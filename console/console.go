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
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Server-side rendered HTML pages
	h := handlers.New(api)
	mux.HandleFunc("/users", h.Users)
	mux.HandleFunc("/policies", h.Policies)
	mux.HandleFunc("/buckets", h.Buckets)

	// Root: serve the static index.html placeholder.
	// When the dashboard is implemented it will call api methods and render HTML directly.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "console index not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	return mux
}

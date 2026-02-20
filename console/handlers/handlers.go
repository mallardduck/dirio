// Package handlers provides the server-side HTML handlers for the DirIO admin console.
// Handlers call consoleapi.API directly and render HTML — no JSON API, no client-side fetching.
// This package imports only consoleapi/ and the standard library — never internal/.
package handlers

import (
	"net/http"

	"github.com/mallardduck/dirio/consoleapi"
)

// Handler holds the console API reference used by all handler methods.
type Handler struct {
	api consoleapi.API
}

// New creates a Handler backed by the given API.
func New(api consoleapi.API) *Handler {
	return &Handler{api: api}
}

// Dashboard handles GET / — renders the admin dashboard.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	// TODO: call h.api.ListUsers, h.api.ListBuckets, etc. and render an HTML page.
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// Users handles GET /users — renders the user list page.
func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	// TODO: call h.api.ListUsers and render an HTML page.
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// Policies handles GET /policies — renders the policy list page.
func (h *Handler) Policies(w http.ResponseWriter, r *http.Request) {
	// TODO: call h.api.ListPolicies and render an HTML page.
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// Buckets handles GET /buckets — renders the bucket list page.
func (h *Handler) Buckets(w http.ResponseWriter, r *http.Request) {
	// TODO: call h.api.ListBuckets and render an HTML page.
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

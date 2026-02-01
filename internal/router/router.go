// Package router provides a named routing wrapper around Chi with reverse URL generation.
package router

import (
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
)

// RouteInfo contains metadata about a registered route
type RouteInfo struct {
	Method  string `json:"method"`
	Pattern string `json:"pattern"`
}

// Router wraps chi.Router with named route support and URL generation.
type Router struct {
	mux       chi.Router
	routes    map[string]RouteInfo // name -> route info
	pathStack []string             // tracks path prefixes in nested groups
	nameStack []string             // tracks name prefixes in nested groups
}

// New creates a new Router instance.
func New() *Router {
	return &Router{
		mux:    chi.NewRouter(),
		routes: make(map[string]RouteInfo),
	}
}

// currentPath returns the combined path prefix from the stack.
func (r *Router) currentPath() string {
	return strings.Join(r.pathStack, "")
}

// currentName returns the combined name prefix from the stack.
func (r *Router) currentName() string {
	return strings.Join(r.nameStack, ".")
}

// register adds a route to the registry with the current prefix.
func (r *Router) register(name, pattern, method string) {
	if name == "" {
		return
	}
	fullName := name
	if prefix := r.currentName(); prefix != "" {
		fullName = prefix + "." + name
	}
	fullPattern := r.currentPath() + pattern
	if existing, ok := r.routes[fullName]; ok {
		panic(fmt.Sprintf("router: duplicate route name %q (existing: %s %s, new: %s %s)",
			fullName, existing.Method, existing.Pattern, method, fullPattern))
	}
	r.routes[fullName] = RouteInfo{
		Method:  method,
		Pattern: fullPattern,
	}
}

// Get registers a GET route with an optional name.
// If name is empty, the route is not added to the registry.
// If handler is nil, the route is registered but not bound to the mux (useful for route listing).
func (r *Router) Get(pattern string, handler http.HandlerFunc, name string) {
	r.register(name, pattern, "GET")
	if handler != nil {
		r.mux.Get(r.currentPath()+pattern, handler)
	}
}

// Post registers a POST route with an optional name.
// If handler is nil, the route is registered but not bound to the mux (useful for route listing).
func (r *Router) Post(pattern string, handler http.HandlerFunc, name string) {
	r.register(name, pattern, "POST")
	if handler != nil {
		r.mux.Post(r.currentPath()+pattern, handler)
	}
}

// Put registers a PUT route with an optional name.
// If handler is nil, the route is registered but not bound to the mux (useful for route listing).
func (r *Router) Put(pattern string, handler http.HandlerFunc, name string) {
	r.register(name, pattern, "PUT")
	if handler != nil {
		r.mux.Put(r.currentPath()+pattern, handler)
	}
}

// Patch registers a PATCH route with an optional name.
// If handler is nil, the route is registered but not bound to the mux (useful for route listing).
func (r *Router) Patch(pattern string, handler http.HandlerFunc, name string) {
	r.register(name, pattern, "PATCH")
	if handler != nil {
		r.mux.Patch(r.currentPath()+pattern, handler)
	}
}

// Delete registers a DELETE route with an optional name.
// If handler is nil, the route is registered but not bound to the mux (useful for route listing).
func (r *Router) Delete(pattern string, handler http.HandlerFunc, name string) {
	r.register(name, pattern, "DELETE")
	if handler != nil {
		r.mux.Delete(r.currentPath()+pattern, handler)
	}
}

// Head registers a HEAD route with an optional name.
// If handler is nil, the route is registered but not bound to the mux (useful for route listing).
func (r *Router) Head(pattern string, handler http.HandlerFunc, name string) {
	r.register(name, pattern, "HEAD")
	if handler != nil {
		r.mux.Head(r.currentPath()+pattern, handler)
	}
}

// Options registers an OPTIONS route with an optional name.
// If handler is nil, the route is registered but not bound to the mux (useful for route listing).
func (r *Router) Options(pattern string, handler http.HandlerFunc, name string) {
	r.register(name, pattern, "OPTIONS")
	if handler != nil {
		r.mux.Options(r.currentPath()+pattern, handler)
	}
}

// Method registers a route for a specific HTTP method with an optional name.
// If handler is nil, the route is registered but not bound to the mux (useful for route listing).
func (r *Router) Method(method, pattern string, handler http.HandlerFunc, name string) {
	r.register(name, pattern, method)
	if handler != nil {
		r.mux.Method(method, r.currentPath()+pattern, handler)
	}
}

// Use appends middleware to the router's middleware stack.
func (r *Router) Use(middlewares ...func(http.Handler) http.Handler) {
	r.mux.Use(middlewares...)
}

// Group creates a new route group with a path prefix but no name prefix.
func (r *Router) Group(pattern string, fn func(*Router)) {
	r.pathStack = append(r.pathStack, pattern)
	fn(r)
	r.pathStack = r.pathStack[:len(r.pathStack)-1]
}

// NameGroup creates a new route group with both path and name prefixes.
// Routes registered within the group will have the name prefix prepended.
func (r *Router) NameGroup(pattern, name string, fn func(*Router)) {
	r.pathStack = append(r.pathStack, pattern)
	r.nameStack = append(r.nameStack, name)
	fn(r)
	r.pathStack = r.pathStack[:len(r.pathStack)-1]
	r.nameStack = r.nameStack[:len(r.nameStack)-1]
}

// MiddlewareGroup creates a route group for applying middleware without a path prefix.
// This is useful for segregating routes by middleware without changing their paths.
// Routes registered within the group will still be tracked in the route registry.
//
// Example:
//
//	r.MiddlewareGroup(func(r *Router) {
//	    r.Use(authMiddleware)
//	    r.Get("/admin", adminHandler, "admin")
//	})
func (r *Router) MiddlewareGroup(fn func(*Router)) {
	r.mux.Group(func(mux chi.Router) {
		// Create a wrapper Router that uses the Chi group's mux
		// but shares the same route registry and stacks
		grouped := &Router{
			mux:       mux,
			routes:    r.routes,    // share the registry
			pathStack: r.pathStack, // preserve current path
			nameStack: r.nameStack, // preserve current name
		}
		fn(grouped)
	})
}

// ResourceHandlers defines the handlers for a RESTful resource.
// All fields are optional - only provided handlers will be registered.
type ResourceHandlers struct {
	// Standard Laravel-style resource handlers
	Index   http.HandlerFunc // GET    /resources           -> {name}.index
	Create  http.HandlerFunc // GET    /resources/create    -> {name}.create
	Store   http.HandlerFunc // POST   /resources           -> {name}.store (or PUT if StoreMethod="PUT")
	Show    http.HandlerFunc // GET    /resources/{id}      -> {name}.show
	Edit    http.HandlerFunc // GET    /resources/{id}/edit -> {name}.edit
	Update  http.HandlerFunc // PUT    /resources/{id}      -> {name}.update
	Destroy http.HandlerFunc // DELETE /resources/{id}      -> {name}.destroy

	// Extended handlers (for APIs like S3 that need HEAD)
	Head http.HandlerFunc // HEAD /resources/{id} -> {name}.head

	// Method customization
	// StoreMethod overrides the HTTP method for Store. Default is "POST".
	// Set to "PUT" for APIs like S3 that use PUT to create resources.
	StoreMethod string
}

// Resource registers a RESTful resource with automatic naming.
// The basePath is the collection path (e.g., "/photos").
// The paramPattern is the parameter pattern (e.g., "{photo}").
// This creates Laravel-style resource routes with optional customizations.
func (r *Router) Resource(name, basePath, paramPattern string, handlers ResourceHandlers) {
	itemPath := basePath + "/" + paramPattern

	if handlers.Index != nil {
		r.Get(basePath, handlers.Index, name+".index")
	}
	if handlers.Create != nil {
		r.Get(basePath+"/create", handlers.Create, name+".create")
	}
	if handlers.Store != nil {
		// Use configured method or default to POST
		switch handlers.StoreMethod {
		case "PUT":
			r.Put(basePath, handlers.Store, name+".store")
		default:
			r.Post(basePath, handlers.Store, name+".store")
		}
	}
	if handlers.Show != nil {
		r.Get(itemPath, handlers.Show, name+".show")
	}
	if handlers.Edit != nil {
		r.Get(itemPath+"/edit", handlers.Edit, name+".edit")
	}
	if handlers.Update != nil {
		r.Put(itemPath, handlers.Update, name+".update")
		r.Patch(itemPath, handlers.Update, "") // PATCH shares the update handler but no separate name
	}
	if handlers.Destroy != nil {
		r.Delete(itemPath, handlers.Destroy, name+".destroy")
	}
	if handlers.Head != nil {
		r.Head(itemPath, handlers.Head, name+".head")
	}
}

// paramRegex matches route parameters like {name} or {name:regex}.
var paramRegex = regexp.MustCompile(`\{([^}:]+)(?::[^}]*)?\}`)

// URL generates a URL for a named route with parameter substitution.
// Parameters are provided as key-value pairs: "key1", "value1", "key2", "value2", etc.
// Returns an error if the route name is not found or if parameters are invalid.
func (r *Router) URL(name string, params ...string) (string, error) {
	info, ok := r.routes[name]
	if !ok {
		return "", fmt.Errorf("router: unknown route name %q", name)
	}

	if len(params)%2 != 0 {
		return "", fmt.Errorf("router: params must be key-value pairs, got odd number: %d", len(params))
	}

	// Build param map
	paramMap := make(map[string]string)
	for i := 0; i < len(params); i += 2 {
		paramMap[params[i]] = params[i+1]
	}

	// Replace all parameters in the pattern
	result := paramRegex.ReplaceAllStringFunc(info.Pattern, func(match string) string {
		// Extract parameter name (without regex part)
		submatch := paramRegex.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		paramName := submatch[1]
		if value, ok := paramMap[paramName]; ok {
			return value
		}
		return match // Leave unreplaced if not provided
	})

	return result, nil
}

// MustURL is like URL but panics on error.
func (r *Router) MustURL(name string, params ...string) string {
	url, err := r.URL(name, params...)
	if err != nil {
		panic(err)
	}
	return url
}

// Routes returns a map of route names to patterns (without methods).
// Kept for backward compatibility.
func (r *Router) Routes() map[string]string {
	cpy := make(map[string]string, len(r.routes))
	for name, info := range r.routes {
		cpy[name] = info.Pattern
	}
	return cpy
}

// RoutesWithMethods returns a copy of the route registry with full metadata.
func (r *Router) RoutesWithMethods() map[string]RouteInfo {
	cpy := make(map[string]RouteInfo, len(r.routes))
	maps.Copy(cpy, r.routes)
	return cpy
}

// ServeHTTP implements http.Handler.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.mux.ServeHTTP(w, req)
}

// Chi returns the underlying chi.Router for advanced use cases.
func (r *Router) Chi() chi.Router {
	return r.mux
}

// With returns a new Router that applies additional middleware to subsequent routes.
// This is useful for route-specific middleware without affecting other routes.
// Example:
//
//	r.With(authMiddleware).Get("/admin", adminHandler, "admin")
func (r *Router) With(middlewares ...func(http.Handler) http.Handler) *Router {
	return &Router{
		mux:       r.mux.With(middlewares...),
		routes:    r.routes, // share the same registry
		pathStack: r.pathStack,
		nameStack: r.nameStack,
	}
}

// URLParam returns the URL parameter from a request's context.
// This wraps chi.URLParam for convenience.
func URLParam(r *http.Request, key string) string {
	return chi.URLParam(r, key)
}

// URLParams returns all URL parameters from a request's context as a map.
// This provides a gorilla/mux-like interface for migration convenience.
func URLParams(r *http.Request) map[string]string {
	ctx := chi.RouteContext(r.Context())
	if ctx == nil {
		return nil
	}
	params := make(map[string]string)
	for i, key := range ctx.URLParams.Keys {
		if i < len(ctx.URLParams.Values) {
			params[key] = ctx.URLParams.Values[i]
		}
	}
	return params
}

package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func dummyHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestNew(t *testing.T) {
	r := New()
	if r == nil {
		t.Fatal("New() returned nil")
	}
	if r.mux == nil {
		t.Error("mux is nil")
	}
	if r.routes == nil {
		t.Error("routes map is nil")
	}
}

func TestBasicRouteRegistration(t *testing.T) {
	r := New()

	r.Get("/users", dummyHandler, "users.index")
	r.Post("/users", dummyHandler, "users.store")
	r.Get("/users/{id}", dummyHandler, "users.show")
	r.Put("/users/{id}", dummyHandler, "users.update")
	r.Delete("/users/{id}", dummyHandler, "users.destroy")

	routes := r.Routes()

	expected := map[string]string{
		"users.index":   "/users",
		"users.store":   "/users",
		"users.show":    "/users/{id}",
		"users.update":  "/users/{id}",
		"users.destroy": "/users/{id}",
	}

	for name, pattern := range expected {
		if routes[name] != pattern {
			t.Errorf("route %q: got %q, want %q", name, routes[name], pattern)
		}
	}
}

func TestEmptyNameNotRegistered(t *testing.T) {
	r := New()
	r.Get("/health", dummyHandler, "")

	routes := r.Routes()
	if len(routes) != 0 {
		t.Errorf("expected no routes registered, got %d", len(routes))
	}
}

func TestDuplicateNamePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for duplicate route name")
		}
	}()

	r := New()
	r.Get("/users", dummyHandler, "users")
	r.Get("/other", dummyHandler, "users") // duplicate name
}

func TestNameGroup(t *testing.T) {
	r := New()

	r.Get("/", dummyHandler, "root")

	r.NameGroup("/admin", "admin", func(r *Router) {
		r.Get("/dashboard", dummyHandler, "dashboard")
		r.Get("/users", dummyHandler, "users.index")

		r.NameGroup("/settings", "settings", func(r *Router) {
			r.Get("/general", dummyHandler, "general")
		})
	})

	routes := r.Routes()

	expected := map[string]string{
		"root":                         "/",
		"admin.dashboard":              "/admin/dashboard",
		"admin.users.index":            "/admin/users",
		"admin.settings.general":       "/admin/settings/general",
	}

	for name, pattern := range expected {
		if routes[name] != pattern {
			t.Errorf("route %q: got %q, want %q", name, routes[name], pattern)
		}
	}
}

func TestGroup(t *testing.T) {
	r := New()

	r.Group("/api/v1", func(r *Router) {
		r.Get("/users", dummyHandler, "users.index")
	})

	routes := r.Routes()

	// Group adds path prefix but not name prefix
	if routes["users.index"] != "/api/v1/users" {
		t.Errorf("expected /api/v1/users, got %s", routes["users.index"])
	}
}

func TestResource(t *testing.T) {
	r := New()

	r.Resource("photos", "/photos", "{photo}", ResourceHandlers{
		Index:   dummyHandler,
		Create:  dummyHandler,
		Store:   dummyHandler,
		Show:    dummyHandler,
		Edit:    dummyHandler,
		Update:  dummyHandler,
		Destroy: dummyHandler,
	})

	routes := r.Routes()

	expected := map[string]string{
		"photos.index":   "/photos",
		"photos.create":  "/photos/create",
		"photos.store":   "/photos",
		"photos.show":    "/photos/{photo}",
		"photos.edit":    "/photos/{photo}/edit",
		"photos.update":  "/photos/{photo}",
		"photos.destroy": "/photos/{photo}",
	}

	for name, pattern := range expected {
		if routes[name] != pattern {
			t.Errorf("route %q: got %q, want %q", name, routes[name], pattern)
		}
	}
}

func TestResourcePartial(t *testing.T) {
	r := New()

	// Only register Index and Show
	r.Resource("articles", "/articles", "{article}", ResourceHandlers{
		Index: dummyHandler,
		Show:  dummyHandler,
	})

	routes := r.Routes()

	if len(routes) != 2 {
		t.Errorf("expected 2 routes, got %d", len(routes))
	}

	if _, ok := routes["articles.index"]; !ok {
		t.Error("missing articles.index")
	}
	if _, ok := routes["articles.show"]; !ok {
		t.Error("missing articles.show")
	}
}

func TestResourceInNameGroup(t *testing.T) {
	r := New()

	r.NameGroup("/admin", "admin", func(r *Router) {
		r.Resource("users", "/users", "{user}", ResourceHandlers{
			Index: dummyHandler,
			Show:  dummyHandler,
		})
	})

	routes := r.Routes()

	expected := map[string]string{
		"admin.users.index": "/admin/users",
		"admin.users.show":  "/admin/users/{user}",
	}

	for name, pattern := range expected {
		if routes[name] != pattern {
			t.Errorf("route %q: got %q, want %q", name, routes[name], pattern)
		}
	}
}

func TestURL(t *testing.T) {
	r := New()
	r.Get("/users", dummyHandler, "users.index")
	r.Get("/users/{id}", dummyHandler, "users.show")
	r.Get("/users/{id}/posts/{post_id}", dummyHandler, "users.posts.show")

	tests := []struct {
		name     string
		params   []string
		expected string
		wantErr  bool
	}{
		{
			name:     "users.index",
			params:   nil,
			expected: "/users",
		},
		{
			name:     "users.show",
			params:   []string{"id", "123"},
			expected: "/users/123",
		},
		{
			name:     "users.posts.show",
			params:   []string{"id", "123", "post_id", "456"},
			expected: "/users/123/posts/456",
		},
		{
			name:    "nonexistent",
			params:  nil,
			wantErr: true,
		},
		{
			name:    "users.show",
			params:  []string{"id"}, // odd number of params
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := r.URL(tt.name, tt.params...)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if url != tt.expected {
				t.Errorf("got %q, want %q", url, tt.expected)
			}
		})
	}
}

func TestURLWithRegex(t *testing.T) {
	r := New()
	r.Get("/buckets/{bucket}/{key:.*}", dummyHandler, "objects.show")

	url, err := r.URL("objects.show", "bucket", "my-bucket", "key", "path/to/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/buckets/my-bucket/path/to/file.txt"
	if url != expected {
		t.Errorf("got %q, want %q", url, expected)
	}
}

func TestMustURLPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected panic for unknown route")
		}
	}()

	r := New()
	r.MustURL("nonexistent")
}

func TestServeHTTP(t *testing.T) {
	r := New()
	r.Get("/hello", func(w http.ResponseWriter, req *http.Request) {
		w.Write([]byte("Hello, World!"))
	}, "hello")

	req := httptest.NewRequest("GET", "/hello", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if rec.Body.String() != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", rec.Body.String())
	}
}

func TestMiddleware(t *testing.T) {
	r := New()

	called := false
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			called = true
			next.ServeHTTP(w, req)
		})
	})

	r.Get("/test", dummyHandler, "test")

	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if !called {
		t.Error("middleware was not called")
	}
}

func TestAllHTTPMethods(t *testing.T) {
	r := New()

	r.Get("/resource", dummyHandler, "get")
	r.Post("/resource", dummyHandler, "post")
	r.Put("/resource", dummyHandler, "put")
	r.Patch("/resource", dummyHandler, "patch")
	r.Delete("/resource", dummyHandler, "delete")
	r.Head("/resource", dummyHandler, "head")
	r.Options("/resource", dummyHandler, "options")

	routes := r.Routes()
	if len(routes) != 7 {
		t.Errorf("expected 7 routes, got %d", len(routes))
	}

	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}
	for _, method := range methods {
		req := httptest.NewRequest(method, "/resource", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("%s /resource: expected 200, got %d", method, rec.Code)
		}
	}
}

func TestChiAccessor(t *testing.T) {
	r := New()
	chi := r.Chi()
	if chi == nil {
		t.Error("Chi() returned nil")
	}
}

func TestWith(t *testing.T) {
	r := New()

	called := false
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			called = true
			next.ServeHTTP(w, req)
		})
	}

	// Register a route without middleware
	r.Get("/public", dummyHandler, "public")

	// Register a route with middleware using With()
	r.With(authMiddleware).Get("/private", dummyHandler, "private")

	// Test public route (middleware should not be called)
	req := httptest.NewRequest("GET", "/public", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if called {
		t.Error("middleware was called for public route")
	}

	// Test private route (middleware should be called)
	req = httptest.NewRequest("GET", "/private", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !called {
		t.Error("middleware was not called for private route")
	}

	// Verify both routes are in the registry
	routes := r.Routes()
	if _, ok := routes["public"]; !ok {
		t.Error("missing public route in registry")
	}
	if _, ok := routes["private"]; !ok {
		t.Error("missing private route in registry")
	}
}

func TestURLParam(t *testing.T) {
	r := New()

	var capturedBucket, capturedKey string
	r.Get("/{bucket}/{key}", func(w http.ResponseWriter, req *http.Request) {
		capturedBucket = URLParam(req, "bucket")
		capturedKey = URLParam(req, "key")
		w.WriteHeader(http.StatusOK)
	}, "test")

	req := httptest.NewRequest("GET", "/my-bucket/my-key", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if capturedBucket != "my-bucket" {
		t.Errorf("expected bucket 'my-bucket', got %q", capturedBucket)
	}
	if capturedKey != "my-key" {
		t.Errorf("expected key 'my-key', got %q", capturedKey)
	}
}

func TestURLParams(t *testing.T) {
	r := New()

	var capturedParams map[string]string
	r.Get("/{bucket}/{key}", func(w http.ResponseWriter, req *http.Request) {
		capturedParams = URLParams(req)
		w.WriteHeader(http.StatusOK)
	}, "test")

	req := httptest.NewRequest("GET", "/my-bucket/my-key", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if capturedParams["bucket"] != "my-bucket" {
		t.Errorf("expected bucket 'my-bucket', got %q", capturedParams["bucket"])
	}
	if capturedParams["key"] != "my-key" {
		t.Errorf("expected key 'my-key', got %q", capturedParams["key"])
	}
}

func TestMiddlewareGroup(t *testing.T) {
	r := New()

	middlewareCalled := false
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			middlewareCalled = true
			next.ServeHTTP(w, req)
		})
	}

	// Route outside the group
	r.Get("/public", dummyHandler, "public")

	// Routes inside middleware group
	r.MiddlewareGroup(func(r *Router) {
		r.Use(middleware)
		r.Get("/admin", dummyHandler, "admin")
		r.Get("/settings", dummyHandler, "settings")
	})

	routes := r.Routes()

	// Verify routes are registered with correct paths (no prefix)
	if routes["admin"] != "/admin" {
		t.Errorf("expected /admin, got %s", routes["admin"])
	}
	if routes["settings"] != "/settings" {
		t.Errorf("expected /settings, got %s", routes["settings"])
	}
	if routes["public"] != "/public" {
		t.Errorf("expected /public, got %s", routes["public"])
	}

	// Test that middleware is applied to grouped routes
	middlewareCalled = false
	req := httptest.NewRequest("GET", "/admin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !middlewareCalled {
		t.Error("middleware was not called for /admin")
	}

	// Test that middleware is NOT applied to routes outside the group
	middlewareCalled = false
	req = httptest.NewRequest("GET", "/public", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if middlewareCalled {
		t.Error("middleware was called for /public (should not be)")
	}
}

func TestMiddlewareGroupWithPathPrefix(t *testing.T) {
	r := New()

	middlewareCalled := false
	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			middlewareCalled = true
			next.ServeHTTP(w, req)
		})
	}

	// Use MiddlewareGroup inside a Group with a path prefix
	r.Group("/api", func(r *Router) {
		r.MiddlewareGroup(func(r *Router) {
			r.Use(middleware)
			r.Get("/users", dummyHandler, "users.index")
		})
	})

	routes := r.Routes()

	// Verify route has the path prefix from Group
	if routes["users.index"] != "/api/users" {
		t.Errorf("expected /api/users, got %s", routes["users.index"])
	}

	// Test that middleware is applied
	req := httptest.NewRequest("GET", "/api/users", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !middlewareCalled {
		t.Error("middleware was not called for /api/users")
	}
}
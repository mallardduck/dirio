package ui

import "github.com/a-h/templ"

// BasePath is the URL prefix under which the admin console is mounted.
const BasePath = "/dirio/ui"

// AppVersion is the running server version, set by console.New() at startup.
var AppVersion = "dev"

// PageURL returns the full console URL for the given page path (e.g. "/buckets").
// Pass "/" for the dashboard root.
func PageURL(path string) templ.SafeURL {
	if path == "/" {
		return templ.SafeURL(BasePath + "/")
	}
	return templ.SafeURL(BasePath + path)
}

// StaticURL returns the full URL for a static asset by filename (e.g. "style.css").
func StaticURL(file string) templ.SafeURL {
	return templ.SafeURL(BasePath + "/static/" + file)
}

// LoginURL returns the URL for the console login page.
func LoginURL() templ.SafeURL { return templ.SafeURL(BasePath + "/login") }

// LogoutURL returns the URL for the console logout endpoint.
func LogoutURL() templ.SafeURL { return templ.SafeURL(BasePath + "/logout") }

// S3Router is the subset of the teapot-router API needed for URL generation.
// *teapot.Router satisfies this interface.
type S3Router interface {
	URL(name string, params ...string) (string, error)
}

// S3URLs wraps an S3Router to provide typed URL helpers for S3 API endpoints,
// using the named routes registered in internal/http/server/routes.go.
type S3URLs struct {
	r S3Router
}

// NewS3URLs creates an S3URLs helper backed by the given router.
func NewS3URLs(r S3Router) S3URLs {
	return S3URLs{r: r}
}

// ListBuckets returns the URL for the list-all-buckets endpoint ("index").
func (u S3URLs) ListBuckets() templ.SafeURL {
	path, _ := u.r.URL("index")
	return templ.SafeURL(path)
}

// Bucket returns the URL for the given bucket ("buckets.show").
func (u S3URLs) Bucket(bucket string) templ.SafeURL {
	path, _ := u.r.URL("buckets.show", "bucket", bucket)
	return templ.SafeURL(path)
}

// Object returns the URL for the given object ("objects.show").
func (u S3URLs) Object(bucket, key string) templ.SafeURL {
	path, _ := u.r.URL("objects.show", "bucket", bucket, "key", key)
	return templ.SafeURL(path)
}

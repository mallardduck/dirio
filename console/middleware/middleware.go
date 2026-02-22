package middleware

import (
	"net/http"

	consoleauth "github.com/mallardduck/dirio/console/auth"
	"github.com/mallardduck/dirio/console/ui"
)

// RequireAdminSession is middleware that redirects unauthenticated requests to the
// login page. It wraps a handler so only requests with a valid session cookie
// are forwarded.
func RequireAdminSession(sessions *consoleauth.Session) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := sessions.Validate(r); !ok {
				http.Redirect(w, r, string(ui.LoginURL()), http.StatusSeeOther)
				return
			}

			// Authorization passed - proceed to handler
			next.ServeHTTP(w, r)
		})
	}
}

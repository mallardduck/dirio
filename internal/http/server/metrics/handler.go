package metrics

import "net/http"

type RouteHandlers interface {
	HandlePrometheus() http.Handler
}

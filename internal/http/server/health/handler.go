package health

import "net/http"

type RouteHandlers interface {
	HandleHealth() http.Handler
	HandleLive() http.Handler
	HandleReady() http.Handler
}

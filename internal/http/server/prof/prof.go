package prof

import (
	"net/http"
	"net/http/pprof"
)

var _ RouteHandlers = (*Handler)(nil)

// Handler implements the /health, /health/ready, and /health/live endpoints.
type Handler struct {
}

func (h Handler) Index() http.Handler {
	return http.HandlerFunc(pprof.Index)
}

func (h Handler) Cmdline() http.Handler {
	return http.HandlerFunc(pprof.Cmdline)
}

func (h Handler) Profile() http.Handler {
	return http.HandlerFunc(pprof.Profile)
}

func (h Handler) Symbol() http.Handler {
	return http.HandlerFunc(pprof.Symbol)
}

func (h Handler) Trace() http.Handler {
	return http.HandlerFunc(pprof.Trace)
}

func (h Handler) ProfileDownload(profileType string) http.Handler {
	return pprof.Handler(profileType)
}

func New() Handler {
	return Handler{}
}

package prof

import (
	"net/http"
)

type RouteHandlers interface {
	Index() http.Handler
	Cmdline() http.Handler
	Profile() http.Handler
	Symbol() http.Handler
	Trace() http.Handler
	ProfileDownload(profileType string) http.Handler
}

package metrics

import (
	"github.com/mallardduck/teapot-router/pkg/teapot"
)

// RegisterRoutes mounts the Prometheus-format metrics endpoint at /.dirio/metrics.
// When the provider is nil (e.g. CLI route listing) a plain 200 stub is registered.
func RegisterRoutes(r *teapot.Router, h RouteHandlers) {
	r.GET("/.dirio/metrics", h.HandlePrometheus()).Name("metrics").Action("dirio:Metrics")
}

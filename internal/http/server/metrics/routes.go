package metrics

import (
	"net/http"

	"github.com/mallardduck/teapot-router/pkg/teapot"

	"github.com/mallardduck/dirio/internal/telemetry"
)

// RegisterRoutes mounts the Prometheus-format metrics endpoint at /.dirio/metrics.
// When the provider is nil (e.g. CLI route listing) a plain 200 stub is registered.
func RegisterRoutes(r *teapot.Router, provider *telemetry.Provider) {
	if provider == nil {
		r.Func().GET("/.dirio/metrics", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}).Name("metrics")
		return
	}

	h := provider.PrometheusHandler()
	r.Func().GET("/.dirio/metrics", h.ServeHTTP).Name("metrics").Action("dirio:Metrics")
}

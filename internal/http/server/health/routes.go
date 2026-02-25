package health

import (
	"net/http"

	"github.com/go-git/go-billy/v5"
	"github.com/mallardduck/teapot-router/pkg/teapot"
)

// RegisterRoutes registers all health check routes on r.
// When metadata or rootFS is nil (e.g. CLI route listing), plain 200 fallbacks are used.
func RegisterRoutes(r *teapot.Router, metadata Pinger, rootFS billy.Filesystem) {
	if metadata == nil || rootFS == nil {
		ok200 := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
		r.Func().GET("/.dirio/healthz", ok200).Name("health.legacy")
		r.Func().GET("/.dirio/health", ok200).Name("health")
		r.Func().GET("/.dirio/health/ready", ok200).Name("health.ready")
		r.Func().GET("/.dirio/health/live", ok200).Name("health.live")
		r.Func().GET("/minio/health/live", ok200).Name("minio.health.live")
		r.Func().GET("/minio/health/ready", ok200).Name("minio.health.ready")
		return
	}

	h := New(metadata, rootFS)
	r.Func().GET("/.dirio/healthz", h.HandleLive).Name("health.legacy").Action("dirio:Health")
	r.Func().GET("/.dirio/health", h.HandleHealth).Name("health").Action("dirio:Health")
	r.Func().GET("/.dirio/health/ready", h.HandleReady).Name("health.ready").Action("dirio:HealthReady")
	r.Func().GET("/.dirio/health/live", h.HandleLive).Name("health.live").Action("dirio:HealthLive")
	r.Func().GET("/minio/health/live", h.HandleLive).Name("minio.health.live").Action("dirio:HealthLive")
	r.Func().GET("/minio/health/ready", h.HandleReady).Name("minio.health.ready").Action("dirio:HealthReady")
}

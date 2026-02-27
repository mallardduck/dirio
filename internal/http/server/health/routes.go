package health

import (
	"github.com/mallardduck/teapot-router/pkg/teapot"
)

// RegisterRoutes registers all health check routes on r.
// When metadata or rootFS is nil (e.g. CLI route listing), plain 200 fallbacks are used.
func RegisterRoutes(r *teapot.Router, h RouteHandlers) {
	r.GET("/.dirio/healthz", h.HandleLive()).Name("health.legacy").Action("dirio:Health")
	r.GET("/.dirio/health", h.HandleHealth()).Name("health").Action("dirio:Health")
	r.GET("/.dirio/health/ready", h.HandleReady()).Name("health.ready").Action("dirio:HealthReady")
	r.GET("/.dirio/health/live", h.HandleLive()).Name("health.live").Action("dirio:HealthLive")
	r.GET("/minio/health/live", h.HandleLive()).Name("minio.health.live").Action("dirio:HealthLive")
	r.GET("/minio/health/ready", h.HandleReady()).Name("minio.health.ready").Action("dirio:HealthReady")
}

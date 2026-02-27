package prof

import "github.com/mallardduck/teapot-router/pkg/teapot"

func RegisterRoutes(r *teapot.Router, h RouteHandlers) {
	r.GET("/debug/pprof/", h.Index())
	r.GET("/debug/pprof/cmdline", h.Cmdline())
	r.GET("/debug/pprof/profile", h.Profile())
	r.GET("/debug/pprof/symbol", h.Symbol())
	r.POST("/debug/pprof/symbol", h.Symbol())
	r.GET("/debug/pprof/trace", h.Trace())
	r.GET("/debug/pprof/goroutine", h.ProfileDownload("goroutine"))
	r.GET("/debug/pprof/heap", h.ProfileDownload("heap"))
	r.GET("/debug/pprof/allocs", h.ProfileDownload("allocs"))
	r.GET("/debug/pprof/block", h.ProfileDownload("block"))
	r.GET("/debug/pprof/mutex", h.ProfileDownload("mutex"))
	r.GET("/debug/pprof/threadcreate", h.ProfileDownload("threadcreate"))
}

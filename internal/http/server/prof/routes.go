package prof

import "github.com/mallardduck/teapot-router/pkg/teapot"

func RegisterRoutes(r *teapot.Router, h RouteHandlers) {
	r.NamedGroup("/debug/pprof", "pprof", func(r *teapot.Router) {
		r.GET("/", h.Index()).Name("index")
		r.GET("/cmdline", h.Cmdline()).Name("cmdline")
		r.GET("/profile", h.Profile()).Name("profile")
		r.GET("/symbol", h.Symbol()).Name("symbol")
		r.POST("/symbol", h.Symbol()).Name("symbol")
		r.GET("/trace", h.Trace()).Name("trace")
		r.GET("/goroutine", h.ProfileDownload("goroutine")).Name("dl.goroutine")
		r.GET("/heap", h.ProfileDownload("heap")).Name("dl.heap")
		r.GET("/allocs", h.ProfileDownload("allocs")).Name("dl.allocs")
		r.GET("/block", h.ProfileDownload("block")).Name("dl.block")
		r.GET("/mutex", h.ProfileDownload("mutex")).Name("dl.mutex")
		r.GET("/threadcreate", h.ProfileDownload("threadcreate")).Name("dl.threadcreate")
	})
}

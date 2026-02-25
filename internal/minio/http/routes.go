package http

import (
	"github.com/mallardduck/teapot-router/pkg/teapot"

	httpresponse "github.com/mallardduck/dirio/internal/http/response"
)

// RegisterAdminRouter registers all MinIO Admin API v3 routes on r.
// When h is nil, routes are registered with nil handlers (for CLI route listing).
func RegisterAdminRouter(r *teapot.Router, h *Handler) {
	var uh userHandler
	var ph policyHandler
	var gh groupHandler
	var sah serviceAccountHandler

	if h != nil {
		uh = h.UserResourceHandler()
		ph = h.PolicyResourceHandler()
		gh = h.GroupResourceHandler()
		sah = h.ServiceAccountResourceHandler()
	}

	// User Management
	r.GET("/list-users", uh.ListHandler).Name("users.list")
	r.PUT("/add-user", uh.AddHandler).Name("users.add")
	r.DELETE("/remove-user", uh.RemoveHandler).Name("users.remove")
	r.GET("/user-info", uh.InfoHandler).Name("users.info")
	r.PUT("/set-user-status", uh.StatusHandler).Name("users.setstatus")

	// Policy Management
	r.GET("/list-canned-policies", ph.ListHandler).Name("policies.list")
	r.POST("/add-canned-policy", ph.AddHandler).Name("policies.add")
	r.PUT("/add-canned-policy", ph.AddHandler).Name("policies.add")
	r.POST("/remove-canned-policy", ph.RemoveHandler).Name("policies.remove")
	r.GET("/info-canned-policy", ph.InfoHandler).Name("policies.info")

	// Service Account Management
	r.GET("/list-service-accounts", sah.ListHandler).Name("serviceaccounts.list")
	r.POST("/add-service-account", sah.AddHandler).Name("serviceaccounts.add")
	r.POST("/delete-service-account", sah.DeleteHandler).Name("serviceaccounts.delete")
	r.GET("/info-service-account", sah.InfoHandler).Name("serviceaccounts.info")
	r.POST("/update-service-account", sah.UpdateHandler).Name("serviceaccounts.update")

	// Group Management
	r.POST("/update-group-members", gh.UpdateMembersHandler).Name("groups.updatemembers")
	r.GET("/group", gh.InfoHandler).Name("groups.info")
	r.GET("/groups", gh.ListHandler).Name("groups.list")
	r.POST("/set-group-status", gh.StatusHandler).Name("groups.setstatus")

	// Policy Attachments
	r.POST("/set-policy", ph.SetHandler).Name("policies.set") // deprecated: mc admin policy set
	r.PUT("/set-user-or-group-policy", ph.SetHandler).Name("policies.set-user-or-group")
	r.POST("/idp/builtin/policy/attach", ph.SetHandler).Name("policies.attach")
	r.POST("/idp/builtin/policy/detach", ph.DetachHandler).Name("policies.detach")
	r.GET("/policy-entities", ph.ListEntitiesHandler).Name("policies.entities")

	// Server Info & Health (not yet implemented)
	r.GET("/info", httpresponse.NotImplemented).Name("server.info")
	r.GET("/health", httpresponse.NotImplemented).Name("server.health")
}

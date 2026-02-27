package http

import (
	"github.com/mallardduck/teapot-router/pkg/teapot"
)

// RegisterRouter registers all MinIO Admin API v3 routes on r.
// When h is nil, routes are registered with nil handlers (for CLI route listing).
func RegisterRouter(r *teapot.Router, h RouteHandlers) {
	r.NamedGroup("/minio/admin/v3", "admin", func(r *teapot.Router) {
		r.Use(h.Middlewares()...)

		// User Management
		r.GET("/list-users", h.HandleListUsers()).Name("users.list")
		r.PUT("/add-user", h.HandleAddUser()).Name("users.add")
		r.DELETE("/remove-user", h.HandleRemoveUser()).Name("users.remove")
		r.GET("/user-info", h.HandleUserInfo()).Name("users.info")
		r.PUT("/set-user-status", h.HandleSetUserStatus()).Name("users.setstatus")

		// Policy Management
		r.GET("/list-canned-policies", h.HandleListCannedPolicies()).Name("policies.list")
		r.POST("/add-canned-policy", h.HandleAddCannedPolicy()).Name("policies.add")
		r.PUT("/add-canned-policy", h.HandleAddCannedPolicy()).Name("policies.add")
		r.POST("/remove-canned-policy", h.HandleRemoveCannedPolicy()).Name("policies.remove")
		r.GET("/info-canned-policy", h.HandleCannedPolicyInfo()).Name("policies.info")

		// Service Account Management
		r.GET("/list-service-accounts", h.HandleListServiceAccounts()).Name("serviceaccounts.list")
		r.POST("/add-service-account", h.HandleAddServiceAccount()).Name("serviceaccounts.add")
		r.POST("/delete-service-account", h.HandleDeleteServiceAccount()).Name("serviceaccounts.delete")
		r.GET("/info-service-account", h.HandleServiceAccountInfo()).Name("serviceaccounts.info")
		r.POST("/update-service-account", h.HandleUpdateServiceAccount()).Name("serviceaccounts.update")

		// Group Management
		r.POST("/update-group-members", h.HandleUpdateGroupMembers()).Name("groups.updatemembers")
		r.GET("/group", h.HandleGroupInfo()).Name("groups.info")
		r.GET("/groups", h.HandleListGroups()).Name("groups.list")
		r.POST("/set-group-status", h.HandleSetGroupStatus()).Name("groups.setstatus")

		// Policy Attachments
		r.POST("/set-policy", h.HandleSetPolicy()).Name("policies.set") // deprecated: mc admin policy set
		r.PUT("/set-user-or-group-policy", h.HandleSetPolicy()).Name("policies.set-user-or-group")
		r.POST("/idp/builtin/policy/attach", h.HandleSetPolicy()).Name("policies.attach")
		r.POST("/idp/builtin/policy/detach", h.HandlerDetachPolicy()).Name("policies.detach")
		r.GET("/policy-entities", h.HandlerListPolicyEntities()).Name("policies.entities")

		// Server Info & Health (not yet implemented)
		r.GET("/info", h.HandleInfo()).Name("server.info")
		r.GET("/health", h.HandleHealth()).Name("server.health")
	})
}

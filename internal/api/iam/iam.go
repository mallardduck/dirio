package iam

import (
	"net/http"

	"github.com/mallardduck/dirio/internal/logging"
	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"
	"github.com/mallardduck/dirio/internal/service"
	"github.com/mallardduck/dirio/internal/service/policy"
	"github.com/mallardduck/dirio/internal/service/user"
)

// Handler handles IAM API requests
type Handler struct {
	user   *user.Service
	policy *policy.Service

	// HTTP service wrappers - created once and reused
	userHTTP   *userHTTPService
	policyHTTP *policyHTTPService
}

func (h *Handler) UserHTTPService() *userHTTPService {
	return h.userHTTP
}

func (h *Handler) PolicyHTTPService() *policyHTTPService {
	return h.policyHTTP
}

func New(serviceFactory *service.ServicesFactory) *Handler {
	userService := serviceFactory.User()
	policyService := serviceFactory.Policy()

	return &Handler{
		user:   userService,
		policy: policyService,
		userHTTP: &userHTTPService{
			users:    userService,
			policies: policyService,
			log:      logging.Component("user-http-service"),
		},
		policyHTTP: &policyHTTPService{
			users:    userService,
			policies: policyService,
			log:      logging.Component("policy-http-service"),
		},
	}
}

/**
r.Post("/add-user", notImplemented, "users.add")
r.Post("/remove-user", notImplemented, "users.remove")
r.Get("/list-users", notImplemented, "users.list")
r.Get("/info-user", notImplemented, "users.info")
r.Post("/set-user-status", notImplemented, "users.setstatus")
*/

type userHandler struct {
	ListHandler   http.HandlerFunc
	AddHandler    http.HandlerFunc
	RemoveHandler http.HandlerFunc
	InfoHandler   http.HandlerFunc
	StatusHandler http.HandlerFunc
}

func (h *Handler) UserResourceHandler() userHandler {
	return userHandler{
		ListHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.ListUsers"
			}
			h.UserHTTPService().ListUsers(w, r)
		},
		AddHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.AddUser"
			}
			h.UserHTTPService().CreateUser(w, r)
		},
		RemoveHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.RemoveUser"
			}
			h.UserHTTPService().RemoveUser(w, r)
		},
		InfoHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.UserInfo"
			}
			h.UserHTTPService().InfoUser(w, r)
		},
		StatusHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.SetUserStatus"
			}
			h.UserHTTPService().SetUserStatus(w, r)
		},
	}
}

type policyHandler struct {
	ListHandler         http.HandlerFunc
	AddHandler          http.HandlerFunc
	RemoveHandler       http.HandlerFunc
	InfoHandler         http.HandlerFunc
	SetHandler          http.HandlerFunc
	ListEntitiesHandler http.HandlerFunc
}

func (h *Handler) PolicyResourceHandler() policyHandler {
	return policyHandler{
		ListHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.ListCannedPolicies"
			}
			h.PolicyHTTPService().ListCannedPolicies(w, r)
		},
		AddHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.AddCannedPolicy"
			}
			h.PolicyHTTPService().AddCannedPolicy(w, r)
		},
		RemoveHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.RemoveCannedPolicy"
			}
			h.PolicyHTTPService().RemoveCannedPolicy(w, r)
		},
		InfoHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.InfoCannedPolicy"
			}
			h.PolicyHTTPService().InfoCannedPolicy(w, r)
		},
		SetHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.SetPolicy"
			}
			h.PolicyHTTPService().SetPolicy(w, r)
		},
		ListEntitiesHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.PolicyEntitiesList"
			}
			h.PolicyHTTPService().PolicyEntitiesList(w, r)
		},
	}
}

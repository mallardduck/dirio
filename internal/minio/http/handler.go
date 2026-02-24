package http

import (
	nethttp "net/http"

	"github.com/mallardduck/dirio/internal/logging"
	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"
	"github.com/mallardduck/dirio/internal/service"
	"github.com/mallardduck/dirio/internal/service/group"
	"github.com/mallardduck/dirio/internal/service/policy"
	"github.com/mallardduck/dirio/internal/service/serviceaccount"
	"github.com/mallardduck/dirio/internal/service/user"
)

// Handler handles MinIO Admin API v3 IAM requests
type Handler struct {
	user           *user.Service
	policy         *policy.Service
	groupSvc       *group.Service
	serviceAcctSvc *serviceaccount.Service

	// HTTP service wrappers - created once and reused
	userHTTP           *userHTTPService
	policyHTTP         *policyHTTPService
	groupHTTP          *groupHTTPService
	serviceAccountHTTP *serviceAccountHTTPService
}

func (h *Handler) UserHTTPService() *userHTTPService {
	return h.userHTTP
}

func (h *Handler) PolicyHTTPService() *policyHTTPService {
	return h.policyHTTP
}

func (h *Handler) GroupHTTPService() *groupHTTPService {
	return h.groupHTTP
}

func (h *Handler) ServiceAccountHTTPService() *serviceAccountHTTPService {
	return h.serviceAccountHTTP
}

func New(serviceFactory *service.ServicesFactory) *Handler {
	userService := serviceFactory.User()
	policyService := serviceFactory.Policy()
	groupService := serviceFactory.Group()
	saService := serviceFactory.ServiceAccount()

	return &Handler{
		user:           userService,
		policy:         policyService,
		groupSvc:       groupService,
		serviceAcctSvc: saService,
		userHTTP: &userHTTPService{
			users:    userService,
			policies: policyService,
			log:      logging.Component("user-http-service"),
		},
		policyHTTP: &policyHTTPService{
			users:    userService,
			groups:   groupService,
			policies: policyService,
			log:      logging.Component("policy-http-service"),
		},
		groupHTTP: &groupHTTPService{
			groups: groupService,
			log:    logging.Component("group-http-service"),
		},
		serviceAccountHTTP: &serviceAccountHTTPService{
			serviceAccounts: saService,
			log:             logging.Component("service-account-http-service"),
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
	ListHandler   nethttp.HandlerFunc
	AddHandler    nethttp.HandlerFunc
	RemoveHandler nethttp.HandlerFunc
	InfoHandler   nethttp.HandlerFunc
	StatusHandler nethttp.HandlerFunc
}

func (h *Handler) UserResourceHandler() userHandler {
	return userHandler{
		ListHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.ListUsers"
			}
			h.UserHTTPService().ListUsers(w, r)
		},
		AddHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.AddUser"
			}
			h.UserHTTPService().CreateUser(w, r)
		},
		RemoveHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.RemoveUser"
			}
			h.UserHTTPService().RemoveUser(w, r)
		},
		InfoHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.UserInfo"
			}
			h.UserHTTPService().InfoUser(w, r)
		},
		StatusHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.SetUserStatus"
			}
			h.UserHTTPService().SetUserStatus(w, r)
		},
	}
}

type groupHandler struct {
	ListHandler          nethttp.HandlerFunc
	InfoHandler          nethttp.HandlerFunc
	UpdateMembersHandler nethttp.HandlerFunc
	StatusHandler        nethttp.HandlerFunc
}

func (h *Handler) GroupResourceHandler() groupHandler {
	return groupHandler{
		ListHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.ListGroups"
			}
			h.GroupHTTPService().ListGroups(w, r)
		},
		InfoHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.GetGroupInfo"
			}
			h.GroupHTTPService().GetGroupInfo(w, r)
		},
		UpdateMembersHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.UpdateGroupMembers"
			}
			h.GroupHTTPService().UpdateGroupMembers(w, r)
		},
		StatusHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.SetGroupStatus"
			}
			h.GroupHTTPService().SetGroupStatus(w, r)
		},
	}
}

type serviceAccountHandler struct {
	ListHandler   nethttp.HandlerFunc
	AddHandler    nethttp.HandlerFunc
	DeleteHandler nethttp.HandlerFunc
	InfoHandler   nethttp.HandlerFunc
	UpdateHandler nethttp.HandlerFunc
}

func (h *Handler) ServiceAccountResourceHandler() serviceAccountHandler {
	return serviceAccountHandler{
		ListHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.ListServiceAccounts"
			}
			h.ServiceAccountHTTPService().ListServiceAccounts(w, r)
		},
		AddHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.AddServiceAccount"
			}
			h.ServiceAccountHTTPService().AddServiceAccount(w, r)
		},
		DeleteHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.DeleteServiceAccount"
			}
			h.ServiceAccountHTTPService().DeleteServiceAccount(w, r)
		},
		InfoHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.InfoServiceAccount"
			}
			h.ServiceAccountHTTPService().InfoServiceAccount(w, r)
		},
		UpdateHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.UpdateServiceAccount"
			}
			h.ServiceAccountHTTPService().UpdateServiceAccount(w, r)
		},
	}
}

type policyHandler struct {
	ListHandler         nethttp.HandlerFunc
	AddHandler          nethttp.HandlerFunc
	RemoveHandler       nethttp.HandlerFunc
	InfoHandler         nethttp.HandlerFunc
	SetHandler          nethttp.HandlerFunc
	DetachHandler       nethttp.HandlerFunc
	ListEntitiesHandler nethttp.HandlerFunc
}

func (h *Handler) PolicyResourceHandler() policyHandler {
	return policyHandler{
		ListHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.ListCannedPolicies"
			}
			h.PolicyHTTPService().ListCannedPolicies(w, r)
		},
		AddHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.AddCannedPolicy"
			}
			h.PolicyHTTPService().AddCannedPolicy(w, r)
		},
		RemoveHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.RemoveCannedPolicy"
			}
			h.PolicyHTTPService().RemoveCannedPolicy(w, r)
		},
		InfoHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.InfoCannedPolicy"
			}
			h.PolicyHTTPService().InfoCannedPolicy(w, r)
		},
		SetHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.SetPolicy"
			}
			h.PolicyHTTPService().SetPolicy(w, r)
		},
		DetachHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.DetachPolicy"
			}
			h.PolicyHTTPService().DetachPolicy(w, r)
		},
		ListEntitiesHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			ctx := r.Context()
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "minio.v3.PolicyEntitiesList"
			}
			h.PolicyHTTPService().PolicyEntitiesList(w, r)
		},
	}
}

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

func requestWithAction(r *nethttp.Request, actionName string) *nethttp.Request {
	ctx := r.Context()
	if data, ok := loggingHttp.GetLogData(ctx); ok {
		data.Action = actionName
	}
	return r.WithContext(ctx)
}

func (h *Handler) UserResourceHandler() userHandler {
	return userHandler{
		ListHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.UserHTTPService().ListUsers(w, requestWithAction(r, "minio.v3.ListUsers"))
		},
		AddHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.UserHTTPService().CreateUser(w, requestWithAction(r, "minio.v3.AddUser"))
		},
		RemoveHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.UserHTTPService().RemoveUser(w, requestWithAction(r, "minio.v3.RemoveUser"))
		},
		InfoHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.UserHTTPService().InfoUser(w, requestWithAction(r, "minio.v3.UserInfo"))
		},
		StatusHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.UserHTTPService().SetUserStatus(w, requestWithAction(r, "minio.v3.SetUserStatus"))
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
			h.GroupHTTPService().ListGroups(w, requestWithAction(r, "minio.v3.ListGroups"))
		},
		InfoHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.GroupHTTPService().GetGroupInfo(w, requestWithAction(r, "minio.v3.GetGroupInfo"))
		},
		UpdateMembersHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.GroupHTTPService().UpdateGroupMembers(w, requestWithAction(r, "minio.v3.UpdateGroupMembers"))
		},
		StatusHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.GroupHTTPService().SetGroupStatus(w, requestWithAction(r, "minio.v3.SetGroupStatus"))
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
			h.ServiceAccountHTTPService().ListServiceAccounts(w, requestWithAction(r, "minio.v3.ListServiceAccounts"))
		},
		AddHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.ServiceAccountHTTPService().AddServiceAccount(w, requestWithAction(r, "minio.v3.AddServiceAccount"))
		},
		DeleteHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.ServiceAccountHTTPService().DeleteServiceAccount(w, requestWithAction(r, "minio.v3.DeleteServiceAccount"))
		},
		InfoHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.ServiceAccountHTTPService().InfoServiceAccount(w, requestWithAction(r, "minio.v3.InfoServiceAccount"))
		},
		UpdateHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.ServiceAccountHTTPService().UpdateServiceAccount(w, requestWithAction(r, "minio.v3.UpdateServiceAccount"))
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
			h.PolicyHTTPService().ListCannedPolicies(w, requestWithAction(r, "minio.v3.ListCannedPolicies"))
		},
		AddHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.PolicyHTTPService().AddCannedPolicy(w, requestWithAction(r, "minio.v3.AddCannedPolicy"))
		},
		RemoveHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.PolicyHTTPService().RemoveCannedPolicy(w, requestWithAction(r, "minio.v3.RemoveCannedPolicy"))
		},
		InfoHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.PolicyHTTPService().InfoCannedPolicy(w, requestWithAction(r, "minio.v3.InfoCannedPolicy"))
		},
		SetHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.PolicyHTTPService().SetPolicy(w, requestWithAction(r, "minio.v3.SetPolicy"))
		},
		DetachHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.PolicyHTTPService().DetachPolicy(w, requestWithAction(r, "minio.v3.DetachPolicy"))
		},
		ListEntitiesHandler: func(w nethttp.ResponseWriter, r *nethttp.Request) {
			h.PolicyHTTPService().PolicyEntitiesList(w, requestWithAction(r, "minio.v3.PolicyEntitiesList"))
		},
	}
}

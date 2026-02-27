package http

import (
	"net/http"

	"github.com/mallardduck/teapot-router/pkg/teapot"

	loggingHttp "github.com/mallardduck/dirio/internal/logging/http"

	"github.com/mallardduck/dirio/internal/http/auth"

	"github.com/mallardduck/dirio/internal/logging"
	"github.com/mallardduck/dirio/internal/service"
	"github.com/mallardduck/dirio/internal/service/group"
	"github.com/mallardduck/dirio/internal/service/policy"
	"github.com/mallardduck/dirio/internal/service/serviceaccount"
	"github.com/mallardduck/dirio/internal/service/user"
)

// Handler handles MinIO Admin API v3 IAM requests
type Handler struct {
	middlewares    []func(http.Handler) http.Handler
	user           *user.Service
	policy         *policy.Service
	groupSvc       *group.Service
	serviceAcctSvc *serviceaccount.Service

	// HTTP service wrappers - created once and reused
	userHTTP           *UserHTTPService
	policyHTTP         *PolicyHTTPService
	groupHTTP          *GroupHTTPService
	serviceAccountHTTP *ServiceAccountHTTPService
}

func New(auth *auth.Authenticator, serviceFactory *service.ServicesFactory) *Handler {
	userService := serviceFactory.User()
	policyService := serviceFactory.Policy()
	groupService := serviceFactory.Group()
	saService := serviceFactory.ServiceAccount()

	return &Handler{
		middlewares:    []func(http.Handler) http.Handler{auth.AuthMiddleware},
		user:           userService,
		policy:         policyService,
		groupSvc:       groupService,
		serviceAcctSvc: saService,
		userHTTP: &UserHTTPService{
			users:    userService,
			policies: policyService,
			log:      logging.Component("user-http-service"),
		},
		policyHTTP: &PolicyHTTPService{
			users:    userService,
			groups:   groupService,
			policies: policyService,
			log:      logging.Component("policy-http-service"),
		},
		groupHTTP: &GroupHTTPService{
			groups: groupService,
			log:    logging.Component("group-http-service"),
		},
		serviceAccountHTTP: &ServiceAccountHTTPService{
			serviceAccounts: saService,
			log:             logging.Component("service-account-http-service"),
		},
	}
}

func (h *Handler) Middlewares() []func(http.Handler) http.Handler {
	return h.middlewares
}

var (
	_ CannedPolicyHandlers = (*Handler)(nil)
	_ RouteHandlers        = (*Handler)(nil)
)

func requestWithAction(r *http.Request, actionName string) *http.Request {
	ctx := r.Context()
	if data, ok := loggingHttp.GetLogData(ctx); ok {
		data.Action = actionName
	}
	return r.WithContext(ctx)
}

func (h *Handler) HandleListCannedPolicies() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.policyHTTP.ListCannedPolicies(w, requestWithAction(r, "minio.v3.ListCannedPolicies"))
	})
}

func (h *Handler) HandleAddCannedPolicy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.policyHTTP.AddCannedPolicy(w, requestWithAction(r, "minio.v3.AddCannedPolicy"))
	})
}

func (h *Handler) HandleRemoveCannedPolicy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.policyHTTP.RemoveCannedPolicy(w, requestWithAction(r, "minio.v3.RemoveCannedPolicy"))
	})
}

func (h *Handler) HandleCannedPolicyInfo() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.policyHTTP.InfoCannedPolicy(w, requestWithAction(r, "minio.v3.InfoCannedPolicy"))
	})
}

var _ GroupHandlers = (*Handler)(nil)

func (h *Handler) HandleUpdateGroupMembers() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.groupHTTP.UpdateGroupMembers(w, requestWithAction(r, "minio.v3.UpdateGroupMembers"))
	})
}

func (h *Handler) HandleListGroups() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.groupHTTP.ListGroups(w, requestWithAction(r, "minio.v3.ListGroups"))
	})
}

func (h *Handler) HandleGroupInfo() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.groupHTTP.GetGroupInfo(w, requestWithAction(r, "minio.v3.GetGroupInfo"))
	})
}

func (h *Handler) HandleSetGroupStatus() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.groupHTTP.SetGroupStatus(w, requestWithAction(r, "minio.v3.SetGroupStatus"))
	})
}

var _ HealthHandlers = (*Handler)(nil)

func (h *Handler) HandleInfo() http.Handler {
	return teapot.NoopHandler
}

func (h *Handler) HandleHealth() http.Handler {
	return teapot.NoopHandler
}

var _ PolicyHandlers = (*Handler)(nil)

func (h *Handler) HandlerListPolicyEntities() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.policyHTTP.PolicyEntitiesList(w, requestWithAction(r, "minio.v3.PolicyEntitiesList"))
	})
}

func (h *Handler) HandleSetPolicy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.policyHTTP.SetPolicy(w, requestWithAction(r, "minio.v3.SetPolicy"))
	})
}

func (h *Handler) HandlerDetachPolicy() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.policyHTTP.DetachPolicy(w, requestWithAction(r, "minio.v3.DetachPolicy"))
	})
}

var _ ServiceAccountHandlers = (*Handler)(nil)

func (h *Handler) HandleListServiceAccounts() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.serviceAccountHTTP.ListServiceAccounts(w, requestWithAction(r, "minio.v3.ListServiceAccounts"))
	})
}

func (h *Handler) HandleAddServiceAccount() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.serviceAccountHTTP.AddServiceAccount(w, requestWithAction(r, "minio.v3.AddServiceAccount"))
	})
}

func (h *Handler) HandleDeleteServiceAccount() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.serviceAccountHTTP.DeleteServiceAccount(w, requestWithAction(r, "minio.v3.DeleteServiceAccount"))
	})
}

func (h *Handler) HandleServiceAccountInfo() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.serviceAccountHTTP.InfoServiceAccount(w, requestWithAction(r, "minio.v3.InfoServiceAccount"))
	})
}

func (h *Handler) HandleUpdateServiceAccount() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.serviceAccountHTTP.UpdateServiceAccount(w, requestWithAction(r, "minio.v3.UpdateServiceAccount"))
	})
}

var _ UserHandlers = (*Handler)(nil)

func (h *Handler) HandleListUsers() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.userHTTP.ListUsers(w, requestWithAction(r, "minio.v3.ListUsers"))
	})
}

func (h *Handler) HandleAddUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.userHTTP.CreateUser(w, requestWithAction(r, "minio.v3.AddUser"))
	})
}

func (h *Handler) HandleRemoveUser() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.userHTTP.RemoveUser(w, requestWithAction(r, "minio.v3.RemoveUser"))
	})
}

func (h *Handler) HandleUserInfo() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.userHTTP.InfoUser(w, requestWithAction(r, "minio.v3.UserInfo"))
	})
}

func (h *Handler) HandleSetUserStatus() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.userHTTP.SetUserStatus(w, requestWithAction(r, "minio.v3.SetUserStatus"))
	})
}

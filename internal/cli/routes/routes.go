package routes

import (
	"net/http"

	"github.com/mallardduck/teapot-router/pkg/teapot"

	"github.com/mallardduck/dirio/internal/http/server/health"
	"github.com/mallardduck/dirio/internal/http/server/metrics"
	"github.com/mallardduck/dirio/internal/http/server/prof"
	minioHTTP "github.com/mallardduck/dirio/internal/minio/http"
)

var _ health.RouteHandlers = (*StubHandler)(nil)
var _ metrics.RouteHandlers = (*StubHandler)(nil)
var _ minioHTTP.RouteHandlers = (*StubHandler)(nil)
var _ prof.RouteHandlers = (*StubHandler)(nil)

type StubHandler struct {
}

func (s StubHandler) Index() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) Cmdline() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) Profile() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) Symbol() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) Trace() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) ProfileDownload(profileType string) http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) Middlewares() []func(http.Handler) http.Handler {
	return []func(http.Handler) http.Handler{}
}

func (s StubHandler) HandleListCannedPolicies() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleAddCannedPolicy() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleRemoveCannedPolicy() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleCannedPolicyInfo() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleUpdateGroupMembers() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleListGroups() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleGroupInfo() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleSetGroupStatus() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleInfo() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandlerListPolicyEntities() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleSetPolicy() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandlerDetachPolicy() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleListServiceAccounts() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleAddServiceAccount() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleDeleteServiceAccount() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleServiceAccountInfo() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleUpdateServiceAccount() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleListUsers() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleAddUser() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleRemoveUser() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleUserInfo() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleSetUserStatus() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandlePrometheus() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleHealth() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleLive() http.Handler {
	return teapot.NoopHandler
}

func (s StubHandler) HandleReady() http.Handler {
	return teapot.NoopHandler
}

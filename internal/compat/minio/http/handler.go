package http

import "net/http"

type CannedPolicyHandlers interface {
	HandleListCannedPolicies() http.Handler
	HandleAddCannedPolicy() http.Handler
	HandleRemoveCannedPolicy() http.Handler
	HandleCannedPolicyInfo() http.Handler
}
type GroupHandlers interface {
	HandleUpdateGroupMembers() http.Handler
	HandleListGroups() http.Handler
	HandleGroupInfo() http.Handler
	HandleSetGroupStatus() http.Handler
}
type HealthHandlers interface {
	HandleInfo() http.Handler
	HandleHealth() http.Handler
}
type PolicyHandlers interface {
	HandlerListPolicyEntities() http.Handler
	HandleSetPolicy() http.Handler
	HandlerDetachPolicy() http.Handler
}
type ServiceAccountHandlers interface {
	HandleListServiceAccounts() http.Handler
	HandleAddServiceAccount() http.Handler
	HandleDeleteServiceAccount() http.Handler
	HandleServiceAccountInfo() http.Handler
	HandleUpdateServiceAccount() http.Handler
}
type UserHandlers interface {
	HandleListUsers() http.Handler
	HandleAddUser() http.Handler
	HandleRemoveUser() http.Handler
	HandleUserInfo() http.Handler
	HandleSetUserStatus() http.Handler
}

type RouteHandlers interface {
	CannedPolicyHandlers
	GroupHandlers
	HealthHandlers
	PolicyHandlers
	ServiceAccountHandlers
	UserHandlers
}

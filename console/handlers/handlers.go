// Package handlers HX-Request header; handlers return only the relevant fragment in that case.
// This package imports only consoleapi/, console/ui/, and the standard library
// — never internal/.
package handlers

import (
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/mallardduck/teapot-router/pkg/teapot"

	"github.com/mallardduck/dirio/console/components/toast"

	"github.com/mallardduck/dirio/console/auth"
	"github.com/mallardduck/dirio/console/ui"
	"github.com/mallardduck/dirio/consoleapi"
)

// Handler holds the console API reference used by all handler methods.
type Handler struct {
	api       consoleapi.API
	s3Router  ui.S3Router
	adminAuth auth.AdminAuth
	sessions  *auth.Session
}

// New creates a Handler backed by the given API, S3 router, admin authenticator,
// and session manager.
func New(api consoleapi.API, s3Router ui.S3Router, adminAuth auth.AdminAuth, sessions *auth.Session) *Handler {
	return &Handler{
		api:       api,
		s3Router:  s3Router,
		adminAuth: adminAuth,
		sessions:  sessions,
	}
}

// LoginPage handles GET /login — renders the login form.
func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.sessions.Validate(r); ok {
		http.Redirect(w, r, string(ui.PageURL("/")), http.StatusSeeOther)
		return
	}
	render(w, r, ui.LoginPage(""))
}

// LoginSubmit handles POST /login — validates credentials and creates a session.
func (h *Handler) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	accessKey := r.FormValue("access_key")
	secretKey := r.FormValue("secret_key")

	if !h.adminAuth.AuthenticateAdmin(r.Context(), accessKey, secretKey) {
		render(w, r, ui.LoginPage("Invalid credentials or insufficient permissions."))
		return
	}

	if err := h.sessions.Create(w, accessKey); err != nil {
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/")), http.StatusSeeOther)
}

// Logout handles POST /logout — destroys the session and redirects to login.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	h.sessions.Clear(w)
	http.Redirect(w, r, string(ui.LoginURL()), http.StatusSeeOther)
}

// render writes a templ component as an HTML response.
func render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// isHTMX reports whether the request was issued by HTMX (partial swap).
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// Toasts handles GET /toasts — returns a list of flash messages as toast components.
func (h *Handler) Toasts(w http.ResponseWriter, r *http.Request) {
	flash, ok := h.sessions.GetFlash(w, r)
	if !ok {
		return
	}
	render(w, r, toast.Toast(toast.Props{
		Description: flash.Message,
		Variant:     toast.Variant(flash.Type),
	}))
}

func (h *Handler) triggerToast(w http.ResponseWriter, message, msgType string) {
	h.sessions.SetFlash(w, message, msgType)
	w.Header().Add("HX-Trigger", "toast")
}

// Dashboard handles GET / — renders the admin dashboard.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	users, _ := h.api.ListUsers(r.Context())
	buckets, _ := h.api.ListBuckets(r.Context())
	policies, _ := h.api.ListPolicies(r.Context())
	groups, _ := h.api.ListGroups(r.Context())

	data := ui.DashboardData{
		UserCount:   len(users),
		BucketCount: len(buckets),
		PolicyCount: len(policies),
		GroupCount:  len(groups),
	}
	render(w, r, ui.DashboardPage(data))
}

// --- Users -------------------------------------------------------------------

func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	users, err := h.api.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := ui.UsersPageData{
		Users: users,
	}
	if isHTMX(r) {
		render(w, r, ui.UsersTable(users))
		return
	}
	render(w, r, ui.UsersPage(data))
}

func (h *Handler) UserCreate(w http.ResponseWriter, r *http.Request) {
	accessKey := r.FormValue("accessKey")
	secretKey := r.FormValue("secretKey")
	_, err := h.api.CreateUser(r.Context(), consoleapi.CreateUserRequest{
		AccessKey: accessKey,
		SecretKey: secretKey,
	})
	users, _ := h.api.ListUsers(r.Context())
	data := ui.UsersPageData{
		Users: users,
	}
	if err != nil {
		data.ErrorMsg = err.Error()
		h.triggerToast(w, "Failed to create user: "+err.Error(), "error")
	} else {
		h.triggerToast(w, "User created successfully", "success")
	}
	render(w, r, ui.UsersPage(data))
}

func (h *Handler) UserDelete(w http.ResponseWriter, r *http.Request) {
	uuid := teapot.URLParam(r, "uuid")
	err := h.api.DeleteUser(r.Context(), uuid)
	if err != nil {
		h.triggerToast(w, "Failed to delete user: "+err.Error(), "error")
	} else {
		h.triggerToast(w, "User deleted successfully", "success")
	}
	users, _ := h.api.ListUsers(r.Context())
	render(w, r, ui.UsersTable(users))
}

func (h *Handler) UserSetStatus(w http.ResponseWriter, r *http.Request) {
	uuid := teapot.URLParam(r, "uuid")
	u, err := h.api.GetUser(r.Context(), uuid)
	if err != nil {
		h.triggerToast(w, "User not found", "error")
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	newStatus := u.Status != "on"
	err = h.api.SetUserStatus(r.Context(), uuid, newStatus)
	if err != nil {
		h.triggerToast(w, "Failed to update user status: "+err.Error(), "error")
	} else {
		msg := "User enabled"
		if !newStatus {
			msg = "User disabled"
		}
		h.triggerToast(w, msg, "success")
	}
	users, _ := h.api.ListUsers(r.Context())
	render(w, r, ui.UsersTable(users))
}

func (h *Handler) UserUpdateSecret(w http.ResponseWriter, r *http.Request) {
	uuid := teapot.URLParam(r, "uuid")
	secretKey := r.FormValue("secretKey")
	_, err := h.api.UpdateUser(r.Context(), uuid, consoleapi.UpdateUserRequest{
		SecretKey: &secretKey,
	})
	if err != nil {
		h.triggerToast(w, "Failed to rotate secret: "+err.Error(), "error")
	} else {
		h.triggerToast(w, "Secret rotated successfully", "success")
	}
	users, _ := h.api.ListUsers(r.Context())
	render(w, r, ui.UsersTable(users))
}

func (h *Handler) UserRevealSecret(w http.ResponseWriter, r *http.Request) {
	uuid := teapot.URLParam(r, "uuid")
	secret, err := h.api.GetUserSecret(r.Context(), uuid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(secret))
}

// --- Policies ----------------------------------------------------------------

func (h *Handler) Policies(w http.ResponseWriter, r *http.Request) {
	policies, err := h.api.ListPolicies(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isHTMX(r) {
		render(w, r, ui.PoliciesTable(policies))
		return
	}
	render(w, r, ui.PoliciesPage(policies))
}

// --- Buckets -----------------------------------------------------------------

func (h *Handler) Buckets(w http.ResponseWriter, r *http.Request) {
	buckets, err := h.api.ListBuckets(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isHTMX(r) {
		render(w, r, ui.BucketsTable(buckets))
		return
	}
	render(w, r, ui.BucketsPage(buckets))
}

func (h *Handler) BucketDetail(w http.ResponseWriter, r *http.Request) {
	bucket := teapot.URLParam(r, "bucket")
	b, err := h.api.GetBucket(r.Context(), bucket)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	policyJSON, _ := h.api.GetBucketPolicy(r.Context(), bucket)
	owner, _ := h.api.GetBucketOwner(r.Context(), bucket)
	data := ui.BucketDetailData{
		Bucket:     b,
		PolicyJSON: policyJSON,
		Owner:      owner,
	}
	render(w, r, ui.BucketDetailPage(data))
}

func (h *Handler) BucketPolicySet(w http.ResponseWriter, r *http.Request) {
	bucket := teapot.URLParam(r, "bucket")
	policyJSON := r.FormValue("policy")
	if err := h.api.SetBucketPolicy(r.Context(), bucket, policyJSON); err != nil {
		h.triggerToast(w, "Failed to save policy: "+err.Error(), "error")
	} else {
		h.triggerToast(w, "Policy saved", "success")
	}
	b, _ := h.api.GetBucket(r.Context(), bucket)
	if b == nil {
		http.Error(w, "Bucket not found", http.StatusNotFound)
		return
	}
	owner, _ := h.api.GetBucketOwner(r.Context(), bucket)
	render(w, r, ui.BucketDetailPage(ui.BucketDetailData{
		Bucket:     b,
		PolicyJSON: policyJSON,
		Owner:      owner,
	}))
}

func (h *Handler) BucketTransferOwnership(w http.ResponseWriter, r *http.Request) {
	bucket := teapot.URLParam(r, "bucket")
	accessKey := r.FormValue("access_key")
	if err := h.api.TransferBucketOwnership(r.Context(), bucket, accessKey); err != nil {
		h.triggerToast(w, "Failed to transfer ownership: "+err.Error(), "error")
	} else {
		h.triggerToast(w, "Ownership transferred to "+accessKey, "success")
	}
	b, _ := h.api.GetBucket(r.Context(), bucket)
	policyJSON, _ := h.api.GetBucketPolicy(r.Context(), bucket)
	owner, _ := h.api.GetBucketOwner(r.Context(), bucket)
	if b == nil {
		http.Error(w, "Bucket not found", http.StatusNotFound)
		return
	}
	render(w, r, ui.BucketDetailPage(ui.BucketDetailData{
		Bucket:     b,
		PolicyJSON: policyJSON,
		Owner:      owner,
	}))
}

// --- Groups ------------------------------------------------------------------

func (h *Handler) Groups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.api.ListGroups(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isHTMX(r) {
		render(w, r, ui.GroupsTable(groups))
		return
	}
	render(w, r, ui.GroupsPage(ui.GroupsPageData{Groups: groups}))
}

func (h *Handler) GroupCreate(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	g, err := h.api.CreateGroup(r.Context(), consoleapi.CreateGroupRequest{Name: name})
	if err != nil {
		groups, _ := h.api.ListGroups(r.Context())
		render(w, r, ui.GroupsPage(ui.GroupsPageData{Groups: groups, ErrorMsg: err.Error()}))
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/groups/"+g.Name)), http.StatusSeeOther)
}

func (h *Handler) GroupDetail(w http.ResponseWriter, r *http.Request) {
	name := teapot.URLParam(r, "group")
	g, err := h.api.GetGroup(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	allPolicies, _ := h.api.ListPolicies(r.Context())
	flash := r.URL.Query().Get("flash")
	errMsg := r.URL.Query().Get("error")
	render(w, r, ui.GroupDetailPage(ui.GroupDetailData{
		Group:       g,
		AllPolicies: allPolicies,
		Flash:       flash,
		ErrorMsg:    errMsg,
	}))
}

func (h *Handler) GroupDelete(w http.ResponseWriter, r *http.Request) {
	name := teapot.URLParam(r, "group")
	if err := h.api.DeleteGroup(r.Context(), name); err != nil {
		http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?error="+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/groups")), http.StatusSeeOther)
}

func (h *Handler) GroupAddMember(w http.ResponseWriter, r *http.Request) {
	name := teapot.URLParam(r, "group")
	userRawUID := r.FormValue("user_uid")
	if err := h.api.AddGroupMember(r.Context(), name, userRawUID); err != nil {
		http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?error="+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?flash=Member+added.", http.StatusSeeOther)
}

func (h *Handler) GroupRemoveMember(w http.ResponseWriter, r *http.Request) {
	name := teapot.URLParam(r, "group")
	accessKey := r.FormValue("access_key")
	if err := h.api.RemoveGroupMember(r.Context(), name, accessKey); err != nil {
		http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?error="+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?flash=Member+removed.", http.StatusSeeOther)
}

func (h *Handler) GroupAttachPolicy(w http.ResponseWriter, r *http.Request) {
	name := teapot.URLParam(r, "group")
	policy := r.FormValue("policy")
	if err := h.api.AttachGroupPolicy(r.Context(), name, policy); err != nil {
		http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?error="+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?flash=Policy+attached.", http.StatusSeeOther)
}

func (h *Handler) GroupDetachPolicy(w http.ResponseWriter, r *http.Request) {
	name := teapot.URLParam(r, "group")
	policy := r.FormValue("policy")
	if err := h.api.DetachGroupPolicy(r.Context(), name, policy); err != nil {
		http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?error="+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?flash=Policy+detached.", http.StatusSeeOther)
}

func (h *Handler) GroupSetStatus(w http.ResponseWriter, r *http.Request) {
	name := teapot.URLParam(r, "group")
	enabled := r.FormValue("enabled") == "true"
	if err := h.api.SetGroupStatus(r.Context(), name, enabled); err != nil {
		http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?error="+err.Error(), http.StatusSeeOther)
		return
	}
	flash := "Group+disabled."
	if enabled {
		flash = "Group+enabled."
	}
	http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?flash="+flash, http.StatusSeeOther)
}

// --- Service Accounts --------------------------------------------------------

func (h *Handler) ServiceAccounts(w http.ResponseWriter, r *http.Request) {
	sas, err := h.api.ListServiceAccounts(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	users, _ := h.api.ListUsers(r.Context())
	policies, _ := h.api.ListPolicies(r.Context())
	data := ui.ServiceAccountsPageData{
		ServiceAccounts: sas,
		Users:           users,
		Policies:        policies,
	}
	if isHTMX(r) {
		render(w, r, ui.ServiceAccountsTable(sas))
		return
	}
	render(w, r, ui.ServiceAccountsPage(data))
}

func (h *Handler) ServiceAccountCreate(w http.ResponseWriter, r *http.Request) {
	parentUserUUID := r.FormValue("parentUserUUID")
	policyMode := r.FormValue("policyMode")
	expiryStr := r.FormValue("expiry")
	var expiry *time.Time
	if expiryStr != "" {
		if t, err := time.Parse("2006-01-02", expiryStr); err == nil {
			expiry = &t
		}
	}

	req := consoleapi.CreateServiceAccountRequest{
		ParentUserUUID: parentUserUUID,
		PolicyMode:     policyMode,
		ExpiresAt:      expiry,
	}
	if policyMode == "override" {
		req.EmbeddedPolicyJSON = r.FormValue("embeddedPolicyJSON")
	}

	sa, err := h.api.CreateServiceAccount(r.Context(), req)
	sas, _ := h.api.ListServiceAccounts(r.Context())
	users, _ := h.api.ListUsers(r.Context())
	policies, _ := h.api.ListPolicies(r.Context())
	data := ui.ServiceAccountsPageData{
		ServiceAccounts: sas,
		Users:           users,
		Policies:        policies,
		NewSA:           sa,
	}
	if err != nil {
		data.ErrorMsg = err.Error()
	}
	render(w, r, ui.ServiceAccountsPage(data))
}

func (h *Handler) ServiceAccountDelete(w http.ResponseWriter, r *http.Request) {
	uuid := teapot.URLParam(r, "uuid")
	_ = h.api.DeleteServiceAccount(r.Context(), uuid)
	sas, _ := h.api.ListServiceAccounts(r.Context())
	render(w, r, ui.ServiceAccountsTable(sas))
}

func (h *Handler) ServiceAccountSetStatus(w http.ResponseWriter, r *http.Request) {
	uuid := teapot.URLParam(r, "uuid")
	sa, err := h.api.GetServiceAccount(r.Context(), uuid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	_ = h.api.SetServiceAccountStatus(r.Context(), uuid, sa.Status != "on")
	sas, _ := h.api.ListServiceAccounts(r.Context())
	render(w, r, ui.ServiceAccountsTable(sas))
}

func (h *Handler) ServiceAccountUpdateSecret(w http.ResponseWriter, r *http.Request) {
	uuid := teapot.URLParam(r, "uuid")
	secretKey := r.FormValue("secretKey")
	_, err := h.api.UpdateServiceAccount(r.Context(), uuid, consoleapi.UpdateServiceAccountRequest{
		SecretKey: &secretKey,
	})
	if err != nil {
		h.triggerToast(w, "Failed to rotate secret: "+err.Error(), "error")
	} else {
		h.triggerToast(w, "Secret rotated successfully", "success")
	}
	sas, _ := h.api.ListServiceAccounts(r.Context())
	render(w, r, ui.ServiceAccountsTable(sas))
}

func (h *Handler) ServiceAccountRevealSecret(w http.ResponseWriter, r *http.Request) {
	uuid := teapot.URLParam(r, "uuid")
	secret, err := h.api.GetServiceAccountSecret(r.Context(), uuid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(secret))
}

// --- Policy Simulator --------------------------------------------------------

func (h *Handler) Simulate(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		render(w, r, ui.SimulatePage(ui.SimulateData{Action: "s3:GetObject"}))
		return
	}
	d := ui.SimulateData{
		AccessKey: r.FormValue("access_key"),
		Bucket:    r.FormValue("bucket"),
		Action:    r.FormValue("action"),
		Key:       r.FormValue("key"),
	}
	switch r.FormValue("mode") {
	case "effective":
		ep, err := h.api.GetEffectivePermissions(r.Context(), d.AccessKey, d.Bucket)
		if err != nil {
			d.Error = err.Error()
		} else {
			d.Effective = ep
		}
	default:
		result, err := h.api.SimulateRequest(r.Context(), consoleapi.SimulateRequest{
			AccessKey: d.AccessKey,
			Bucket:    d.Bucket,
			Action:    d.Action,
			Key:       d.Key,
		})
		if err != nil {
			d.Error = err.Error()
		} else {
			d.Result = result
		}
	}
	render(w, r, ui.SimulatePage(d))
}

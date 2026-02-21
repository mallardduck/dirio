// Package handlers provides the server-side HTML handlers for the DirIO admin console.
// Handlers call consoleapi.API directly and render HTML via templ components — no
// JSON API, no client-side fetching. HTMX partial requests are detected via the
// HX-Request header; handlers return only the relevant fragment in that case.
// This package imports only consoleapi/, console/ui/, and the standard library
// — never internal/.
package handlers

import (
	"net/http"

	"github.com/a-h/templ"
	"github.com/mallardduck/dirio/console/auth"
	"github.com/mallardduck/dirio/console/ui"
	"github.com/mallardduck/dirio/consoleapi"
)

// Handler holds the console API reference used by all handler methods.
type Handler struct {
	api       consoleapi.API
	s3URLs    ui.S3URLs
	adminAuth auth.AdminAuth
	sessions  *auth.Session
}

// New creates a Handler backed by the given API, S3 router, admin authenticator,
// and session manager.
func New(api consoleapi.API, s3Router ui.S3Router, adminAuth auth.AdminAuth, sessions *auth.Session) *Handler {
	return &Handler{
		api:       api,
		s3URLs:    ui.NewS3URLs(s3Router),
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

// Logout handles POST /logout — clears the session and redirects to login.
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
// When true, handlers should return only the relevant fragment, not the full
// page layout.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// Dashboard handles GET /{$} — renders the admin dashboard.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	// Collect counts, ignoring individual errors so partial data still renders.
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

// Users handles GET /users — renders the user list page.
func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	users, err := h.api.ListUsers(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if isHTMX(r) {
		render(w, r, ui.UsersTable(users))
		return
	}
	render(w, r, ui.UsersPage(users))
}

// Policies handles GET /policies — renders the policy list page.
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

// Buckets handles GET /buckets — renders the bucket list page.
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

// BucketDetail handles GET /buckets/{bucket} — renders the bucket detail page.
func (h *Handler) BucketDetail(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	b, err := h.api.GetBucket(r.Context(), bucket)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	policyJSON, _ := h.api.GetBucketPolicy(r.Context(), bucket)
	flash := r.URL.Query().Get("flash")
	render(w, r, ui.BucketDetailPage(ui.BucketDetailData{
		Bucket:     b,
		PolicyJSON: policyJSON,
		Flash:      flash,
	}))
}

// BucketPolicySet handles POST /buckets/{bucket}/policy — saves or clears the bucket policy.
func (h *Handler) BucketPolicySet(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	policyJSON := r.FormValue("policy")
	if err := h.api.SetBucketPolicy(r.Context(), bucket, policyJSON); err != nil {
		b, _ := h.api.GetBucket(r.Context(), bucket)
		if b == nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		render(w, r, ui.BucketDetailPage(ui.BucketDetailData{
			Bucket:     b,
			PolicyJSON: policyJSON,
			ErrorMsg:   err.Error(),
		}))
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/buckets/"+bucket))+"?flash=Policy+saved.", http.StatusSeeOther)
}

// BucketTransferOwnership handles POST /buckets/{bucket}/ownership — transfers bucket ownership.
func (h *Handler) BucketTransferOwnership(w http.ResponseWriter, r *http.Request) {
	bucket := r.PathValue("bucket")
	accessKey := r.FormValue("access_key")
	if err := h.api.TransferBucketOwnership(r.Context(), bucket, accessKey); err != nil {
		b, _ := h.api.GetBucket(r.Context(), bucket)
		policyJSON, _ := h.api.GetBucketPolicy(r.Context(), bucket)
		if b == nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		render(w, r, ui.BucketDetailPage(ui.BucketDetailData{
			Bucket:     b,
			PolicyJSON: policyJSON,
			ErrorMsg:   err.Error(),
		}))
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/buckets/"+bucket))+"?flash=Ownership+transferred.", http.StatusSeeOther)
}

// Groups handles GET /groups — renders the group list page.
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

// GroupCreate handles POST /groups — creates a new group.
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

// GroupDetail handles GET /groups/{group} — renders the group detail page.
func (h *Handler) GroupDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("group")
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

// GroupDelete handles POST /groups/{group}/delete — deletes a group.
func (h *Handler) GroupDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("group")
	if err := h.api.DeleteGroup(r.Context(), name); err != nil {
		http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?error="+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/groups")), http.StatusSeeOther)
}

// GroupAddMember handles POST /groups/{group}/members — adds a user to the group.
func (h *Handler) GroupAddMember(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("group")
	accessKey := r.FormValue("access_key")
	if err := h.api.AddGroupMember(r.Context(), name, accessKey); err != nil {
		http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?error="+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?flash=Member+added.", http.StatusSeeOther)
}

// GroupRemoveMember handles POST /groups/{group}/members/remove — removes a user from the group.
func (h *Handler) GroupRemoveMember(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("group")
	accessKey := r.FormValue("access_key")
	if err := h.api.RemoveGroupMember(r.Context(), name, accessKey); err != nil {
		http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?error="+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?flash=Member+removed.", http.StatusSeeOther)
}

// GroupAttachPolicy handles POST /groups/{group}/policies — attaches a policy to the group.
func (h *Handler) GroupAttachPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("group")
	policy := r.FormValue("policy")
	if err := h.api.AttachGroupPolicy(r.Context(), name, policy); err != nil {
		http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?error="+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?flash=Policy+attached.", http.StatusSeeOther)
}

// GroupDetachPolicy handles POST /groups/{group}/policies/detach — detaches a policy from the group.
func (h *Handler) GroupDetachPolicy(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("group")
	policy := r.FormValue("policy")
	if err := h.api.DetachGroupPolicy(r.Context(), name, policy); err != nil {
		http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?error="+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, string(ui.PageURL("/groups/"+name))+"?flash=Policy+detached.", http.StatusSeeOther)
}

// GroupSetStatus handles POST /groups/{group}/status — enables or disables a group.
func (h *Handler) GroupSetStatus(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("group")
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

// Simulate handles GET and POST /simulate — the policy simulator.
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

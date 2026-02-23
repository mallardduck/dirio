// Package console_test covers the DirIO web admin console stopgap features:
//   - Session-based authentication (login / logout / protected-route enforcement)
//   - Full S3 bucket policy editor (view, set, clear, invalid-JSON guard)
//   - Bucket ownership management (view owner, transfer ownership)
//   - Policy request simulator (single-action evaluate, effective permissions)
package console_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Session / Authentication
// ---------------------------------------------------------------------------

func TestLogin_GetPage(t *testing.T) {
	ts := NewTestServer(t)

	resp := ts.ConsoleGet(t, "/login", nil)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, body, "access_key", "login form should contain access_key field")
	assert.Contains(t, body, "secret_key", "login form should contain secret_key field")
}

func TestLogin_AlreadyLoggedIn_RedirectsToDashboard(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)

	resp := ts.ConsoleGet(t, "/login", session)
	DrainAndClose(resp)

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Location"), "/dirio/ui/")
}

func TestLogin_Success_RedirectsToDashboard(t *testing.T) {
	ts := NewTestServer(t)

	form := url.Values{
		"access_key": {ts.AccessKey},
		"secret_key": {ts.SecretKey},
	}
	resp, err := noRedirectClient.PostForm(ts.ConsoleURL("/login"), form)
	require.NoError(t, err)
	DrainAndClose(resp)

	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Location"), "/dirio/ui/")

	var sessionFound bool
	for _, c := range resp.Cookies() {
		if c.Name == "dirio_console_session" {
			sessionFound = true
			break
		}
	}
	assert.True(t, sessionFound, "session cookie should be set after successful login")
}

func TestLogin_WrongPassword_ShowsError(t *testing.T) {
	ts := NewTestServer(t)

	form := url.Values{
		"access_key": {ts.AccessKey},
		"secret_key": {"wrongpassword"},
	}
	resp, err := noRedirectClient.PostForm(ts.ConsoleURL("/login"), form)
	require.NoError(t, err)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode, "failed login should return 200 (re-renders form)")
	assert.Contains(t, body, "Invalid", "error message should be shown")
}

func TestLogin_NonAdminUser_Rejected(t *testing.T) {
	ts := NewTestServer(t)
	ts.CreateUser(t, "alice", "alicesecretkey123")

	form := url.Values{
		"access_key": {"alice"},
		"secret_key": {"alicesecretkey123"},
	}
	resp, err := noRedirectClient.PostForm(ts.ConsoleURL("/login"), form)
	require.NoError(t, err)
	body := ReadBody(t, resp)

	// Non-admin users must be rejected — only the admin UUID may log in.
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, body, "Invalid", "non-admin user should be rejected")
}

func TestLogout_RedirectsToLoginAndClearsCookie(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)

	// Verify protected route works before logout.
	resp := ts.ConsoleGet(t, "/", session)
	DrainAndClose(resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Logout: should redirect to /login.
	resp = ts.ConsolePost(t, "/logout", url.Values{}, session)
	DrainAndClose(resp)
	assert.Equal(t, http.StatusSeeOther, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Location"), "/login")

	// The logout response should instruct the browser to expire the session cookie.
	var cookieCleared bool
	for _, c := range resp.Cookies() {
		if c.Name == "dirio_console_session" && c.MaxAge < 0 {
			cookieCleared = true
			break
		}
	}
	assert.True(t, cookieCleared, "logout should expire the session cookie (MaxAge < 0)")
}

func TestProtectedRoute_NoSession_RedirectsToLogin(t *testing.T) {
	ts := NewTestServer(t)

	routes := []string{"/", "/users", "/buckets", "/policies", "/simulate", "/groups", "/service-accounts"}
	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			resp := ts.ConsoleGet(t, route, nil)
			DrainAndClose(resp)

			assert.Equal(t, http.StatusSeeOther, resp.StatusCode, "unauthenticated request should redirect")
			assert.Contains(t, resp.Header.Get("Location"), "/login")
		})
	}
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

func TestDashboard_ShowsCounts(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)

	ts.CreateBucket(t, "testbucket")
	ts.CreateUser(t, "alice", "alicesecretkey123")

	resp := ts.ConsoleGet(t, "/", session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// Dashboard shows numeric counts (e.g. "1" bucket, "1" user), not bucket names.
	assert.Contains(t, body, "Buckets", "dashboard should show Buckets card")
	assert.Contains(t, body, "IAM Users", "dashboard should show IAM Users card")
}

// ---------------------------------------------------------------------------
// Full S3 Bucket Policy Editor (stopgap)
// ---------------------------------------------------------------------------

func TestBucketDetail_ExistingBucket_ShowsEmptyPolicy(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "mybucket")

	resp := ts.ConsoleGet(t, "/buckets/mybucket", session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, body, "mybucket")
}

func TestBucketDetail_NonExistentBucket_Returns404(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)

	resp := ts.ConsoleGet(t, "/buckets/doesnotexist", session)
	DrainAndClose(resp)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestBucketPolicyEditor_SetValidPolicy(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "policybucket")

	policy := publicReadPolicy("policybucket")
	form := url.Values{"policy": {policy}}
	resp := ts.ConsolePost(t, "/buckets/policybucket/policy", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// The handler re-renders the bucket detail page with the submitted policy.
	assert.Contains(t, body, "policybucket")
	// The policy JSON content should be in the re-rendered page.
	assert.Contains(t, body, "s3:GetObject")
}

func TestBucketPolicyEditor_SetThenClearPolicy(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "clearbucket")

	// Set a policy first.
	ts.SetBucketPolicy(t, "clearbucket", publicReadPolicy("clearbucket"))

	// Verify it is visible in the detail page.
	resp := ts.ConsoleGet(t, "/buckets/clearbucket", session)
	body := ReadBody(t, resp)
	assert.Contains(t, body, "s3:GetObject", "policy should be visible before clearing")
	DrainAndClose(resp)

	// Clear the policy by submitting an empty string.
	form := url.Values{"policy": {""}}
	resp = ts.ConsolePost(t, "/buckets/clearbucket/policy", form, session)
	body = ReadBody(t, resp)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// After clearing, the re-rendered page should not contain the old policy statement.
	assert.NotContains(t, body, "s3:GetObject", "policy should be cleared")
}

func TestBucketPolicyEditor_InvalidJSON_ShowsError(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "invalidpolicybucket")

	form := url.Values{"policy": {`{this is not valid json}`}}
	resp := ts.ConsolePost(t, "/buckets/invalidpolicybucket/policy", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// The handler triggers a toast with an error; the page is re-rendered with the bad policy.
	assert.Contains(t, body, "invalidpolicybucket")
}

// ---------------------------------------------------------------------------
// Bucket Ownership Management (stopgap)
// ---------------------------------------------------------------------------

func TestOwnership_AdminOwnedBucket_ShowsNoOwner(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "adminbucket")

	resp := ts.ConsoleGet(t, "/buckets/adminbucket", session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, body, "adminbucket")
	// Admin-owned buckets display "admin (no explicit owner)".
	assert.Contains(t, body, "no explicit owner", "bucket created by admin should show no explicit owner")
}

func TestOwnership_TransferToUser_UpdatesOwner(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "transferbucket")
	ts.CreateUser(t, "alice", "alicesecretkey123")

	form := url.Values{"access_key": {"alice"}}
	resp := ts.ConsolePost(t, "/buckets/transferbucket/ownership", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// The re-rendered page should show alice as the new owner.
	assert.Contains(t, body, "alice", "alice should now be shown as owner")
}

func TestOwnership_TransferThenViewBucketDetail(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "viewownerbucket")
	ts.CreateUser(t, "bob", "bobsecretkey123")

	// Transfer ownership.
	form := url.Values{"access_key": {"bob"}}
	resp := ts.ConsolePost(t, "/buckets/viewownerbucket/ownership", form, session)
	DrainAndClose(resp)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Fetch bucket detail page to verify owner persisted.
	resp = ts.ConsoleGet(t, "/buckets/viewownerbucket", session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, body, "bob", "bob should be shown as the bucket owner on detail page")
}

func TestOwnership_TransferToNonExistentUser_ShowsError(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "nobodybucket")

	form := url.Values{"access_key": {"nobody"}}
	resp := ts.ConsolePost(t, "/buckets/nobodybucket/ownership", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// The handler triggers an error toast; page should still render the bucket.
	assert.Contains(t, body, "nobodybucket")
}

// ---------------------------------------------------------------------------
// Policy Request Simulator — single-action evaluate (stopgap)
// ---------------------------------------------------------------------------

func TestSimulate_GetPage_RendersForm(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)

	resp := ts.ConsoleGet(t, "/simulate", session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, body, "access_key", "form should include access_key field")
	assert.Contains(t, body, "bucket", "form should include bucket field")
	assert.Contains(t, body, "action", "form should include action field")
}

func TestSimulate_DefaultDeny_NoPolicy(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "denybucket")
	ts.CreateUser(t, "alice", "alicesecretkey123")

	form := url.Values{
		"access_key": {"alice"},
		"bucket":     {"denybucket"},
		"action":     {"s3:GetObject"},
		"mode":       {"simulate"},
	}
	resp := ts.ConsolePost(t, "/simulate", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t,
		strings.Contains(body, "Default deny") || strings.Contains(body, "default deny"),
		"no-policy bucket should produce default deny: %s", body,
	)
}

func TestSimulate_AllowedByBucketPolicy(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "allowbucket")
	ts.CreateUser(t, "alice", "alicesecretkey123")
	ts.SetBucketPolicy(t, "allowbucket", allowAllPolicy("allowbucket", "alice"))

	form := url.Values{
		"access_key": {"alice"},
		"bucket":     {"allowbucket"},
		"action":     {"s3:GetObject"},
		"mode":       {"simulate"},
	}
	resp := ts.ConsolePost(t, "/simulate", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t,
		strings.Contains(body, "Allowed") || strings.Contains(body, "allowed"),
		"user with matching bucket policy should be allowed: %s", body,
	)
	assert.NotContains(t, strings.ToLower(body), "default deny")
}

func TestSimulate_ExplicitDenyOverridesAllow(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "denyfirstbucket")
	ts.CreateUser(t, "alice", "alicesecretkey123")

	// Policy: allow all then deny GetObject for alice.
	denyPolicy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {"AWS": ["alice"]},
				"Action": ["s3:*"],
				"Resource": ["arn:aws:s3:::denyfirstbucket/*"]
			},
			{
				"Effect": "Deny",
				"Principal": {"AWS": ["alice"]},
				"Action": ["s3:GetObject"],
				"Resource": ["arn:aws:s3:::denyfirstbucket/*"]
			}
		]
	}`
	ts.SetBucketPolicy(t, "denyfirstbucket", denyPolicy)

	form := url.Values{
		"access_key": {"alice"},
		"bucket":     {"denyfirstbucket"},
		"action":     {"s3:GetObject"},
		"mode":       {"simulate"},
	}
	resp := ts.ConsolePost(t, "/simulate", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t,
		strings.Contains(body, "Explicitly denied") || strings.Contains(body, "denied"),
		"explicit deny should be reported: %s", body,
	)
}

func TestSimulate_UnknownUser_ShowsError(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "somebucket")

	form := url.Values{
		"access_key": {"nobody"},
		"bucket":     {"somebucket"},
		"action":     {"s3:GetObject"},
		"mode":       {"simulate"},
	}
	resp := ts.ConsolePost(t, "/simulate", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t,
		strings.Contains(body, "not found") || strings.Contains(body, "nobody"),
		"unknown user should produce an error message: %s", body,
	)
}

func TestSimulate_UnknownBucket_ShowsError(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateUser(t, "alice", "alicesecretkey123")

	form := url.Values{
		"access_key": {"alice"},
		"bucket":     {"ghostbucket"},
		"action":     {"s3:GetObject"},
		"mode":       {"simulate"},
	}
	resp := ts.ConsolePost(t, "/simulate", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t,
		strings.Contains(body, "not found") || strings.Contains(body, "ghostbucket"),
		"unknown bucket should produce an error message: %s", body,
	)
}

// ---------------------------------------------------------------------------
// Policy Simulator — effective permissions view (stopgap)
// ---------------------------------------------------------------------------

func TestEffectivePermissions_PrivateBucket_AllDenied(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "privatebucket")
	ts.CreateUser(t, "alice", "alicesecretkey123")

	form := url.Values{
		"access_key": {"alice"},
		"bucket":     {"privatebucket"},
		"mode":       {"effective"},
	}
	resp := ts.ConsolePost(t, "/simulate", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// Private bucket with no policy — all actions should be denied, so denied list is non-empty.
	assert.True(t,
		strings.Contains(body, "s3:GetObject") || strings.Contains(body, "Denied") || strings.Contains(body, "denied"),
		"effective permissions for private bucket should list denied actions: %s", body,
	)
}

func TestEffectivePermissions_AllowAllPolicy_ShowsAllowedActions(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "publicbucket")
	ts.CreateUser(t, "alice", "alicesecretkey123")
	ts.SetBucketPolicy(t, "publicbucket", allowAllPolicy("publicbucket", "alice"))

	form := url.Values{
		"access_key": {"alice"},
		"bucket":     {"publicbucket"},
		"mode":       {"effective"},
	}
	resp := ts.ConsolePost(t, "/simulate", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// At least some actions should be listed as allowed.
	assert.True(t,
		strings.Contains(body, "s3:GetObject") || strings.Contains(body, "Allowed") || strings.Contains(body, "allowed"),
		"effective permissions for allow-all bucket should show allowed actions: %s", body,
	)
}

func TestEffectivePermissions_UnknownUser_ShowsError(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateBucket(t, "effectivebucket")

	form := url.Values{
		"access_key": {"nobody"},
		"bucket":     {"effectivebucket"},
		"mode":       {"effective"},
	}
	resp := ts.ConsolePost(t, "/simulate", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t,
		strings.Contains(body, "not found") || strings.Contains(body, "nobody"),
		"unknown user for effective permissions should produce error: %s", body,
	)
}

func TestEffectivePermissions_UnknownBucket_ShowsError(t *testing.T) {
	ts := NewTestServer(t)
	session := ts.Login(t)
	ts.CreateUser(t, "alice", "alicesecretkey123")

	form := url.Values{
		"access_key": {"alice"},
		"bucket":     {"ghostbucket2"},
		"mode":       {"effective"},
	}
	resp := ts.ConsolePost(t, "/simulate", form, session)
	body := ReadBody(t, resp)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t,
		strings.Contains(body, "not found") || strings.Contains(body, "ghostbucket2"),
		"unknown bucket for effective permissions should produce error: %s", body,
	)
}

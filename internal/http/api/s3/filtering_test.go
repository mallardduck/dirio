package s3

import (
	stdcontext "context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/pkg/iam"
)

// Test helper functions that don't require HTTPHandler

func TestGetRequestUser(t *testing.T) {
	tests := []struct {
		name     string
		ctx      stdcontext.Context
		wantUser *iam.User
	}{
		{
			name:     "nil context",
			ctx:      nil,
			wantUser: nil,
		},
		{
			name:     "empty context",
			ctx:      stdcontext.Background(),
			wantUser: nil,
		},
		{
			name: "context with user",
			ctx: stdcontext.WithValue(
				stdcontext.Background(),
				context.RequestUserKey,
				&iam.User{AccessKey: "test-user"},
			),
			wantUser: &iam.User{AccessKey: "test-user"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRequestUser(tt.ctx)
			if got == nil && tt.wantUser == nil {
				return
			}
			if got == nil || tt.wantUser == nil {
				t.Errorf("getRequestUser() = %v, want %v", got, tt.wantUser)
				return
			}
			if got.AccessKey != tt.wantUser.AccessKey {
				t.Errorf("getRequestUser() = %v, want %v", got.AccessKey, tt.wantUser.AccessKey)
			}
		})
	}
}

func TestIsAdminUser(t *testing.T) {
	tests := []struct {
		name             string
		user             *iam.User
		rootAccessKey    string
		altRootAccessKey string
		want             bool
	}{
		{
			name:             "nil user",
			user:             nil,
			rootAccessKey:    "root",
			altRootAccessKey: "",
			want:             false,
		},
		{
			name:             "root access key",
			user:             &iam.User{AccessKey: "root"},
			rootAccessKey:    "root",
			altRootAccessKey: "",
			want:             true,
		},
		{
			name:             "alt root access key",
			user:             &iam.User{AccessKey: "altroot"},
			rootAccessKey:    "root",
			altRootAccessKey: "altroot",
			want:             true,
		},
		{
			name:             "regular user",
			user:             &iam.User{AccessKey: "alice"},
			rootAccessKey:    "root",
			altRootAccessKey: "altroot",
			want:             false,
		},
		{
			name:             "empty alt root access key",
			user:             &iam.User{AccessKey: ""},
			rootAccessKey:    "root",
			altRootAccessKey: "",
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAdminUser(tt.user, tt.rootAccessKey, tt.altRootAccessKey); got != tt.want {
				t.Errorf("isAdminUser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRequestPrincipal(t *testing.T) {
	tests := []struct {
		name             string
		ctx              stdcontext.Context
		rootAccessKey    string
		altRootAccessKey string
		wantIsAnonymous  bool
		wantIsAdmin      bool
		wantAccessKey    string
	}{
		{
			name:             "anonymous request",
			ctx:              context.WithAnonymousRequest(stdcontext.Background()),
			rootAccessKey:    "root",
			altRootAccessKey: "",
			wantIsAnonymous:  true,
			wantIsAdmin:      false,
			wantAccessKey:    "",
		},
		{
			name: "admin user",
			ctx: stdcontext.WithValue(
				stdcontext.Background(),
				context.RequestUserKey,
				&iam.User{AccessKey: "root"},
			),
			rootAccessKey:    "root",
			altRootAccessKey: "",
			wantIsAnonymous:  false,
			wantIsAdmin:      true,
			wantAccessKey:    "root",
		},
		{
			name: "regular user",
			ctx: stdcontext.WithValue(
				stdcontext.Background(),
				context.RequestUserKey,
				&iam.User{AccessKey: "alice"},
			),
			rootAccessKey:    "root",
			altRootAccessKey: "",
			wantIsAnonymous:  false,
			wantIsAdmin:      false,
			wantAccessKey:    "alice",
		},
		{
			name:             "nil context user is anonymous",
			ctx:              stdcontext.Background(),
			rootAccessKey:    "root",
			altRootAccessKey: "",
			wantIsAnonymous:  true,
			wantIsAdmin:      false,
			wantAccessKey:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getRequestPrincipal(tt.ctx, tt.rootAccessKey, tt.altRootAccessKey)

			if got.IsAnonymous != tt.wantIsAnonymous {
				t.Errorf("getRequestPrincipal() IsAnonymous = %v, want %v", got.IsAnonymous, tt.wantIsAnonymous)
			}
			if got.IsAdmin != tt.wantIsAdmin {
				t.Errorf("getRequestPrincipal() IsAdmin = %v, want %v", got.IsAdmin, tt.wantIsAdmin)
			}

			if tt.wantAccessKey != "" && (got.User == nil || got.User.AccessKey != tt.wantAccessKey) {
				gotKey := ""
				if got.User != nil {
					gotKey = got.User.AccessKey
				}
				t.Errorf("getRequestPrincipal() AccessKey = %v, want %v", gotKey, tt.wantAccessKey)
			}
		})
	}
}

func TestBuildConditionContext(t *testing.T) {
	tests := []struct {
		name          string
		setupRequest  func() *http.Request
		wantSourceIP  string
		wantUserAgent string
		wantSecure    bool
	}{
		{
			name: "basic HTTP request",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "http://example.com", http.NoBody)
				req.Header.Set("User-Agent", "test-agent")
				req.RemoteAddr = "192.168.1.1:1234"
				return req
			},
			wantSourceIP:  "192.168.1.1",
			wantUserAgent: "test-agent",
			wantSecure:    false,
		},
		{
			name: "request with X-Forwarded-For",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "http://example.com", http.NoBody)
				req.Header.Set("X-Forwarded-For", "10.0.0.1, 10.0.0.2")
				req.RemoteAddr = "192.168.1.1:1234"
				return req
			},
			wantSourceIP:  "10.0.0.1",
			wantUserAgent: "",
			wantSecure:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			got := buildConditionContext(req)

			if got.SourceIP != tt.wantSourceIP {
				t.Errorf("buildConditionContext() SourceIP = %v, want %v", got.SourceIP, tt.wantSourceIP)
			}
			if got.UserAgent != tt.wantUserAgent {
				t.Errorf("buildConditionContext() UserAgent = %v, want %v", got.UserAgent, tt.wantUserAgent)
			}
			if got.SecureTransport != tt.wantSecure {
				t.Errorf("buildConditionContext() SecureTransport = %v, want %v", got.SecureTransport, tt.wantSecure)
			}
			if got.CurrentTime.IsZero() {
				t.Error("buildConditionContext() CurrentTime should not be zero")
			}
		})
	}
}

func TestExtractClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xForwarded string
		want       string
	}{
		{
			name:       "simple IPv4",
			remoteAddr: "192.168.1.1:1234",
			xForwarded: "",
			want:       "192.168.1.1",
		},
		{
			name:       "IPv4 no port",
			remoteAddr: "192.168.1.1",
			xForwarded: "",
			want:       "192.168.1.1",
		},
		{
			name:       "IPv6 with port",
			remoteAddr: "[::1]:8080",
			xForwarded: "",
			want:       "::1",
		},
		{
			name:       "X-Forwarded-For single IP",
			remoteAddr: "192.168.1.1:1234",
			xForwarded: "10.0.0.1",
			want:       "10.0.0.1",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			remoteAddr: "192.168.1.1:1234",
			xForwarded: "10.0.0.1, 10.0.0.2, 10.0.0.3",
			want:       "10.0.0.1",
		},
		{
			name:       "X-Forwarded-For with whitespace",
			remoteAddr: "192.168.1.1:1234",
			xForwarded: "  10.0.0.1  ",
			want:       "10.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com", http.NoBody)
			req.RemoteAddr = tt.remoteAddr
			if tt.xForwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.xForwarded)
			}

			got := extractClientIP(req)
			if got != tt.want {
				t.Errorf("extractClientIP() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrincipalString(t *testing.T) {
	tests := []struct {
		name      string
		principal *policy.Principal
		want      string
	}{
		{
			name:      "anonymous",
			principal: &policy.Principal{IsAnonymous: true},
			want:      "anonymous",
		},
		{
			name: "user with access key",
			principal: &policy.Principal{
				User: &iam.User{AccessKey: "alice"},
			},
			want: "alice",
		},
		{
			name:      "unknown (nil user, not anonymous)",
			principal: &policy.Principal{IsAnonymous: false, User: nil},
			want:      "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := principalString(tt.principal); got != tt.want {
				t.Errorf("principalString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildOwnerFromContext(t *testing.T) {
	tests := []struct {
		name            string
		ctx             stdcontext.Context
		wantID          string
		wantDisplayName string
	}{
		{
			name: "user in context",
			ctx: stdcontext.WithValue(
				stdcontext.Background(),
				context.RequestUserKey,
				&iam.User{
					AccessKey: "alice-key",
					Username:  "alice",
				},
			),
			wantID:          "alice-key",
			wantDisplayName: "alice",
		},
		{
			name:            "no user - defaults to root",
			ctx:             stdcontext.Background(),
			wantID:          "root",
			wantDisplayName: "root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildOwnerFromContext(tt.ctx)

			if got.ID != tt.wantID {
				t.Errorf("buildOwnerFromContext() ID = %v, want %v", got.ID, tt.wantID)
			}
			if got.DisplayName != tt.wantDisplayName {
				t.Errorf("buildOwnerFromContext() DisplayName = %v, want %v", got.DisplayName, tt.wantDisplayName)
			}
		})
	}
}

// Note: Full integration tests for filterBuckets() and filterObjects()
// are in tests/integration/list_filtering_test.go
// These unit tests cover the helper functions only

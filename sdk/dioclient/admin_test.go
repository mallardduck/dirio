package dioclient

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// mockAdminBackend is a hand-rolled test double for adminBackend.
// Every method returns zero values unless overridden by the test.
type mockAdminBackend struct {
	// Which V1/V2 policy info methods were invoked.
	v1Called bool
	v2Called bool
	// err is returned by InfoCannedPolicyV1/V2 to test error propagation.
	err error
}

func (m *mockAdminBackend) ListServiceAccounts(_ context.Context, _ string) (ServiceAccountsListResp, error) {
	return ServiceAccountsListResp{}, nil
}
func (m *mockAdminBackend) AddServiceAccount(_ context.Context, _ AddServiceAccountReq) (Credentials, error) {
	return Credentials{}, nil
}
func (m *mockAdminBackend) InfoServiceAccount(_ context.Context, _ string) (ServiceAccountInfoResp, error) {
	return ServiceAccountInfoResp{}, nil
}
func (m *mockAdminBackend) UpdateServiceAccount(_ context.Context, _ string, _ UpdateServiceAccountReq) error {
	return nil
}
func (m *mockAdminBackend) DeleteServiceAccount(_ context.Context, _ string) error { return nil }
func (m *mockAdminBackend) ListUsers(_ context.Context) (map[string]UserInfo, error) {
	return nil, nil
}
func (m *mockAdminBackend) AddUser(_ context.Context, _, _ string) error { return nil }
func (m *mockAdminBackend) RemoveUser(_ context.Context, _ string) error { return nil }
func (m *mockAdminBackend) GetUserInfo(_ context.Context, _ string) (UserInfo, error) {
	return UserInfo{}, nil
}
func (m *mockAdminBackend) SetUserStatus(_ context.Context, _ string, _ AccountStatus) error {
	return nil
}
func (m *mockAdminBackend) ListCannedPolicies(_ context.Context) (map[string]json.RawMessage, error) {
	return nil, nil
}
func (m *mockAdminBackend) AddCannedPolicy(_ context.Context, _ string, _ []byte) error { return nil }
func (m *mockAdminBackend) InfoCannedPolicyV1(_ context.Context, name string) (*PolicyInfo, error) {
	m.v1Called = true
	return &PolicyInfo{PolicyName: name}, m.err
}
func (m *mockAdminBackend) InfoCannedPolicyV2(_ context.Context, name string) (*PolicyInfo, error) {
	m.v2Called = true
	return &PolicyInfo{PolicyName: name}, m.err
}
func (m *mockAdminBackend) DeleteCannedPolicy(_ context.Context, _ string) error { return nil }
func (m *mockAdminBackend) AttachPolicy(_ context.Context, _ PolicyAssociationReq) (PolicyAssociationResp, error) {
	return PolicyAssociationResp{}, nil
}
func (m *mockAdminBackend) DetachPolicy(_ context.Context, _ PolicyAssociationReq) (PolicyAssociationResp, error) {
	return PolicyAssociationResp{}, nil
}

func TestAdminClient_InfoCannedPolicy_DefaultUsesV2(t *testing.T) {
	mock := &mockAdminBackend{}
	client := &AdminClient{proxy: mock}

	if _, err := client.InfoCannedPolicy(context.Background(), "test-policy"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.v2Called {
		t.Error("expected V2 API to be called by default")
	}
	if mock.v1Called {
		t.Error("V1 API must not be called without WithV1API context")
	}
}

func TestAdminClient_InfoCannedPolicy_V1APIContext(t *testing.T) {
	mock := &mockAdminBackend{}
	client := &AdminClient{proxy: mock}

	ctx := WithV1API(context.Background())
	if _, err := client.InfoCannedPolicy(ctx, "test-policy"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.v1Called {
		t.Error("expected V1 API to be called with WithV1API context")
	}
	if mock.v2Called {
		t.Error("V2 API must not be called with WithV1API context")
	}
}

func TestAdminClient_InfoCannedPolicy_ErrorPropagated(t *testing.T) {
	want := errors.New("backend unavailable")
	mock := &mockAdminBackend{err: want}
	client := &AdminClient{proxy: mock}

	_, got := client.InfoCannedPolicy(context.Background(), "policy")
	if !errors.Is(got, want) {
		t.Errorf("error = %v, want %v", got, want)
	}
}

func TestNewAdminClient_EmptyEndpointUsesDefault(t *testing.T) {
	// Empty endpoint should be filled in by NewAdminClient without error.
	// No network call is made during construction.
	client, err := NewAdminClient(Config{Endpoint: "", AccessKey: "key", SecretKey: "secret"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewAdminClient_InvalidEndpoint(t *testing.T) {
	_, err := NewAdminClient(Config{Endpoint: "://bad", AccessKey: "k", SecretKey: "s"})
	if err == nil {
		t.Fatal("expected error for invalid endpoint URL")
	}
}

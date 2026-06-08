package minio

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/minio/madmin-go/v3"
)

func TestMapServiceAccountInfo(t *testing.T) {
	exp := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)
	in := madmin.ServiceAccountInfo{
		AccessKey:     "AKID123",
		ParentUser:    "alice",
		AccountStatus: "on",
		Name:          "my-sa",
		Expiration:    &exp,
	}
	got := mapServiceAccountInfo(in)

	if got.AccessKey != "AKID123" {
		t.Errorf("AccessKey = %q, want %q", got.AccessKey, "AKID123")
	}
	if got.ParentUser != "alice" {
		t.Errorf("ParentUser = %q, want %q", got.ParentUser, "alice")
	}
	if got.AccountStatus != "on" {
		t.Errorf("AccountStatus = %q, want %q", got.AccountStatus, "on")
	}
	if got.Name != "my-sa" {
		t.Errorf("Name = %q, want %q", got.Name, "my-sa")
	}
	if got.Expiration == nil || !got.Expiration.Equal(exp) {
		t.Errorf("Expiration = %v, want %v", got.Expiration, exp)
	}
}

func TestMapServiceAccountInfo_NilExpiration(t *testing.T) {
	in := madmin.ServiceAccountInfo{AccessKey: "AKID", Expiration: nil}
	got := mapServiceAccountInfo(in)
	if got.Expiration != nil {
		t.Errorf("Expiration = %v, want nil", got.Expiration)
	}
}

func TestMapCredentials(t *testing.T) {
	in := madmin.Credentials{AccessKey: "AK", SecretKey: "SK", SessionToken: "ignored"}
	got := mapCredentials(in)
	if got.AccessKey != "AK" {
		t.Errorf("AccessKey = %q, want %q", got.AccessKey, "AK")
	}
	if got.SecretKey != "SK" {
		t.Errorf("SecretKey = %q, want %q", got.SecretKey, "SK")
	}
}

func TestMapInfoServiceAccountResp(t *testing.T) {
	exp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	in := madmin.InfoServiceAccountResp{
		ParentUser:    "bob",
		AccountStatus: "on",
		Name:          "sa-name",
		Description:   "sa-desc",
		ImpliedPolicy: true,
		Expiration:    &exp,
	}
	got := mapInfoServiceAccountResp(in)

	if got.ParentUser != "bob" {
		t.Errorf("ParentUser = %q, want %q", got.ParentUser, "bob")
	}
	if got.Description != "sa-desc" {
		t.Errorf("Description = %q, want %q", got.Description, "sa-desc")
	}
	if !got.ImpliedPolicy {
		t.Error("ImpliedPolicy = false, want true")
	}
	if got.Expiration == nil || !got.Expiration.Equal(exp) {
		t.Errorf("Expiration = %v, want %v", got.Expiration, exp)
	}
}

func TestMapUserInfo_StatusCast(t *testing.T) {
	ts := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	in := madmin.UserInfo{
		Status:     madmin.AccountEnabled,
		PolicyName: "s3-full",
		MemberOf:   []string{"devs", "ops"},
		UpdatedAt:  ts,
	}
	got := mapUserInfo(in)

	if got.Status != AccountEnabled {
		t.Errorf("Status = %q, want %q", got.Status, AccountEnabled)
	}
	if got.PolicyName != "s3-full" {
		t.Errorf("PolicyName = %q, want %q", got.PolicyName, "s3-full")
	}
	if len(got.MemberOf) != 2 || got.MemberOf[0] != "devs" || got.MemberOf[1] != "ops" {
		t.Errorf("MemberOf = %v, want [devs ops]", got.MemberOf)
	}
	if !got.UpdatedAt.Equal(ts) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, ts)
	}
}

func TestMapUserInfo_DisabledStatus(t *testing.T) {
	in := madmin.UserInfo{Status: madmin.AccountDisabled}
	got := mapUserInfo(in)
	if got.Status != AccountDisabled {
		t.Errorf("Status = %q, want %q", got.Status, AccountDisabled)
	}
}

func TestMapPolicyAssociationResp(t *testing.T) {
	in := madmin.PolicyAssociationResp{
		PoliciesAttached: []string{"read-only", "list-buckets"},
		PoliciesDetached: []string{"admin"},
	}
	got := mapPolicyAssociationResp(in)

	if len(got.PoliciesAttached) != 2 || got.PoliciesAttached[0] != "read-only" {
		t.Errorf("PoliciesAttached = %v, want [read-only list-buckets]", got.PoliciesAttached)
	}
	if len(got.PoliciesDetached) != 1 || got.PoliciesDetached[0] != "admin" {
		t.Errorf("PoliciesDetached = %v, want [admin]", got.PoliciesDetached)
	}
}

func TestMapPolicyAssociationResp_NilSlices(t *testing.T) {
	in := madmin.PolicyAssociationResp{}
	got := mapPolicyAssociationResp(in)
	if got.PoliciesAttached != nil {
		t.Errorf("PoliciesAttached = %v, want nil", got.PoliciesAttached)
	}
	if got.PoliciesDetached != nil {
		t.Errorf("PoliciesDetached = %v, want nil", got.PoliciesDetached)
	}
}

func TestMapPolicyInfo(t *testing.T) {
	create := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	update := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"Version":"2012-10-17"}`)
	in := madmin.PolicyInfo{
		PolicyName: "my-policy",
		Policy:     raw,
		CreateDate: create,
		UpdateDate: update,
	}
	got := mapPolicyInfo(in)

	if got.PolicyName != "my-policy" {
		t.Errorf("PolicyName = %q, want %q", got.PolicyName, "my-policy")
	}
	if string(got.Policy) != string(raw) {
		t.Errorf("Policy = %s, want %s", got.Policy, raw)
	}
	if !got.CreateDate.Equal(create) {
		t.Errorf("CreateDate = %v, want %v", got.CreateDate, create)
	}
	if !got.UpdateDate.Equal(update) {
		t.Errorf("UpdateDate = %v, want %v", got.UpdateDate, update)
	}
}

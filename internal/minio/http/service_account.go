package http

import (
	"bytes"
	"io"
	"log/slog"
	nethttp "net/http"
	"time"

	"github.com/mallardduck/go-http-helpers/pkg/headers"
	"github.com/mallardduck/go-http-helpers/pkg/query"

	"github.com/mallardduck/dirio/internal/http/auth"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/internal/service/serviceaccount"
	iamPkg "github.com/mallardduck/dirio/sdk/iam"
)

type ServiceAccountHTTPService struct {
	serviceAccounts *serviceaccount.Service
	log             *slog.Logger
}

// ListServiceAccounts handles GET /minio/admin/v3/list-service-accounts
// madmin expects an encrypted response containing {"accounts": [ServiceAccountInfo, ...]}
func (s *ServiceAccountHTTPService) ListServiceAccounts(w nethttp.ResponseWriter, r *nethttp.Request) {
	adminUser := auth.GetRequestUser(r.Context())
	if adminUser == nil {
		w.WriteHeader(nethttp.StatusUnauthorized)
		return
	}

	keys, err := s.serviceAccounts.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list service accounts", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	type saInfo struct {
		AccessKey     string `json:"accessKey"`
		ParentUser    string `json:"parentUser"`
		AccountStatus string `json:"accountStatus"`
		ImpliedPolicy bool   `json:"impliedPolicy"`
		Name          string `json:"name,omitempty"`
		Description   string `json:"description,omitempty"`
		// Always include expiration as a non-nil time.Time. mc 2025 dereferences
		// Expiration without a nil check and panics when the field is omitted.
		// Zero time means "no expiry".
		Expiration time.Time `json:"expiration"`
	}

	accounts := make([]saInfo, 0, len(keys))
	for _, key := range keys {
		sa, err := s.serviceAccounts.Get(r.Context(), key)
		if err != nil {
			continue
		}
		var expiration time.Time
		if sa.ExpiresAt != nil {
			expiration = *sa.ExpiresAt
		}
		accounts = append(accounts, saInfo{
			AccessKey:     sa.AccessKey,
			AccountStatus: string(sa.Status),
			ImpliedPolicy: sa.PolicyMode != iamPkg.PolicyModeOverride,
			Name:          sa.Name,
			Description:   sa.Description,
			Expiration:    expiration,
		})
	}

	response := map[string]any{"accounts": accounts}
	s.writeEncryptedJSON(w, adminUser.SecretKey, response)
}

// AddServiceAccount handles PUT /minio/admin/v3/add-service-account
// Request body: madmin-encrypted AddServiceAccountReq JSON.
// Response: madmin-encrypted {"credentials": {"accessKey":"...","secretKey":"...",...}}
func (s *ServiceAccountHTTPService) AddServiceAccount(w nethttp.ResponseWriter, r *nethttp.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("Failed to read request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	adminUser := auth.GetRequestUser(r.Context())
	if adminUser == nil {
		w.WriteHeader(nethttp.StatusUnauthorized)
		return
	}

	var body struct {
		AccessKey   string     `json:"accessKey"`
		SecretKey   string     `json:"secretKey"`
		Name        string     `json:"name"`
		Description string     `json:"description"`
		TargetUser  string     `json:"targetUser"`
		PolicyMode  string     `json:"policyMode"`
		Expiration  *time.Time `json:"expiration"`
	}
	if err := decryptAndUnmarshal(adminUser.SecretKey, bytes.NewReader(bodyBytes), &body); err != nil {
		s.log.Error("Failed to decrypt/parse request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	var parentUser *string
	if body.TargetUser != "" {
		parentUser = &body.TargetUser
	}

	sa, err := s.serviceAccounts.Create(r.Context(), &serviceaccount.CreateServiceAccountRequest{
		AccessKey:   body.AccessKey,
		SecretKey:   body.SecretKey,
		Name:        body.Name,
		Description: body.Description,
		ParentUser:  parentUser,
		PolicyMode:  iamPkg.PolicyMode(body.PolicyMode),
		ExpiresAt:   body.Expiration,
	})
	if err != nil {
		s.log.Error("Failed to create service account", "error", err)
		if svcerrors.IsAlreadyExists(err) {
			w.WriteHeader(nethttp.StatusConflict)
			return
		}
		if svcerrors.IsValidation(err) {
			w.WriteHeader(nethttp.StatusBadRequest)
			return
		}
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	// madmin.AddServiceAccount expects {"credentials": {"accessKey":"...","secretKey":"...",...}}
	type credBody struct {
		AccessKey    string     `json:"accessKey"`
		SecretKey    string     `json:"secretKey"`
		SessionToken string     `json:"sessionToken,omitempty"`
		Expiration   *time.Time `json:"expiration,omitempty"`
	}
	response := map[string]any{
		"credentials": credBody{
			AccessKey:  sa.AccessKey,
			SecretKey:  sa.SecretKey,
			Expiration: sa.ExpiresAt,
		},
	}
	s.writeEncryptedJSON(w, adminUser.SecretKey, response)
}

// DeleteServiceAccount handles DELETE /minio/admin/v3/delete-service-account?accessKey=...
func (s *ServiceAccountHTTPService) DeleteServiceAccount(w nethttp.ResponseWriter, r *nethttp.Request) {
	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	if err := s.serviceAccounts.Delete(r.Context(), accessKey); err != nil {
		s.log.Error("Failed to delete service account", "error", err, "accessKey", accessKey)
		if svcerrors.IsNotFound(err) {
			w.WriteHeader(nethttp.StatusNotFound)
			return
		}
		if svcerrors.IsValidation(err) {
			w.WriteHeader(nethttp.StatusBadRequest)
			return
		}
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	w.WriteHeader(nethttp.StatusNoContent)
}

// InfoServiceAccount handles GET /minio/admin/v3/info-service-account?accessKey=...
// madmin expects an encrypted InfoServiceAccountResp.
func (s *ServiceAccountHTTPService) InfoServiceAccount(w nethttp.ResponseWriter, r *nethttp.Request) {
	adminUser := auth.GetRequestUser(r.Context())
	if adminUser == nil {
		w.WriteHeader(nethttp.StatusUnauthorized)
		return
	}

	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	sa, err := s.serviceAccounts.Get(r.Context(), accessKey)
	if err != nil {
		s.log.Error("Failed to get service account", "error", err, "accessKey", accessKey)
		if svcerrors.IsNotFound(err) {
			w.WriteHeader(nethttp.StatusNotFound)
			return
		}
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	// madmin.InfoServiceAccountResp field names.
	response := map[string]any{
		"parentUser":    "",
		"accountStatus": string(sa.Status),
		"impliedPolicy": sa.PolicyMode != iamPkg.PolicyModeOverride,
		"policy":        sa.EmbeddedPolicyJSON,
		"name":          sa.Name,
		"description":   sa.Description,
		"expiration":    sa.ExpiresAt,
	}
	s.writeEncryptedJSON(w, adminUser.SecretKey, response)
}

// UpdateServiceAccount handles POST /minio/admin/v3/update-service-account?accessKey=...
// Body is madmin-encrypted UpdateServiceAccountReq JSON.
// Returns 204 No Content on success (as madmin expects).
func (s *ServiceAccountHTTPService) UpdateServiceAccount(w nethttp.ResponseWriter, r *nethttp.Request) {
	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("Failed to read request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	req, ok := s.parseUpdateBody(w, r, bodyBytes)
	if !ok {
		return
	}

	if _, err := s.serviceAccounts.Update(r.Context(), accessKey, req); err != nil {
		s.log.Error("Failed to update service account", "error", err, "accessKey", accessKey)
		if svcerrors.IsNotFound(err) {
			w.WriteHeader(nethttp.StatusNotFound)
			return
		}
		if svcerrors.IsValidation(err) {
			w.WriteHeader(nethttp.StatusBadRequest)
			return
		}
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	w.WriteHeader(nethttp.StatusNoContent)
}

func (s *ServiceAccountHTTPService) parseUpdateBody(w nethttp.ResponseWriter, r *nethttp.Request, bodyBytes []byte) (*serviceaccount.UpdateServiceAccountRequest, bool) {
	var req serviceaccount.UpdateServiceAccountRequest
	if len(bodyBytes) == 0 {
		return &req, true
	}

	adminUser := auth.GetRequestUser(r.Context())
	if adminUser == nil {
		w.WriteHeader(nethttp.StatusUnauthorized)
		return nil, false
	}

	var body struct {
		NewSecretKey   string     `json:"newSecretKey"`
		NewStatus      string     `json:"newStatus"`
		NewName        string     `json:"newName"`
		NewDescription string     `json:"newDescription"`
		NewExpiration  *time.Time `json:"newExpiration"`
	}
	if err := decryptAndUnmarshal(adminUser.SecretKey, bytes.NewReader(bodyBytes), &body); err != nil {
		s.log.Error("Failed to decrypt/parse request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return nil, false
	}

	if body.NewSecretKey != "" {
		req.SecretKey = &body.NewSecretKey
	}
	if body.NewStatus != "" {
		iamStatus := statusStringToServiceAcct(body.NewStatus)
		if iamStatus == "" {
			w.WriteHeader(nethttp.StatusBadRequest)
			return nil, false
		}
		req.Status = &iamStatus
	}
	if body.NewName != "" {
		req.Name = &body.NewName
	}
	if body.NewDescription != "" {
		req.Description = &body.NewDescription
	}
	if body.NewExpiration != nil {
		req.ExpiresAt = &body.NewExpiration
	}

	return &req, true
}

func statusStringToServiceAcct(s string) iamPkg.ServiceAcctStatus {
	switch s {
	case "on", "enabled":
		return iamPkg.ServiceAcctStatusActive
	case "off", "disabled":
		return iamPkg.ServiceAcctStatusDisabled
	default:
		return ""
	}
}

// writeEncryptedJSON marshals v, encrypts it with secretKey, and writes the result
// as application/octet-stream with status 200.
func (s *ServiceAccountHTTPService) writeEncryptedJSON(w nethttp.ResponseWriter, secretKey string, v any) {
	encrypted, err := marshalAndEncrypt(secretKey, v)
	if err != nil {
		s.log.Error("Failed to marshal/encrypt response", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	w.Header().Set(headers.ContentType, "application/octet-stream")
	w.WriteHeader(nethttp.StatusOK)
	_, _ = w.Write(encrypted)
}

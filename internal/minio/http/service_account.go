package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	nethttp "net/http"

	"github.com/mallardduck/go-http-helpers/pkg/headers"
	"github.com/mallardduck/go-http-helpers/pkg/query"
	"github.com/minio/madmin-go/v3"

	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/jsonutil"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/internal/service/serviceaccount"
	iamPkg "github.com/mallardduck/dirio/pkg/iam"
)

type ServiceAccountHTTPService struct {
	serviceAccounts *serviceaccount.Service
	log             *slog.Logger
}

// ListServiceAccounts handles GET /minio/admin/v3/list-service-accounts
// Returns a JSON array of service account access keys.
func (s *ServiceAccountHTTPService) ListServiceAccounts(w nethttp.ResponseWriter, r *nethttp.Request) {
	keys, err := s.serviceAccounts.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list service accounts", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	if keys == nil {
		keys = []string{}
	}

	// MinIO returns an object with an "accounts" field
	response := map[string]any{
		"accounts": keys,
	}

	w.Header().Set(headers.ContentType, "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.log.Error("Failed to encode service account list", "error", err)
	}
}

// AddServiceAccount handles POST /minio/admin/v3/add-service-account
// Request body is madmin-encrypted JSON: {"accessKey":"...","secretKey":"...","name":"...","parentUser":"..."}
// Returns madmin-encrypted JSON: {"accessKey":"...","secretKey":"...","sessionToken":"","expiration":""}
func (s *ServiceAccountHTTPService) AddServiceAccount(w nethttp.ResponseWriter, r *nethttp.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("Failed to read request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	// Get the authenticated admin user for decryption
	adminUser := auth.GetRequestUser(r.Context())
	if adminUser == nil {
		s.log.Error("No authenticated user in context")
		w.WriteHeader(nethttp.StatusUnauthorized)
		return
	}

	// Decrypt the body
	decryptedData, err := madmin.DecryptData(adminUser.SecretKey, bytes.NewReader(bodyBytes))
	if err != nil {
		s.log.Error("Failed to decrypt request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	var body struct {
		AccessKey  string `json:"accessKey"`
		SecretKey  string `json:"secretKey"`
		Name       string `json:"name"`
		ParentUser string `json:"parentUser"`
		PolicyMode string `json:"policyMode"`
	}
	if err := json.Unmarshal(decryptedData, &body); err != nil {
		s.log.Error("Failed to parse request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	if body.AccessKey == "" {
		s.log.Error("Missing accessKey in request body")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	var parentUser *string
	if body.ParentUser != "" {
		parentUser = &body.ParentUser
	}

	sa, err := s.serviceAccounts.Create(r.Context(), &serviceaccount.CreateServiceAccountRequest{
		AccessKey:  body.AccessKey,
		SecretKey:  body.SecretKey,
		ParentUser: parentUser,
		PolicyMode: iamPkg.PolicyMode(body.PolicyMode),
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

	response := map[string]any{
		"accessKey":    sa.AccessKey,
		"secretKey":    sa.SecretKey,
		"sessionToken": "",
		"expiration":   "",
	}

	responseJSON, err := jsonutil.Marshal(response)
	if err != nil {
		s.log.Error("Failed to marshal response", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	encrypted, err := madmin.EncryptData(adminUser.SecretKey, responseJSON)
	if err != nil {
		s.log.Error("Failed to encrypt response", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	w.Header().Set(headers.ContentType, "application/octet-stream")
	w.WriteHeader(nethttp.StatusOK)
	_, _ = w.Write(encrypted)
}

// DeleteServiceAccount handles POST /minio/admin/v3/delete-service-account?accessKey=...
func (s *ServiceAccountHTTPService) DeleteServiceAccount(w nethttp.ResponseWriter, r *nethttp.Request) {
	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		s.log.Error("Missing accessKey parameter")
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

	w.WriteHeader(nethttp.StatusOK)
}

// InfoServiceAccount handles GET /minio/admin/v3/info-service-account?accessKey=...
// Returns service account details.
func (s *ServiceAccountHTTPService) InfoServiceAccount(w nethttp.ResponseWriter, r *nethttp.Request) {
	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		s.log.Error("Missing accessKey parameter")
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
		if svcerrors.IsValidation(err) {
			w.WriteHeader(nethttp.StatusBadRequest)
			return
		}
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	response := map[string]any{
		"accessKey":      sa.AccessKey,
		"parentUserUUID": sa.ParentUserUUID,
		"policyMode":     sa.PolicyMode,
		"status":         sa.Status,
		"sessionPolicy":  sa.EmbeddedPolicyJSON, // inline policy JSON (override mode only)
		"createdAt":      sa.CreatedAt,
		"updatedAt":      sa.UpdatedAt,
		"expiration":     sa.ExpiresAt,
	}

	w.Header().Set(headers.ContentType, "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.log.Error("Failed to encode service account info", "error", err)
	}
}

// UpdateServiceAccount handles POST /minio/admin/v3/update-service-account
// Supports status updates and secret key rotation.
// Body is madmin-encrypted JSON: {"newSecretKey":"...","newStatus":"on|off"}
func (s *ServiceAccountHTTPService) UpdateServiceAccount(w nethttp.ResponseWriter, r *nethttp.Request) {
	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		s.log.Error("Missing accessKey parameter")
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

	w.WriteHeader(nethttp.StatusOK)
}

// parseUpdateBody decrypts and parses the UpdateServiceAccount request body.
// Returns the populated request and true on success; writes an HTTP error and returns false on failure.
func (s *ServiceAccountHTTPService) parseUpdateBody(w nethttp.ResponseWriter, r *nethttp.Request, bodyBytes []byte) (*serviceaccount.UpdateServiceAccountRequest, bool) {
	var req serviceaccount.UpdateServiceAccountRequest
	if len(bodyBytes) == 0 {
		return &req, true
	}

	adminUser := auth.GetRequestUser(r.Context())
	if adminUser == nil {
		s.log.Error("No authenticated user in context")
		w.WriteHeader(nethttp.StatusUnauthorized)
		return nil, false
	}

	decryptedData, err := madmin.DecryptData(adminUser.SecretKey, bytes.NewReader(bodyBytes))
	if err != nil {
		s.log.Error("Failed to decrypt request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return nil, false
	}

	var body struct {
		NewSecretKey string `json:"newSecretKey"`
		NewStatus    string `json:"newStatus"`
	}
	if err := json.Unmarshal(decryptedData, &body); err != nil {
		s.log.Error("Failed to parse request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return nil, false
	}

	if body.NewSecretKey != "" {
		req.SecretKey = &body.NewSecretKey
	}
	if body.NewStatus != "" {
		iamStatus := statusStringToServiceAcct(body.NewStatus)
		if iamStatus == "" {
			s.log.Error("Invalid status value", "status", body.NewStatus)
			w.WriteHeader(nethttp.StatusBadRequest)
			return nil, false
		}
		req.Status = &iamStatus
	}

	return &req, true
}

// statusStringToServiceAcct converts MinIO status strings to internal format.
// Returns zero value if invalid.
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

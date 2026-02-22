package iam

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/mallardduck/go-http-helpers/pkg/headers"
	"github.com/mallardduck/go-http-helpers/pkg/query"
	"github.com/minio/madmin-go/v3"

	"github.com/mallardduck/dirio/internal/http/auth"

	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/internal/service/policy"
	"github.com/mallardduck/dirio/internal/service/user"
	iamPkg "github.com/mallardduck/dirio/pkg/iam"
)

type userHTTPService struct {
	users    *user.Service
	policies *policy.Service
	log      *slog.Logger
}

func (s *userHTTPService) ListUsers(w http.ResponseWriter, r *http.Request) {
	uids, err := s.users.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list users", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// MinIO API expects access keys; translate UUID list to access key list.
	accessKeys := make([]string, 0, len(uids))
	for _, uid := range uids {
		u, err := s.users.Get(r.Context(), uid)
		if err != nil {
			continue
		}
		accessKeys = append(accessKeys, u.AccessKey)
	}

	w.Header().Set(headers.ContentType, "application/json")
	if err := json.NewEncoder(w).Encode(accessKeys); err != nil {
		s.log.Error("Failed to marshal response", "error", err)
	}
}

func (s *userHTTPService) CreateUser(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("Failed to read request body", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		s.log.Error("Missing accessKey parameter")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	adminUser := auth.GetRequestUser(r.Context())
	if adminUser == nil {
		s.log.Error("No authenticated user in context")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	decryptedData, err := madmin.DecryptData(adminUser.SecretKey, bytes.NewReader(bodyBytes))
	if err != nil {
		s.log.Error("Failed to decrypt request body", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.log.With(
		"request_host", r.URL.Host,
		"request_query", r.URL.Query(),
		"content_type", r.Header.Get("Content-Type"),
		"content_length", r.ContentLength,
		"body_length", len(decryptedData),
		"body", string(decryptedData),
		"headers", r.Header,
	).Info("CreateUser request details")

	var body map[string]string
	if err := json.Unmarshal(decryptedData, &body); err != nil {
		s.log.Error("Failed to parse request body as JSON", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	secretKey := body["secretKey"]
	enabled := body["status"] == "enabled"
	s.log.With("accessKey", accessKey, "adminUser", adminUser.AccessKey).Info("Creating user")

	status := iamPkg.UserStatusDisabled
	if enabled {
		status = iamPkg.UserStatusActive
	}

	_, err = s.users.Create(r.Context(), &user.CreateUserRequest{
		AccessKey: accessKey,
		SecretKey: secretKey,
		Status:    status,
	})
	if err != nil {
		s.log.Error("Failed to create user", "error", err)
		if svcerrors.IsAlreadyExists(err) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		if svcerrors.IsValidation(err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	s.log.With("accessKey", accessKey).Info("User created successfully")
	w.WriteHeader(http.StatusOK)
}

func (s *userHTTPService) RemoveUser(w http.ResponseWriter, r *http.Request) {
	accessKey := query.String(r, "accessKey", "")

	// Translate access key → UUID at the HTTP boundary.
	u, err := s.users.GetByAccessKey(r.Context(), accessKey)
	if err != nil {
		s.log.Error("Failed to find user", "error", err, "accessKey", accessKey)
		if svcerrors.IsNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if svcerrors.IsValidation(err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if err := s.users.Delete(r.Context(), u.UUID); err != nil {
		s.log.Error("Failed to delete user", "error", err, "accessKey", accessKey)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	s.log.With("accessKey", accessKey).Info("User deleted successfully")
	w.WriteHeader(http.StatusOK)
}

func (s *userHTTPService) InfoUser(w http.ResponseWriter, r *http.Request) {
	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		s.log.Error("Missing accessKey parameter")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userEntity, err := s.users.GetByAccessKey(r.Context(), accessKey)
	if err != nil {
		s.log.Error("Failed to get user", "error", err, "accessKey", accessKey)
		if svcerrors.IsNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if svcerrors.IsValidation(err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"accessKey":        userEntity.AccessKey,
		"status":           userEntity.Status,
		"policyName":       "",
		"memberOf":         []string{},
		"updatedAt":        userEntity.UpdatedAt,
		"attachedPolicies": userEntity.AttachedPolicies,
	}

	w.Header().Set(headers.ContentType, "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.log.Error("Failed to encode response", "error", err)
	}
}

func (s *userHTTPService) SetUserStatus(w http.ResponseWriter, r *http.Request) {
	accessKey := query.String(r, "accessKey", "")
	status := query.String(r, "status", "")

	if accessKey == "" {
		s.log.Error("Missing accessKey parameter")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if status == "" {
		s.log.Error("Missing status parameter")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var userStatus iamPkg.UserStatus
	switch status {
	case "enabled":
		userStatus = iamPkg.UserStatusActive
	case "disabled":
		userStatus = iamPkg.UserStatusDisabled
	default:
		s.log.Error("Invalid status value", "status", status)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Translate access key → UUID at the HTTP boundary.
	u, err := s.users.GetByAccessKey(r.Context(), accessKey)
	if err != nil {
		s.log.Error("Failed to find user", "error", err, "accessKey", accessKey)
		if svcerrors.IsNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if svcerrors.IsValidation(err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, err := s.users.Update(r.Context(), u.UUID, &user.UpdateUserRequest{Status: &userStatus}); err != nil {
		s.log.Error("Failed to update user status", "error", err, "accessKey", accessKey)
		if svcerrors.IsNotFound(err) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if svcerrors.IsValidation(err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	s.log.Info("User status updated successfully", "accessKey", accessKey, "status", userStatus)
	w.WriteHeader(http.StatusOK)
}

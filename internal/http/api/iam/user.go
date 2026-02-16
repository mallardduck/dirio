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
	usernames, err := s.users.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list users", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set(headers.ContentType, "application/json")
	err = json.NewEncoder(w).Encode(usernames)
	if err != nil {
		s.log.Error("Failed to marshal response", "error", err)
		return
	}
}

func (s *userHTTPService) CreateUser(w http.ResponseWriter, r *http.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("Failed to read request body", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// The accessKey comes from query parameter
	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		s.log.Error("Missing accessKey parameter")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get the authenticated admin user from context
	adminUser := auth.GetRequestUser(r.Context())
	if adminUser == nil {
		s.log.Error("No authenticated user in context")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Decrypt the body using the admin's secret key
	decryptedData, err := madmin.DecryptData(adminUser.SecretKey, bytes.NewReader(bodyBytes))
	if err != nil {
		s.log.Error("Failed to decrypt request body", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Log request details for debugging
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
	s.log.With(
		"accessKey", accessKey,
		"adminUser", adminUser.AccessKey,
	).Info("Creating user")

	status := iamPkg.UserStatusDisabled
	if enabled {
		status = iamPkg.UserStatusActive
	}

	// Use the user service to create the user
	_, err = s.users.Create(r.Context(), &user.CreateUserRequest{
		AccessKey: accessKey,
		SecretKey: secretKey,
		Status:    status,
	})

	if err != nil {
		s.log.Error("Failed to create user", "error", err)

		// Map service errors to HTTP status codes
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
	username := query.String(r, "accessKey", "")

	err := s.users.Delete(r.Context(), username)
	if err != nil {
		s.log.Error("Failed to delete user", "error", err, "username", username)

		// Map service errors to HTTP status codes
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

	s.log.With("username", username).Info("User deleted successfully")
	w.WriteHeader(http.StatusOK)
}

func (s *userHTTPService) InfoUser(w http.ResponseWriter, r *http.Request) {
	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		s.log.Error("Missing accessKey parameter")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	userEntity, err := s.users.Get(r.Context(), accessKey)
	if err != nil {
		s.log.Error("Failed to get userEntity", "error", err, "accessKey", accessKey)

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

	// Convert to MinIO format
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

	// Convert MinIO status format (enabled/disabled) to our format (on/off)
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

	// Update the user status
	_, err := s.users.Update(r.Context(), accessKey, &user.UpdateUserRequest{
		Status: &userStatus,
	})

	if err != nil {
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

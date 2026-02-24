package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	nethttp "net/http"

	"github.com/mallardduck/go-http-helpers/pkg/headers"
	"github.com/mallardduck/go-http-helpers/pkg/query"

	"github.com/mallardduck/dirio/internal/global"
	"github.com/mallardduck/dirio/internal/http/auth"
	httpMiddleware "github.com/mallardduck/dirio/internal/http/middleware"
	"github.com/mallardduck/dirio/internal/jsonutil"
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

func (s *userHTTPService) ListUsers(w nethttp.ResponseWriter, r *nethttp.Request) {
	adminUser := auth.GetRequestUser(r.Context())
	if adminUser == nil {
		s.log.Error("No authenticated user in context")
		w.WriteHeader(nethttp.StatusUnauthorized)
		return
	}

	uids, err := s.users.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list users", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	// MinIO madmin ListUsers expects map[accessKey]UserInfo encrypted response.
	result := make(map[string]any, len(uids))
	for _, uid := range uids {
		u, err := s.users.Get(r.Context(), uid)
		if err != nil {
			continue
		}

		groups := make([]string, 0)
		if gs, err := s.users.GetGroups(r.Context(), uid); err != nil {
			s.log.Error("Failed to list groups for user", "error", err)
		} else {
			groups = gs
		}

		result[u.AccessKey] = map[string]any{
			"status":    u.Status.MinioString(),
			"memberOf":  groups,
			"updatedAt": u.UpdatedAt,
		}
	}

	encrypted, err := jsonutil.MarshalAndEncrypt(adminUser.SecretKey, result)
	if err != nil {
		s.log.Error("Failed to encrypt response", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	w.Header().Set(headers.ContentType, "application/octet-stream")
	w.WriteHeader(nethttp.StatusOK)
	_, _ = w.Write(encrypted)
}

func (s *userHTTPService) CreateUser(w nethttp.ResponseWriter, r *nethttp.Request) {
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("Failed to read request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		s.log.Error("Missing accessKey parameter")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	adminUser := auth.GetRequestUser(r.Context())
	if adminUser == nil {
		s.log.Error("No authenticated user in context")
		w.WriteHeader(nethttp.StatusUnauthorized)
		return
	}

	var body map[string]string
	if err := jsonutil.DecryptAndUnmarshal(adminUser.SecretKey, bytes.NewReader(bodyBytes), &body); err != nil {
		s.log.Error("Failed to parse request body as JSON", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	s.log.With(
		"request_host", r.URL.Host,
		"request_query", r.URL.Query(),
		"content_type", r.Header.Get("Content-Type"),
		"content_length", r.ContentLength,
		"body_length", len(bodyBytes),
		"body", string(bodyBytes),
		"headers", r.Header,
	).Info("CreateUser request details")

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

	s.log.With("accessKey", accessKey).Info("User created successfully")
	w.WriteHeader(nethttp.StatusOK)
}

func (s *userHTTPService) RemoveUser(w nethttp.ResponseWriter, r *nethttp.Request) {
	accessKey := query.String(r, "accessKey", "")

	// Translate access key → UUID at the HTTP boundary.
	u, err := s.users.GetByAccessKey(r.Context(), accessKey)
	if err != nil {
		s.log.Error("Failed to find user", "error", err, "accessKey", accessKey)
		if svcerrors.IsNotFound(err) {
			// MinIO 2022+ returns 404 with a JSON error body.
			errBody := map[string]string{
				"Code":      "XMinioAdminNoSuchUser",
				"Message":   "The specified user does not exist. (Specified user does not exist)",
				"Resource":  r.URL.Path,
				"RequestId": httpMiddleware.GetRequestID(r.Context()),
				"HostId":    global.GlobalInstanceID().String(),
			}
			data, _ := jsonutil.Marshal(errBody)
			w.Header().Set(headers.ContentType, "application/json")
			w.WriteHeader(nethttp.StatusNotFound)
			_, _ = w.Write(data)
			return
		}
		if svcerrors.IsValidation(err) {
			// TODO potentially this should get a body response too.
			w.WriteHeader(nethttp.StatusBadRequest)
			return
		}
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	if err := s.users.Delete(r.Context(), u.UUID); err != nil {
		s.log.Error("Failed to delete user", "error", err, "accessKey", accessKey)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	s.log.With("accessKey", accessKey).Info("User deleted successfully")
	w.WriteHeader(nethttp.StatusOK)
}

func (s *userHTTPService) InfoUser(w nethttp.ResponseWriter, r *nethttp.Request) {
	accessKey := query.String(r, "accessKey", "")
	if accessKey == "" {
		s.log.Error("Missing accessKey parameter")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	userEntity, err := s.users.GetByAccessKey(r.Context(), accessKey)
	if err != nil {
		s.log.Error("Failed to get user", "error", err, "accessKey", accessKey)
		if svcerrors.IsNotFound(err) {
			errBody := map[string]string{
				"Code":      "XMinioAdminNoSuchUser",
				"Message":   "The specified user does not exist. (Specified user does not exist)",
				"Resource":  r.URL.Path,
				"RequestId": httpMiddleware.GetRequestID(r.Context()),
				"HostId":    global.GlobalInstanceID().String(),
			}
			data, _ := jsonutil.Marshal(errBody)
			w.Header().Set(headers.ContentType, "application/json")
			w.WriteHeader(nethttp.StatusNotFound)
			_, _ = w.Write(data)
			return
		}
		if svcerrors.IsValidation(err) {
			w.WriteHeader(nethttp.StatusBadRequest)
			return
		}
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	groups := make([]string, 0)
	if gs, err := s.users.GetGroups(r.Context(), userEntity.UUID); err != nil {
		s.log.Error("Failed to list groups for user", "error", err)
	} else {
		groups = gs
	}

	response := map[string]any{
		"accessKey":        userEntity.AccessKey,
		"status":           userEntity.Status.MinioString(),
		"policyName":       "", // TODO: figure out how they handle these
		"memberOf":         groups,
		"updatedAt":        userEntity.UpdatedAt,
		"attachedPolicies": userEntity.AttachedPolicies,
	}

	w.Header().Set(headers.ContentType, "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.log.Error("Failed to encode response", "error", err)
	}
}

func (s *userHTTPService) SetUserStatus(w nethttp.ResponseWriter, r *nethttp.Request) {
	accessKey := query.String(r, "accessKey", "")
	status := query.String(r, "status", "")

	if accessKey == "" {
		s.log.Error("Missing accessKey parameter")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}
	if status == "" {
		s.log.Error("Missing status parameter")
		w.WriteHeader(nethttp.StatusBadRequest)
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
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	// Translate access key → UUID at the HTTP boundary.
	u, err := s.users.GetByAccessKey(r.Context(), accessKey)
	if err != nil {
		s.log.Error("Failed to find user", "error", err, "accessKey", accessKey)
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

	if _, err := s.users.Update(r.Context(), u.UUID, &user.UpdateUserRequest{Status: &userStatus}); err != nil {
		s.log.Error("Failed to update user status", "error", err, "accessKey", accessKey)
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

	s.log.Info("User status updated successfully", "accessKey", accessKey, "status", userStatus)
	w.WriteHeader(nethttp.StatusOK)
}

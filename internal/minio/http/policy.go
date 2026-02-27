package http

import (
	"io"
	"log/slog"
	nethttp "net/http"
	"slices"

	"github.com/mallardduck/go-http-helpers/pkg/headers"
	"github.com/mallardduck/go-http-helpers/pkg/query"

	"github.com/mallardduck/dirio/internal/http/auth"
	"github.com/mallardduck/dirio/internal/jsonutil"
	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/internal/service/group"
	"github.com/mallardduck/dirio/internal/service/policy"
	"github.com/mallardduck/dirio/internal/service/user"
	"github.com/mallardduck/dirio/pkg/iam"
)

type PolicyHTTPService struct {
	users    *user.Service
	groups   *group.Service
	policies *policy.Service
	log      *slog.Logger
}

func (s PolicyHTTPService) AddCannedPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	// Get policy name from query parameter (MinIO API format)
	policyName := query.String(r, "name", "")
	if policyName == "" {
		s.log.Error("Missing policy name in query parameter")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	s.log.Debug("Received request to add canned policy", "name", policyName)

	// Read the policy document from the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("Failed to read request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	// Parse the policy document
	var policyDoc iam.PolicyDocument
	if err := jsonutil.Unmarshal(bodyBytes, &policyDoc); err != nil {
		s.log.Error("Failed to parse policy document", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	// Use the policy service to create the policy
	_, err = s.policies.Create(r.Context(), &policy.CreatePolicyRequest{
		Name:           policyName,
		PolicyDocument: &policyDoc,
	})

	if err != nil {
		// Map service errors to HTTP status codes
		s.log.Error("Failed to create policy", "error", err)

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

	s.log.Info("Policy created successfully", "name", policyName)
	w.WriteHeader(nethttp.StatusOK)
}

func (s PolicyHTTPService) ListCannedPolicies(w nethttp.ResponseWriter, r *nethttp.Request) {
	policies, err := s.policies.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list policies", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	w.Header().Set(headers.ContentType, "application/json")
	data, err := jsonutil.Marshal(policies)
	if err != nil {
		s.log.Error("Failed to marshal response", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		s.log.Error("Failed to write response", "error", err)
		return
	}
}

func (s PolicyHTTPService) RemoveCannedPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	policyName := query.String(r, "name", "")
	if policyName == "" {
		s.log.Error("Missing policy name in query parameter")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	err := s.policies.Delete(r.Context(), policyName)
	if err != nil {
		s.log.Error("Failed to delete policy", "error", err, "name", policyName)

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

	s.log.Info("Policy deleted successfully", "name", policyName)
	w.WriteHeader(nethttp.StatusOK)
}

func (s PolicyHTTPService) InfoCannedPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	policyName := query.String(r, "name", "")
	if policyName == "" {
		s.log.Error("Missing policy name in query parameter")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	cannedPolicy, err := s.policies.Get(r.Context(), policyName)
	if err != nil {
		s.log.Error("Failed to get policy", "error", err, "name", policyName)

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

	w.Header().Set(headers.ContentType, "application/json")
	data, err := jsonutil.Marshal(cannedPolicy)
	if err != nil {
		s.log.Error("Failed to marshal response", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		s.log.Error("Failed to write response", "error", err)
		return
	}
}

func (s PolicyHTTPService) SetPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	// SetPolicy attaches/detaches a policy to/from a user
	// Supports both old and new MinIO admin API parameter formats:
	// Old: ?policyName=X&userOrGroup=Y&isGroup=false
	// New: Encrypted JSON body with {User, Group, Policies[]}

	policyName, userOrGroup, isGroup, ok := s.parsePolicyAssocParams(w, r)
	if !ok {
		return
	}

	if policyName == "" || userOrGroup == "" {
		s.log.Error("Missing required parameters", "policyName", policyName, "userOrGroup", userOrGroup)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	var attachErr error
	if isGroup {
		attachErr = s.groups.AttachPolicy(r.Context(), userOrGroup, policyName)
	} else {
		// Translate access key → UUID at the HTTP boundary.
		u, err := s.users.GetByAccessKey(r.Context(), userOrGroup)
		if err != nil {
			s.log.Error("Failed to find user", "error", err, "accessKey", userOrGroup)
			if svcerrors.IsNotFound(err) {
				w.WriteHeader(nethttp.StatusNotFound)
				return
			}
			w.WriteHeader(nethttp.StatusInternalServerError)
			return
		}
		attachErr = s.users.AttachPolicy(r.Context(), u.UUID, policyName)
	}
	if err := attachErr; err != nil {
		s.log.Error("Failed to attach policy", "error", err, "userOrGroup", userOrGroup, "policy", policyName, "isGroup", isGroup)

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

	s.log.Info("Policy attached successfully", "userOrGroup", userOrGroup, "policy", policyName, "isGroup", isGroup)

	// For encrypted requests, return encrypted response (MinIO format)
	if r.Header.Get(headers.ContentType) == "application/octet-stream" {
		adminUser := auth.GetRequestUser(r.Context())
		if adminUser == nil {
			w.WriteHeader(nethttp.StatusInternalServerError)
			return
		}

		// Build and encrypt response matching MinIO's PolicyAssociationResp
		response := map[string]any{
			"updatedAt":        nil, // Could add timestamp if needed
			"policiesAttached": []string{policyName},
			"policiesDetached": []string{},
		}

		encrypted, err := jsonutil.MarshalAndEncrypt(adminUser.SecretKey, response)
		if err != nil {
			s.log.Error("Failed to marshal/encrypt response", "error", err)
			w.WriteHeader(nethttp.StatusInternalServerError)
			return
		}

		w.Header().Set(headers.ContentType, "application/octet-stream")
		w.WriteHeader(nethttp.StatusOK)
		_, err = w.Write(encrypted)
		if err != nil {
			s.log.Error("Failed to write response", "error", err)
			return
		}
	} else {
		// Old format - just return OK
		w.WriteHeader(nethttp.StatusOK)
	}
}

func (s PolicyHTTPService) DetachPolicy(w nethttp.ResponseWriter, r *nethttp.Request) {
	policyName, userOrGroup, isGroup, ok := s.parsePolicyAssocParams(w, r)
	if !ok {
		return
	}

	if policyName == "" || userOrGroup == "" {
		s.log.Error("Missing required parameters", "policyName", policyName, "userOrGroup", userOrGroup)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	var detachErr error
	if isGroup {
		detachErr = s.groups.DetachPolicy(r.Context(), userOrGroup, policyName)
	} else {
		// Translate access key → UUID at the HTTP boundary.
		u, err := s.users.GetByAccessKey(r.Context(), userOrGroup)
		if err != nil {
			s.log.Error("Failed to find user", "error", err, "accessKey", userOrGroup)
			if svcerrors.IsNotFound(err) {
				w.WriteHeader(nethttp.StatusNotFound)
				return
			}
			w.WriteHeader(nethttp.StatusInternalServerError)
			return
		}
		detachErr = s.users.DetachPolicy(r.Context(), u.UUID, policyName)
	}
	if err := detachErr; err != nil {
		s.log.Error("Failed to detach policy", "error", err, "userOrGroup", userOrGroup, "policy", policyName, "isGroup", isGroup)

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

	s.log.Info("Policy detached successfully", "userOrGroup", userOrGroup, "policy", policyName, "isGroup", isGroup)

	if r.Header.Get(headers.ContentType) == "application/octet-stream" {
		adminUser := auth.GetRequestUser(r.Context())
		if adminUser == nil {
			w.WriteHeader(nethttp.StatusInternalServerError)
			return
		}

		response := map[string]any{
			"updatedAt":        nil,
			"policiesAttached": []string{},
			"policiesDetached": []string{policyName},
		}

		encrypted, err := jsonutil.MarshalAndEncrypt(adminUser.SecretKey, response)
		if err != nil {
			s.log.Error("Failed to marshal/encrypt response", "error", err)
			w.WriteHeader(nethttp.StatusInternalServerError)
			return
		}

		w.Header().Set(headers.ContentType, "application/octet-stream")
		w.WriteHeader(nethttp.StatusOK)
		_, err = w.Write(encrypted)
		if err != nil {
			s.log.Error("Failed to write response", "error", err)
			return
		}
	} else {
		w.WriteHeader(nethttp.StatusOK)
	}
}

func (s PolicyHTTPService) PolicyEntitiesList(w nethttp.ResponseWriter, r *nethttp.Request) {
	policyName := query.String(r, "policy", "")
	if policyName == "" {
		s.log.Error("Missing policy name in query parameter")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	// Get all users and filter by those with this policy attached.
	// Translate back to access keys for the MinIO wire format.
	uids, err := s.users.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list users", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	usersWithPolicy := make([]string, 0)
	for _, uid := range uids {
		userEntity, err := s.users.Get(r.Context(), uid)
		if err != nil {
			continue
		}
		if slices.Contains(userEntity.AttachedPolicies, policyName) {
			usersWithPolicy = append(usersWithPolicy, userEntity.AccessKey)
		}
	}

	// Get all groups and filter by those with this policy attached
	groupNames, err := s.groups.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list groups", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	groupsWithPolicy := make([]string, 0)
	for _, name := range groupNames {
		grp, err := s.groups.Get(r.Context(), name)
		if err != nil {
			continue
		}
		if slices.Contains(grp.AttachedPolicies, policyName) {
			groupsWithPolicy = append(groupsWithPolicy, name)
		}
	}

	response := map[string]any{
		"userMappings":  usersWithPolicy,
		"groupMappings": groupsWithPolicy,
	}

	w.Header().Set(headers.ContentType, "application/json")
	data, err := jsonutil.Marshal(response)
	if err != nil {
		s.log.Error("Failed to marshal response", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		s.log.Error("Failed to write response", "error", err)
		return
	}
}

// parsePolicyAssocParams extracts policy association parameters from the request.
// It handles both the new encrypted-body format (application/octet-stream) and
// the legacy query-parameter format, writing an HTTP error and returning false on failure.
func (s PolicyHTTPService) parsePolicyAssocParams(w nethttp.ResponseWriter, r *nethttp.Request) (policyName, userOrGroup string, isGroup, ok bool) {
	if r.Header.Get(headers.ContentType) != "application/octet-stream" || r.ContentLength <= 0 {
		s.log.Debug("Parsed query parameters", "policy", query.String(r, "policyName", ""), "userOrGroup", query.String(r, "userOrGroup", ""))
		return query.String(r, "policyName", ""),
			query.String(r, "userOrGroup", ""),
			query.Bool(r, "isGroup", false),
			true
	}

	adminUser := auth.GetRequestUser(r.Context())
	if adminUser == nil {
		s.log.Error("No authenticated user in context")
		w.WriteHeader(nethttp.StatusUnauthorized)
		return "", "", false, false
	}

	var req struct {
		User     string   `json:"User"`
		Group    string   `json:"Group"`
		Policies []string `json:"Policies"`
	}
	if err := jsonutil.DecryptAndUnmarshal(adminUser.SecretKey, r.Body, &req); err != nil {
		s.log.Error("Failed to decrypt/parse request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return "", "", false, false
	}

	s.log.Debug("Decrypted request", "user", req.User, "group", req.Group, "policies", req.Policies)

	if req.User != "" {
		userOrGroup = req.User
	} else if req.Group != "" {
		userOrGroup = req.Group
		isGroup = true
	}
	if len(req.Policies) > 0 {
		policyName = req.Policies[0]
	}

	s.log.Debug("Parsed encrypted request", "policy", policyName, "userOrGroup", userOrGroup, "isGroup", isGroup)
	return policyName, userOrGroup, isGroup, true
}

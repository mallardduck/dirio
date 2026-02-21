package iam

import (
	"io"
	"log/slog"
	"net/http"

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

type policyHTTPService struct {
	users    *user.Service
	groups   *group.Service
	policies *policy.Service
	log      *slog.Logger
}

func (s policyHTTPService) AddCannedPolicy(w http.ResponseWriter, r *http.Request) {
	// Get policy name from query parameter (MinIO API format)
	policyName := query.String(r, "name", "")
	if policyName == "" {
		s.log.Error("Missing policy name in query parameter")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.log.Debug("Received request to add canned policy", "name", policyName)

	// Read the policy document from the request body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		s.log.Error("Failed to read request body", "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Parse the policy document
	var policyDoc iam.PolicyDocument
	if err := jsonutil.Unmarshal(bodyBytes, &policyDoc); err != nil {
		s.log.Error("Failed to parse policy document", "error", err)
		w.WriteHeader(http.StatusBadRequest)
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

	s.log.Info("Policy created successfully", "name", policyName)
	w.WriteHeader(http.StatusOK)
}

func (s policyHTTPService) ListCannedPolicies(w http.ResponseWriter, r *http.Request) {
	policies, err := s.policies.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list policies", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set(headers.ContentType, "application/json")
	data, err := jsonutil.Marshal(policies)
	if err != nil {
		s.log.Error("Failed to marshal response", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		s.log.Error("Failed to write response", "error", err)
		return
	}
}

func (s policyHTTPService) RemoveCannedPolicy(w http.ResponseWriter, r *http.Request) {
	policyName := query.String(r, "name", "")
	if policyName == "" {
		s.log.Error("Missing policy name in query parameter")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	err := s.policies.Delete(r.Context(), policyName)
	if err != nil {
		s.log.Error("Failed to delete policy", "error", err, "name", policyName)

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

	s.log.Info("Policy deleted successfully", "name", policyName)
	w.WriteHeader(http.StatusOK)
}

func (s policyHTTPService) InfoCannedPolicy(w http.ResponseWriter, r *http.Request) {
	policyName := query.String(r, "name", "")
	if policyName == "" {
		s.log.Error("Missing policy name in query parameter")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	cannedPolicy, err := s.policies.Get(r.Context(), policyName)
	if err != nil {
		s.log.Error("Failed to get policy", "error", err, "name", policyName)

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

	w.Header().Set(headers.ContentType, "application/json")
	data, err := jsonutil.Marshal(cannedPolicy)
	if err != nil {
		s.log.Error("Failed to marshal response", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		s.log.Error("Failed to write response", "error", err)
		return
	}
}

func (s policyHTTPService) SetPolicy(w http.ResponseWriter, r *http.Request) {
	// SetPolicy attaches/detaches a policy to/from a user
	// Supports both old and new MinIO admin API parameter formats:
	// Old: ?policyName=X&userOrGroup=Y&isGroup=false
	// New: Encrypted JSON body with {User, Group, Policies[]}

	var policyName, userOrGroup string
	var isGroup bool

	// Try encrypted body first (new MinIO admin API format)
	if r.Header.Get(headers.ContentType) == "application/octet-stream" && r.ContentLength > 0 {
		// Get the authenticated admin user's secret key for decryption
		adminUser := auth.GetRequestUser(r.Context())
		if adminUser == nil {
			s.log.Error("No authenticated user in context")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Decrypt and parse the request body (MinIO PolicyAssociationReq format)
		var req struct {
			User     string   `json:"User"`
			Group    string   `json:"Group"`
			Policies []string `json:"Policies"`
		}
		if err := jsonutil.DecryptAndUnmarshal(adminUser.SecretKey, r.Body, &req); err != nil {
			s.log.Error("Failed to decrypt/parse request body", "error", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		s.log.Debug("Decrypted request", "user", req.User, "group", req.Group, "policies", req.Policies)

		// Extract parameters
		if req.User != "" {
			userOrGroup = req.User
			isGroup = false
		} else if req.Group != "" {
			userOrGroup = req.Group
			isGroup = true
		}
		// Take the first policy (mc sends one at a time)
		if len(req.Policies) > 0 {
			policyName = req.Policies[0]
		}

		s.log.Debug("Parsed encrypted request", "policy", policyName, "userOrGroup", userOrGroup, "isGroup", isGroup)
	} else {
		// Fall back to old format (query parameters)
		policyName = query.String(r, "policyName", "")
		userOrGroup = query.String(r, "userOrGroup", "")
		isGroup = query.Bool(r, "isGroup", false)

		s.log.Debug("Parsed query parameters", "policy", policyName, "userOrGroup", userOrGroup, "isGroup", isGroup)
	}

	if policyName == "" || userOrGroup == "" {
		s.log.Error("Missing required parameters", "policyName", policyName, "userOrGroup", userOrGroup)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var attachErr error
	if isGroup {
		attachErr = s.groups.AttachPolicy(r.Context(), userOrGroup, policyName)
	} else {
		attachErr = s.users.AttachPolicy(r.Context(), userOrGroup, policyName)
	}
	if err := attachErr; err != nil {
		s.log.Error("Failed to attach policy", "error", err, "userOrGroup", userOrGroup, "policy", policyName, "isGroup", isGroup)

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

	s.log.Info("Policy attached successfully", "userOrGroup", userOrGroup, "policy", policyName, "isGroup", isGroup)

	// For encrypted requests, return encrypted response (MinIO format)
	if r.Header.Get(headers.ContentType) == "application/octet-stream" {
		adminUser := auth.GetRequestUser(r.Context())
		if adminUser == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Build and encrypt response matching MinIO's PolicyAssociationResp
		response := map[string]interface{}{
			"updatedAt":        nil, // Could add timestamp if needed
			"policiesAttached": []string{policyName},
			"policiesDetached": []string{},
		}

		encrypted, err := jsonutil.MarshalAndEncrypt(adminUser.SecretKey, response)
		if err != nil {
			s.log.Error("Failed to marshal/encrypt response", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set(headers.ContentType, "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(encrypted)
		if err != nil {
			s.log.Error("Failed to write response", "error", err)
			return
		}
	} else {
		// Old format - just return OK
		w.WriteHeader(http.StatusOK)
	}
}

func (s policyHTTPService) DetachPolicy(w http.ResponseWriter, r *http.Request) {
	var policyName, userOrGroup string
	var isGroup bool

	if r.Header.Get(headers.ContentType) == "application/octet-stream" && r.ContentLength > 0 {
		adminUser := auth.GetRequestUser(r.Context())
		if adminUser == nil {
			s.log.Error("No authenticated user in context")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var req struct {
			User     string   `json:"User"`
			Group    string   `json:"Group"`
			Policies []string `json:"Policies"`
		}
		if err := jsonutil.DecryptAndUnmarshal(adminUser.SecretKey, r.Body, &req); err != nil {
			s.log.Error("Failed to decrypt/parse request body", "error", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		s.log.Debug("Decrypted request", "user", req.User, "group", req.Group, "policies", req.Policies)

		if req.User != "" {
			userOrGroup = req.User
			isGroup = false
		} else if req.Group != "" {
			userOrGroup = req.Group
			isGroup = true
		}
		if len(req.Policies) > 0 {
			policyName = req.Policies[0]
		}

		s.log.Debug("Parsed encrypted request", "policy", policyName, "userOrGroup", userOrGroup, "isGroup", isGroup)
	} else {
		policyName = query.String(r, "policyName", "")
		userOrGroup = query.String(r, "userOrGroup", "")
		isGroup = query.Bool(r, "isGroup", false)

		s.log.Debug("Parsed query parameters", "policy", policyName, "userOrGroup", userOrGroup, "isGroup", isGroup)
	}

	if policyName == "" || userOrGroup == "" {
		s.log.Error("Missing required parameters", "policyName", policyName, "userOrGroup", userOrGroup)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var detachErr error
	if isGroup {
		detachErr = s.groups.DetachPolicy(r.Context(), userOrGroup, policyName)
	} else {
		detachErr = s.users.DetachPolicy(r.Context(), userOrGroup, policyName)
	}
	if err := detachErr; err != nil {
		s.log.Error("Failed to detach policy", "error", err, "userOrGroup", userOrGroup, "policy", policyName, "isGroup", isGroup)

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

	s.log.Info("Policy detached successfully", "userOrGroup", userOrGroup, "policy", policyName, "isGroup", isGroup)

	if r.Header.Get(headers.ContentType) == "application/octet-stream" {
		adminUser := auth.GetRequestUser(r.Context())
		if adminUser == nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		response := map[string]interface{}{
			"updatedAt":        nil,
			"policiesAttached": []string{},
			"policiesDetached": []string{policyName},
		}

		encrypted, err := jsonutil.MarshalAndEncrypt(adminUser.SecretKey, response)
		if err != nil {
			s.log.Error("Failed to marshal/encrypt response", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set(headers.ContentType, "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(encrypted)
		if err != nil {
			s.log.Error("Failed to write response", "error", err)
			return
		}
	} else {
		w.WriteHeader(http.StatusOK)
	}
}

func (s policyHTTPService) PolicyEntitiesList(w http.ResponseWriter, r *http.Request) {
	policyName := query.String(r, "policy", "")
	if policyName == "" {
		s.log.Error("Missing policy name in query parameter")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Get all users and filter by those with this policy attached
	userKeys, err := s.users.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list users", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var usersWithPolicy []string
	for _, accessKey := range userKeys {
		userEntity, err := s.users.Get(r.Context(), accessKey)
		if err != nil {
			continue
		}
		for _, p := range userEntity.AttachedPolicies {
			if p == policyName {
				usersWithPolicy = append(usersWithPolicy, accessKey)
				break
			}
		}
	}

	// Get all groups and filter by those with this policy attached
	groupNames, err := s.groups.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list groups", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	var groupsWithPolicy []string
	for _, name := range groupNames {
		grp, err := s.groups.Get(r.Context(), name)
		if err != nil {
			continue
		}
		for _, p := range grp.AttachedPolicies {
			if p == policyName {
				groupsWithPolicy = append(groupsWithPolicy, name)
				break
			}
		}
	}

	response := map[string]interface{}{
		"userMappings":  usersWithPolicy,
		"groupMappings": groupsWithPolicy,
	}

	w.Header().Set(headers.ContentType, "application/json")
	data, err := jsonutil.Marshal(response)
	if err != nil {
		s.log.Error("Failed to marshal response", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	_, err = w.Write(data)
	if err != nil {
		s.log.Error("Failed to write response", "error", err)
		return
	}
}

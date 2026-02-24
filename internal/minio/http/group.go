package http

import (
	"context"
	"encoding/json"
	"log/slog"
	nethttp "net/http"

	"github.com/mallardduck/go-http-helpers/pkg/headers"
	"github.com/mallardduck/go-http-helpers/pkg/query"

	svcerrors "github.com/mallardduck/dirio/internal/service/errors"
	"github.com/mallardduck/dirio/internal/service/group"
	iamPkg "github.com/mallardduck/dirio/pkg/iam"
)

type groupHTTPService struct {
	groups *group.Service
	log    *slog.Logger
}

// ListGroups handles GET /minio/admin/v3/groups
// Returns a JSON array of group names.
func (s *groupHTTPService) ListGroups(w nethttp.ResponseWriter, r *nethttp.Request) {
	names, err := s.groups.List(r.Context())
	if err != nil {
		s.log.Error("Failed to list groups", "error", err)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	if names == nil {
		names = []string{}
	}

	w.Header().Set(headers.ContentType, "application/json")
	if err := json.NewEncoder(w).Encode(names); err != nil {
		s.log.Error("Failed to encode group list", "error", err)
	}
}

// GetGroupInfo handles GET /minio/admin/v3/group?name=...
// Returns group info including members (as access keys) and attached policies.
func (s *groupHTTPService) GetGroupInfo(w nethttp.ResponseWriter, r *nethttp.Request) {
	name := query.String(r, "group", "")
	if name == "" {
		s.log.Error("Missing group parameter")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	g, err := s.groups.Get(r.Context(), name)
	if err != nil {
		s.log.Error("Failed to get group", "error", err, "group", name)
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

	// Resolve stored UUIDs to access keys for the MinIO-compatible response.
	memberKeys, err := s.groups.GetMemberAccessKeys(r.Context(), name)
	if err != nil {
		s.log.Error("Failed to resolve group member access keys", "error", err, "group", name)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return
	}

	response := map[string]any{
		"name":      g.Name,
		"members":   memberKeys,
		"policies":  g.AttachedPolicies,
		"status":    g.Status,
		"updatedAt": g.UpdatedAt,
	}

	w.Header().Set(headers.ContentType, "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.log.Error("Failed to encode group info", "error", err)
	}
}

// UpdateGroupMembers handles POST /minio/admin/v3/update-group-members
// Body JSON: {"group":"...", "members":["alice","bob"], "isRemove":false}
// Members are access key strings; they are resolved to UUIDs internally.
// When isRemove=false: creates group if not exists, adds members.
// When isRemove=true: removes members from group.
func (s *groupHTTPService) UpdateGroupMembers(w nethttp.ResponseWriter, r *nethttp.Request) {
	var body struct {
		Group    string   `json:"group"`
		Members  []string `json:"members"`
		IsRemove bool     `json:"isRemove"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.log.Error("Failed to decode request body", "error", err)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	if body.Group == "" {
		s.log.Error("Missing group field in request body")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	ctx := r.Context()

	if body.IsRemove {
		if !s.removeGroupMembers(w, ctx, body.Group, body.Members) {
			return
		}
	} else {
		if !s.ensureGroupAndAddMembers(w, ctx, body.Group, body.Members) {
			return
		}
	}

	w.WriteHeader(nethttp.StatusOK)
}

// removeGroupMembers removes each member from the group by access key.
// Writes an HTTP error and returns false on first failure.
func (s *groupHTTPService) removeGroupMembers(w nethttp.ResponseWriter, ctx context.Context, groupName string, members []string) bool {
	for _, accessKey := range members {
		if err := s.groups.RemoveMemberByAccessKey(ctx, groupName, accessKey); err != nil {
			s.log.Error("Failed to remove member from group", "error", err, "group", groupName, "accessKey", accessKey)
			if svcerrors.IsNotFound(err) {
				w.WriteHeader(nethttp.StatusNotFound)
				return false
			}
			w.WriteHeader(nethttp.StatusInternalServerError)
			return false
		}
	}
	return true
}

// ensureGroupAndAddMembers creates the group if needed, then adds each member by access key.
// Writes an HTTP error and returns false on first failure.
func (s *groupHTTPService) ensureGroupAndAddMembers(w nethttp.ResponseWriter, ctx context.Context, groupName string, members []string) bool {
	_, err := s.groups.Get(ctx, groupName)
	if svcerrors.IsNotFound(err) {
		if _, createErr := s.groups.Create(ctx, &group.CreateGroupRequest{Name: groupName}); createErr != nil {
			if !svcerrors.IsAlreadyExists(createErr) {
				s.log.Error("Failed to create group", "error", createErr, "group", groupName)
				w.WriteHeader(nethttp.StatusInternalServerError)
				return false
			}
		}
	} else if err != nil {
		s.log.Error("Failed to get group", "error", err, "group", groupName)
		w.WriteHeader(nethttp.StatusInternalServerError)
		return false
	}

	for _, accessKey := range members {
		if err := s.groups.AddMemberByAccessKey(ctx, groupName, accessKey); err != nil {
			s.log.Error("Failed to add member to group", "error", err, "group", groupName, "accessKey", accessKey)
			if svcerrors.IsNotFound(err) {
				w.WriteHeader(nethttp.StatusNotFound)
				return false
			}
			w.WriteHeader(nethttp.StatusInternalServerError)
			return false
		}
	}
	return true
}

// SetGroupStatus handles POST /minio/admin/v3/set-group-status?group=...&status=enabled|disabled
func (s *groupHTTPService) SetGroupStatus(w nethttp.ResponseWriter, r *nethttp.Request) {
	name := query.String(r, "group", "")
	status := query.String(r, "status", "")

	if name == "" {
		s.log.Error("Missing group parameter")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}
	if status == "" {
		s.log.Error("Missing status parameter")
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	// Convert MinIO format (enabled/disabled) to internal (on/off)
	var groupStatus iamPkg.GroupStatus
	switch status {
	case "enabled":
		groupStatus = iamPkg.GroupStatusActive
	case "disabled":
		groupStatus = iamPkg.GroupStatusDisabled
	default:
		s.log.Error("Invalid status value", "status", status)
		w.WriteHeader(nethttp.StatusBadRequest)
		return
	}

	if err := s.groups.SetStatus(r.Context(), name, groupStatus); err != nil {
		s.log.Error("Failed to set group status", "error", err, "group", name)
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

package group

import "github.com/mallardduck/dirio/pkg/iam"

// CreateGroupRequest represents a request to create a new group
type CreateGroupRequest struct {
	Name string
}

// UpdateGroupMembersRequest represents a request to add or remove members from a group
type UpdateGroupMembersRequest struct {
	GroupName string
	Members   []string
	IsRemove  bool // true = remove members, false = add members
}

// Group is a service-layer view of an IAM group
type Group = iam.Group

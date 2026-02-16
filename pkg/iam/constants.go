package iam

import (
	"github.com/mallardduck/dirio/internal/consts"
)

// AdminUserUUID is the stable UUID for all admin/root credentials.
// This UUID is shared by both the CLI admin and data config admin accounts,
// ensuring consistent ownership tracking even when access keys are rotated.
//
// "BAD CODE ADD COFFEE DADBEEF" - because admin needs coffee
//
// Note: This re-exports consts.AdminUUID for package-level access.
var AdminUserUUID = consts.AdminUUID

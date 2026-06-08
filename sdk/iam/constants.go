package iam

import "github.com/google/uuid"

// AdminUUIDString is the canonical string form of the admin account UUID.
// Must match internal/consts.AdminUUIDString and api.AdminUserUUID.
const AdminUUIDString = "badfc0de-fadd-fc0f-fee0-000dadbeef00"

// AdminUserUUID is the stable UUID for all admin/root credentials.
// This UUID is shared by both the CLI admin and data config admin accounts,
// ensuring consistent ownership tracking even when access keys are rotated.
//
// "BAD CODE ADD COFFEE DADBEEF" - because admin needs coffee
var AdminUserUUID = uuid.MustParse(AdminUUIDString)

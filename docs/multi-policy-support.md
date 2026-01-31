# Multi-Policy Support Implementation

## Summary

DirIO now supports importing MinIO users with multiple IAM policies attached. This matches MinIO 2022's behavior where users can have multiple policies attached simultaneously.

## MinIO Policy Storage Format

MinIO 2022 stores multiple policies in `.minio.sys/config/iam/policydb/users/<username>.json`:

### Single Policy
```json
{
  "version": 1,
  "policy": "readwrite",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

### Multiple Policies (comma-separated string)
```json
{
  "version": 1,
  "policy": "alpha-rw,beta-rw",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

## Implementation Changes

### 1. Created `PolicyList` Type
**File:** `internal/minio/policy_list.go`

Custom type that handles unmarshaling from multiple formats:
- Single string: `"policy1"`
- Comma-separated string: `"policy1,policy2"`
- JSON array: `["policy1", "policy2"]`

Marshals back to JSON array format for consistency.

### 2. Updated Type Definitions

**`internal/minio/types.go`:**
```go
// Before
type UserPolicyMapping struct {
    Policy    string    `json:"policy"`
}

type User struct {
    AttachedPolicy string
}

// After
type UserPolicyMapping struct {
    Policy    PolicyList `json:"policy"`  // Supports multiple policies
}

type User struct {
    AttachedPolicy []string  // Array of policy names
}
```

**`internal/metadata/metadata.go`:**
```go
// Before
type User struct {
    AttachedPolicy string `json:"attachedPolicy,omitempty"`
}

// After
type User struct {
    AttachedPolicies []string `json:"attachedPolicies,omitempty"`
}
```

### 3. Updated Import Logic

**`internal/minio/import.go`:**
- Now converts `PolicyList` to `[]string`
- Logs number of policies attached per user
- Example output: `Attached 2 policy(ies) to user charlie: alpha-rw,beta-rw`

**`internal/metadata/import.go`:**
- Updated to use `AttachedPolicies` instead of `AttachedPolicy`
- Preserves all policies from MinIO import

### 4. Updated Test Script

**`scripts/minio-import-2019-to-2022.sh`:**
```bash
# Before (overwrites policy)
mc admin policy set minio2022 alpha-rw user=charlie
mc admin policy set minio2022 beta-rw user=charlie

# After (attaches multiple policies)
mc admin policy set minio2022 alpha-rw,beta-rw user=charlie
```

### 5. Comprehensive Testing

**Added tests:**
- `policy_list_test.go` - Unit tests for PolicyList unmarshaling/marshaling
- Updated `import_test.go` - Tests for single policy import
- Updated `import_2022_test.go` - Tests for multi-policy import with charlie user

**Test coverage:**
- Single policy as string ✓
- Multiple policies as comma-separated string ✓
- Multiple policies with spaces ✓
- Single policy as array ✓
- Multiple policies as array ✓
- Empty string/array ✓

## Test Results

```
Attached 2 policy(ies) to user charlie: alpha-rw,beta-rw
```

The test successfully imports:
- **alice**: 1 policy (alpha-rw)
- **bob**: 1 policy (beta-rw)
- **charlie**: 2 policies (alpha-rw, beta-rw)

## Usage

### Creating Multi-Policy Users in MinIO 2022

```bash
# Attach multiple policies (comma-separated)
mc admin policy set minio alpha-rw,beta-rw user=username
```

### DirIO Import

Multi-policy users are automatically imported:

```bash
dirio serve --data-dir ./minio-data-2022-import
```

User files in `.dirio/iam/users/<username>.json`:
```json
{
  "version": "1.0.0",
  "accessKey": "charlie",
  "secretKey": "charliepass1234",
  "status": "on",
  "updatedAt": "2024-01-01T00:00:00Z",
  "attachedPolicies": ["alpha-rw", "beta-rw"]
}
```

## Backward Compatibility

- **Field name changed:** `attachedPolicy` (singular) → `attachedPolicies` (plural)
- **Type changed:** `string` → `[]string`
- This is a breaking change, but acceptable since nothing is released yet
- Single-policy users work correctly (stored as single-element array)

## Future Work

- Implement policy evaluation that considers ALL attached policies (union of permissions)
- Add CLI commands to attach/detach policies from users
- Support IAM groups (MinIO feature not yet imported)

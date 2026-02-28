# S3 Action-to-Permission Mapping

## Overview

This document defines the mapping between S3 API actions (as used in route definitions) and the actual IAM permissions required to perform those operations. This mapping is a **critical component** of the Policy Engine implementation.

**Key Insight:** S3 API action names do NOT always match 1:1 with the IAM permissions required to perform them. Some operations require different permissions than their names suggest, while others require multiple permissions.

## Why This Matters

Without this mapping layer, the policy engine would incorrectly evaluate authorization:
- ❌ `HeadObject` would check for `s3:HeadObject` permission (doesn't exist)
- ❌ `CopyObject` would only check `s3:CopyObject` (missing source read check)
- ❌ `ListObjectsV2` would check for `s3:ListObjectsV2` (should be `s3:ListBucket`)

## Special Case: ListBuckets (Result Filtering) ⚠️

**ListBuckets is unique** - it requires **result filtering** instead of binary authorization:

- **Problem**: Anonymous users should see public buckets, authenticated users should see public + their buckets
- **Solution**: Allow the operation, but filter results based on per-bucket permissions
- **Details**: See [authorization-patterns.md](authorization-patterns.md) for complete implementation strategy

**Action Mapping:**
- Route action: `s3:ListBuckets`
- Permission to execute operation: `s3:ListAllMyBuckets` (on resource `*`)
- Permission to see bucket in results: `s3:ListBucket` (on each bucket ARN)

**Phase 3.1 MVP**: ListBuckets requires authentication (no filtering implemented yet)
**Phase 3.2**: Add result filtering for anonymous and authenticated users

---

## Mapping Categories

### 1. Simple 1:1 (Same Name)

Most common case - action name matches permission name exactly:

| Action | Permission |
|--------|-----------|
| `s3:GetObject` | `s3:GetObject` |
| `s3:PutObject` | `s3:PutObject` |
| `s3:DeleteObject` | `s3:DeleteObject` |
| `s3:CreateBucket` | `s3:CreateBucket` |
| `s3:DeleteBucket` | `s3:DeleteBucket` |
| `s3:GetBucketPolicy` | `s3:GetBucketPolicy` |
| `s3:PutBucketPolicy` | `s3:PutBucketPolicy` |
| `s3:DeleteBucketPolicy` | `s3:DeleteBucketPolicy` |
| `s3:GetBucketLocation` | `s3:GetBucketLocation` |
| `s3:GetObjectAcl` | `s3:GetObjectAcl` |
| `s3:PutObjectAcl` | `s3:PutObjectAcl` |
| `s3:GetObjectTagging` | `s3:GetObjectTagging` |
| `s3:PutObjectTagging` | `s3:PutObjectTagging` |

### 2. Simple 1:1 (Different Name) 🔴

**CRITICAL:** Action name differs from required permission:

| Action | Required Permission | Notes |
|--------|-------------------|-------|
| `s3:HeadBucket` | `s3:ListBucket` | Cannot grant metadata-only access |
| `s3:HeadObject` | `s3:GetObject` | Cannot grant metadata-only access |
| `s3:ListObjects` | `s3:ListBucket` | Both v1 and v2 use same permission |
| `s3:ListObjectsV2` | `s3:ListBucket` | Both v1 and v2 use same permission |
| `s3:ListBuckets` | `s3:ListAllMyBuckets` | Service-level operation |
| `s3:ListObjectVersions` | `s3:ListBucketVersions` | Versioned bucket operation |
| `s3:ListMultipartUploads` | `s3:ListBucketMultipartUploads` | Bucket-level multipart |
| `s3:ListParts` | `s3:ListMultipartUploadParts` | Object-level multipart |

### 3. Multipart Upload Mapping 🔴

**CRITICAL:** Most multipart operations use `PutObject` permission:

| Action | Required Permission | Notes |
|--------|-------------------|-------|
| `s3:CreateMultipartUpload` | `s3:PutObject` | Initiate uses Put permission |
| `s3:UploadPart` | `s3:PutObject` | Each part uses Put permission |
| `s3:CompleteMultipartUpload` | `s3:PutObject` | Complete uses Put permission |
| `s3:AbortMultipartUpload` | `s3:AbortMultipartUpload` | Only this has unique permission |
| `s3:ListParts` | `s3:ListMultipartUploadParts` | Different name |

### 4. Multi-Resource Operations 🔴

**CRITICAL:** These operations always require permissions on multiple resources:

#### CopyObject
```
Action: s3:CopyObject
Required Permissions:
  - s3:GetObject on SOURCE (arn:aws:s3:::source-bucket/source-key)
  - s3:PutObject on DESTINATION (arn:aws:s3:::dest-bucket/dest-key)
```

**Example:**
```
Copy: s3://bucket-a/file.txt → s3://bucket-b/file.txt
Checks:
  ✓ Principal has s3:GetObject on arn:aws:s3:::bucket-a/file.txt
  ✓ Principal has s3:PutObject on arn:aws:s3:::bucket-b/file.txt
```

#### UploadPartCopy
```
Action: s3:UploadPartCopy
Required Permissions:
  - s3:GetObject on SOURCE
  - s3:PutObject on DESTINATION
```

### 5. Bulk Operations

Operations that affect multiple objects but use singular permission:

| Action | Required Permission | Notes |
|--------|-------------------|-------|
| `s3:DeleteObjects` | `s3:DeleteObject` | Bulk delete uses singular permission |

**Example:**
```
DeleteObjects request deleting 100 objects
Each object checked with: s3:DeleteObject permission
```

### 6. Conditional Multi-Permission (Phase 2) 🚧

Some operations MAY require additional permissions based on request parameters:

#### PutObject with Headers
```
Base: s3:PutObject (always required)
Conditional (based on request headers):
  - x-amz-acl → s3:PutObjectAcl
  - x-amz-tagging → s3:PutObjectTagging
  - x-amz-server-side-encryption: aws:kms → kms:GenerateDataKey
  - x-amz-object-lock-* → s3:PutObjectRetention or s3:PutObjectLegalHold
```

#### GetObject with Features
```
Base: s3:GetObject (always required)
Conditional:
  - KMS encrypted object → kms:Decrypt
  - Retrieve tags → s3:GetObjectTagging
  - Object Lock → s3:GetObjectRetention, s3:GetObjectLegalHold
```

**Note:** Phase 1 (MVP) will NOT implement conditional permissions. Phase 2 will add this capability.

## Implementation Strategy

### Phase 1: Static Mapping (MVP) ✅

Implement a static action-to-permission mapper that handles:
- Simple 1:1 mappings (same and different names)
- Multi-resource operations (CopyObject)
- Multipart operation mappings
- Bulk operation mappings

**Code Structure:**
```go
package policy

// ActionMapper translates S3 actions to required permissions
type ActionMapper struct {
    // Static mapping table
    actionToPermissions map[string][]string
}

// GetRequiredPermissions returns the permission(s) needed for an action
// Returns: []string of permission names (e.g., ["s3:GetObject", "s3:PutObject"])
func (m *ActionMapper) GetRequiredPermissions(action string) []string

// IsMultiResourceAction returns true if action requires checking multiple resources
func (m *ActionMapper) IsMultiResourceAction(action string) bool

// GetResourceARNs returns the ARN(s) to check for this action
// For most operations: single ARN
// For CopyObject: [sourceARN, destARN]
func (m *ActionMapper) GetResourceARNs(action string, req *RequestContext) []string
```

**Files to Create:**
- `internal/policy/action_mapper.go` - Implementation
- `internal/policy/action_mapper_test.go` - Comprehensive tests

### Phase 2: Conditional Permissions (Future) 🚧

Add request-aware permission expansion:
```go
// GetConditionalPermissions inspects HTTP request for additional permissions
func (m *ActionMapper) GetConditionalPermissions(
    action string,
    req *http.Request,
) []string
```

**Examples:**
- Parse `x-amz-acl` header → add `s3:PutObjectAcl`
- Parse `x-amz-tagging` header → add `s3:PutObjectTagging`
- Detect KMS encryption → add `kms:Decrypt` or `kms:GenerateDataKey`

## Integration with Authorization Middleware

### Current Flow (Without Mapper)
```
1. Extract action from route context: "s3:HeadObject"
2. Build RequestContext with action: "s3:HeadObject"
3. Policy engine evaluates: "s3:HeadObject" ❌ (no such permission)
4. Result: DENY (incorrect)
```

### New Flow (With Mapper) ✅
```
1. Extract action from route context: "s3:HeadObject"
2. ActionMapper translates: "s3:HeadObject" → "s3:GetObject"
3. Build RequestContext with mapped permission: "s3:GetObject"
4. Policy engine evaluates: "s3:GetObject" ✓
5. Result: ALLOW (if policy grants s3:GetObject)
```

### CopyObject Flow (Multi-Resource)
```
1. Extract action from route context: "s3:CopyObject"
2. ActionMapper translates:
   - Permissions: ["s3:GetObject", "s3:PutObject"]
   - Resources: [sourceARN, destARN]
3. Build TWO RequestContexts:
   - Context A: action="s3:GetObject", resource=sourceARN
   - Context B: action="s3:PutObject", resource=destARN
4. Policy engine evaluates BOTH contexts
5. Result: ALLOW only if BOTH succeed
```

## Testing Requirements

### Unit Tests (action_mapper_test.go)

**Test Categories:**

1. **Simple 1:1 Same Name**
   ```go
   TestActionMapper_Simple1to1SameName(t *testing.T)
   // GetObject → s3:GetObject
   // PutObject → s3:PutObject
   ```

2. **Simple 1:1 Different Name**
   ```go
   TestActionMapper_Simple1to1DifferentName(t *testing.T)
   // HeadBucket → s3:ListBucket
   // HeadObject → s3:GetObject
   // ListObjectsV2 → s3:ListBucket
   ```

3. **Multipart Operations**
   ```go
   TestActionMapper_MultipartOperations(t *testing.T)
   // CreateMultipartUpload → s3:PutObject
   // UploadPart → s3:PutObject
   // CompleteMultipartUpload → s3:PutObject
   // AbortMultipartUpload → s3:AbortMultipartUpload (unique)
   ```

4. **Multi-Resource Operations**
   ```go
   TestActionMapper_CopyObject(t *testing.T)
   // CopyObject → [s3:GetObject, s3:PutObject]
   // IsMultiResourceAction(CopyObject) → true
   ```

5. **Bulk Operations**
   ```go
   TestActionMapper_BulkOperations(t *testing.T)
   // DeleteObjects → s3:DeleteObject (singular)
   ```

### Integration Tests

Test authorization middleware with real policy documents:

```go
// Test HeadObject with GetObject policy
policy := `{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": "s3:GetObject",
    "Resource": "arn:aws:s3:::bucket/*"
  }]
}`

// Request: HEAD /bucket/file.txt (action: s3:HeadObject)
// Expected: ALLOW (mapper translates to s3:GetObject)
```

## Complete Mapping Reference Table

### Service Level
| Route Action | IAM Permission(s) | Type | Notes |
|-------------|------------------|------|-------|
| `s3:ListBuckets` | `s3:ListAllMyBuckets` | 1:1 diff | Resource must be `*`. **Special**: Requires result filtering - see [authorization-patterns.md](authorization-patterns.md) |

### Bucket Operations
| Route Action | IAM Permission(s) | Type | Notes |
|-------------|------------------|------|-------|
| `s3:HeadBucket` | `s3:ListBucket` | 1:1 diff | HEAD uses List permission |
| `s3:CreateBucket` | `s3:CreateBucket` | 1:1 same | |
| `s3:DeleteBucket` | `s3:DeleteBucket` | 1:1 same | |
| `s3:ListObjects` | `s3:ListBucket` | 1:1 diff | Both v1/v2 same |
| `s3:ListObjectsV2` | `s3:ListBucket` | 1:1 diff | Both v1/v2 same |
| `s3:ListObjectVersions` | `s3:ListBucketVersions` | 1:1 diff | |
| `s3:ListMultipartUploads` | `s3:ListBucketMultipartUploads` | 1:1 diff | |
| `s3:DeleteObjects` | `s3:DeleteObject` | 1:1 diff | Bulk uses singular |

### Bucket Configuration
| Route Action | IAM Permission(s) | Type | Notes |
|-------------|------------------|------|-------|
| `s3:GetBucketLocation` | `s3:GetBucketLocation` | 1:1 same | |
| `s3:GetBucketPolicy` | `s3:GetBucketPolicy` | 1:1 same | |
| `s3:PutBucketPolicy` | `s3:PutBucketPolicy` | 1:1 same | |
| `s3:DeleteBucketPolicy` | `s3:DeleteBucketPolicy` | 1:1 same | |
| `s3:GetBucketVersioning` | `s3:GetBucketVersioning` | 1:1 same | |
| `s3:PutBucketVersioning` | `s3:PutBucketVersioning` | 1:1 same | |
| `s3:GetBucketAcl` | `s3:GetBucketAcl` | 1:1 same | |
| `s3:PutBucketAcl` | `s3:PutBucketAcl` | 1:1 same | |
| `s3:GetBucketCors` | `s3:GetBucketCors` | 1:1 same | |
| `s3:PutBucketCors` | `s3:PutBucketCors` | 1:1 same | |

### Object Operations
| Route Action | IAM Permission(s) | Type | Notes |
|-------------|------------------|------|-------|
| `s3:HeadObject` | `s3:GetObject` | 1:1 diff | HEAD uses Get permission |
| `s3:GetObject` | `s3:GetObject` | 1:1 same | |
| `s3:PutObject` | `s3:PutObject` | 1:1 same | |
| `s3:DeleteObject` | `s3:DeleteObject` | 1:1 same | |
| `s3:CopyObject` | `s3:GetObject` + `s3:PutObject` | 1:many | Two resources! |
| `s3:GetObjectAcl` | `s3:GetObjectAcl` | 1:1 same | |
| `s3:PutObjectAcl` | `s3:PutObjectAcl` | 1:1 same | |
| `s3:GetObjectTagging` | `s3:GetObjectTagging` | 1:1 same | |
| `s3:PutObjectTagging` | `s3:PutObjectTagging` | 1:1 same | |

### Multipart Operations
| Route Action | IAM Permission(s) | Type | Notes |
|-------------|------------------|------|-------|
| `s3:CreateMultipartUpload` | `s3:PutObject` | 1:1 diff | Uses Put permission |
| `s3:UploadPart` | `s3:PutObject` | 1:1 diff | Uses Put permission |
| `s3:UploadPartCopy` | `s3:GetObject` + `s3:PutObject` | 1:many | Two resources! |
| `s3:CompleteMultipartUpload` | `s3:PutObject` | 1:1 diff | Uses Put permission |
| `s3:AbortMultipartUpload` | `s3:AbortMultipartUpload` | 1:1 same | Unique permission |
| `s3:ListParts` | `s3:ListMultipartUploadParts` | 1:1 diff | |

## References

- [AWS S3 Actions and Required Permissions](https://docs.aws.amazon.com/AmazonS3/latest/userguide/using-with-s3-policy-actions.html)
- [HeadObject API Permissions](https://docs.aws.amazon.com/AmazonS3/latest/API/API_HeadObject.html)
- [CopyObject Permissions Guide](https://docs.aws.amazon.com/AmazonS3/latest/API/API_CopyObject.html)
- [Multipart Upload Permissions](https://docs.aws.amazon.com/AmazonS3/latest/userguide/mpuoverview.html)

## Status

- ✅ **Analysis Complete** - All mappings documented
- 🚧 **Implementation Pending** - ActionMapper not yet created
- ⏳ **Integration Pending** - Authorization middleware not yet updated

---

**Next Steps:**
1. Create `internal/policy/action_mapper.go` with static mapping table
2. Write comprehensive unit tests
3. Update authorization middleware to use mapper
4. Add integration tests with real policy documents

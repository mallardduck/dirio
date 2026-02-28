# DirIO IAM Architecture: Hybrid S3 + MinIO Compatibility

## Executive Summary

DirIO implements a **hybrid IAM architecture** that combines:

- **S3-native authorization semantics** (bucket policies, actions, resources, conditions, variables)
- **MinIO-compatible admin API** (user/policy management via `mc admin`)
- **Unified metadata backend** (all IAM data in `.dirio/iam/`)

This approach provides:
- ✅ Full S3 bucket policy compatibility (public buckets, resource-based access control)
- ✅ MinIO client compatibility (`mc admin` for user management)
- ✅ AWS-like authorization behavior (ownership, conditions, variables, result filtering)
- ✅ Honest compatibility boundaries (no false AWS IAM API promises)

---

## The Hybrid Approach

DirIO uses **different conventions for different API layers**, while sharing the same backend storage:

### S3 API Layer (Port 9000)

**Convention:** AWS S3 standards

**Features:**
- Bucket policies via `PUT /{bucket}?policy`, `GET /{bucket}?policy`, `DELETE /{bucket}?policy`
- S3-style actions: `s3:GetObject`, `s3:PutObject`, `s3:ListBucket`, `s3:DeleteObject`, etc.
- S3-style resources: `arn:aws:s3:::bucket`, `arn:aws:s3:::bucket/*`, `arn:aws:s3:::bucket/prefix/*`
- Principal ARNs: `arn:aws:iam::*:user/username` (wildcard account ID for single-tenant)
- Policy conditions: `IpAddress`, `NotIpAddress`, `StringEquals`, `StringLike`, `DateLessThan`, `NumericEquals`, `Bool`, `Null`
- Policy variables: `${aws:username}`, `${aws:userid}`, `${aws:SourceIp}`, `${s3:prefix}`, `${aws:CurrentTime}`
- NotAction, NotResource, NotPrincipal support

**Compatible with:** AWS CLI (`aws s3api`), boto3, MinIO mc (data plane operations)

**Use cases:**
- Public bucket access (`Principal: "*"`)
- User-specific folder access (`Resource: "arn:aws:s3:::bucket/${aws:username}/*"`)
- IP-restricted access (`Condition: {"IpAddress": {"aws:SourceIp": "10.0.0.0/8"}}`)
- Time-based access (`Condition: {"DateLessThan": {"aws:CurrentTime": "2026-12-31T23:59:59Z"}}`)

### MinIO Admin API Layer (Port 9000, `/minio/admin/v3/*`)

**Convention:** MinIO admin REST API

**Features:**
- User management: `mc admin user add/remove/list/info/enable/disable`
- Policy management: `mc admin policy create/remove/list/info/attach/detach`
- Service accounts: `mc admin account add/remove/list/info` (routes exist, implementation pending)
- JSON request/response format (NOT XML Query API)
- Standard HTTP methods (GET, POST, PUT, DELETE)

**Compatible with:** MinIO mc (control plane operations)

**Use cases:**
- Create users with credentials: `mc admin user add local alice alicepass123`
- Create reusable policies: `mc admin policy create local readonly policy.json`
- Attach policies to users: `mc admin policy attach local readonly --user alice`
- List users with policies: `mc admin user list local`

### Shared IAM Backend (`.dirio/iam/`)

**Format:** DirIO-native JSON metadata

**Storage structure:**
```
.dirio/
├── iam/
│   ├── users/
│   │   └── {access-key}.json      # User metadata with UUID, policies, status
│   ├── policies/
│   │   └── {policy-name}.json     # S3-standard PolicyDocument JSON
│   └── service-accounts/
│       └── {access-key}.json      # Service account metadata (future)
└── buckets/
    └── {bucket}.json               # Bucket metadata including bucket policy
```

**Key insight:** Both S3 API and MinIO Admin API read/write the same metadata files, just using different HTTP endpoints and conventions.

**Backend features:**
- Thread-safe policy cache with RWMutex
- Atomic file writes for consistency
- UUID-based user identity (stable across access key rotation)
- S3-standard PolicyDocument format for all policies
- Policy validation and normalization

---

## Why a Hybrid Approach?

### The Problem with "Pick One"

**Pure AWS IAM compatibility** would require:
- ❌ XML Query API (`aws iam create-user`, `aws iam attach-user-policy`, etc.)
- ❌ Complex STS assume-role semantics
- ❌ Full Terraform AWS provider support
- ❌ Massive implementation scope with high risk of subtle compatibility bugs
- ❌ Path `/` multiplexing for IAM vs S3 operations

**Result:** Partial AWS IAM implementations break tooling expectations and erode trust. No widely-adopted self-hosted object store fully supports AWS IAM.

**Pure MinIO IAM compatibility** would mean:
- ❌ No S3 bucket policies (MinIO uses different policy attachment semantics)
- ❌ No standard public bucket access patterns
- ❌ Missing AWS-standard conditions and policy variables
- ❌ Harder migration from AWS S3 usage patterns
- ❌ Access key-based ownership (not UUID-based like AWS)

**Result:** Limited authorization flexibility, especially for public/shared bucket scenarios.

### DirIO's Solution: Best of Both Worlds

We implement **S3-native authorization** because:
1. **Bucket policies are the S3-standard way** to grant public access and resource-based permissions
2. **AWS-style conditions/variables** are well-documented, powerful, and widely understood
3. **Resource-based policies** are simpler than IAM roles for most self-hosted use cases
4. **Migration from AWS S3** is more straightforward when bucket policies work identically
5. **UUID-based ownership** provides stable identity across credential rotation

We implement **MinIO admin API** because:
1. **Operators already know** `mc admin` commands from MinIO deployments
2. **MinIO admin API is REST-based**, simpler than AWS IAM's XML Query API
3. **Existing MinIO users** can migrate with minimal retraining
4. **No need to build custom admin CLI** - use `mc` directly for user management
5. **Proven at scale** in production self-hosted environments

We use a **unified backend** because:
1. **Single source of truth** for all IAM data
2. **Consistent ownership and permission model** across both APIs
3. **Simpler testing and debugging** - one metadata format
4. **Clear upgrade path** - metadata evolution is centralized
5. **Future DirIO-native client** can expose full power while maintaining compatibility

---

## What We Implement

### ✅ S3 Authorization Layer (Data Plane)

#### Bucket Policies
- **S3 API endpoints:** `PUT /{bucket}?policy`, `GET /{bucket}?policy`, `DELETE /{bucket}?policy`
- **Format:** S3-standard PolicyDocument (Version, Statement, Effect, Principal, Action, Resource, Condition)
- **Public access:** `Principal: "*"` for anonymous access
- **User-specific access:** `Principal: {"AWS": "arn:aws:iam::*:user/alice"}`
- **Explicit deny:** `Effect: "Deny"` overrides all allows (fail-closed security)

#### S3 Actions
Standard S3 permission names with action mapping:
- `s3:GetObject` (also covers HeadObject)
- `s3:PutObject` (also covers CopyObject as destination)
- `s3:DeleteObject`
- `s3:ListBucket` (ListObjectsV1, ListObjectsV2)
- `s3:ListAllMyBuckets` (ListBuckets)
- `s3:GetBucketLocation` (HeadBucket, GetBucketLocation)
- `s3:GetBucketPolicy`, `s3:PutBucketPolicy`, `s3:DeleteBucketPolicy`
- `s3:GetObjectTagging`, `s3:PutObjectTagging`, `s3:DeleteObjectTagging`
- Multipart upload actions (CreateMultipartUpload, UploadPart, CompleteMultipartUpload, AbortMultipartUpload, ListMultipartUploads, ListParts)

#### S3 Resources
ARN-based resource identification with wildcard support:
- `arn:aws:s3:::bucket` - Bucket-level operations
- `arn:aws:s3:::bucket/*` - All objects in bucket
- `arn:aws:s3:::bucket/prefix/*` - Prefix-based object access
- Wildcard patterns: `*` (match all), `?` (match single character)

#### Policy Conditions
AWS-standard condition operators across 6 categories:

**String Conditions:**
- `StringEquals`, `StringNotEquals` - Exact string matching
- `StringLike`, `StringNotLike` - Glob pattern matching (`*`, `?`)
- Case-insensitive variants: `StringEqualsIgnoreCase`, `StringLikeIgnoreCase`

**Numeric Conditions:**
- `NumericEquals`, `NumericNotEquals`
- `NumericLessThan`, `NumericLessThanEquals`
- `NumericGreaterThan`, `NumericGreaterThanEquals`

**Date/Time Conditions:**
- `DateEquals`, `DateNotEquals`
- `DateLessThan`, `DateLessThanEquals`
- `DateGreaterThan`, `DateGreaterThanEquals`
- ISO 8601 timestamp parsing

**IP Address Conditions:**
- `IpAddress`, `NotIpAddress`
- CIDR range support (e.g., `10.0.0.0/8`, `192.168.1.0/24`)

**Boolean Conditions:**
- `Bool` - True/false evaluation

**Null Conditions:**
- `Null` - Check if key exists in context

**Evaluation semantics:** AND across operators, OR across values (matches AWS IAM behavior)

#### Policy Variables
Dynamic variable substitution in Resource and Condition values:
- `${aws:username}` - Current authenticated user name
- `${aws:userid}` - User UUID (stable identifier)
- `${aws:SourceIp}` - Request source IP address
- `${s3:prefix}` - Object key prefix for list operations
- `${aws:CurrentTime}` - Current request timestamp (ISO 8601)

**Example:** Per-user folder access
```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"AWS": "*"},
    "Action": ["s3:GetObject", "s3:PutObject"],
    "Resource": ["arn:aws:s3:::shared/${aws:username}/*"]
  }]
}
```

#### Ownership Model
UUID-based ownership (AWS-like):
- Buckets have `OwnerID` (UUID) and `OwnerPrincipal` (ARN) fields
- Objects have `OwnerID` (UUID) and `OwnerPrincipal` (ARN) fields
- Bucket owners have implicit full access to all objects in bucket
- Explicit deny in policies overrides ownership permissions
- Admin UUID constant (`badfc0de-fadd-fc0f-fee0-000dadbeef00`) for root-owned resources

#### Result Filtering
AWS-like list operation filtering:
- **ListBuckets:** Returns only buckets where user has `s3:GetBucketLocation` permission
- **ListObjects:** Filters objects based on per-object `s3:GetObject` permission
- **Admin fast path:** Users with AdminUUID bypass filtering (performance optimization)
- **Per-object evaluation:** Each object checked against policies (security over performance)

### ✅ MinIO Admin API Layer (Control Plane)

Implemented via `/minio/admin/v3/*` endpoints (same port as S3, path-based routing):

#### User Management (7 operations)
- **ListUsers** - `GET /minio/admin/v3/list-users` - List all users
- **CreateUser** - `POST /minio/admin/v3/add-user` - Create user with access/secret key
- **RemoveUser** - `DELETE /minio/admin/v3/remove-user` - Delete user (with safety checks)
- **InfoUser** - `GET /minio/admin/v3/info-user` - Get user details and attached policies
- **SetUserStatus** - `POST /minio/admin/v3/set-user-status` - Enable/disable user
- **AttachPolicy** - `POST /minio/admin/v3/set-policy` - Attach policy to user
- **DetachPolicy** - Via service layer (remove policy from user)

#### Policy Management (6 operations)
- **ListCannedPolicies** - `GET /minio/admin/v3/list-canned-policies` - List all policies
- **AddCannedPolicy** - `PUT /minio/admin/v3/add-canned-policy` - Create new policy
- **RemoveCannedPolicy** - `DELETE /minio/admin/v3/remove-canned-policy` - Delete policy
- **InfoCannedPolicy** - `GET /minio/admin/v3/info-canned-policy` - Get policy document
- **SetPolicy** - `POST /minio/admin/v3/set-policy` - Attach policy to user/group
- **PolicyEntitiesList** - `GET /minio/admin/v3/list-policy-entities` - List users with policy attached

**Policy format:** All policies stored as S3-standard PolicyDocument JSON (same format as bucket policies)

#### Service Accounts (Routes Exist, Implementation Pending)
- AddServiceAccount, RemoveServiceAccount, ListServiceAccounts
- InfoServiceAccount, UpdateServiceAccount
- Long-lived or temporary credentials scoped to parent user (with optional expiration)
- Policy inheritance from parent user with optional override

### ✅ Shared IAM Backend

#### Metadata Storage (`.dirio/iam/`)
- **Users:** `.dirio/iam/users/{uuid}.json`
  - Fields: UUID, Username, AccessKey, SecretKey (hashed), Status, AttachedPolicies, CreatedAt, UpdatedAt
- **Policies:** `.dirio/iam/policies/{policy-name}.json`
  - S3-standard PolicyDocument format
  - Validation on creation (required fields, valid actions/resources)
- **Bucket policies:** Stored in `.dirio/buckets/{bucket}.json` (bucket metadata)
- **Atomic writes:** Temp file + rename for consistency
- **Thread-safe:** RWMutex for concurrent policy cache access

#### Policy Evaluation Engine
- **Fast path:** Admin user bypasses all policy checks
- **Owner check:** Bucket/object owners get implicit permissions
- **Explicit deny:** Any deny in any policy fails the request (fail-closed)
- **Allow check:** At least one allow required (after no denies)
- **Result filtering:** Post-evaluation filtering for List* operations
- **Variable substitution:** Runtime replacement in Resource/Condition strings
- **Condition evaluation:** Full AWS-standard condition operator support

---

## What We Explicitly Do NOT Implement

### ❌ AWS IAM API

**Not supported:**
- AWS IAM Query API (`aws iam create-user`, `aws iam attach-user-policy`, etc.)
- XML request/response format for IAM operations
- Path `/` multiplexing for IAM actions
- `sts:AssumeRole` and role-based access
- AWS account IDs in ARNs (use wildcard: `arn:aws:iam::*:user/alice`)

**Why:**
- Extremely complex API surface (hundreds of operations)
- Requires perfect XML Query API semantics
- High implementation cost with little self-hosted value
- Partial implementations break tooling and erode trust
- No widely-adopted self-hosted store supports this

**Alternative:**
- Use `mc admin user add` instead of `aws iam create-user`
- Use `mc admin policy attach` instead of `aws iam attach-user-policy`
- Manage IAM via MinIO-compatible API (proven, simpler)

### ❌ Terraform AWS Provider

**Not supported:**
- `aws_iam_user`, `aws_iam_policy`, `aws_iam_role` resources
- `aws_s3_bucket_policy` (use S3 API directly instead)

**Why:**
- Requires full AWS IAM API compatibility
- Terraform AWS provider expects XML Query API
- Better to use native tooling (`mc admin`) or future DirIO provider

**Alternative:**
- Manage users/policies via shell scripts using `mc admin`
- Future Terraform DirIO provider (native API)

### ❌ Advanced IAM Features

**Not planned:**
- **IAM Roles & STS:** Complex assume-role semantics not needed for self-hosted
- **Cross-account access:** Single-tenant architecture
- **Resource-based policies for objects:** S3 standard uses bucket policies, not per-object ACLs
- **MFA & session tokens:** Out of scope for initial IAM implementation
- **Permission boundaries:** Advanced AWS feature, low self-hosted value
- **IAM Groups:** Routes exist but not yet implemented (low priority - policies cover most use cases)

---

## Migration Stories

### From MinIO

**✅ Direct migration** - Point DirIO at existing MinIO data directory

**What works:**
- User credentials import automatically from `.minio.sys/iam/users/`
- Policy documents converted to S3-standard format
- Bucket policies preserved (if using MinIO bucket policies)
- `mc admin` commands work the same way

**What changes:**
- IAM metadata location: `.minio.sys/iam/` → `.dirio/iam/`
- Policy format normalized to S3-standard PolicyDocument
- UUIDs assigned to users (stable identity across key rotation)
- Ownership model changes to UUID-based (from access-key-based)

**What stays the same:**
- Access keys remain valid (no re-provisioning needed)
- Bucket and object data untouched
- `mc` client compatibility maintained
- Operator workflows unchanged

**Migration steps:**
1. Stop MinIO server
2. Point DirIO at data directory: `dirio --data /path/to/minio/data`
3. DirIO auto-migrates IAM metadata on first start
4. Verify users: `mc admin user list local`
5. Test access patterns with existing credentials

### From AWS S3

**⚠️ Partial compatibility** - Bucket policies work, IAM users require recreation

**What works:**
- S3 bucket policies can be copied directly (identical JSON format)
- Bucket policy conditions and variables work identically
- S3 API operations work the same way

**What doesn't work:**
- IAM users must be recreated via `mc admin user add` (no AWS IAM API)
- IAM roles not supported (use direct user policies or service accounts)
- Cross-account access not applicable (single-tenant)

**Migration steps:**
1. Export bucket policies from AWS: `aws s3api get-bucket-policy --bucket mybucket`
2. Import to DirIO: `aws --endpoint-url http://localhost:9000 s3api put-bucket-policy --bucket mybucket --policy file://policy.json`
3. Create users: `mc admin user add local alice alicepass123`
4. Create policies: `mc admin policy create local mypolicy policy.json`
5. Attach policies: `mc admin policy attach local mypolicy --user alice`
6. Test access patterns match AWS behavior

---

## Strategic Positioning

### How We Describe DirIO

> **"S3-compatible object storage with hybrid IAM: S3-native authorization + MinIO-compatible admin API, designed for self-hosted environments."**

**Not:**
- "AWS S3 replacement" (we don't claim full AWS parity)
- "AWS IAM compatible" (we don't support `aws iam` commands)
- "MinIO fork" (we're a reimplementation with different design choices)

### Why This Positioning Is Honest

**We document exactly what works:**
- ✅ S3 bucket policies (identical to AWS S3)
- ✅ S3 authorization semantics (conditions, variables, ownership)
- ✅ MinIO admin API (user/policy management via `mc admin`)

**We document what doesn't:**
- ❌ AWS IAM API (`aws iam` commands)
- ❌ Terraform AWS provider
- ❌ IAM roles and STS

**Users know what to expect:**
- No surprises or hidden incompatibilities
- Clear compatibility boundaries
- Honest migration guidance

### Why This Positioning Is Defensible

**S3 bucket policies are S3-standard:**
- Not our invention, but AWS's published standard
- Widely documented and understood
- Reference implementation behavior well-defined

**MinIO admin API is MinIO-standard:**
- Proven API used in production deployments
- Widely adopted for self-hosted S3 storage
- Well-documented and stable

**Hybrid approach is explicit design choice:**
- Not a technical limitation or compromise
- Deliberate combination of best features
- Optimized for self-hosted use case

### Why This Is the Right Solution for Self-Hosted S3 in 2026

**Technical soundness:**
- Proven authorization model (S3 bucket policies)
- Proven admin model (MinIO API)
- Clear separation of concerns (data plane vs control plane)

**Operational maturity:**
- Operators already know these APIs
- Tools already exist (`aws`, `mc`)
- Migration paths already established

**Strategic advantages:**
- Faster time to stability (proven components)
- Lower cognitive load (familiar patterns)
- Smaller API surface (testable and maintainable)
- Freedom to evolve without breaking AWS tooling
- Clear versioning and upgrade strategy

---

## Long-Term Benefits

### For Operators

1. **Use existing tools** - No new CLI to learn for basic operations
2. **Leverage existing knowledge** - S3 and MinIO concepts transfer directly
3. **Clear migration path** - From MinIO or AWS S3 to DirIO
4. **Honest expectations** - No surprises about what works

### For Developers

1. **Smaller codebase** - No AWS IAM Query API complexity
2. **Easier testing** - Well-defined API surface
3. **Clear boundaries** - S3 semantics for authorization, MinIO for admin
4. **Future extensibility** - Can add DirIO-native features without breaking compatibility

### For the Project

1. **Faster time to production** - Proven APIs reduce unknowns
2. **Lower maintenance burden** - Less code, clearer responsibilities
3. **Better compatibility story** - Explicit about what works and what doesn't
4. **Sustainable growth** - Can evolve IAM without AWS API constraints

---

## Future: DirIO-Native Client

While we support S3 and MinIO clients today, future DirIO-native tooling can expose capabilities beyond both:

**Potential DirIO-native features:**
- **Advanced result filtering** - More granular than AWS ListBuckets filtering
- **Metadata search** - Beyond S3's tag-based queries
- **Ownership transfer** - Change bucket/object owners (not in S3 standard)
- **Policy analytics** - Who has access to what (beyond AWS IAM Access Analyzer)
- **Unified admin API** - Both S3 and MinIO patterns in one client

**Key principle:** DirIO-native client should **never break** S3 or MinIO client compatibility. New features should be additive, not replacement.

---

## Summary

DirIO's hybrid IAM architecture combines:
- **S3-native authorization** for powerful, standard resource-based access control
- **MinIO-compatible admin** for familiar, proven user/policy management
- **Unified backend** for consistency and simplicity

This is the right solution for self-hosted S3 in 2026 - honest, defensible, and operationally sound.
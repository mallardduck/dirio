# Authorization Patterns in DirIO

## Overview

DirIO's policy engine supports two distinct authorization patterns depending on the S3 operation:

1. **Binary Authorization** - Allow or deny the entire operation (most operations)
2. **Result Filtering** - Allow the operation, but filter results based on per-item permissions (**all `List*` operations**)

**Key Insight:** This is a **generic pattern** for all list operations (ListBuckets, ListObjects, ListMultipartUploads, etc.), not just a special case for ListBuckets.

## Pattern 1: Binary Authorization (Default)

**Used for:** Most S3 operations that target a specific resource.

### Examples
- `GetObject` - Either allow access to the object, or deny (403)
- `PutObject` - Either allow upload, or deny (403)
- `DeleteBucket` - Either allow deletion, or deny (403)
- `HeadObject` - Either allow metadata access, or deny (403)

### Flow
```
Request: GET /bucket/file.txt
         ↓
Auth Middleware → Extract user (may be anonymous)
         ↓
Authorization Middleware → Evaluate policy for s3:GetObject on arn:aws:s3:::bucket/file.txt
         ↓
Decision: ALLOW or DENY
         ↓
Result:
  - ALLOW → Proceed to handler, return object
  - DENY → Return 403 Forbidden (do not execute handler)
```

### Implementation
- Authorization middleware blocks request with 403 if policy evaluation returns DENY
- Handler only executes if authorization passes
- Simple, clean separation of concerns

---

## Pattern 2: Result Filtering (List Operations)

**Used for:** Operations that return collections of resources where different items may have different permissions.

### Examples (All List* Operations)

| Operation | Filter By | Priority | AWS Behavior |
|-----------|-----------|----------|--------------|
| **`ListBuckets`** | Which buckets user can access | 🔴 **CRITICAL** | Returns only accessible buckets |
| **`ListObjects` / `ListObjectsV2`** | Which objects user can access | 🟡 **HIGH** | Usually bucket-level policy, but can filter by prefix-based policies |
| **`ListObjectVersions`** | Which object versions user can access | 🟡 **MEDIUM** | Similar to ListObjects + version permissions |
| **`ListMultipartUploads`** | Uploads user initiated or has access to | 🟢 **LOW** | Returns user's uploads + uploads they can manage |
| **`ListParts`** | Parts of uploads user has access to | 🟢 **LOW** | Usually only upload initiator |

**Common Pattern:** All `List*` operations return collections where individual items may have different permissions.

### The Problem: Generic List* Filtering

**The Challenge:** List operations return collections where items may have different permissions.

#### Scenario 1: ListBuckets (Service-Level)
```
Buckets in system:
  - public-bucket (policy: allow Principal "*" to s3:ListBucket)
  - user-a-bucket (policy: allow user-a to s3:*)
  - user-b-bucket (policy: allow user-b to s3:*)
  - admin-bucket (no public policy)

Question: What should ListBuckets return for different users?

Anonymous user → [public-bucket]
User A (authenticated) → [public-bucket, user-a-bucket]
User B (authenticated) → [public-bucket, user-b-bucket]
Admin (authenticated) → [public-bucket, user-a-bucket, user-b-bucket, admin-bucket]
```

#### Scenario 2: ListObjects (Bucket-Level with Prefix Policies)
```
Objects in bucket "shared-bucket":
  - public/readme.txt (policy: allow Principal "*" to s3:GetObject on public/*)
  - users/alice/data.txt (policy: allow alice to s3:GetObject on users/alice/*)
  - users/bob/data.txt (policy: allow bob to s3:GetObject on users/bob/*)
  - admin/config.json (no public policy)

Question: What should ListObjects return for different users?

Anonymous user → [public/readme.txt]
Alice (authenticated) → [public/readme.txt, users/alice/data.txt]
Bob (authenticated) → [public/readme.txt, users/bob/data.txt]
Admin (authenticated) → [all objects]
```

#### Scenario 3: ListMultipartUploads (Bucket-Level)
```
Multipart uploads in bucket:
  - Upload 1: /file.txt by user-a
  - Upload 2: /shared.txt by user-b (policy allows user-a to complete)
  - Upload 3: /private.txt by user-b

Question: What should ListMultipartUploads return?

User A → [Upload 1, Upload 2] (initiated by them OR has access)
User B → [Upload 2, Upload 3] (initiated by them)
Admin → [Upload 1, Upload 2, Upload 3]
```

**Why binary authorization doesn't work:**
- We can't deny the entire ListBuckets operation (anonymous users need to see public buckets)
- We can't allow the entire operation without filtering (would expose private buckets)
- The "resource" for ListBuckets is `*` (service-level), not a specific bucket

### Flow for ListBuckets

```
Request: GET / (ListBuckets)
         ↓
Auth Middleware → Extract user (may be anonymous)
         ↓
Authorization Middleware →
  - Check: Can user perform s3:ListAllMyBuckets on resource "*"?
  - For ListBuckets: ALWAYS ALLOW (operation itself is allowed)
  - Set "filtering required" flag in context
         ↓
Handler → Get all buckets from storage
         ↓
Result Filtering Layer →
  For each bucket:
    - Evaluate: Can this user access this bucket?
    - Check bucket policy for public access (Principal: "*")
    - Check bucket policy for user-specific access
    - Check user's IAM policies (Phase 5)
    - Include bucket in results if ANY check passes
         ↓
Response: Return filtered list of buckets
```

### Generic Filtering Pattern

All List* operations follow the same filtering pattern:

```go
// Generic list filtering pattern
func FilterListResults[T any](
    items []T,
    user *metadata.User,
    engine *policy.Engine,
    getResourceARN func(T) string,
    getRequiredAction func(T) string,
) []T {
    // Admin sees everything
    if user != nil && user.IsAdmin {
        return items
    }

    var filtered []T
    for _, item := range items {
        if canUserSeeItem(item, user, engine, getResourceARN, getRequiredAction) {
            filtered = append(filtered, item)
        }
    }
    return filtered
}

func canUserSeeItem[T any](
    item T,
    user *metadata.User,
    engine *policy.Engine,
    getResourceARN func(T) string,
    getRequiredAction func(T) string,
) bool {
    principal := &policy.Principal{
        User:        user,
        IsAnonymous: user == nil,
        IsAdmin:     false,
    }

    reqCtx := &policy.RequestContext{
        Principal: principal,
        Action:    policy.Action(getRequiredAction(item)),
        Resource:  parseARN(getResourceARN(item)),
    }

    decision := engine.Evaluate(context.Background(), reqCtx)
    return decision.IsAllowed()
}
```

**Usage Examples:**

```go
// ListBuckets filtering
filteredBuckets := FilterListResults(
    allBuckets,
    user,
    engine,
    func(b *Bucket) string { return "arn:aws:s3:::" + b.Name },
    func(b *Bucket) string { return "s3:ListBucket" },
)

// ListObjects filtering (prefix-based policies)
filteredObjects := FilterListResults(
    allObjects,
    user,
    engine,
    func(o *Object) string { return "arn:aws:s3:::" + bucket + "/" + o.Key },
    func(o *Object) string { return "s3:GetObject" }, // Or s3:ListBucket
)

// ListMultipartUploads filtering
filteredUploads := FilterListResults(
    allUploads,
    user,
    engine,
    func(u *Upload) string { return "arn:aws:s3:::" + bucket + "/" + u.Key },
    func(u *Upload) string { return "s3:ListBucketMultipartUploads" },
)
```

### Implementation Strategy

#### Option A: Filter in Handler (Recommended for MVP)

**Pros:** Simple, no middleware complexity
**Cons:** Filtering logic in handler, not reusable

```go
// In ListBuckets handler
func (h *Handler) ListBuckets(w http.ResponseWriter, r *http.Request) {
    // Get all buckets
    allBuckets := h.metadata.ListAllBuckets()

    // Get user from context
    user := auth.GetRequestUser(r.Context())

    // Filter buckets based on permissions
    visibleBuckets := h.filterBuckets(allBuckets, user)

    // Return filtered list
    writeListBucketsResponse(w, visibleBuckets)
}

func (h *Handler) filterBuckets(buckets []*metadata.Bucket, user *metadata.User) []*metadata.Bucket {
    var visible []*metadata.Bucket

    for _, bucket := range buckets {
        if h.canUserSeeBucket(bucket, user) {
            visible = append(visible, bucket)
        }
    }

    return visible
}

func (h *Handler) canUserSeeBucket(bucket *metadata.Bucket, user *metadata.User) bool {
    // Admin can see everything
    if user != nil && user.IsAdmin {
        return true
    }

    // Build principal
    principal := &policy.Principal{
        User:        user,
        IsAnonymous: user == nil,
        IsAdmin:     false,
    }

    // Build request context for this bucket
    reqCtx := &policy.RequestContext{
        Principal: principal,
        Action:    policy.Action("s3:ListBucket"), // Permission to see bucket
        Resource: &policy.Resource{
            Bucket: bucket.Name,
            Key:    "",
        },
    }

    // Evaluate policy for THIS bucket
    decision := h.policyEngine.Evaluate(context.Background(), reqCtx)

    return decision.IsAllowed()
}
```

#### Option B: Filter in Middleware (Phase 3.2+)

**Pros:** Reusable, centralized filtering logic
**Cons:** More complex, requires response interception

```go
// Result filtering middleware (future)
func ResultFiltering(engine *policy.Engine) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Check if this route requires result filtering
            if requiresFiltering := context.GetRequiresFiltering(r.Context()); !requiresFiltering {
                next.ServeHTTP(w, r)
                return
            }

            // Create response interceptor
            interceptor := &responseInterceptor{
                ResponseWriter: w,
                engine:         engine,
                user:           auth.GetRequestUser(r.Context()),
            }

            // Execute handler (writes to interceptor)
            next.ServeHTTP(interceptor, r)

            // Interceptor filters results before sending to client
        })
    }
}
```

### Authorization Modes

To support result filtering, we need **conditional authentication**:

#### Auth Mode: REQUIRED (Default)
- Authentication is required
- Anonymous requests → 403 Forbidden
- Used by: GetObject, PutObject, DeleteBucket, etc.

#### Auth Mode: OPTIONAL (List Operations)
- Authentication is optional
- Anonymous requests allowed, but results differ
- Used by: ListBuckets, potentially ListObjects

#### Auth Mode: NONE (Public Routes)
- No authentication required
- Used by: Health checks, favicon, debug routes

### Route Registration

```go
// Binary authorization (default)
r.GET("/{bucket}/{key:.*}", deps.getObject).
    Name("objects.show").
    Action("s3:GetObject")
    // Auth mode: REQUIRED (default)

// Result filtering
r.GET("/", deps.listBuckets).
    Name("index").
    Action("s3:ListBuckets").
    AuthMode(teapot.AuthOptional).  // NEW: Support anonymous
    RequiresFiltering(true)          // NEW: Flag for result filtering
```

### Policy Evaluation for Filtering

For each bucket, evaluate with `s3:ListBucket` permission (NOT `s3:ListAllMyBuckets`):

**Why?**
- `s3:ListAllMyBuckets` is the permission to **execute** the ListBuckets operation
- `s3:ListBucket` is the permission to **see** a specific bucket in the results
- Bucket policies use `s3:ListBucket` to control visibility

**Example Bucket Policy (Public Bucket):**
```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": "*",
    "Action": "s3:ListBucket",
    "Resource": "arn:aws:s3:::public-bucket"
  }]
}
```

**Filtering Logic:**
```go
// For anonymous user
for each bucket:
    Evaluate: s3:ListBucket on arn:aws:s3:::bucket-name
    - Check bucket policy for Principal: "*"
    - If ALLOW → include in results
    - If DENY → exclude from results

// For authenticated user (Phase 5)
for each bucket:
    Evaluate: s3:ListBucket on arn:aws:s3:::bucket-name
    - Check bucket policy for Principal: "*" (public)
    - Check bucket policy for Principal: user-arn (user-specific)
    - Check user's IAM policies for s3:ListBucket on this bucket
    - If ANY check returns ALLOW → include in results
    - If ALL checks return DENY → exclude from results
```

---

## Comparison Table

| Aspect | Binary Authorization | Result Filtering |
|--------|---------------------|------------------|
| **Operations** | GetObject, PutObject, DeleteBucket, HeadObject, etc. | ListBuckets, ListObjects, ListMultipartUploads |
| **Resource** | Specific (bucket or object) | Collection (`*` or bucket) |
| **Decision** | ALLOW or DENY entire operation | ALLOW operation, filter individual items |
| **Anonymous Access** | Usually denied (403) | Allowed, but limited results |
| **Policy Check** | Single check before handler | Multiple checks (one per item) |
| **Performance** | Fast (single evaluation) | Slower (N evaluations for N items) |
| **Implementation** | Authorization middleware | Handler or result filtering layer |

---

## Which List Operations Need Filtering?

### Priority 1: ListBuckets (Phase 3.2) 🔴
**Why critical:**
- Most common list operation
- Anonymous users need access to public buckets
- Multi-tenant scenarios require bucket isolation

**Filtering logic:**
- Check `s3:ListBucket` permission on each bucket
- Include if bucket policy allows Principal "*" OR user has explicit access

### Priority 2: ListObjects / ListObjectsV2 (Phase 3.3 or 4) 🟡
**When needed:**
- Prefix-based policies (e.g., users can only see `users/{username}/*`)
- Object-level ACLs (less common in modern S3)
- Fine-grained access control within shared buckets

**Filtering logic:**
- Check `s3:GetObject` permission on each object (or `s3:ListBucket` with conditions)
- Include if object matches user's allowed prefix patterns

**Note:** Most use cases don't need object-level filtering - bucket policy applies to all objects. This is **optional optimization** for advanced use cases.

### Priority 3: ListObjectVersions (Phase 4+) 🟢
**When needed:**
- Versioned buckets with per-version policies (rare)
- Similar to ListObjects but with version-specific permissions

### Priority 4: ListMultipartUploads (Phase 4+) 🟢
**When needed:**
- Multi-user buckets where users shouldn't see each other's uploads
- Filter by: upload initiator OR explicit object permission

### Priority 5: ListParts (Phase 5+) 🟢
**When needed:**
- Usually only upload initiator can see parts (implicit filtering)
- Could add explicit permission check for shared upload management

## Implementation Phases

### Phase 3.1 MVP - Binary Authorization Only ✅
- Implement authorization middleware with binary allow/deny
- **ListBuckets workaround**: Require authentication, return all buckets for authenticated users
- **Known limitation**: Anonymous users cannot see public buckets
- **All other List operations**: Binary authorization (can list or can't)

### Phase 3.2 - ListBuckets Result Filtering 🔴
- Implement conditional authentication (AuthMode: OPTIONAL)
- Implement generic `FilterListResults[T]()` helper
- Add result filtering in ListBuckets handler
- Anonymous users see public buckets, authenticated users see public + their buckets
- **Test thoroughly**: Anonymous, authenticated, and admin scenarios

### Phase 3.3 - ListObjects Result Filtering (Optional) 🟡
- **Only if needed** for prefix-based policies
- Reuse generic `FilterListResults[T]()` helper
- Filter objects by `s3:GetObject` or prefix-based `s3:ListBucket` conditions
- **Performance concern**: Could be slow for buckets with many objects

### Phase 4+ - Other List Operations 🟢
- Add filtering for ListObjectVersions, ListMultipartUploads, ListParts as needed
- Reuse generic filtering pattern
- Optimize performance with caching

### Phase 5 - Full IAM Support
- Extend filtering to check user's IAM policies
- Support group-based filtering
- Optimize filtering performance with batch evaluation and caching

---

## Performance Considerations

### Filtering Overhead

**Problem:** Evaluating policy for each item is expensive:

```
ListBuckets:
  100 buckets × 1 policy evaluation = 100 evaluations per request
  (Typically acceptable - most deployments have < 1000 buckets)

ListObjects:
  10,000 objects × 1 policy evaluation = 10,000 evaluations per request
  (Potentially problematic - needs optimization or pagination)
```

**Impact by Operation:**
- **ListBuckets**: Low impact (< 1000 buckets typical)
- **ListObjects**: High impact (could be millions of objects)
- **ListMultipartUploads**: Low impact (< 100 uploads typical)
- **ListObjectVersions**: Medium-high impact (depends on versioning usage)

**Optimizations:**

#### 1. Early Termination for Admin
```go
if user.IsAdmin {
    return allBuckets // Skip filtering entirely
}
```

#### 2. Cache Bucket Visibility
```go
type BucketVisibilityCache struct {
    mu sync.RWMutex
    // key: bucket-name, value: visibility level
    cache map[string]BucketVisibility
}

type BucketVisibility int
const (
    VisibilityPrivate BucketVisibility = iota
    VisibilityPublic
    VisibilityConditional // Depends on user
)

// Pre-compute public buckets
func (c *BucketVisibilityCache) IsPublic(bucket string) bool {
    // Check if bucket policy has Principal: "*" with ListBucket
}
```

#### 3. Batch Policy Evaluation (Phase 5)
```go
// Instead of 100 individual evaluations:
decisions := engine.EvaluateBatch(reqCtxs...)
```

#### 4. Limit Filtering Scope
```go
// Only filter if there are many buckets
if len(allBuckets) > 10 && !user.IsAdmin {
    return filterBuckets(allBuckets, user)
}
return allBuckets // Small number, return all
```

#### 5. Smart Filtering Decision (ListObjects)
```go
// For ListObjects: only filter if bucket has prefix-based policies
func (h *Handler) ListObjects(bucket string, user *User) []Object {
    allObjects := h.storage.ListObjects(bucket)

    // Check if bucket has object-level policies
    bucketPolicy := h.policyEngine.GetBucketPolicy(bucket)
    if !hasPrefixBasedPolicies(bucketPolicy) {
        // Bucket-level policy applies to all objects - no filtering needed
        return allObjects
    }

    // Has prefix policies - need to filter
    return FilterListResults(allObjects, user, ...)
}

func hasPrefixBasedPolicies(policy *PolicyDocument) bool {
    for _, stmt := range policy.Statement {
        for _, resource := range stmt.Resource {
            // Check if resource has wildcards: "arn:aws:s3:::bucket/prefix/*"
            if strings.Contains(resource, "*") || strings.Contains(resource, "?") {
                return true
            }
        }
    }
    return false
}
```

#### 6. Pagination-Aware Filtering (ListObjects)
```go
// Only filter the current page, not all objects
func (h *Handler) ListObjectsV2(bucket string, maxKeys int, token string, user *User) []Object {
    // Get paginated results from storage (e.g., 1000 objects)
    page := h.storage.ListObjectsPaginated(bucket, maxKeys, token)

    // Filter only this page (1000 evaluations instead of 1M)
    filtered := FilterListResults(page.Objects, user, ...)

    // If filtered results < maxKeys, fetch next page and continue
    // (Complexity: need to handle continuation tokens correctly)
    return filtered
}
```

---

## Testing Strategy

### Binary Authorization Tests
```go
// Test: GetObject with allow policy
policy := `{"Statement": [{"Effect": "Allow", "Action": "s3:GetObject", ...}]}`
response := GET("/bucket/file.txt")
assert.Equal(200, response.StatusCode)

// Test: GetObject with deny policy
policy := `{"Statement": [{"Effect": "Deny", "Action": "s3:GetObject", ...}]}`
response := GET("/bucket/file.txt")
assert.Equal(403, response.StatusCode)
```

### Result Filtering Tests
```go
// Setup: 3 buckets with different policies
CreateBucket("public-bucket") // Policy: Principal "*"
CreateBucket("user-a-bucket") // Policy: Principal user-a
CreateBucket("private-bucket") // No policy

// Test: Anonymous user sees only public
response := GET("/") // No auth
buckets := parseListBucketsResponse(response)
assert.Equal([]string{"public-bucket"}, buckets)

// Test: User A sees public + their bucket
response := GET("/").WithAuth("user-a")
buckets := parseListBucketsResponse(response)
assert.Equal([]string{"public-bucket", "user-a-bucket"}, buckets)

// Test: Admin sees all
response := GET("/").WithAuth("admin")
buckets := parseListBucketsResponse(response)
assert.Equal([]string{"public-bucket", "user-a-bucket", "private-bucket"}, buckets)
```

---

## AWS S3 Behavior Reference

### ListBuckets in AWS
- **Anonymous requests**: Returns only publicly accessible buckets (rare, usually requires specific configuration)
- **Authenticated users**: Returns buckets where user has `s3:ListBucket` permission
- **Filtering is implicit**: AWS returns only buckets the user can access, no 403 error

### ListObjects in AWS
- **Binary authorization**: Either allow or deny based on bucket policy
- **Prefix-based filtering**: Less common, usually handled at bucket policy level
- **No object-level filtering**: If you can list, you see all objects (but may not be able to GetObject)

---

## Recommendations

### Phase 3.1 MVP
1. **Keep it simple**: Use binary authorization only
2. **ListBuckets**: Require authentication, return all buckets to authenticated users
3. **Document limitation**: Anonymous users cannot list any buckets
4. **Plan for Phase 3.2**: Design handlers to support filtering later

### Phase 3.2
1. **Implement conditional auth**: Add AuthMode.OPTIONAL for ListBuckets
2. **Add filtering in handler**: Use `canUserSeeBucket()` pattern
3. **Test thoroughly**: Anonymous, authenticated, and admin users
4. **Optimize if needed**: Cache public bucket list

### Phase 5 (IAM)
1. **Extend filtering**: Include user's IAM policies in evaluation
2. **Optimize performance**: Batch evaluations, caching
3. **Support groups**: Filter based on group memberships

---

## Related Documentation

- [action-permission-mapping.md](action-permission-mapping.md) - S3 action to IAM permission translation
- [policy-engine-implementation.md](policy-engine-implementation.md) - Core policy engine design
- [TODO.md](../../TODO.md) - Conditional Auth Middleware section (Phase 3)

---

## Summary: Generic List* Filtering Pattern

### The Pattern
**All `List*` operations** (ListBuckets, ListObjects, ListObjectVersions, ListMultipartUploads, ListParts) follow the same pattern:

1. **Allow the operation** - User can execute the List* API call
2. **Filter results** - Evaluate permissions for each item in the collection
3. **Return filtered list** - Only show items the user has access to

### Generic Implementation
```go
// Reusable for ALL List* operations
filteredItems := FilterListResults(
    allItems,
    user,
    policyEngine,
    getResourceARN,   // Extract ARN from item
    getRequiredAction // What permission to check
)
```

### Priority Order
1. 🔴 **ListBuckets** (Phase 3.2) - Most critical, enables multi-tenancy
2. 🟡 **ListObjects** (Phase 3.3+) - Only if prefix-based policies needed
3. 🟢 **Other List ops** (Phase 4+) - Add as needed

### Performance Strategy
- **ListBuckets**: Direct filtering (< 1000 buckets typical)
- **ListObjects**: Smart filtering (only if prefix policies exist) + pagination
- **All**: Cache public items, admin bypass, batch evaluation

### Key Takeaway
**Result filtering is not a special case** - it's a fundamental pattern for all collection-returning operations in an authorization system. Plan for it generically, implement it incrementally.

---

## Status

- ✅ **Analysis Complete** - Both patterns documented
- ✅ **Generic Pattern Identified** - Applies to all List* operations
- ✅ **Design Complete** - Implementation strategies defined
- ✅ **Performance Strategy** - Optimizations documented
- 🚧 **Implementation Pending** - Phase 3.1 binary authorization only
- ⏳ **ListBuckets Filtering** - Planned for Phase 3.2
- ⏳ **ListObjects Filtering** - Planned for Phase 3.3+ (optional)
- ⏳ **Other List Operations** - Planned for Phase 4+ (as needed)

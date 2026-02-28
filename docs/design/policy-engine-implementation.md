# Policy Engine Foundation - Implementation Plan

## Executive Summary

Build a comprehensive Policy Engine for S3-compatible authorization to enable public bucket access and lay the groundwork for Phase 5 IAM. This implements the "HIGHEST PRIORITY" section of Phase 3 from TODO.md.

**Key Design Decision:** Tag routes with S3 action metadata at registration time (router layer), making the action available to authorization middleware without inference logic. This is cleaner than the current approach where actions are set inside handlers.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    HTTP Request                              │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  Router Middleware - Sets S3 action in context from route   │
│  (NEW - reads action from route metadata)                   │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  Auth Middleware - Validates AWS SigV4, sets User in ctx    │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  Authorization Middleware - Calls Policy Engine             │
│  (NEW - builds RequestContext, evaluates policy)            │
└────────────────────┬────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  Handler - Executes S3 operation                            │
└─────────────────────────────────────────────────────────────┘

         ┌───────────────────────────────┐
         │     Policy Engine             │
         │  - In-memory policy cache     │
         │  - Fast evaluation logic      │
         │  - No FS dependencies         │
         └────────┬──────────────────────┘
                  │
                  │ Notified of changes
                  │
         ┌────────▼──────────────────────┐
         │  Policy Service               │
         │  (existing metadata/service)  │
         │  - CRUD operations            │
         │  - Persistence to FS          │
         └───────────────────────────────┘
```

## Core Design Principles

1. **Outside-In Development**
   - **The End:** Authorization middleware API - `Evaluate(requestContext) -> Decision`
   - **The Start:** PE initialization from storage at server startup
   - **The Middle:** Runtime updates via service layer notifications

2. **Decouple PE from Storage**
   - PE is a pure in-memory evaluation engine
   - No direct FS access (makes testing easy)
   - Service layer notifies PE of policy changes

3. **Router-Based Action Tagging**
   - S3 actions tagged on routes at registration
   - Router middleware sets action in context
   - Authorization middleware reads from context (no inference)

4. **Action-to-Permission Mapping** 🔴 **CRITICAL**
   - **S3 actions ≠ IAM permissions** in many cases
   - `HeadObject` requires `s3:GetObject` permission (NOT `s3:HeadObject`)
   - `CopyObject` requires BOTH `s3:GetObject` and `s3:PutObject`
   - See [action-permission-mapping.md](action-permission-mapping.md) for complete specification
   - ActionMapper component translates route actions to required permissions

5. **Two Authorization Patterns** ⚠️ **IMPORTANT**
   - **Binary Authorization** (default): Allow or deny the entire operation
     - Used for: GetObject, PutObject, DeleteBucket, etc.
     - Middleware blocks with 403 if denied
   - **Result Filtering** (special cases): Allow operation, filter results per-item
     - Used for: ListBuckets (Phase 3.2+)
     - Handler filters results based on per-bucket permissions
     - See [authorization-patterns.md](authorization-patterns.md) for complete specification
   - **Phase 3.1 MVP**: Binary authorization only (ListBuckets requires auth, no filtering)

6. **Thread-Safe In-Memory Cache**
   - `sync.RWMutex` for concurrent access
   - Optimized for fast reads (common case)
   - Nothing in cache that's not on disk (defensive)

## Package Structure

```
internal/
├── http/
│   ├── api/
│   │   └── handler.go            # ENHANCE - Remove action setting (read from context instead)
│   ├── middleware/
│   │   └── authorization.go      # NEW - Authorization middleware
│   └── server/
│       ├── server.go             # ENHANCE - Initialize PE, load policies
│       └── routes.go             # ENHANCE - Tag routes with actions, add auth middleware
└── service/
    ├── s3/
    │   └── bucket_policy.go  # ENHANCE - Notify PE on policy changes
    └── policy/                   # NEW - Policy Engine (pure evaluation)
        ├── engine.go             # Core engine, Evaluate(), cache management
        ├── engine_test.go        # Unit tests (no FS dependencies)
        ├── types.go              # RequestContext, Decision, Action, Resource, Principal
        ├── evaluator.go          # Statement evaluation logic
        ├── matcher.go            # Action/Resource/Principal matching
        ├── action_mapper.go      # NEW - Translates S3 actions to IAM permissions
        ├── action_mapper_test.go # Tests for action mapping logic
        └── cache.go              # Thread-safe in-memory cache
```

## Implementation Details

### 1. Router Enhancement - S3 Action Metadata

This is now done in the router package - teapot-router, so no change needed here.

### 2. Context Enhancement - S3 Action Key

This is now done in the router package - teapot-router, so no change needed here.

### 3. Router Middleware - S3 Action Setter

**File:** `internal/middleware/s3action.go` (NEW)

This is now done in the router package - teapot-router, so no change needed here.

### 4. Policy Engine Types

**File:** `internal/policy/types.go` (NEW)

```go
package policy

import (
	"net/http"
	"time"

	"github.com/mallardduck/dirio/internal/metadata"
)

// RequestContext contains all information for policy evaluation
type RequestContext struct {
	Principal       *Principal
	Action          Action
	Resource        *Resource
	Conditions      *ConditionContext
	OriginalRequest *http.Request // Access to raw request if needed
}

// Principal represents the requester
type Principal struct {
	User        *metadata.User // nil for anonymous
	IsAnonymous bool
	IsAdmin     bool // Root admin bypass
}

// Action represents an S3 API operation
type Action string

// S3 Actions
const (
	// Bucket operations
	ActionListBuckets        Action = "s3:ListAllMyBuckets"
	ActionCreateBucket       Action = "s3:CreateBucket"
	ActionDeleteBucket       Action = "s3:DeleteBucket"
	ActionListBucket         Action = "s3:ListBucket"
	ActionGetBucketLocation  Action = "s3:GetBucketLocation"
	ActionGetBucketPolicy    Action = "s3:GetBucketPolicy"
	ActionPutBucketPolicy    Action = "s3:PutBucketPolicy"
	ActionDeleteBucketPolicy Action = "s3:DeleteBucketPolicy"

	// Object operations
	ActionGetObject        Action = "s3:GetObject"
	ActionPutObject        Action = "s3:PutObject"
	ActionDeleteObject     Action = "s3:DeleteObject"
	ActionHeadObject       Action = "s3:HeadObject"
	ActionCopyObject       Action = "s3:CopyObject" // Phase 3 future
	ActionGetObjectTagging Action = "s3:GetObjectTagging"
	ActionPutObjectTagging Action = "s3:PutObjectTagging"
)

// Resource represents the S3 resource being accessed
type Resource struct {
	Bucket string // Bucket name
	Key    string // Object key (empty for bucket operations)
}

// ARN returns AWS ARN format
func (r *Resource) ARN() string {
	if r.Key == "" {
		return "arn:aws:s3:::" + r.Bucket
	}
	return "arn:aws:s3:::" + r.Bucket + "/" + r.Key
}

// ConditionContext contains request metadata for condition evaluation
type ConditionContext struct {
	SourceIP        string
	UserAgent       string
	SecureTransport bool
	CurrentTime     time.Time
	// Expand in Phase 3.2 for condition operators
}

// Decision is the result of policy evaluation
type Decision int

const (
	DecisionDeny         Decision = iota // Default - no explicit allow
	DecisionAllow                        // Explicit allow found
	DecisionExplicitDeny                 // Explicit deny (highest precedence)
)

func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "Allow"
	case DecisionExplicitDeny:
		return "ExplicitDeny"
	default:
		return "Deny"
	}
}

func (d Decision) IsAllowed() bool {
	return d == DecisionAllow
}
```

### 5. Action Mapper (S3 Action → IAM Permission Translation) 🔴

**File:** `internal/policy/action_mapper.go` (NEW)

**Purpose:** Translates S3 API action names (from routes) into the actual IAM permissions required. This is a critical component because S3 action names do NOT always match permission names.

**Key Insight:** See [action-permission-mapping.md](action-permission-mapping.md) for complete specification and examples.

```go
package policy

import (
	"strings"
)

// ActionMapper translates S3 API actions to required IAM permissions
type ActionMapper struct {
	// Static mapping: action → permission(s)
	mappings map[string][]string

	// Multi-resource actions (like CopyObject)
	multiResource map[string]bool
}

// NewActionMapper creates a new action mapper with static mappings
func NewActionMapper() *ActionMapper {
	return &ActionMapper{
		mappings:      buildActionMappings(),
		multiResource: buildMultiResourceActions(),
	}
}

// GetRequiredPermissions returns the IAM permission(s) needed for an S3 action
// Examples:
//   "s3:HeadObject" → ["s3:GetObject"]
//   "s3:CopyObject" → ["s3:GetObject", "s3:PutObject"]
//   "s3:GetObject"  → ["s3:GetObject"]
func (m *ActionMapper) GetRequiredPermissions(action string) []string {
	if perms, ok := m.mappings[action]; ok {
		return perms
	}
	// Default: assume 1:1 mapping if not in table
	return []string{action}
}

// IsMultiResourceAction returns true if action requires checking multiple resources
// Example: CopyObject requires checking both source and destination
func (m *ActionMapper) IsMultiResourceAction(action string) bool {
	return m.multiResource[action]
}

// buildActionMappings creates the static action-to-permission mapping table
func buildActionMappings() map[string][]string {
	return map[string][]string{
		// Service Level
		"s3:ListBuckets": {"s3:ListAllMyBuckets"},

		// Bucket Operations - Different Names
		"s3:HeadBucket":             {"s3:ListBucket"},
		"s3:ListObjects":            {"s3:ListBucket"},
		"s3:ListObjectsV2":          {"s3:ListBucket"},
		"s3:ListObjectVersions":     {"s3:ListBucketVersions"},
		"s3:ListMultipartUploads":   {"s3:ListBucketMultipartUploads"},
		"s3:DeleteObjects":          {"s3:DeleteObject"}, // Bulk uses singular

		// Object Operations - Different Names
		"s3:HeadObject": {"s3:GetObject"},

		// Multi-Resource Operations (require TWO permissions on different resources)
		"s3:CopyObject":      {"s3:GetObject", "s3:PutObject"},
		"s3:UploadPartCopy":  {"s3:GetObject", "s3:PutObject"},

		// Multipart Upload - All use PutObject except Abort and ListParts
		"s3:CreateMultipartUpload":   {"s3:PutObject"},
		"s3:UploadPart":              {"s3:PutObject"},
		"s3:CompleteMultipartUpload": {"s3:PutObject"},
		"s3:ListParts":               {"s3:ListMultipartUploadParts"},
		// "s3:AbortMultipartUpload" is 1:1, not in map

		// All other operations are 1:1 (same name), so they don't need mapping entries
		// Examples: GetObject, PutObject, DeleteObject, CreateBucket, etc.
	}
}

// buildMultiResourceActions identifies actions that need multiple resource checks
func buildMultiResourceActions() map[string]bool {
	return map[string]bool{
		"s3:CopyObject":     true,
		"s3:UploadPartCopy": true,
	}
}

// GetResourceARNs returns the resource ARN(s) to check for an action
// Most operations: single ARN (bucket or object)
// CopyObject: [sourceARN, destARN]
func (m *ActionMapper) GetResourceARNs(action string, bucket, key, sourceKey string) []string {
	if !m.IsMultiResourceAction(action) {
		// Single resource
		if key == "" {
			return []string{"arn:aws:s3:::" + bucket}
		}
		return []string{"arn:aws:s3:::" + bucket + "/" + key}
	}

	// Multi-resource: CopyObject or UploadPartCopy
	sourceARN := "arn:aws:s3:::" + bucket + "/" + sourceKey
	destARN := "arn:aws:s3:::" + bucket + "/" + key
	return []string{sourceARN, destARN}
}
```

**Key Features:**
- Static mapping table for all non-1:1 action-to-permission translations
- Handles multi-resource operations (CopyObject needs TWO permission checks)
- Falls back to 1:1 mapping if action not in table (safe default)
- Pure function with no external dependencies (easy to test)

**Testing Strategy:**
```go
// TestActionMapper_HeadObject
mapper := NewActionMapper()
perms := mapper.GetRequiredPermissions("s3:HeadObject")
// Expected: ["s3:GetObject"]

// TestActionMapper_CopyObject
perms := mapper.GetRequiredPermissions("s3:CopyObject")
// Expected: ["s3:GetObject", "s3:PutObject"]
isMulti := mapper.IsMultiResourceAction("s3:CopyObject")
// Expected: true
```

### 6. Policy Engine Cache

**File:** `internal/policy/cache.go` (NEW)

```go
package policy

import (
	"sync"

	"github.com/mallardduck/dirio/pkg/iam"
)

// Cache holds all policies in memory for fast evaluation
type Cache struct {
	mu sync.RWMutex

	// Bucket policies: map[bucketName]*PolicyDocument
	bucketPolicies map[string]*iam.PolicyDocument

	// IAM user policies (Phase 5 - not needed for MVP)
	userPolicies map[string][]*iam.PolicyDocument
}

// NewCache creates a new policy cache
func NewCache() *Cache {
	return &Cache{
		bucketPolicies: make(map[string]*iam.PolicyDocument),
		userPolicies:   make(map[string][]*iam.PolicyDocument),
	}
}

// GetBucketPolicy retrieves bucket policy (thread-safe read)
func (c *Cache) GetBucketPolicy(bucket string) *iam.PolicyDocument {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bucketPolicies[bucket]
}

// SetBucketPolicy updates bucket policy (thread-safe write)
func (c *Cache) SetBucketPolicy(bucket string, policy *iam.PolicyDocument) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if policy == nil {
		delete(c.bucketPolicies, bucket)
	} else {
		c.bucketPolicies[bucket] = policy
	}
}

// LoadBucketPolicies replaces all bucket policies (used at startup)
func (c *Cache) LoadBucketPolicies(policies map[string]*iam.PolicyDocument) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bucketPolicies = policies
}
```

### 7. Policy Engine Core

**File:** `internal/policy/engine.go` (NEW)

```go
package policy

import (
	"context"

	"github.com/mallardduck/dirio/pkg/iam"
)

// Engine is the core policy evaluation engine
type Engine struct {
	cache *Cache
}

// New creates a new policy engine
func New() *Engine {
	return &Engine{
		cache: NewCache(),
	}
}

// Evaluate evaluates a request against all applicable policies
func (e *Engine) Evaluate(ctx context.Context, req *RequestContext) Decision {
	// Phase 3.1 MVP: Simplified logic

	// 1. Admin bypass (authenticated admin can do everything)
	if req.Principal.IsAdmin {
		return DecisionAllow
	}

	// 2. Bucket policy evaluation (if bucket specified)
	if req.Resource.Bucket != "" {
		bucketPolicy := e.cache.GetBucketPolicy(req.Resource.Bucket)
		if bucketPolicy != nil {
			decision := e.evaluatePolicy(bucketPolicy, req)
			if decision == DecisionExplicitDeny {
				return DecisionExplicitDeny // Deny always wins
			}
			if decision == DecisionAllow {
				return DecisionAllow
			}
		}
	}

	// 3. IAM user policy evaluation (Phase 5 - defer)

	// 4. Default authenticated user policy
	if !req.Principal.IsAnonymous {
		// Phase 3.1: Authenticated non-admin users denied by default
		// Phase 5: Evaluate user's attached IAM policies
		return DecisionDeny
	}

	// 5. Default deny for anonymous
	return DecisionDeny
}

// evaluatePolicy evaluates a single policy document
func (e *Engine) evaluatePolicy(policy *iam.PolicyDocument, req *RequestContext) Decision {
	hasAllow := false

	for _, stmt := range policy.Statement {
		result := e.evaluateStatement(&stmt, req)

		// Explicit deny always wins immediately
		if result == DecisionExplicitDeny {
			return DecisionExplicitDeny
		}

		// Track if we found any allows
		if result == DecisionAllow {
			hasAllow = true
		}
	}

	if hasAllow {
		return DecisionAllow
	}
	return DecisionDeny
}

// evaluateStatement evaluates a single policy statement
func (e *Engine) evaluateStatement(stmt *iam.Statement, req *RequestContext) Decision {
	// 1. Check if principal matches
	if !matchPrincipal(stmt.Principal, req.Principal) {
		return DecisionDeny
	}

	// 2. Check if action matches
	if !matchAction(stmt.Action, req.Action) {
		return DecisionDeny
	}

	// 3. Check if resource matches
	if !matchResource(stmt.Resource, req.Resource) {
		return DecisionDeny
	}

	// 4. Check conditions (Phase 3.2 - defer for MVP)

	// 5. Return effect
	if stmt.Effect == "Deny" {
		return DecisionExplicitDeny
	}
	return DecisionAllow
}

// ===== LIFECYCLE METHODS =====

// LoadBucketPolicies loads all bucket policies at startup
func (e *Engine) LoadBucketPolicies(ctx context.Context, policies map[string]*iam.PolicyDocument) {
	e.cache.LoadBucketPolicies(policies)
}

// UpdateBucketPolicy updates a single bucket policy at runtime
func (e *Engine) UpdateBucketPolicy(bucket string, policy *iam.PolicyDocument) {
	e.cache.SetBucketPolicy(bucket, policy)
}

// DeleteBucketPolicy removes a bucket policy at runtime
func (e *Engine) DeleteBucketPolicy(bucket string) {
	e.cache.SetBucketPolicy(bucket, nil)
}
```

### 8. Policy Matchers

**File:** `internal/policy/matcher.go` (NEW)

See the Plan agent's comprehensive matcher implementation for:
- `matchPrincipal()` - Handles `*` for public, map format for AWS principals
- `matchAction()` - Handles single string, array, wildcards (`s3:*`, `s3:Get*`)
- `matchResource()` - Handles ARN patterns (`arn:aws:s3:::bucket/*`)
- `matchSingleAction()` - Wildcard matching logic
- `matchSingleResource()` - ARN pattern matching logic

### 9. Authorization Middleware

**File:** `internal/middleware/authorization.go` (NEW)

**Important Change:** This middleware now uses the ActionMapper to translate S3 actions to IAM permissions before policy evaluation.

```go
package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/mallardduck/dirio/internal/auth"
	contextInt "github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/teapot-router/pkg/teapot"
)

// Authorization enforces policy-based authorization with action-to-permission mapping
func Authorization(engine *policy.Engine, mapper *policy.ActionMapper, rootAccessKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract authenticated user from context (set by auth middleware)
			user := auth.GetRequestUser(r.Context())

			// Build principal
			principal := &policy.Principal{
				User:        user,
				IsAnonymous: user == nil,
				IsAdmin:     user != nil && user.AccessKey == rootAccessKey,
			}

			// Extract S3 action from route context (set by teapot-router)
			routeAction := contextInt.GetS3Action(r.Context())

			// 🔴 CRITICAL: Translate S3 action to required IAM permission(s)
			requiredPermissions := mapper.GetRequiredPermissions(routeAction)

			// Extract resource from route params
			bucket := teapot.URLParam(r, "bucket")
			key := teapot.URLParam(r, "key")

			// Build condition context
			conditions := &policy.ConditionContext{
				SourceIP:        extractClientIP(r),
				UserAgent:       r.UserAgent(),
				SecureTransport: r.TLS != nil,
				CurrentTime:     time.Now(),
			}

			// Check if multi-resource operation (e.g., CopyObject)
			if mapper.IsMultiResourceAction(routeAction) {
				// Handle CopyObject / UploadPartCopy
				sourceKey := extractCopySource(r) // Parse x-amz-copy-source header

				// Evaluate BOTH source and destination
				sourceDecision := evaluatePermission(engine, principal, requiredPermissions[0], bucket, sourceKey, conditions, r)
				destDecision := evaluatePermission(engine, principal, requiredPermissions[1], bucket, key, conditions, r)

				if !sourceDecision.IsAllowed() || !destDecision.IsAllowed() {
					writeAccessDeniedError(w, r)
					return
				}
			} else {
				// Single resource operation
				// Use FIRST (and typically only) required permission
				permission := requiredPermissions[0]

				resource := &policy.Resource{
					Bucket: bucket,
					Key:    key,
				}

				// Build request context with MAPPED permission
				reqCtx := &policy.RequestContext{
					Principal:       principal,
					Action:          policy.Action(permission), // Use mapped permission, not route action!
					Resource:        resource,
					Conditions:      conditions,
					OriginalRequest: r,
				}

				// Evaluate policy
				decision := engine.Evaluate(r.Context(), reqCtx)

				if !decision.IsAllowed() {
					writeAccessDeniedError(w, r)
					return
				}

				// Store decision in context for logging
				ctx := context.WithValue(r.Context(), contextInt.AuthzDecisionKey, decision)
				r = r.WithContext(ctx)
			}

			// Authorization passed - proceed
			next.ServeHTTP(w, r)
		})
	}
}

// evaluatePermission is a helper for multi-resource operations
func evaluatePermission(
	engine *policy.Engine,
	principal *policy.Principal,
	permission string,
	bucket, key string,
	conditions *policy.ConditionContext,
	r *http.Request,
) policy.Decision {
	resource := &policy.Resource{
		Bucket: bucket,
		Key:    key,
	}

	reqCtx := &policy.RequestContext{
		Principal:       principal,
		Action:          policy.Action(permission),
		Resource:        resource,
		Conditions:      conditions,
		OriginalRequest: r,
	}

	return engine.Evaluate(r.Context(), reqCtx)
}

// extractCopySource parses the x-amz-copy-source header
// Example: "/source-bucket/source-key" → "source-key"
func extractCopySource(r *http.Request) string {
	copySource := r.Header.Get("X-Amz-Copy-Source")
	if copySource == "" {
		return ""
	}
	// Parse: /bucket/key or bucket/key
	copySource = strings.TrimPrefix(copySource, "/")
	parts := strings.SplitN(copySource, "/", 2)
	if len(parts) == 2 {
		return parts[1] // Return key part
	}
	return ""
}

// extractClientIP gets client IP from X-Forwarded-For or RemoteAddr
func extractClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx] // Strip port
	}
	return addr
}

// writeAccessDeniedError writes S3 AccessDenied error response
func writeAccessDeniedError(w http.ResponseWriter, r *http.Request) {
	// Use existing error response pattern from s3 package
	// TODO: Extract to shared utility
}
```

### 10. Server Integration

**File:** `internal/server/server.go`

```go
// Server struct enhancement
type Server struct {
	config       *Config
	storage      *storage.Storage
	metadata     *metadata.Manager
	auth         *auth.Authenticator
	policyEngine *policy.Engine  // NEW
	log          *slog.Logger
}

// New() enhancement
func New(config *Config) (*Server, error) {
	// ... existing code ...

	// Initialize policy engine
	policyEngine := policy.New()

	// Load all bucket policies from metadata at startup
	if err := loadBucketPoliciesIntoEngine(context.Background(), metaMgr, policyEngine); err != nil {
		log.Warn("failed to load bucket policies into policy engine", "error", err)
	}

	srv := &Server{
		config:       config,
		storage:      store,
		metadata:     metaMgr,
		auth:         authenticator,
		policyEngine: policyEngine, // NEW
		log:          log,
	}

	// ... rest of setup ...
}

// Helper to load bucket policies at startup
func loadBucketPoliciesIntoEngine(ctx context.Context, meta *metadata.Manager, engine *policy.Engine) error {
	// Iterate through .dirio/buckets/*.json
	// Extract bucket policies
	// Call engine.LoadBucketPolicies(ctx, policies)
}
```

### 11. Routes Enhancement

**File:** `internal/server/routes.go`

```go
// RouteDependencies enhancement
type RouteDependencies struct {
	Auth          *auth.Authenticator
	PolicyEngine  *policy.Engine        // NEW
	RootAccessKey string                // NEW
	APIHandler    *api.Handler
	Debug         bool
}

// SetupRoutes enhancement
func SetupRoutes(r *router.Router, deps *RouteDependencies) {
	// ... existing public routes ...

	// Authenticated routes with AUTHORIZATION
	r.MiddlewareGroup(func(r *router.Router) {
		if deps != nil {
			r.Use(deps.Auth.AuthMiddleware)
			// NEW - Authorization middleware after authentication
			r.Use(middleware.Authorization(deps.PolicyEngine, deps.RootAccessKey))
			r.Use(middleware.ChunkedEncoding(...))
		}

		// Root - ListBuckets with action tag
		r.GetWithAction("/", listBuckets, "index", "s3:ListAllMyBuckets")

		// Bucket operations with action tags
		r.HeadWithAction("/{bucket}", bucketHead, "buckets.head", "s3:ListBucket")
		r.PutWithAction("/{bucket}", bucketStore, "buckets.store", "s3:CreateBucket")
		r.GetWithAction("/{bucket}", bucketShow, "buckets.show", "s3:ListBucket")
		r.DeleteWithAction("/{bucket}", bucketDestroy, "buckets.destroy", "s3:DeleteBucket")

		// Object operations with action tags
		r.HeadWithAction("/{bucket}/*", objectHead, "objects.head", "s3:HeadObject")
		r.PutWithAction("/{bucket}/*", objectStore, "objects.create", "s3:PutObject")
		r.GetWithAction("/{bucket}/*", objectShow, "objects.show", "s3:GetObject")
		r.DeleteWithAction("/{bucket}/*", objectDestroy, "objects.destroy", "s3:DeleteObject")
	})
}
```

**IMPORTANT:** Bucket operations that check query parameters (policy, location) need special handling. Options:
1. Register multiple routes with same pattern but different query param matchers
2. Use middleware to detect query params and update action in context
3. Handler still determines final action for complex cases

**Recommended:** Hybrid approach - simple routes get action from router, complex routes (with query params) update action in handler wrapper.

### 12. Handler Enhancement

**File:** `internal/api/handler.go`

```go
// BucketResourceHandler enhancement
func (h *Handler) BucketResourceHandler() routeHandler {
	return routeHandler{
		ShowHandler: func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			requestID := middleware.GetRequestID(ctx)
			bucket := router.URLParam(r, "bucket")

			query := r.URL.Query()

			// Override S3 action for query parameter routes
			if _, ok := query["policy"]; ok {
				// Update action in context for authorization (if needed)
				ctx = context.WithValue(ctx, contextInt.S3ActionKey, "s3:GetBucketPolicy")

				// Update logging action
				if data, ok := loggingHttp.GetLogData(ctx); ok {
					data.Action = "GetBucketPolicy"
				}
				h.S3Handler.GetBucketPolicy(w, r.WithContext(ctx), bucket, requestID)
				return
			}

			// ... similar for location, list-type, etc.

			// Default: ListObjects
			// Action already set by router, just update logging
			if data, ok := loggingHttp.GetLogData(ctx); ok {
				data.Action = "ListObjects"
			}
			h.S3Handler.ListObjects(w, r, bucket, requestID)
		},
		// ... other handlers ...
	}
}
```

**NOTE:** For MVP, handlers that check query params need to update action in context. In the future, we could use query param matchers in the router.

### 13. Service Layer Notifications

**File:** `internal/service/s3/bucket_policy.go` (or equivalent bucket service layer)

```go
// Service struct enhancement
type Service struct {
	storage      *storage.Storage
	metadata     *metadata.Manager
	policyEngine *policy.Engine // NEW - optional, injected from server
}

// PutBucketPolicy enhancement
func (s *Service) PutBucketPolicy(ctx context.Context, req *PutBucketPolicyRequest) error {
	// ... existing validation and persistence ...

	if err := s.metadata.SetBucketPolicy(ctx, req.Bucket, req.PolicyDocument); err != nil {
		return err
	}

	// NEW - Notify policy engine of change
	if s.policyEngine != nil {
		s.policyEngine.UpdateBucketPolicy(req.Bucket, req.PolicyDocument)
	}

	return nil
}

// DeleteBucketPolicy enhancement
func (s *Service) DeleteBucketPolicy(ctx context.Context, bucket string) error {
	if err := s.metadata.DeleteBucketPolicy(ctx, bucket); err != nil {
		return err
	}

	// NEW - Notify policy engine of deletion
	if s.policyEngine != nil {
		s.policyEngine.DeleteBucketPolicy(bucket)
	}

	return nil
}
```

## Phase 3.1 MVP Scope

**What to Build:**

1. ✅ Core types (`types.go`) - RequestContext, Decision, Action, Resource, Principal
2. ✅ Thread-safe cache (`cache.go`) - Bucket policies only
3. 🔴 **Action mapper** (`action_mapper.go`) - Translates S3 actions to IAM permissions
4. ✅ Basic evaluation (`engine.go`, `evaluator.go`) - Admin bypass + bucket policy
5. ✅ Simple matching (`matcher.go`) - Action/Resource/Principal (no conditions)
6. ✅ Authorization middleware (`authorization.go`) - Policy enforcement WITH action mapping
7. ✅ Router enhancement (`router.go`) - S3 action metadata (DONE via teapot-router)
8. ✅ Server integration (`server.go`) - PE initialization, policy loading
9. ✅ Routes wiring (`routes.go`) - Add authorization middleware
10. ✅ Service notifications (`bucket_policy.go`) - Notify PE on changes

**What to Defer:**

- ❌ Condition evaluation (IpAddress, StringEquals, etc.)
- ❌ IAM user policy evaluation (Phase 5)
- ❌ Advanced ARN wildcards
- ❌ NotAction, NotResource, NotPrincipal
- ❌ Policy variables (`${aws:username}`)

## Implementation Order

**Step-by-step build sequence:**

1. **Router enhancement** (`internal/router/router.go`) ✅ **DONE**
   - teapot-router already supports `.Action()` metadata on routes
   - All routes in `routes.go` already tagged with S3 actions

2. **Context enhancement** (`internal/context/context.go`)
   - Add `S3ActionKey` and `AuthzDecisionKey` constants
   - Add `GetS3Action()` helper

3. **Policy types** (`internal/policy/types.go`)
   - Define all types: RequestContext, Principal, Action, Resource, Decision
   - Define action constants
   - Write ARN() method

4. 🔴 **Action mapper** (`internal/policy/action_mapper.go`) **CRITICAL - NEW**
   - Implement ActionMapper struct with static mapping table
   - Implement GetRequiredPermissions() - translates action to permission(s)
   - Implement IsMultiResourceAction() - identifies CopyObject, etc.
   - Implement GetResourceARNs() - handles multi-resource operations
   - Write comprehensive unit tests covering:
     - 1:1 same name (GetObject → s3:GetObject)
     - 1:1 different name (HeadObject → s3:GetObject)
     - 1:many (CopyObject → [s3:GetObject, s3:PutObject])
     - Multipart operations (CreateMultipartUpload → s3:PutObject)

5. **Policy cache** (`internal/policy/cache.go`)
   - Implement Cache struct with RWMutex
   - Implement Get/Set/Load methods
   - Write unit tests for thread-safety

6. **Policy matcher** (`internal/policy/matcher.go`)
   - Implement matchPrincipal (handle `*` only for MVP)
   - Implement matchAction with wildcards
   - Implement matchResource with ARN patterns
   - Write comprehensive unit tests

7. **Policy evaluator** (`internal/policy/evaluator.go`)
   - Implement evaluateStatement
   - Implement evaluatePolicy
   - Write unit tests

8. **Policy engine** (`internal/policy/engine.go`)
   - Implement Engine struct with cache
   - Implement Evaluate() with admin bypass + bucket policy
   - Implement LoadBucketPolicies, UpdateBucketPolicy
   - Write unit tests for full evaluation flow

9. **Authorization middleware** (`internal/middleware/authorization.go`) 🔴 **UPDATED**
   - Create ActionMapper instance
   - Extract action from route context
   - Use ActionMapper to translate to permission(s)
   - Handle multi-resource operations (CopyObject)
   - Build RequestContext with MAPPED permission
   - Call policy engine
   - Return 403 on deny

10. **Server integration** (`internal/server/server.go`)
    - Add PolicyEngine to Server struct
    - Create policy engine in New()
    - Create ActionMapper instance
    - Load bucket policies at startup
    - Pass policy engine, action mapper, and root access key to routes

11. **Routes wiring** (`internal/server/routes.go`)
    - Add PolicyEngine and ActionMapper to RouteDependencies
    - All routes already tagged with S3 actions (teapot-router)
    - Insert Authorization middleware after Auth

12. **Handler cleanup** (`internal/api/handler.go`)
    - Remove action setting for simple routes (read from route context)
    - Keep action override for query param routes if needed (policy, location)

13. **Service notifications** (`internal/service/bucket/bucket.go` or similar)
    - Add PolicyEngine to Service struct
    - Notify on PutBucketPolicy
    - Notify on DeleteBucketPolicy

14. **Integration tests**
    - Test admin bypass
    - Test action mapping (HeadObject works with GetObject policy)
    - Test multi-resource (CopyObject requires both permissions)
    - Test public bucket read policy
    - Test policy updates at runtime
    - Test anonymous access denied by default

## Critical Files

**Files to Create:**
- `internal/policy/types.go`
- `internal/policy/cache.go`
- 🔴 `internal/policy/action_mapper.go` **NEW - CRITICAL**
- 🔴 `internal/policy/action_mapper_test.go` **NEW - CRITICAL**
- `internal/policy/matcher.go`
- `internal/policy/evaluator.go`
- `internal/policy/engine.go`
- `internal/policy/engine_test.go`
- `internal/middleware/authorization.go`

**Files to Modify:**
- ✅ `internal/router/router.go` - **DONE** (teapot-router already supports actions)
- `internal/context/context.go` - Add S3ActionKey
- `internal/server/server.go` - Initialize PE, ActionMapper, load policies
- ✅ `internal/server/routes.go` - **DONE** (routes already tagged with actions)
- `internal/middleware/authorization.go` - **UPDATE** to use ActionMapper
- `internal/api/handler.go` - Cleanup action setting (optional)
- `internal/service/bucket/` - Notify PE on policy changes

## Testing Strategy

**Unit Tests (No FS Dependencies):**
- 🔴 **Action mapping** - Comprehensive tests for all mapping types:
  - 1:1 same name (GetObject → s3:GetObject)
  - 1:1 different name (HeadObject → s3:GetObject, HeadBucket → s3:ListBucket)
  - 1:many (CopyObject → [s3:GetObject, s3:PutObject])
  - Multipart operations (CreateMultipartUpload → s3:PutObject)
  - Multi-resource detection (IsMultiResourceAction)
- Policy matching logic (wildcards, ARNs)
- Policy evaluation (allow/deny precedence)
- Thread-safety of cache

**Integration Tests:**
- Admin bypass works
- 🔴 **Action mapping works end-to-end**:
  - HeadObject request allowed with GetObject policy
  - CopyObject requires both source GetObject and dest PutObject policies
  - ListObjectsV2 works with ListBucket policy
- Public bucket policy allows anonymous reads
- Policy updates reflected immediately
- Anonymous access denied by default

## Verification

**How to test end-to-end:**

### Basic Authorization Flow
1. **Start server** with test data directory
2. **Create bucket** as admin
3. **Upload object** as admin
4. **Verify anonymous access denied** (GET returns 403)
5. **Set public read policy** on bucket
6. **Verify anonymous access allowed** (GET returns 200)
7. **Delete bucket policy**
8. **Verify anonymous access denied again** (GET returns 403)

### Action Mapping Verification 🔴
9. **Test HeadObject with GetObject policy**:
   ```json
   Policy: { "Action": "s3:GetObject", "Resource": "arn:aws:s3:::bucket/*" }
   Request: HEAD /bucket/file.txt (route action: s3:HeadObject)
   Expected: ActionMapper translates to s3:GetObject → ALLOW
   ```

10. **Test CopyObject with dual permissions**:
    ```json
    Policy 1: { "Action": "s3:GetObject", "Resource": "arn:aws:s3:::bucket-a/*" }
    Policy 2: { "Action": "s3:PutObject", "Resource": "arn:aws:s3:::bucket-b/*" }
    Request: PUT /bucket-b/newfile.txt with X-Amz-Copy-Source: /bucket-a/oldfile.txt
    Expected: ALLOW (both permissions granted)

    Request: PUT /bucket-b/newfile.txt with X-Amz-Copy-Source: /bucket-c/oldfile.txt
    Expected: DENY (no GetObject on bucket-c)
    ```

11. **Test ListObjectsV2 with ListBucket policy**:
    ```json
    Policy: { "Action": "s3:ListBucket", "Resource": "arn:aws:s3:::bucket" }
    Request: GET /bucket?list-type=2 (route action: s3:ListObjectsV2)
    Expected: ActionMapper translates to s3:ListBucket → ALLOW
    ```

## Success Criteria

- ✅ Policy Engine evaluates bucket policies correctly
- 🔴 **Action Mapper correctly translates all S3 actions to IAM permissions**
- 🔴 **HeadObject requests work with GetObject policies**
- 🔴 **CopyObject enforces dual-permission checks (source + destination)**
- ✅ Admin can access everything (bypass)
- ✅ Anonymous users can read from public buckets
- ✅ Anonymous users denied from private buckets
- ✅ Policy changes reflected immediately (no restart needed)
- ✅ Thread-safe concurrent access
- ✅ No FS dependencies in policy engine (pure in-memory)

## Future Enhancements (Phase 3.2+)

- Condition evaluation support (IpAddress, StringEquals, etc.)
- Conditional permissions based on request headers (x-amz-acl, x-amz-tagging, etc.)
- Pre-signed URL policy validation
- Version-aware permission evaluation (GetObjectVersion vs GetObject)
- Range request authorization
- KMS encryption permission checks (kms:Decrypt, kms:GenerateDataKey)
- IAM user policy evaluation (Phase 5)

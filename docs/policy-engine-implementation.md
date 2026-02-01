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

4. **Thread-Safe In-Memory Cache**
   - `sync.RWMutex` for concurrent access
   - Optimized for fast reads (common case)
   - Nothing in cache that's not on disk (defensive)

## Package Structure

```
internal/
├── middleware/
│   ├── authorization.go      # NEW - Authorization middleware
│   └── s3action.go           # NEW - Router middleware to set action from route
├── router/
│   └── router.go             # ENHANCE - Add S3Action field to RouteInfo
├── context/
│   └── context.go            # ENHANCE - Add S3ActionKey constant
├── api/
│   └── handler.go            # ENHANCE - Remove action setting (read from context instead)
├── server/
│   ├── server.go             # ENHANCE - Initialize PE, load policies
│   └── routes.go             # ENHANCE - Tag routes with actions, add auth middleware
└── service/
    ├── s3/
    │   └── bucket_policy.go  # ENHANCE - Notify PE on policy changes
    └── policy/                   # NEW - Policy Engine (pure evaluation)
        ├── engine.go             # Core engine, Evaluate(), cache management
        ├── engine_test.go        # Unit tests (no FS dependencies)
        ├── types.go              # RequestContext, Decision, Action, Resource, Principal
        ├── evaluator.go          # Statement evaluation logic
        ├── matcher.go            # Action/Resource/Principal matching
        └── cache.go              # Thread-safe in-memory cache
```

## Implementation Details

### 1. Router Enhancement - S3 Action Metadata

**File:** `internal/router/router.go`

Add S3 action metadata to routes:

```go
type RouteInfo struct {
	Method   string `json:"method"`
	Pattern  string `json:"pattern"`
	S3Action string `json:"s3_action,omitempty"` // NEW - S3 action name
}

// register enhancement - accept s3Action parameter
func (r *Router) register(name, pattern, method, s3Action string) {
	// ... existing code ...

	r.routes[key] = RouteInfo{
		Method:   method,
		Pattern:  fullPattern,
		S3Action: s3Action, // NEW
	}
}

// GetWithAction registers a GET route with S3 action metadata
func (r *Router) GetWithAction(pattern string, handler http.HandlerFunc, name, s3Action string) {
	r.register(name, pattern, "GET", s3Action)
	if handler != nil {
		r.mux.Get(r.currentPath()+pattern, handler)
	}
}

// Similar for Put, Post, Delete, Head with action parameter
```

### 2. Context Enhancement - S3 Action Key

**File:** `internal/context/context.go`

```go
const (
	RequestUserKey      KeyID = "requestUser"
	RequestIDKey        KeyID = "requestID"
	RequestStartTimeKey KeyID = "requestStartTime"
	TraceIDKey          KeyID = "traceID"
	S3ActionKey         KeyID = "s3Action"         // NEW
	AuthzDecisionKey    KeyID = "authzDecision"    // NEW - for logging
)

// GetS3Action retrieves the S3 action from context
func GetS3Action(ctx context.Context) string {
	if action, ok := ctx.Value(S3ActionKey).(string); ok {
		return action
	}
	return ""
}
```

### 3. Router Middleware - S3 Action Setter

**File:** `internal/middleware/s3action.go` (NEW)

```go
package middleware

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	contextInt "github.com/mallardduck/dirio/internal/context"
)

// S3Action middleware extracts S3 action from matched route and stores in context
// This must run AFTER routing but BEFORE authorization
func S3Action() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get the route context from chi router
			rctx := chi.RouteContext(r.Context())
			if rctx == nil {
				// No route matched, continue without action
				next.ServeHTTP(w, r)
				return
			}

			// Extract S3 action from route pattern metadata
			// The action is stored when routes are registered
			// For now, we need to map route patterns to actions
			// TODO: This needs route-level metadata storage in chi

			// Store in context
			ctx := context.WithValue(r.Context(), contextInt.S3ActionKey, action)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

**NOTE:** Chi router doesn't natively support custom route metadata. We have two options:
1. **Build our own route pattern → action mapping** in the middleware
2. **Enhance our router wrapper** to store action in a separate map and look it up

**Recommended:** Option 2 - add `GetRouteAction(pattern, method)` to router package.

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

### 5. Policy Engine Cache

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

### 6. Policy Engine Core

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

### 7. Policy Matchers

**File:** `internal/policy/matcher.go` (NEW)

See the Plan agent's comprehensive matcher implementation for:
- `matchPrincipal()` - Handles `*` for public, map format for AWS principals
- `matchAction()` - Handles single string, array, wildcards (`s3:*`, `s3:Get*`)
- `matchResource()` - Handles ARN patterns (`arn:aws:s3:::bucket/*`)
- `matchSingleAction()` - Wildcard matching logic
- `matchSingleResource()` - ARN pattern matching logic

### 8. Authorization Middleware

**File:** `internal/middleware/authorization.go` (NEW)

```go
package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/mallardduck/dirio/internal/auth"
	contextInt "github.com/mallardduck/dirio/internal/context"
	"github.com/mallardduck/dirio/internal/policy"
	"github.com/mallardduck/dirio/internal/router"
	"github.com/mallardduck/dirio/pkg/s3types"
)

// Authorization enforces policy-based authorization
func Authorization(engine *policy.Engine, rootAccessKey string) func(http.Handler) http.Handler {
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

			// Extract action from context (set by router middleware)
			actionStr := contextInt.GetS3Action(r.Context())
			action := policy.Action(actionStr)

			// Extract resource from route params
			bucket := router.URLParam(r, "bucket")
			key := router.URLParam(r, "*")
			resource := &policy.Resource{
				Bucket: bucket,
				Key:    key,
			}

			// Build condition context
			conditions := &policy.ConditionContext{
				SourceIP:        extractClientIP(r),
				UserAgent:       r.UserAgent(),
				SecureTransport: r.TLS != nil,
				CurrentTime:     time.Now(),
			}

			// Build request context
			reqCtx := &policy.RequestContext{
				Principal:       principal,
				Action:          action,
				Resource:        resource,
				Conditions:      conditions,
				OriginalRequest: r,
			}

			// Evaluate policy
			decision := engine.Evaluate(r.Context(), reqCtx)

			if !decision.IsAllowed() {
				// Return 403 Forbidden
				writeAccessDeniedError(w, r)
				return
			}

			// Store decision in context for logging
			ctx := context.WithValue(r.Context(), contextInt.AuthzDecisionKey, decision)

			// Authorization passed - proceed
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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

### 9. Server Integration

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

### 10. Routes Enhancement

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

### 11. Handler Enhancement

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

### 12. Service Layer Notifications

**File:** `internal/service/s3/bucket_policy.go`

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
3. ✅ Basic evaluation (`engine.go`, `evaluator.go`) - Admin bypass + bucket policy
4. ✅ Simple matching (`matcher.go`) - Action/Resource/Principal (no conditions)
5. ✅ Authorization middleware (`authorization.go`) - Policy enforcement
6. ✅ Router enhancement (`router.go`) - S3 action metadata
7. ✅ Server integration (`server.go`) - PE initialization, policy loading
8. ✅ Routes wiring (`routes.go`) - Add authorization middleware
9. ✅ Service notifications (`bucket_policy.go`) - Notify PE on changes

**What to Defer:**

- ❌ Condition evaluation (IpAddress, StringEquals, etc.)
- ❌ IAM user policy evaluation (Phase 5)
- ❌ Advanced ARN wildcards
- ❌ NotAction, NotResource, NotPrincipal
- ❌ Policy variables (`${aws:username}`)

## Implementation Order

**Step-by-step build sequence:**

1. **Router enhancement** (`internal/router/router.go`)
   - Add `S3Action` field to `RouteInfo`
   - Add `*WithAction()` methods (GetWithAction, etc.)
   - Update `register()` to accept s3Action parameter

2. **Context enhancement** (`internal/context/context.go`)
   - Add `S3ActionKey` and `AuthzDecisionKey` constants
   - Add `GetS3Action()` helper

3. **Policy types** (`internal/policy/types.go`)
   - Define all types: RequestContext, Principal, Action, Resource, Decision
   - Define action constants
   - Write ARN() method

4. **Policy cache** (`internal/policy/cache.go`)
   - Implement Cache struct with RWMutex
   - Implement Get/Set/Load methods
   - Write unit tests for thread-safety

5. **Policy matcher** (`internal/policy/matcher.go`)
   - Implement matchPrincipal (handle `*` only for MVP)
   - Implement matchAction with wildcards
   - Implement matchResource with ARN patterns
   - Write comprehensive unit tests

6. **Policy evaluator** (`internal/policy/evaluator.go`)
   - Implement evaluateStatement
   - Implement evaluatePolicy
   - Write unit tests

7. **Policy engine** (`internal/policy/engine.go`)
   - Implement Engine struct with cache
   - Implement Evaluate() with admin bypass + bucket policy
   - Implement LoadBucketPolicies, UpdateBucketPolicy
   - Write unit tests for full evaluation flow

8. **Authorization middleware** (`internal/middleware/authorization.go`)
   - Extract action from context
   - Extract resource from route params
   - Build RequestContext
   - Call policy engine
   - Return 403 on deny

9. **Server integration** (`internal/server/server.go`)
   - Add PolicyEngine to Server struct
   - Create policy engine in New()
   - Load bucket policies at startup
   - Pass root access key to routes

10. **Routes wiring** (`internal/server/routes.go`)
    - Add PolicyEngine to RouteDependencies
    - Update route registration with action tags
    - Insert Authorization middleware after Auth

11. **Handler cleanup** (`internal/api/handler.go`)
    - Remove action setting for simple routes
    - Keep action override for query param routes (policy, location)

12. **Service notifications** (`internal/service/s3/bucket_policy.go`)
    - Add PolicyEngine to Service struct
    - Notify on PutBucketPolicy
    - Notify on DeleteBucketPolicy

13. **Integration tests**
    - Test admin bypass
    - Test public bucket read policy
    - Test policy updates at runtime
    - Test anonymous access denied by default

## Critical Files

**Files to Create:**
- `internal/policy/types.go`
- `internal/policy/cache.go`
- `internal/policy/matcher.go`
- `internal/policy/evaluator.go`
- `internal/policy/engine.go`
- `internal/policy/engine_test.go`
- `internal/middleware/authorization.go`

**Files to Modify:**
- `internal/router/router.go` - Add S3Action metadata
- `internal/context/context.go` - Add S3ActionKey
- `internal/server/server.go` - Initialize PE, load policies
- `internal/server/routes.go` - Tag routes, add middleware
- `internal/api/handler.go` - Cleanup action setting
- `internal/service/s3/bucket_policy.go` - Notify PE

## Testing Strategy

**Unit Tests (No FS Dependencies):**
- Policy matching logic (wildcards, ARNs)
- Policy evaluation (allow/deny precedence)
- Thread-safety of cache

**Integration Tests:**
- Admin bypass works
- Public bucket policy allows anonymous reads
- Policy updates reflected immediately
- Anonymous access denied by default

## Verification

**How to test end-to-end:**

1. **Start server** with test data directory
2. **Create bucket** as admin
3. **Upload object** as admin
4. **Verify anonymous access denied** (GET returns 403)
5. **Set public read policy** on bucket
6. **Verify anonymous access allowed** (GET returns 200)
7. **Delete bucket policy**
8. **Verify anonymous access denied again** (GET returns 403)

## Success Criteria

- ✅ Policy Engine evaluates bucket policies correctly
- ✅ Admin can access everything (bypass)
- ✅ Anonymous users can read from public buckets
- ✅ Anonymous users denied from private buckets
- ✅ Policy changes reflected immediately (no restart needed)
- ✅ Thread-safe concurrent access
- ✅ No FS dependencies in policy engine (pure in-memory)

## Future Enhancements (Phase 3.2+)

- Condition evaluation support
- Pre-signed URL policy validation
- CopyObject dual-permission checks
- Range request authorization
- IAM user policy evaluation (Phase 5)

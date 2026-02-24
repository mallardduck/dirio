package policy

import (
	"maps"
	"sync"

	"github.com/mallardduck/dirio/pkg/iam"
)

// Cache holds all policies in memory for fast evaluation.
// It provides thread-safe access to bucket policies using sync.RWMutex.
//
// Design principles:
// - Optimized for fast reads (common case)
// - Write operations are rare (policy changes)
// - Nothing in cache that's not on disk (defensive)
type Cache struct {
	mu sync.RWMutex

	// Bucket policies: map[bucketName]*PolicyDocument
	bucketPolicies map[string]*iam.PolicyDocument

	// IAM user policies: map[username][]*PolicyDocument
	// Phase 5 - not needed for MVP
	userPolicies map[string][]*iam.PolicyDocument
}

// NewCache creates a new empty policy cache
func NewCache() *Cache {
	return &Cache{
		bucketPolicies: make(map[string]*iam.PolicyDocument),
		userPolicies:   make(map[string][]*iam.PolicyDocument),
	}
}

// GetBucketPolicy retrieves the bucket policy for a specific bucket.
// Returns nil if no policy is set for the bucket.
// Thread-safe for concurrent reads.
func (c *Cache) GetBucketPolicy(bucket string) *iam.PolicyDocument {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.bucketPolicies[bucket]
}

// SetBucketPolicy updates or removes a bucket policy.
// Pass nil to remove the policy for a bucket.
// Thread-safe for writes.
func (c *Cache) SetBucketPolicy(bucket string, policy *iam.PolicyDocument) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if policy == nil {
		delete(c.bucketPolicies, bucket)
	} else {
		c.bucketPolicies[bucket] = policy
	}
}

// LoadBucketPolicies replaces all bucket policies at once.
// Used at server startup to bulk-load policies from storage.
// Thread-safe for writes.
func (c *Cache) LoadBucketPolicies(policies map[string]*iam.PolicyDocument) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bucketPolicies = policies
}

// HasBucketPolicy checks if a bucket has a policy set.
// Thread-safe for concurrent reads.
func (c *Cache) HasBucketPolicy(bucket string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.bucketPolicies[bucket]
	return exists
}

// GetAllBucketPolicies returns a copy of all bucket policies.
// Used for debugging and testing.
// Thread-safe for reads.
func (c *Cache) GetAllBucketPolicies() map[string]*iam.PolicyDocument {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]*iam.PolicyDocument, len(c.bucketPolicies))
	maps.Copy(result, c.bucketPolicies)
	return result
}

// BucketPolicyCount returns the number of bucket policies in cache.
// Thread-safe for reads.
func (c *Cache) BucketPolicyCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.bucketPolicies)
}

// ============================================================
// IAM User Policies (Phase 5)
// ============================================================

// GetUserPolicies retrieves all policies attached to a user.
// Returns nil if user has no policies attached.
// Thread-safe for concurrent reads.
func (c *Cache) GetUserPolicies(username string) []*iam.PolicyDocument {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userPolicies[username]
}

// SetUserPolicies sets all policies for a user.
// Pass nil or empty slice to remove all policies for a user.
// Thread-safe for writes.
func (c *Cache) SetUserPolicies(username string, policies []*iam.PolicyDocument) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(policies) == 0 {
		delete(c.userPolicies, username)
	} else {
		c.userPolicies[username] = policies
	}
}

// LoadUserPolicies replaces all user policies at once.
// Used at server startup to bulk-load policies from storage.
// Thread-safe for writes.
func (c *Cache) LoadUserPolicies(policies map[string][]*iam.PolicyDocument) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userPolicies = policies
}

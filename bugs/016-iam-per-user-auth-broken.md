# Bug #016: IAM Per-User Access Control Not Enforced Correctly

**Status:** Open
**Priority:** Critical
**Discovered:** 2026-02-23
**Affects:** All IAM user access control — authenticated non-admin users

## Summary

IAM user policies are not being enforced correctly for authenticated non-admin users.
After importing MinIO data, alice (who should have read-write access to `alpha` only) is
denied access to `alpha` and incorrectly granted access to `beta`. The observed behaviour
is consistent with IAM policy evaluation being skipped or failing, leaving bucket policy
as the only gate — which grants access to public buckets and denies access to private ones.

## Evidence

```
━━━ 7. Per-User Auth — alice and bob ━━━
  ✗ alice: HeadObject alpha/alice-object.bin → denied (should be allowed)
  ✓ bob: HeadObject beta/bob-object.bin → allowed
  ✗ alice: HeadObject beta/bob-object.bin → allowed (should be denied)
  ✓ bob: HeadObject alpha/alice-object.bin → denied (correct ✓)
```

The symmetric result for bob (correct) and alice (inverted) is notable: bob's correct
behaviour may be coincidental — `beta` is a public-read bucket, so bob's access to his
own bucket is granted by the bucket policy rather than by IAM.

Cross-checking with anonymous access results:
- `beta` is public-read → anonymous GET returns 200
- `alpha` is private → anonymous GET returns 403

Alice getting 200 on `beta` and 403 on `alpha` matches exactly what an unauthenticated
(or zero-policy) user would see. This strongly suggests alice's IAM policies are silently
not being applied.

## Reproduction Steps

1. Import MinIO 2019 dataset with alice/bob users and their policies
2. Run: `aws --endpoint-url http://localhost:9000 s3api head-object \`
   `  --bucket alpha --key alice-object.bin` with alice credentials
3. **Expected:** HTTP 200 (alice's `alpha-rw` IAM policy grants access)
4. **Actual:** HTTP 403 (access denied, as if alice has no policies)

## Root Cause Analysis

The policy engine evaluation order is:
1. Admin bypass
2. **Bucket policy** — explicit deny wins; explicit allow returns immediately
3. IAM user policies
4. Ownership check
5. Default deny

`internal/policy/engine.go:Evaluate` (line 52)

For alice accessing `alpha`:
- Step 2: `alpha` has no bucket policy allowing alice → `DecisionDeny` (not explicit deny)
- Step 3: Should evaluate alice's `alpha-rw` IAM policy and return `DecisionAllow`
- Step 5: Falls through to default deny if step 3 fails

For alice accessing `beta`:
- Step 2: `beta` has a public-read bucket policy with `Principal: "*"` → `DecisionAllow`
  **returns immediately**, bypassing IAM checks entirely

### Likely causes for IAM step failing for alice on alpha:

**Theory A: Bucket policy explicit deny overrides IAM**
If the `alpha` bucket policy (imported from MinIO's "private" setting) contains an explicit
`Deny` statement with `Principal: "*"`, step 2 returns `DecisionExplicitDeny` immediately
and IAM is never evaluated. This is technically correct AWS behaviour but may not be what
the MinIO setup intended.

**Theory B: IAM policy resolver lookup fails silently**
`engine.go:76`:
```go
for _, name := range policyNames {
    doc, err := e.resolver.GetPolicyDocument(ctx, name)
    if err != nil {
        continue // skip missing or inaccessible policies
    }
```
If `GetPolicyDocument` returns an error for alice's `alpha-rw` policy (e.g., wrong key
format, case mismatch, or the policy wasn't imported correctly), it is silently skipped,
leaving alice with no effective policies.

**Theory C: AttachedPolicies not populated on user struct at auth time**
The user struct retrieved during auth may not have `AttachedPolicies` populated, so
`resolveEffectivePolicyNames` returns an empty slice.

`engine.go:190`:
```go
names := make([]string, len(principal.User.AttachedPolicies))
copy(names, principal.User.AttachedPolicies)
```

If `principal.User.AttachedPolicies` is nil/empty, no IAM policies are evaluated.

## Impact

**Functionality:**
- IAM-based access control is effectively non-functional
- Any user whose bucket is private cannot access their own data
- Any user can access objects in public-read buckets regardless of IAM restrictions
- This is a security issue: users can access resources they should not have access to

**Clients Affected:**
- ❌ All AWS SDK clients authenticating as non-admin IAM users
- ❌ AWS CLI with per-user credentials
- ✅ Admin (minioadmin) — bypasses all policy checks, unaffected

**Workarounds:**
- None — the feature is broken

## Proposed Investigation Steps

1. **Log alice's user struct at auth time** — check if `AttachedPolicies` is populated
   after import. Print `principal.User.AttachedPolicies` in `engine.go:resolveEffectivePolicyNames`.

2. **Check the alpha bucket policy** — inspect the imported bucket policy JSON for alpha
   to determine if it contains an explicit deny. If it does, that is the root cause.

3. **Trace the resolver** — add logging to `resolver.go:GetPolicyDocument` to confirm
   whether the lookup for `alpha-rw` succeeds or errors.

4. **Check policy name case** — MinIO policy names may be stored with different casing
   than expected by the resolver.

## Testing

Confirmed in: `scripts/validate-setup.sh` (section 7)

```bash
AWS_ACCESS_KEY_ID=alice AWS_SECRET_ACCESS_KEY=alicepass1234 \
  aws --endpoint-url http://localhost:9000 s3api head-object \
  --bucket alpha --key alice-object.bin
# Returns 403 — should return 200
```

## Related Issues

- Bug #010: Presigned URLs 403 — may share the same IAM evaluation path
- `internal/policy/engine.go:52` — Evaluate() is the main entry point
- `internal/policy/resolver.go` — GetPolicyDocument lookup to investigate

## Files to Investigate

- `internal/policy/engine.go:52` — `Evaluate()`, especially step 2 explicit deny path
- `internal/policy/resolver.go` — `GetPolicyDocument` implementation
- `internal/persistence/metadata/import.go:97` — user import, `AttachedPolicies` field
- `internal/minio/import.go:261` — `importUserPolicyMappings`, check policy attachment
- `internal/http/auth/middleware.go` — user struct population at auth time
- The alpha bucket's imported policy JSON (in DirIO metadata after import)

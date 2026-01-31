# DirIO Known Bugs & Issues

This directory documents all confirmed bugs and partially working features in DirIO.

## Bug Tracking System

Each bug is documented in a separate file with the naming convention:
- `{number}-{short-name}.md`
- Example: `001-chunked-encoding-corruption.md`

## Bug Status Legend

- 🚨 **CRITICAL** - Blocks production use, causes data corruption
- ⚠️ **HIGH** - Breaks major functionality, client compatibility issues
- 📌 **MEDIUM** - Feature doesn't work as expected, workaround available
- 📝 **LOW** - Minor issue, cosmetic, or edge case

## Active Bugs

### 🚨 Critical (Data Corruption)

| ID | Title | File | Discovered | Status |
|----|-------|------|------------|--------|
| 001 | AWS SigV4 Chunked Encoding Corruption | [001-chunked-encoding-corruption.md](001-chunked-encoding-corruption.md) | 2026-01-31 | Open |

**Impact:** All write operations store corrupted data with encoding headers. Affects PutObject, multipart uploads, object tagging.

### ⚠️ High Priority (Client Compatibility)

| ID | Title | Impact | Discovered | Status |
|----|-------|--------|------------|--------|
| 002 | DeleteObject returns 405 for MinIO mc | mc rm command fails | 2026-01-31 | Open |
| 003 | DeleteBucket returns 405 for MinIO mc | mc rb command fails | 2026-01-31 | Open |
| 004 | Object Tagging stores tags as content | Tags replace object data | 2026-01-31 | Open (root cause: #001) |
| 005 | Multipart Upload content corruption | Extra 14KB in downloaded files | 2026-01-31 | Open (root cause: #001) |

### 📌 Medium Priority (Feature Incomplete)

| ID | Title | Impact | Discovered | Status |
|----|-------|--------|------------|--------|
| 006 | ListObjectsV2 delimiter (boto3) | Returns 0 CommonPrefixes | 2026-01-31 | Open |
| 007 | ListObjectsV2 max-keys ignored | Returns all objects instead of limit | 2026-01-31 | Open |
| 008 | Range requests not implemented | Returns full content | 2026-01-31 | Open |
| 009 | CopyObject creates 0-byte files | Server-side copy fails | 2026-01-31 | Open |
| 010 | Pre-signed URLs return 403 | Temporary access sharing broken | 2026-01-31 | Open |
| 011 | Custom metadata key case wrong | Returns Title-Case instead of lowercase | 2026-01-31 | Open |
| 012 | Custom metadata not returned in mc | mc stat doesn't show metadata | 2026-01-31 | Open |
| 013 | Multipart upload 405 for boto3 | boto3 multipart operations fail | 2026-01-31 | Open |

## Bug Dependencies

Some bugs are related or have dependencies:

```
001-chunked-encoding-corruption (ROOT CAUSE)
├── 004-object-tagging
└── 005-multipart-corruption

002-deleteobject-405
└── 003-deletebucket-405 (blocked by 002)
```

## Contributing Bug Reports

When documenting a new bug:

1. Create a new file: `{next-number}-{short-name}.md`
2. Use the template below
3. Update this README with a summary entry
4. Link to relevant test output or reproduction steps

### Bug Report Template

```markdown
# Bug #{number}: {Title}

**Status:** Open/In Progress/Fixed
**Priority:** Critical/High/Medium/Low
**Discovered:** YYYY-MM-DD
**Affects:** {List of clients/operations}

## Summary
Brief description of the bug.

## Evidence
Test output, error messages, or observations.

## Reproduction Steps
1. Step one
2. Step two
3. Expected vs actual result

## Root Cause
Technical explanation if known.

## Impact
Who/what is affected.

## Proposed Fix
Ideas for resolving the issue.

## Related Issues
Links to related bugs or dependencies.
```

## Fixed Bugs (Archive)

| ID | Title | Fixed Date | PR/Commit |
|----|-------|------------|-----------|
| - | - | - | - |

## Testing Coverage

To verify bugs and prevent regressions:

- ✅ Sanity tests: Verify tests actually detect failures
- ✅ Content verification: Byte-for-byte integrity checks
- ✅ Client compatibility: AWS CLI, boto3, MinIO mc
- ✅ Defensive testing: Validate actual responses, not just status codes

See `tests/clients/` for the comprehensive test suite.

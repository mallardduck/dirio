# Bug #{number}: {Title}

**Status:** Open | In Progress | Fixed
**Priority:** Critical | High | Medium | Low
**Discovered:** YYYY-MM-DD
**Affects:** {List of affected clients, operations, or components}

## Summary

{Brief 1-3 sentence description of the bug. What goes wrong and what should happen instead?}

## Evidence

{Concrete test output, error messages, logs, or observations that demonstrate the bug.}

### Example subsections:
- Test output from specific client (boto3, mc, AWS CLI)
- Error messages
- Unexpected response data
- Code analysis findings

## Reproduction Steps

1. {First step to reproduce}
2. {Second step}
3. {Continue with steps...}
4. **Expected:** {What should happen}
5. **Actual:** {What actually happens}

## Root Cause Analysis

{Technical explanation of why the bug occurs. What code or logic is incorrect?}

### Possible Issues
1. {First theory}
2. {Second theory}
3. {Continue with theories...}

## Impact

{Describe who and what is affected by this bug}

**Functionality:**
- {Impact on features}
- {Impact on workflows}
- {Impact on data integrity}

**Clients Affected:**
- ✅ {Client name}: {How it's affected}
- ❌ {Client name}: {How it's affected}
- ❓ {Client name}: Needs verification

**Workarounds:**
- {Available workaround if any}
- None available

## Proposed Fix

{Suggested approach to fix the bug}

1. {First step}
2. {Second step}
3. {Continue with steps...}

### Phase 1: {Optional phased approach}
{Details}

### Phase 2: {Optional}
{Details}

## Testing

{How to test for this bug, where tests are located, or what tests need to be added}

Confirmed in: `{test file path}`
```{language}
{relevant test code or commands}
```

## Related Issues

- #{bug number}: {Relationship description}
- May be related to #{bug number} {because...}
- Blocked by #{bug number}

## Investigation Notes

{Optional: Notes from investigating the issue, areas of code to examine, or theories to explore}

**Files to Investigate:**
- `{file path}` - {Why this file}
- `{file path}` - {Why this file}

## References

{Optional: Links to relevant documentation, specs, or discussions}
- AWS S3 API Documentation: {URL}
- Related GitHub issues: {URL}
- Test evidence: {file path or URL}

## Priority Justification

{Optional: For critical/high priority bugs, explain why this priority level}

## Technical Details

{Optional: Additional technical context, API specifications, protocol details}

### Example subsection:
**Expected Request/Response Format:**
```
{example}
```

**Actual Request/Response Format:**
```
{example}
```

## Implementation Notes

{Optional: Specific implementation considerations, edge cases, or architectural decisions}

## Current Behavior

{Optional: Table or list showing how different clients/scenarios behave}

| Client | Result | Details |
|--------|--------|---------|
| {name} | ✅/❌ | {description} |

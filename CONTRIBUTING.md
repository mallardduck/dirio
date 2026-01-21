# Contributing to DirIO

Thanks for considering contributing!

## Setup

```bash
git clone https://github.com/yourusername/dirio.git
cd dirio
go mod tidy
make build
```

## Making Changes

1. **Pick a task** from [TODO.md](TODO.md) or fix a bug
2. **Create a branch**: `git checkout -b fix-thing`
3. **Make your changes**
4. **Test**: `make test`
5. **Format**: `make fmt`  
6. **Commit**: `git commit -m "Fix thing"`
7. **Push**: `git push origin fix-thing`
8. **Open PR**

## Guidelines

## Guidelines

**Code:**
- Follow standard Go conventions
- Keep functions small
- No global state
- Pass dependencies explicitly

**Tests:**
- Add tests for new features
- Don't break existing tests
- Test edge cases

**Commits:**
- One logical change per commit
- Clear commit messages
- No "WIP" or "fix" commits in PRs

**PRs:**
- Describe what and why
- Link related issues
- Keep changes focused

## What to Work On

Check [TODO.md](TODO.md) for tasks.

**Good first issues:**
- Add tests for existing code
- Improve error messages
- Fix documentation typos
- Add examples

**Bigger projects:**
- Implement missing S3 operations
- Add authentication (Phase 2)
- Optimize performance
- Add metrics/monitoring

## Testing

## Testing

```bash
# Unit tests
go test ./...

# Run server locally
./dirio-server --data-dir ./testdata

# Test with AWS CLI
aws --endpoint-url http://localhost:9000 s3 ls
```

## Questions?

Open an issue or start a discussion.

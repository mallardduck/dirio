# Frequently Asked Questions

## General

### What is DirIO?

DirIO is an S3-compatible server where objects are stored as regular files on your filesystem. Built to replace MinIO's discontinued single-node filesystem mode.

### Why not just use MinIO?

MinIO removed single-node filesystem mode after version `RELEASE.2022-10-24T18-35-07Z`. If you want that feature with newer updates, DirIO is your option.

### Why not use [other S3 implementation]?

Most S3 implementations either:
- Don't store objects as plain files (they chunk/encode them)
- Don't support filesystem mode
- Are designed for distributed storage
- Are too complex for homelab use

DirIO is specifically for the "files on disk" use case.

### Is DirIO production-ready?

No. DirIO is in early development (Phase 1 scaffold). Use for:
- Homelab experimentation
- Development/testing environments
- Learning about S3 APIs

Don't use for:
- Production workloads
- Business-critical data
- Anything you can't afford to lose

### How stable is the API?

The filesystem layout and metadata format may change before 1.0. Always backup your data before upgrading.

## Compatibility

### Which S3 clients work with DirIO?

Anything that supports custom endpoints:
- AWS CLI
- boto3 (Python)
- MinIO client (mc)
- Rclone
- Cyberduck
- S3 Browser

### Does it work with [my application]?

If your application uses standard S3 operations (GET/PUT/DELETE/LIST), probably yes. If it uses advanced features (versioning, replication, Select), probably no.

Test it and see. Open an issue if something basic doesn't work.

### Can I use both DirIO and MinIO?

Not at the same time on the same data directory. Pick one.

You can switch back and forth if needed—DirIO leaves `.minio.sys/` untouched.

## Migration

### How do I migrate from MinIO?

1. Stop MinIO
2. Point DirIO at your MinIO data directory
3. DirIO will import metadata on first boot
4. Test that everything works
5. Keep MinIO container around for a week in case you need to rollback

See [DEPLOYMENT.md](DEPLOYMENT.md) for details.

### Will my existing buckets/objects still work?

Yes. Objects in `buckets/` are just files—DirIO reads them directly.

### What happens to MinIO metadata?

DirIO imports:
- Users and credentials
- Bucket policies
- Object metadata (content-type, etag)

Then stores it in `.metadata/` as JSON. Your original `.minio.sys/` stays intact.

### Can I migrate back to MinIO?

Yes. Just stop DirIO and start MinIO again. Your buckets and objects are unchanged.

The `.metadata/` directory won't interfere with MinIO.

## Features

### What S3 operations are supported?

**Phase 1 (MVP)**:
- Object: GET, PUT, HEAD, DELETE, LIST
- Bucket: CREATE, DELETE, HEAD, LIST, GetLocation

**Phase 2+** (planned):
- Multipart uploads
- Pre-signed URLs
- Range requests
- More bucket operations

### Does it support bucket policies?

Basic support. You can import MinIO policies, and DirIO will store them. Enforcement is Phase 2 work.

Public-read buckets will work eventually.

### Does it support versioning?

No, and probably won't. This adds significant complexity for minimal homelab benefit.

### Does it support encryption?

Not built-in. Use filesystem-level encryption (LUKS, ZFS encryption, etc.) if you need it.

### Does it have a web UI?

Not yet. Phase 6, low priority. Use AWS CLI or other S3 clients for now.

## Performance

### How fast is it?

Fast enough for homelab use. Bottleneck is usually disk I/O, not DirIO itself.

Expect 50-200 req/sec for small objects on typical NAS hardware.

### Can it handle large files?

Yes. There's no practical size limit—it's just `io.Copy()` from file to HTTP response.

### How many objects can it handle?

Tested with up to 10,000 objects. Should work fine with 100k+ objects, but performance will degrade on very large buckets (millions of objects).

If you have that many objects, use a real object store.

### Is it faster than MinIO?

No. MinIO is heavily optimized. DirIO prioritizes simplicity over speed.

Use MinIO if you need maximum performance.

## Storage

### Where is my data?

In `data/buckets/bucket-name/path/to/object`. Regular files on disk.

### Can I edit objects directly on the filesystem?

Yes, but be careful:
- Metadata (etag, content-type) may become stale
- DirIO won't know about the change until next read
- Consider restarting DirIO or triggering a re-scan (future feature)

For safety, always use S3 operations to modify objects.

### Can I use network storage (NFS, SMB)?

Yes, but performance may suffer. Local disk is recommended.

### What filesystems are supported?

Any POSIX filesystem. Tested on:
- ext4
- XFS  
- ZFS
- btrfs

NTFS via FUSE probably works but isn't tested.

### How much disk space does metadata use?

Minimal. JSON files are small:
- `users.json`: ~1-10 KB
- Per-bucket metadata: ~1 KB
- Per-object metadata: 0-1 KB (optional)

For 1000 objects, expect <1 MB of metadata.

## Troubleshooting

### DirIO won't start

Check:
- Port 9000 not already in use
- Data directory exists and is writable
- No conflicting MinIO instance running

### Import failed

Check:
- `.minio.sys/` directory exists
- MinIO data is from a compatible version
- Logs for specific error messages

### Objects missing after import

Check:
- Objects exist in `buckets/bucket-name/`
- File permissions are correct
- DirIO has read access to data directory

### Cannot upload large files

Increase timeouts:
- Reverse proxy timeout (Nginx, Caddy)
- Client timeout (AWS CLI, boto3)
- Network timeout

## Development

### How can I contribute?

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

Short version:
1. Fork the repo
2. Make your changes
3. Add tests
4. Open a pull request

### Where should I start?

Check [TODO.md](TODO.md) for tasks. Good first issues:
- Adding tests for existing code
- Implementing missing S3 operations
- Improving error messages
- Documentation improvements

### Can I use DirIO as a library?

Not recommended. The `internal/` packages are not designed for external use.

If you need S3 server functionality in your Go app, consider using MinIO's libraries directly.

## Security

### Is it secure?

No formal security audit has been done. Use at your own risk.

Known limitations:
- No AWS Signature V4 auth yet (Phase 2)
- No rate limiting
- No request validation beyond basic S3 format
- No protection against path traversal (yet)

Don't expose DirIO directly to the internet.

### Should I change the default credentials?

Yes! Default is `minioadmin:minioadmin`. Change this immediately.

### Can I restrict access per-bucket?

Not yet. Phase 2 will add bucket policy enforcement.

For now, all authenticated users have full access.

### Does it log requests?

Basic logging to stdout. No access logs yet (Phase 4).

## Future

### What's the roadmap?

See [TODO.md](TODO.md) for detailed roadmap.

Summary:
- Phase 1: MVP (current)
- Phase 2: Authentication
- Phase 3: Advanced features
- Phase 4: Operations/monitoring
- Phase 5: CLI client
- Phase 6: Web UI

### When will [feature X] be ready?

No timeline yet. This is a side project.

Want it sooner? Contributions welcome!

### Will you support [advanced S3 feature]?

Maybe. Depends on:
- Complexity
- Usefulness for homelab scenarios  
- Alignment with "filesystem-first" philosophy

Open an issue to discuss.

## Miscellaneous

### Why is it called DirIO?

**Dir**ect **I**/**O** for S3. Objects are files in directories, no abstraction layer.

### Is it related to MinIO?

No official relationship. DirIO is inspired by MinIO's (now-removed) single-node filesystem mode.

### Can I use this in production?

Please don't. Use real S3, or MinIO in distributed mode, or another production-grade solution.

### Where can I get help?

- Read the docs (README, QUICKSTART, DEPLOYMENT)
- Check existing GitHub issues
- Open a new issue
- Ask in discussions

### Can I donate?

Not accepting donations. If you find DirIO useful, contribute code or docs instead!

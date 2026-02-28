# IAM Quick Reference

Quick examples showing how DirIO's hybrid IAM works in practice.

## Understanding the Hybrid Approach

DirIO combines two APIs for IAM:
- **S3 API** - Bucket policies (resource-based access control)
- **MinIO Admin API** - User and policy management (identity-based access control)

Both APIs share the same backend (`.dirio/iam/`) and work together.

---

## S3 Bucket Policies (S3 API)

Bucket policies are attached to buckets and grant access based on principals (users or `"*"` for public).

### Example 1: Public Read Access

Make a bucket publicly readable:

```bash
# Create the bucket
aws --endpoint-url http://localhost:9000 s3 mb s3://public-data

# Create policy file
cat > public-read-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": "*",
    "Action": ["s3:GetObject"],
    "Resource": ["arn:aws:s3:::public-data/*"]
  }]
}
EOF

# Apply bucket policy
aws --endpoint-url http://localhost:9000 s3api put-bucket-policy \
  --bucket public-data \
  --policy file://public-read-policy.json
```

Now anyone can read objects from `public-data`:
```bash
# No credentials needed!
curl http://localhost:9000/public-data/file.txt
```

### Example 2: User-Specific Folder Access

Each user can only access their own folder using policy variables:

```bash
cat > user-folders-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"AWS": "*"},
    "Action": ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"],
    "Resource": ["arn:aws:s3:::shared/${aws:username}/*"]
  }]
}
EOF

aws --endpoint-url http://localhost:9000 s3api put-bucket-policy \
  --bucket shared \
  --policy file://user-folders-policy.json
```

When user `alice` accesses the bucket, `${aws:username}` becomes `alice`:
- ✅ Alice can access `s3://shared/alice/*`
- ❌ Alice cannot access `s3://shared/bob/*`

### Example 3: IP-Restricted Access

Allow access only from specific IP ranges:

```bash
cat > ip-restricted-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": "*",
    "Action": ["s3:GetObject", "s3:PutObject"],
    "Resource": ["arn:aws:s3:::private-data/*"],
    "Condition": {
      "IpAddress": {
        "aws:SourceIp": ["10.0.0.0/8", "192.168.1.0/24"]
      }
    }
  }]
}
EOF

aws --endpoint-url http://localhost:9000 s3api put-bucket-policy \
  --bucket private-data \
  --policy file://ip-restricted-policy.json
```

### Example 4: Time-Based Access

Grant access only until a specific date:

```bash
cat > time-limited-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"AWS": "arn:aws:iam::*:user/contractor"},
    "Action": ["s3:GetObject", "s3:ListBucket"],
    "Resource": [
      "arn:aws:s3:::project-data",
      "arn:aws:s3:::project-data/*"
    ],
    "Condition": {
      "DateLessThan": {
        "aws:CurrentTime": "2026-12-31T23:59:59Z"
      }
    }
  }]
}
EOF

aws --endpoint-url http://localhost:9000 s3api put-bucket-policy \
  --bucket project-data \
  --policy file://time-limited-policy.json
```

Access automatically expires on 2026-12-31.

### Example 5: Prefix-Based Access

Grant different users access to different prefixes:

```bash
cat > prefix-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {"AWS": "arn:aws:iam::*:user/alice"},
      "Action": ["s3:GetObject", "s3:PutObject"],
      "Resource": ["arn:aws:s3:::team-data/alice/*"]
    },
    {
      "Effect": "Allow",
      "Principal": {"AWS": "arn:aws:iam::*:user/bob"},
      "Action": ["s3:GetObject", "s3:PutObject"],
      "Resource": ["arn:aws:s3:::team-data/bob/*"]
    },
    {
      "Effect": "Allow",
      "Principal": {"AWS": "*"},
      "Action": ["s3:GetObject"],
      "Resource": ["arn:aws:s3:::team-data/public/*"]
    }
  ]
}
EOF

aws --endpoint-url http://localhost:9000 s3api put-bucket-policy \
  --bucket team-data \
  --policy file://prefix-policy.json
```

Result:
- Alice: read/write `team-data/alice/*`
- Bob: read/write `team-data/bob/*`
- Everyone: read `team-data/public/*`

---

## MinIO Admin API (Control Plane)

Use `mc admin` commands to manage users and policies.

### Setup: Configure mc Alias

```bash
mc alias set local http://localhost:9000 admin adminpass123
```

### Example 6: Create a User

```bash
# Create user
mc admin user add local alice alicepass123

# Verify
mc admin user list local
```

User `alice` now has credentials but no permissions yet.

### Example 7: Create and Attach a Read-Only Policy

```bash
# Create policy file
cat > readonly-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": [
      "s3:GetObject",
      "s3:ListBucket",
      "s3:ListAllMyBuckets"
    ],
    "Resource": ["arn:aws:s3:::*"]
  }]
}
EOF

# Upload policy to DirIO
mc admin policy create local readonly readonly-policy.json

# Attach policy to user
mc admin policy attach local readonly --user alice

# Verify
mc admin user info local alice
```

Now `alice` can list and download from all buckets but cannot upload or delete.

### Example 8: Create a Write-Only Policy for Specific Bucket

```bash
cat > write-uploads-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["s3:PutObject"],
    "Resource": ["arn:aws:s3:::uploads/*"]
  }]
}
EOF

mc admin policy create local write-uploads write-uploads-policy.json
mc admin policy attach local write-uploads --user bob
```

User `bob` can only upload to the `uploads` bucket.

### Example 9: List All Policies

```bash
# List policy names
mc admin policy list local

# View specific policy details
mc admin policy info local readonly
```

### Example 10: Disable a User (Soft Delete)

```bash
# Disable user (revoke access without deleting)
mc admin user disable local alice

# Re-enable later
mc admin user enable local alice
```

---

## How S3 Policies and User Policies Work Together

Both bucket policies (S3 API) and user policies (Admin API) are evaluated together.

### Policy Evaluation Order

1. **Admin Fast Path:** Admin user bypasses all policy checks → **Request ALLOWED**
2. **Owner Check:** Bucket/object owner gets implicit permissions → **Request ALLOWED**
3. **Explicit Deny:** If any policy (bucket or user) denies access → **Request DENIED**
4. **Explicit Allow:** If any policy (bucket or user) allows access → **Request ALLOWED**
5. **Default Deny:** If no policy allows → **Request DENIED**

### Example 11: Combining Bucket Policy + User Policy

**Scenario:** Public bucket with read-only user:

```bash
# 1. Create public bucket policy (allows everyone to read)
cat > public-bucket.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": "*",
    "Action": ["s3:GetObject"],
    "Resource": ["arn:aws:s3:::public-files/*"]
  }]
}
EOF

aws --endpoint-url http://localhost:9000 s3api put-bucket-policy \
  --bucket public-files \
  --policy file://public-bucket.json

# 2. Create user with read-only policy
mc admin user add local charlie charliepass123
mc admin policy attach local readonly --user charlie
```

**Result:**
- Anonymous users: Can read `public-files` (bucket policy allows)
- User `charlie`: Can read from `public-files` AND all other buckets (user policy allows)
- User `charlie`: Cannot write anywhere (no policy allows)

### Example 12: Explicit Deny Overrides Allow

**Scenario:** User can read all buckets EXCEPT `sensitive`:

```bash
# Create user policy with explicit deny for sensitive bucket
cat > deny-sensitive-policy.json <<'EOF'
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:GetObject", "s3:ListBucket"],
      "Resource": ["arn:aws:s3:::*"]
    },
    {
      "Effect": "Deny",
      "Action": ["s3:GetObject", "s3:ListBucket"],
      "Resource": [
        "arn:aws:s3:::sensitive",
        "arn:aws:s3:::sensitive/*"
      ]
    }
  ]
}
EOF

mc admin policy create local deny-sensitive deny-sensitive-policy.json
mc admin policy attach local deny-sensitive --user david
```

**Result:**
- User `david`: Can access all buckets except `sensitive`
- Even if `sensitive` bucket has public policy → David still denied (explicit deny wins)

---

## Testing Access with Different Credentials

Configure multiple mc aliases for different users:

```bash
# Admin credentials
mc alias set admin http://localhost:9000 admin adminpass123

# Alice's credentials
mc alias set alice http://localhost:9000 alice alicepass123

# Bob's credentials
mc alias set bob http://localhost:9000 bob bobpass123

# Test access
mc ls admin/              # Admin sees all buckets
mc ls alice/              # Alice sees only permitted buckets (filtered)
mc ls bob/                # Bob sees only permitted buckets (filtered)

mc cp file.txt alice/uploads/   # May succeed or fail based on policy
mc cp file.txt bob/uploads/     # May succeed or fail based on policy
```

---

## Common Policy Patterns

### Pattern 1: Full Admin Access

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["s3:*"],
    "Resource": ["arn:aws:s3:::*"]
  }]
}
```

### Pattern 2: Read-Only All Buckets

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["s3:GetObject", "s3:ListBucket", "s3:ListAllMyBuckets"],
    "Resource": ["arn:aws:s3:::*"]
  }]
}
```

### Pattern 3: Per-User Home Directory

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["s3:*"],
    "Resource": [
      "arn:aws:s3:::home/${aws:username}",
      "arn:aws:s3:::home/${aws:username}/*"
    ]
  }]
}
```

### Pattern 4: Deny Delete for Everyone (Append-Only)

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": "*",
      "Action": ["s3:GetObject", "s3:PutObject"],
      "Resource": ["arn:aws:s3:::logs/*"]
    },
    {
      "Effect": "Deny",
      "Principal": "*",
      "Action": ["s3:DeleteObject"],
      "Resource": ["arn:aws:s3:::logs/*"]
    }
  ]
}
```

### Pattern 5: Temporary Contractor Access

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"AWS": "arn:aws:iam::*:user/contractor"},
    "Action": ["s3:GetObject", "s3:ListBucket"],
    "Resource": [
      "arn:aws:s3:::project-q4",
      "arn:aws:s3:::project-q4/*"
    ],
    "Condition": {
      "DateLessThan": {
        "aws:CurrentTime": "2026-12-31T23:59:59Z"
      },
      "IpAddress": {
        "aws:SourceIp": ["203.0.113.0/24"]
      }
    }
  }]
}
```

---

## Troubleshooting

### "Access Denied" - How to Debug

1. **Check if user exists:**
   ```bash
   mc admin user list local
   ```

2. **Check user's attached policies:**
   ```bash
   mc admin user info local alice
   ```

3. **Check bucket policy:**
   ```bash
   aws --endpoint-url http://localhost:9000 s3api get-bucket-policy --bucket mybucket
   ```

4. **Check if there's an explicit deny:**
   - Look for `"Effect": "Deny"` in any policy
   - Explicit deny always wins

5. **Check conditions:**
   - IP address restrictions (`IpAddress` condition)
   - Time restrictions (`DateLessThan` condition)
   - Verify conditions match your request context

### Testing Policy Evaluation

Use different user credentials to test:

```bash
# As admin (should work)
AWS_ACCESS_KEY_ID=admin AWS_SECRET_ACCESS_KEY=adminpass123 \
  aws --endpoint-url http://localhost:9000 s3 ls s3://mybucket/

# As alice (may fail depending on policy)
AWS_ACCESS_KEY_ID=alice AWS_SECRET_ACCESS_KEY=alicepass123 \
  aws --endpoint-url http://localhost:9000 s3 ls s3://mybucket/
```

### Policy Not Working?

1. **Verify policy was applied:**
   ```bash
   mc admin policy info local policyname
   aws --endpoint-url http://localhost:9000 s3api get-bucket-policy --bucket mybucket
   ```

2. **Check JSON syntax:**
   - Use `jq` to validate: `cat policy.json | jq .`
   - DirIO validates on upload and returns errors

3. **Check action names:**
   - Use S3 standard: `s3:GetObject` not `GetObject`
   - Case-sensitive

4. **Check resource ARNs:**
   - Bucket: `arn:aws:s3:::mybucket`
   - Objects: `arn:aws:s3:::mybucket/*`
   - Wildcard account: `arn:aws:iam::*:user/alice`

---

## Summary

**DirIO's hybrid IAM:**
- **S3 API** → Bucket policies for resource-based access (public buckets, conditional access)
- **MinIO Admin API** → User/policy management for identity-based access
- **Both** → Evaluated together (explicit deny wins, then explicit allow)

**When to use what:**
- Public access → Use bucket policy with `Principal: "*"`
- User-specific access → Use user policies via `mc admin`
- Complex conditions (IP, time, variables) → Use bucket policies
- Reusable permissions → Create policies and attach to multiple users

**Key principle:** Explicit deny always wins, explicit allow is required, default is deny.
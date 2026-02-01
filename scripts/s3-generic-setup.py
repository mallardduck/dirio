#!/usr/bin/env python3
"""
Generic S3 API Setup Script (boto3 version)
--------------------------------------------
This script sets up test data on ANY S3-compatible API endpoint using boto3.

Usage:
    S3_ENDPOINT=http://localhost:8080 \
    S3_ACCESS_KEY=admin \
    S3_SECRET_KEY=password123 \
    python3 s3-generic-setup.py

Optional environment variables:
    S3_REGION       - AWS region if needed (default: "us-east-1")
    OBJECT_SIZE     - Size of test objects in bytes (default: 65536)
    SKIP_USERS      - Set to "true" to skip user/policy creation (default: false)
    SKIP_POLICIES   - Set to "true" to skip bucket policies (default: false)
    VERIFY_SSL      - Set to "false" to disable SSL verification (default: true)
"""

import os
import sys
import json
import random
import gzip
from io import BytesIO

try:
    import boto3
    from botocore.exceptions import ClientError, NoCredentialsError
    from botocore.client import Config
except ImportError:
    print("❌ Error: boto3 is not installed")
    print("")
    print("Install boto3:")
    print("  pip install boto3")
    print("")
    sys.exit(1)


# -----------------------
# Config
# -----------------------
S3_ENDPOINT = os.getenv("S3_ENDPOINT", "")
S3_ACCESS_KEY = os.getenv("S3_ACCESS_KEY", "")
S3_SECRET_KEY = os.getenv("S3_SECRET_KEY", "")
S3_REGION = os.getenv("S3_REGION", "us-east-1")
OBJECT_SIZE = int(os.getenv("OBJECT_SIZE", "65536"))
SKIP_USERS = os.getenv("SKIP_USERS", "false").lower() == "true"
SKIP_POLICIES = os.getenv("SKIP_POLICIES", "false").lower() == "true"
VERIFY_SSL = os.getenv("VERIFY_SSL", "true").lower() != "false"

# Users (for IAM creation, if supported)
ALICE_USER = "alice"
ALICE_PASS = "alicepass1234"
BOB_USER = "bob"
BOB_PASS = "bobpass1234"


# -----------------------
# Validation
# -----------------------
if not S3_ENDPOINT:
    print("❌ Error: S3_ENDPOINT is required")
    print("")
    print("Usage:")
    print("  S3_ENDPOINT=http://localhost:8080 \\")
    print("  S3_ACCESS_KEY=admin \\")
    print("  S3_SECRET_KEY=password123 \\")
    print(f"  python3 {sys.argv[0]}")
    print("")
    print("Optional variables:")
    print("  S3_REGION=us-east-1     # AWS region")
    print("  OBJECT_SIZE=65536       # Test object size in bytes")
    print("  SKIP_USERS=true         # Skip IAM user creation")
    print("  SKIP_POLICIES=true      # Skip bucket policy creation")
    print("  VERIFY_SSL=false        # Disable SSL verification")
    sys.exit(1)

if not S3_ACCESS_KEY:
    print("❌ Error: S3_ACCESS_KEY is required")
    sys.exit(1)

if not S3_SECRET_KEY:
    print("❌ Error: S3_SECRET_KEY is required")
    sys.exit(1)


# -----------------------
# Setup boto3 clients
# -----------------------
print(f"🔧 Configuring boto3 client for {S3_ENDPOINT}...")

session = boto3.session.Session()
s3_client = session.client(
    "s3",
    endpoint_url=S3_ENDPOINT,
    aws_access_key_id=S3_ACCESS_KEY,
    aws_secret_access_key=S3_SECRET_KEY,
    region_name=S3_REGION,
    config=Config(signature_version="s3v4"),
    verify=VERIFY_SSL,
)

# IAM client (may not be supported by all S3 providers)
iam_client = session.client(
    "iam",
    endpoint_url=S3_ENDPOINT,
    aws_access_key_id=S3_ACCESS_KEY,
    aws_secret_access_key=S3_SECRET_KEY,
    region_name=S3_REGION,
    verify=VERIFY_SSL,
)


# -----------------------
# Test connection
# -----------------------
print("🔌 Testing connection...")
try:
    s3_client.list_buckets()
    print(f"✓ Connected to {S3_ENDPOINT}")
except Exception as e:
    print(f"❌ Failed to connect to S3 endpoint")
    print(f"   Endpoint: {S3_ENDPOINT}")
    print(f"   Error: {e}")
    sys.exit(1)


# -----------------------
# Helper functions
# -----------------------
def bucket_exists(bucket_name):
    """Check if a bucket exists."""
    try:
        s3_client.head_bucket(Bucket=bucket_name)
        return True
    except ClientError:
        return False


def create_bucket(bucket_name):
    """Create a bucket."""
    if bucket_exists(bucket_name):
        print(f"  ⚠️  Bucket '{bucket_name}' already exists, skipping")
        return

    try:
        if S3_REGION == "us-east-1":
            s3_client.create_bucket(Bucket=bucket_name)
        else:
            s3_client.create_bucket(
                Bucket=bucket_name,
                CreateBucketConfiguration={"LocationConstraint": S3_REGION},
            )
        print(f"  ✓ Created bucket '{bucket_name}'")
    except ClientError as e:
        print(f"  ❌ Failed to create bucket '{bucket_name}': {e}")


def upload_random_object(bucket, key, size=OBJECT_SIZE, metadata=None, content_type=None, content_encoding=None):
    """Upload a random binary object."""
    data = os.urandom(size)
    extra_args = {}

    if metadata:
        extra_args["Metadata"] = metadata
    if content_type:
        extra_args["ContentType"] = content_type
    if content_encoding:
        extra_args["ContentEncoding"] = content_encoding

    try:
        s3_client.put_object(Bucket=bucket, Key=key, Body=data, **extra_args)
        return True
    except ClientError as e:
        print(f"  ❌ Failed to upload {bucket}/{key}: {e}")
        return False


def upload_text_object(bucket, key, content, metadata=None, content_type=None, content_encoding=None):
    """Upload a text object."""
    extra_args = {}

    if metadata:
        extra_args["Metadata"] = metadata
    if content_type:
        extra_args["ContentType"] = content_type
    if content_encoding:
        extra_args["ContentEncoding"] = content_encoding

    try:
        s3_client.put_object(Bucket=bucket, Key=key, Body=content.encode(), **extra_args)
        return True
    except ClientError as e:
        print(f"  ❌ Failed to upload {bucket}/{key}: {e}")
        return False


def set_bucket_public_read(bucket_name):
    """Set bucket policy to allow public read access."""
    policy = {
        "Version": "2012-10-17",
        "Statement": [
            {
                "Sid": "PublicReadGetObject",
                "Effect": "Allow",
                "Principal": "*",
                "Action": "s3:GetObject",
                "Resource": f"arn:aws:s3:::{bucket_name}/*",
            }
        ],
    }

    try:
        s3_client.put_bucket_policy(Bucket=bucket_name, Policy=json.dumps(policy))
        return True
    except ClientError as e:
        print(f"  ⚠️  Failed to set public-read on '{bucket_name}': {e}")
        return False


# -----------------------
# Create buckets
# -----------------------
print("📦 Creating test buckets...")
for bucket in ["alpha", "beta", "gamma"]:
    create_bucket(bucket)


# -----------------------
# Create IAM users and policies (if not skipped)
# -----------------------
if not SKIP_USERS:
    print("👥 Creating IAM users and policies...")
    print("  (This may fail if the S3 API doesn't support IAM)")

    # Policy documents
    alpha_policy = {
        "Version": "2012-10-17",
        "Statement": [
            {
                "Effect": "Allow",
                "Action": ["s3:*"],
                "Resource": ["arn:aws:s3:::alpha", "arn:aws:s3:::alpha/*"],
            }
        ],
    }

    beta_policy = {
        "Version": "2012-10-17",
        "Statement": [
            {
                "Effect": "Allow",
                "Action": ["s3:*"],
                "Resource": ["arn:aws:s3:::beta", "arn:aws:s3:::beta/*"],
            }
        ],
    }

    # Try to create policies
    try:
        iam_client.create_policy(
            PolicyName="alpha-rw",
            PolicyDocument=json.dumps(alpha_policy),
        )
        print("  ✓ Created policy 'alpha-rw'")
    except Exception as e:
        print(f"  ⚠️  Failed to create policy 'alpha-rw' (IAM not supported?): {e}")

    try:
        iam_client.create_policy(
            PolicyName="beta-rw",
            PolicyDocument=json.dumps(beta_policy),
        )
        print("  ✓ Created policy 'beta-rw'")
    except Exception as e:
        print(f"  ⚠️  Failed to create policy 'beta-rw' (IAM not supported?): {e}")

    # Try to create users
    try:
        iam_client.create_user(UserName=ALICE_USER)
        print(f"  ✓ Created user '{ALICE_USER}'")

        # Attach policy
        try:
            iam_client.attach_user_policy(
                UserName=ALICE_USER,
                PolicyArn=f"arn:aws:iam:::policy/alpha-rw",
            )
        except Exception as e:
            print(f"  ⚠️  Failed to attach policy to '{ALICE_USER}': {e}")
    except Exception as e:
        print(f"  ⚠️  Failed to create user '{ALICE_USER}' (IAM not supported?): {e}")

    try:
        iam_client.create_user(UserName=BOB_USER)
        print(f"  ✓ Created user '{BOB_USER}'")

        # Attach policy
        try:
            iam_client.attach_user_policy(
                UserName=BOB_USER,
                PolicyArn=f"arn:aws:iam:::policy/beta-rw",
            )
        except Exception as e:
            print(f"  ⚠️  Failed to attach policy to '{BOB_USER}': {e}")
    except Exception as e:
        print(f"  ⚠️  Failed to create user '{BOB_USER}' (IAM not supported?): {e}")
else:
    print("⏭️  Skipping IAM user creation (SKIP_USERS=true)")


# -----------------------
# Set bucket policies (if not skipped)
# -----------------------
if not SKIP_POLICIES:
    print("🌐 Setting bucket policies...")

    if set_bucket_public_read("gamma"):
        print("  ✓ Set public-read on bucket 'gamma'")

    if set_bucket_public_read("beta"):
        print("  ✓ Set public-read on bucket 'beta'")
else:
    print("⏭️  Skipping bucket policies (SKIP_POLICIES=true)")


# -----------------------
# Upload basic objects
# -----------------------
print("📤 Uploading basic test objects...")

if upload_random_object("alpha", "alice-object.bin"):
    print("  ✓ Uploaded alpha/alice-object.bin")

if upload_random_object("beta", "bob-object.bin"):
    print("  ✓ Uploaded beta/bob-object.bin")

if upload_random_object("gamma", "public-object.bin"):
    print("  ✓ Uploaded gamma/public-object.bin")


# -----------------------
# Create folder structure
# -----------------------
print("📁 Creating folder structure for ListObjects testing...")

folder_objects = [
    ("alpha", "folder1/file1.txt"),
    ("alpha", "folder1/file2.txt"),
    ("alpha", "folder1/subfolder/deep.txt"),
    ("alpha", "folder2/file1.txt"),
    ("alpha", "root-file.txt"),
    ("beta", "prefix/test.txt"),
    ("beta", "prefix/data/nested.txt"),
    ("beta", "other/file.txt"),
]

for bucket, key in folder_objects:
    if upload_random_object(bucket, key):
        print(f"  ✓ Uploaded {bucket}/{key}")


# -----------------------
# Upload objects with metadata
# -----------------------
print("🔖 Uploading objects with various metadata...")

# Object with custom metadata
if upload_random_object(
    "alpha",
    "metadata-test.bin",
    metadata={"author": "TestUser", "project": "DirIO", "version": "1"},
):
    print("  ✓ Uploaded alpha/metadata-test.bin (with custom metadata)")

# Object with Content-Type
if upload_text_object(
    "alpha",
    "data.json",
    '{"test": "json data"}',
    content_type="application/json",
):
    print("  ✓ Uploaded alpha/data.json (Content-Type: application/json)")

# Object with Content-Type and custom metadata
if upload_text_object(
    "gamma",
    "index.html",
    "<html><body>Test</body></html>",
    content_type="text/html",
    metadata={"page": "index"},
):
    print("  ✓ Uploaded gamma/index.html (Content-Type + custom metadata)")

# Object with Content-Encoding
compressed_data = BytesIO()
with gzip.GzipFile(fileobj=compressed_data, mode="wb") as gz:
    gz.write(b"compressed data")

try:
    s3_client.put_object(
        Bucket="beta",
        Key="data.gz",
        Body=compressed_data.getvalue(),
        ContentType="application/gzip",
        ContentEncoding="gzip",
    )
    print("  ✓ Uploaded beta/data.gz (Content-Encoding: gzip)")
except ClientError as e:
    print(f"  ❌ Failed to upload beta/data.gz: {e}")

# Object with multiple custom metadata fields
if upload_text_object(
    "alpha",
    "userdata.txt",
    "user data",
    metadata={"user-id": "12345", "department": "engineering", "uploaded-by": "alice"},
):
    print("  ✓ Uploaded alpha/userdata.txt (multiple custom metadata fields)")


# -----------------------
# Test CopyObject (server-side copy)
# -----------------------
print("📋 Testing CopyObject (server-side copy)...")

try:
    s3_client.copy_object(
        Bucket="alpha",
        Key="alice-copy.bin",
        CopySource={"Bucket": "alpha", "Key": "alice-object.bin"},
    )
    print("  ✓ Server-side copy: alpha/alice-object.bin → alpha/alice-copy.bin")
except ClientError as e:
    print(f"  ⚠️  Server-side copy failed: {e}")

try:
    s3_client.copy_object(
        Bucket="beta",
        Key="copied-from-alpha.txt",
        CopySource={"Bucket": "alpha", "Key": "folder1/file1.txt"},
    )
    print("  ✓ Cross-bucket copy: alpha/folder1/file1.txt → beta/copied-from-alpha.txt")
except ClientError as e:
    print(f"  ⚠️  Cross-bucket copy failed: {e}")


# -----------------------
# Upload large file (multipart)
# -----------------------
print("📦 Uploading large file for multipart testing...")

large_data = os.urandom(10 * 1024 * 1024)  # 10 MB
try:
    s3_client.put_object(Bucket="alpha", Key="large-file.dat", Body=large_data)
    print("  ✓ Uploaded alpha/large-file.dat (10MB, likely multipart)")
except ClientError as e:
    print(f"  ❌ Failed to upload large file: {e}")


# -----------------------
# List created objects
# -----------------------
print("")
print("📋 Listing created objects...")
print("")

for bucket_name in ["alpha", "beta", "gamma"]:
    print(f"{bucket_name.capitalize()} bucket:")
    try:
        response = s3_client.list_objects_v2(Bucket=bucket_name)
        if "Contents" in response:
            for obj in response["Contents"][:20]:
                size = obj["Size"]
                key = obj["Key"]
                modified = obj["LastModified"].strftime("%Y-%m-%d %H:%M:%S")
                print(f"  {modified}  {size:>10} bytes  {key}")
        else:
            print("  (empty)")
    except ClientError as e:
        print(f"  ❌ Failed to list: {e}")
    print("")


# -----------------------
# Done
# -----------------------
print("")
print("✅ Generic S3 setup complete!")
print("")
print(f"Endpoint: {S3_ENDPOINT}")
print(f"Region: {S3_REGION}")
print("")
print("Buckets created:")
print("  alpha → private (alice if IAM supported)")
print("  beta  → public-read (if bucket policies supported)")
print("  gamma → public-read (if bucket policies supported)")
print("")
print("Objects created:")
print("  - Basic objects (alice-object.bin, bob-object.bin, public-object.bin)")
print("  - Folder structures (folder1/file1.txt, folder2/file1.txt, etc.)")
print("  - Objects with standard metadata (Content-Type, Content-Encoding)")
print("  - Objects with custom metadata (x-amz-meta-*)")
print("  - Server-side copies (alice-copy.bin, copied-from-alpha.txt)")
print("  - Large file for multipart testing (large-file.dat - 10MB)")
print("")

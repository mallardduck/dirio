#!/usr/bin/env python3
import os
import time
import boto3
import requests
from botocore.config import Config
from botocore.exceptions import ClientError

passed = 0
failed = 0

def log_pass(name):
    global passed
    print(f"PASS: {name}")
    passed += 1

def log_fail(name, error=""):
    global failed
    print(f"FAIL: {name} - {error}")
    failed += 1

endpoint = os.environ.get("DIRIO_ENDPOINT")
access_key = os.environ.get("DIRIO_ACCESS_KEY")
secret_key = os.environ.get("DIRIO_SECRET_KEY")
region = os.environ.get("DIRIO_REGION", "us-east-1")

config = Config(signature_version="s3v4", s3={"addressing_style": "path"}, retries={'max_attempts': 1})
s3 = boto3.client(
    "s3",
    endpoint_url=endpoint,
    aws_access_key_id=access_key,
    aws_secret_access_key=secret_key,
    region_name=region,
    config=config,
)

bucket = f"boto3-test-bucket-{int(time.time())}"

print("=== boto3 Tests ===")
print(f"Endpoint: {endpoint}")

# ListBuckets
try:
    s3.list_buckets()
    log_pass("ListBuckets")
except Exception as e:
    log_fail("ListBuckets", str(e))

# CreateBucket
try:
    s3.create_bucket(Bucket=bucket)
    log_pass("CreateBucket")
except Exception as e:
    log_fail("CreateBucket", str(e))

# GetBucketLocation
try:
    response = s3.get_bucket_location(Bucket=bucket)
    log_pass("GetBucketLocation")
except Exception as e:
    log_fail("GetBucketLocation", str(e))

# HeadBucket
try:
    s3.head_bucket(Bucket=bucket)
    log_pass("HeadBucket")
except Exception as e:
    log_fail("HeadBucket", str(e))

# PutObject
try:
    s3.put_object(Bucket=bucket, Key="test.txt", Body=b"test content")
    log_pass("PutObject")
except Exception as e:
    log_fail("PutObject", str(e))

# HeadObject
try:
    s3.head_object(Bucket=bucket, Key="test.txt")
    log_pass("HeadObject")
except Exception as e:
    log_fail("HeadObject", str(e))

# GetObject
try:
    response = s3.get_object(Bucket=bucket, Key="test.txt")
    body = response["Body"].read()
    if body == b"test content":
        log_pass("GetObject")
    else:
        log_fail("GetObject", "content mismatch")
except Exception as e:
    log_fail("GetObject", str(e))

# ListObjectsV2 (basic)
try:
    s3.list_objects_v2(Bucket=bucket)
    log_pass("ListObjectsV2 (basic)")
except Exception as e:
    log_fail("ListObjectsV2 (basic)", str(e))

# Create some objects for advanced list tests
try:
    s3.put_object(Bucket=bucket, Key="folder1/file1.txt", Body=b"f1")
    s3.put_object(Bucket=bucket, Key="folder1/file2.txt", Body=b"f2")
    s3.put_object(Bucket=bucket, Key="folder2/file3.txt", Body=b"f3")
    s3.put_object(Bucket=bucket, Key="root.txt", Body=b"root")
except Exception as e:
    print(f"Warning: failed to create test objects: {e}")

# ListObjectsV2 with prefix
try:
    response = s3.list_objects_v2(Bucket=bucket, Prefix="folder1/")
    contents = response.get("Contents", [])
    if len(contents) == 2:
        log_pass("ListObjectsV2 (prefix)")
    else:
        log_fail("ListObjectsV2 (prefix)", f"expected 2 objects, got {len(contents)}")
except Exception as e:
    log_fail("ListObjectsV2 (prefix)", str(e))

# ListObjectsV2 with delimiter (CommonPrefixes)
try:
    response = s3.list_objects_v2(Bucket=bucket, Delimiter="/")
    prefixes = response.get("CommonPrefixes", [])
    contents = response.get("Contents", [])
    # Should have folder1/ and folder2/ as common prefixes, and root.txt + test.txt as contents
    if len(prefixes) >= 2:
        log_pass("ListObjectsV2 (delimiter)")
    else:
        log_fail("ListObjectsV2 (delimiter)", f"expected 2+ CommonPrefixes, got {len(prefixes)}: {prefixes}")
except Exception as e:
    log_fail("ListObjectsV2 (delimiter)", str(e))

# ListObjectsV2 with max-keys
try:
    response = s3.list_objects_v2(Bucket=bucket, MaxKeys=2)
    contents = response.get("Contents", [])
    is_truncated = response.get("IsTruncated", False)
    if len(contents) == 2 and is_truncated:
        log_pass("ListObjectsV2 (max-keys)")
    elif len(contents) == 2:
        log_fail("ListObjectsV2 (max-keys)", "IsTruncated should be True")
    else:
        log_fail("ListObjectsV2 (max-keys)", f"expected 2 objects, got {len(contents)}")
except Exception as e:
    log_fail("ListObjectsV2 (max-keys)", str(e))

# ListObjectsV1
try:
    response = s3.list_objects(Bucket=bucket)
    contents = response.get("Contents", [])
    if len(contents) > 0:
        log_pass("ListObjectsV1")
    else:
        log_fail("ListObjectsV1", "no objects returned")
except Exception as e:
    log_fail("ListObjectsV1", str(e))

# PutObject with metadata
try:
    s3.put_object(
        Bucket=bucket,
        Key="metadata.txt",
        Body=b"test with metadata",
        Metadata={"custom-key": "custom-value"},
    )
    log_pass("PutObject with metadata")
except Exception as e:
    log_fail("PutObject with metadata", str(e))

# GetObject metadata (verify custom metadata is returned)
try:
    response = s3.head_object(Bucket=bucket, Key="metadata.txt")
    metadata = response.get("Metadata", {})
    if metadata.get("custom-key") == "custom-value":
        log_pass("GetObject metadata")
    else:
        log_fail("GetObject metadata", f"metadata not returned correctly: {metadata}")
except Exception as e:
    log_fail("GetObject metadata", str(e))

# Range request
try:
    # Put a larger object
    large_content = b"0123456789" * 10  # 100 bytes
    s3.put_object(Bucket=bucket, Key="range-test.txt", Body=large_content)
    response = s3.get_object(Bucket=bucket, Key="range-test.txt", Range="bytes=0-9")
    body = response["Body"].read()
    if body == b"0123456789":
        log_pass("Range request")
    else:
        log_fail("Range request", f"expected first 10 bytes, got {len(body)} bytes: {body[:20]}")
except Exception as e:
    log_fail("Range request", str(e))

# CopyObject
try:
    s3.copy_object(
        Bucket=bucket,
        Key="copied.txt",
        CopySource={"Bucket": bucket, "Key": "test.txt"},
    )
    # Verify copy exists AND has correct content
    response = s3.get_object(Bucket=bucket, Key="copied.txt")
    copied_body = response["Body"].read()
    if copied_body == b"test content":
        log_pass("CopyObject")
    else:
        log_fail("CopyObject", f"copied content mismatch: expected 'test content', got '{copied_body[:50]}'")
except Exception as e:
    log_fail("CopyObject", str(e))

# Pre-signed URL (generate and fetch)
try:
    url = s3.generate_presigned_url(
        "get_object",
        Params={"Bucket": bucket, "Key": "test.txt"},
        ExpiresIn=300,
    )
    response = requests.get(url)
    if response.status_code == 200 and response.content == b"test content":
        log_pass("Pre-signed URL")
    else:
        log_fail("Pre-signed URL", f"status={response.status_code}, body={response.content[:50]}")
except Exception as e:
    log_fail("Pre-signed URL", str(e))

# Multipart upload
try:
    # Create multipart upload
    mpu = s3.create_multipart_upload(Bucket=bucket, Key="multipart.txt")
    upload_id = mpu["UploadId"]

    # Upload parts (minimum 5MB for real S3, but we'll test with smaller)
    part1 = s3.upload_part(
        Bucket=bucket,
        Key="multipart.txt",
        UploadId=upload_id,
        PartNumber=1,
        Body=b"part1 content",
    )
    part2 = s3.upload_part(
        Bucket=bucket,
        Key="multipart.txt",
        UploadId=upload_id,
        PartNumber=2,
        Body=b"part2 content",
    )

    # Complete multipart upload
    s3.complete_multipart_upload(
        Bucket=bucket,
        Key="multipart.txt",
        UploadId=upload_id,
        MultipartUpload={
            "Parts": [
                {"PartNumber": 1, "ETag": part1["ETag"]},
                {"PartNumber": 2, "ETag": part2["ETag"]},
            ]
        },
    )

    # Verify object exists
    s3.head_object(Bucket=bucket, Key="multipart.txt")
    log_pass("Multipart upload")
except Exception as e:
    log_fail("Multipart upload", str(e))

# Object tagging
try:
    s3.put_object_tagging(
        Bucket=bucket,
        Key="test.txt",
        Tagging={"TagSet": [{"Key": "env", "Value": "test"}]},
    )
    response = s3.get_object_tagging(Bucket=bucket, Key="test.txt")
    tags = response.get("TagSet", [])
    if any(t["Key"] == "env" and t["Value"] == "test" for t in tags):
        log_pass("Object tagging")
    else:
        log_fail("Object tagging", f"tags not returned correctly: {tags}")
except Exception as e:
    log_fail("Object tagging", str(e))

# DeleteObject
try:
    s3.delete_object(Bucket=bucket, Key="test.txt")
    log_pass("DeleteObject")
except Exception as e:
    log_fail("DeleteObject", str(e))

# Cleanup and DeleteBucket
try:
    response = s3.list_objects_v2(Bucket=bucket)
    for obj in response.get("Contents", []):
        s3.delete_object(Bucket=bucket, Key=obj["Key"])
    s3.delete_bucket(Bucket=bucket)
    log_pass("DeleteBucket")
except Exception as e:
    log_fail("DeleteBucket", str(e))

print()
print("=== Summary ===")
print(f"Passed: {passed}")
print(f"Failed: {failed}")
if failed == 0:
    print("All tests passed")
exit(1 if failed > 0 else 0)
#!/usr/bin/env python3
"""
boto3 (Python AWS SDK) compatibility tests for DirIO

Prerequisites:
    pip install boto3

Usage:
    python test_boto3.py

Environment variables:
    DIRIO_ENDPOINT - Server endpoint (default: http://localhost:9876)
    DIRIO_ACCESS_KEY - Access key (default: testaccess)
    DIRIO_SECRET_KEY - Secret key (default: testsecret)
    DIRIO_REGION - AWS region (default: us-east-1)
"""

import os
import sys
import time
import io
import hashlib
from typing import Optional, Tuple

# Check for boto3
try:
    import boto3
    from botocore.exceptions import ClientError
    from botocore.config import Config
except ImportError:
    print("boto3 not found. Install with: pip install boto3")
    sys.exit(1)

# Configuration
ENDPOINT = os.environ.get("DIRIO_ENDPOINT", "http://localhost:9876")
ACCESS_KEY = os.environ.get("DIRIO_ACCESS_KEY", "testaccess")
SECRET_KEY = os.environ.get("DIRIO_SECRET_KEY", "testsecret")
REGION = os.environ.get("DIRIO_REGION", "us-east-1")

# Test data
TEST_BUCKET = f"test-bucket-{int(time.time())}"
TEST_OBJECT_KEY = "test-object.txt"
TEST_OBJECT_CONTENT = b"Hello from DirIO boto3 test!"

# ANSI colors
RED = "\033[0;31m"
GREEN = "\033[0;32m"
YELLOW = "\033[0;33m"
BLUE = "\033[0;34m"
NC = "\033[0m"

# Test counters
tests_passed = 0
tests_failed = 0
tests_skipped = 0


def print_header(msg: str) -> None:
    print(f"\n{BLUE}========================================{NC}")
    print(f"{BLUE}{msg}{NC}")
    print(f"{BLUE}========================================{NC}")


def print_test(msg: str) -> None:
    print(f"\n{YELLOW}TEST: {msg}{NC}")


def print_pass(msg: str) -> None:
    global tests_passed
    print(f"{GREEN}PASS: {msg}{NC}")
    tests_passed += 1


def print_fail(msg: str, error: Optional[str] = None) -> None:
    global tests_failed
    print(f"{RED}FAIL: {msg}{NC}")
    if error:
        print(f"{RED}      Error: {error}{NC}")
    tests_failed += 1


def print_skip(msg: str) -> None:
    global tests_skipped
    print(f"{YELLOW}SKIP: {msg}{NC}")
    tests_skipped += 1


def print_summary() -> bool:
    print(f"\n{BLUE}========================================{NC}")
    print(f"{BLUE}TEST SUMMARY{NC}")
    print(f"{BLUE}========================================{NC}")
    print(f"{GREEN}Passed:  {tests_passed}{NC}")
    print(f"{RED}Failed:  {tests_failed}{NC}")
    print(f"{YELLOW}Skipped: {tests_skipped}{NC}")
    print(f"{BLUE}----------------------------------------{NC}")

    if tests_failed == 0:
        print(f"{GREEN}All tests passed!{NC}")
        return True
    else:
        print(f"{RED}Some tests failed.{NC}")
        return False


def get_s3_client():
    """Create boto3 S3 client configured for DirIO."""
    config = Config(
        signature_version="s3v4",
        s3={"addressing_style": "path"},
    )
    return boto3.client(
        "s3",
        endpoint_url=ENDPOINT,
        aws_access_key_id=ACCESS_KEY,
        aws_secret_access_key=SECRET_KEY,
        region_name=REGION,
        config=config,
    )


def get_s3_resource():
    """Create boto3 S3 resource configured for DirIO."""
    return boto3.resource(
        "s3",
        endpoint_url=ENDPOINT,
        aws_access_key_id=ACCESS_KEY,
        aws_secret_access_key=SECRET_KEY,
        region_name=REGION,
    )


def run_test(name: str, test_func) -> Tuple[bool, Optional[str]]:
    """Run a single test and return (success, error_message)."""
    print_test(name)
    try:
        result = test_func()
        if result is True or result is None:
            print_pass(name)
            return True, None
        else:
            print_fail(name, str(result))
            return False, str(result)
    except ClientError as e:
        error_code = e.response.get("Error", {}).get("Code", "Unknown")
        error_msg = e.response.get("Error", {}).get("Message", str(e))
        print_fail(name, f"{error_code}: {error_msg}")
        return False, f"{error_code}: {error_msg}"
    except Exception as e:
        print_fail(name, str(e))
        return False, str(e)


def main():
    print_header("boto3 Compatibility Tests")
    print(f"boto3 version: {boto3.__version__}")
    print(f"Endpoint: {ENDPOINT}")
    print(f"Test bucket: {TEST_BUCKET}")

    s3 = get_s3_client()

    # ========================================================================
    # Bucket Operations
    # ========================================================================

    def test_list_buckets_empty():
        response = s3.list_buckets()
        return "Buckets" in response

    run_test("ListBuckets (empty)", test_list_buckets_empty)

    def test_create_bucket():
        s3.create_bucket(Bucket=TEST_BUCKET)
        return True

    run_test("CreateBucket", test_create_bucket)

    def test_list_buckets_with_bucket():
        response = s3.list_buckets()
        bucket_names = [b["Name"] for b in response.get("Buckets", [])]
        if TEST_BUCKET in bucket_names:
            return True
        return f"Bucket {TEST_BUCKET} not found in {bucket_names}"

    run_test("ListBuckets (with bucket)", test_list_buckets_with_bucket)

    def test_head_bucket():
        s3.head_bucket(Bucket=TEST_BUCKET)
        return True

    run_test("HeadBucket", test_head_bucket)

    # ========================================================================
    # Object Operations
    # ========================================================================

    def test_put_object():
        s3.put_object(
            Bucket=TEST_BUCKET,
            Key=TEST_OBJECT_KEY,
            Body=TEST_OBJECT_CONTENT,
        )
        return True

    run_test("PutObject (simple)", test_put_object)

    def test_head_object():
        response = s3.head_object(Bucket=TEST_BUCKET, Key=TEST_OBJECT_KEY)
        return "ContentLength" in response

    run_test("HeadObject", test_head_object)

    def test_get_object():
        response = s3.get_object(Bucket=TEST_BUCKET, Key=TEST_OBJECT_KEY)
        body = response["Body"].read()
        if body == TEST_OBJECT_CONTENT:
            return True
        return f"Content mismatch: expected {TEST_OBJECT_CONTENT!r}, got {body!r}"

    run_test("GetObject", test_get_object)

    def test_put_object_with_metadata():
        s3.put_object(
            Bucket=TEST_BUCKET,
            Key="metadata-test.txt",
            Body=b"test content",
            Metadata={"custom-key": "custom-value", "another-key": "another-value"},
            ContentType="text/plain",
            CacheControl="max-age=3600",
        )
        return True

    run_test("PutObject (with custom metadata)", test_put_object_with_metadata)

    def test_head_object_verify_metadata():
        response = s3.head_object(Bucket=TEST_BUCKET, Key="metadata-test.txt")
        metadata = response.get("Metadata", {})
        if "custom-key" in metadata or "Custom-Key" in metadata:
            return True
        return f"Metadata not found. Got: {metadata}"

    run_test("HeadObject (verify custom metadata)", test_head_object_verify_metadata)

    def test_put_object_with_content_type():
        s3.put_object(
            Bucket=TEST_BUCKET,
            Key="content-type-test.json",
            Body=b'{"test": true}',
            ContentType="application/json",
        )
        return True

    run_test("PutObject (with content-type)", test_put_object_with_content_type)

    def test_head_object_verify_content_type():
        response = s3.head_object(Bucket=TEST_BUCKET, Key="content-type-test.json")
        content_type = response.get("ContentType", "")
        if "application/json" in content_type:
            return True
        return f"Wrong content-type: {content_type}"

    run_test("HeadObject (verify content-type)", test_head_object_verify_content_type)

    # ========================================================================
    # ListObjects Operations
    # ========================================================================

    # Create more objects for list tests
    for i in range(1, 4):
        s3.put_object(
            Bucket=TEST_BUCKET,
            Key=f"folder/file{i}.txt",
            Body=f"file {i} content".encode(),
        )

    def test_list_objects_v2():
        response = s3.list_objects_v2(Bucket=TEST_BUCKET)
        return "Contents" in response or "KeyCount" in response

    run_test("ListObjectsV2 (basic)", test_list_objects_v2)

    def test_list_objects_v2_with_prefix():
        response = s3.list_objects_v2(Bucket=TEST_BUCKET, Prefix="folder/")
        contents = response.get("Contents", [])
        keys = [c["Key"] for c in contents]
        if any("file" in k for k in keys):
            return True
        return f"Files not found with prefix. Got: {keys}"

    run_test("ListObjectsV2 (with prefix)", test_list_objects_v2_with_prefix)

    def test_list_objects_v2_with_delimiter():
        response = s3.list_objects_v2(Bucket=TEST_BUCKET, Delimiter="/")
        if "CommonPrefixes" in response:
            return True
        return f"CommonPrefixes not found in response"

    run_test("ListObjectsV2 (with delimiter)", test_list_objects_v2_with_delimiter)

    def test_list_objects_v2_with_max_keys():
        response = s3.list_objects_v2(Bucket=TEST_BUCKET, MaxKeys=2)
        return "KeyCount" in response

    run_test("ListObjectsV2 (with max-keys)", test_list_objects_v2_with_max_keys)

    def test_list_objects_v1():
        response = s3.list_objects(Bucket=TEST_BUCKET)
        return "Contents" in response or response.get("IsTruncated") is not None

    run_test("ListObjectsV1", test_list_objects_v1)

    # ========================================================================
    # Pagination
    # ========================================================================

    def test_list_objects_v2_pagination():
        # Create enough objects for pagination
        for i in range(5):
            s3.put_object(Bucket=TEST_BUCKET, Key=f"page-test-{i}.txt", Body=b"x")

        paginator = s3.get_paginator("list_objects_v2")
        pages = list(paginator.paginate(Bucket=TEST_BUCKET, PaginationConfig={"PageSize": 3}))
        return len(pages) >= 1

    run_test("ListObjectsV2 (pagination)", test_list_objects_v2_pagination)

    # ========================================================================
    # Range Requests
    # ========================================================================

    def test_range_request():
        # Create a larger file
        large_content = b"A" * 102400  # 100KB
        s3.put_object(Bucket=TEST_BUCKET, Key="large-file.txt", Body=large_content)

        response = s3.get_object(
            Bucket=TEST_BUCKET, Key="large-file.txt", Range="bytes=0-1023"
        )
        body = response["Body"].read()
        if len(body) == 1024:
            return True
        return f"Expected 1024 bytes, got {len(body)}"

    run_test("GetObject (range request)", test_range_request)

    # ========================================================================
    # Copy Object
    # ========================================================================

    def test_copy_object():
        s3.copy_object(
            Bucket=TEST_BUCKET,
            Key="copied-object.txt",
            CopySource=f"{TEST_BUCKET}/{TEST_OBJECT_KEY}",
        )
        return True

    run_test("CopyObject", test_copy_object)

    # ========================================================================
    # Pre-signed URLs
    # ========================================================================

    def test_presigned_url_get():
        url = s3.generate_presigned_url(
            "get_object",
            Params={"Bucket": TEST_BUCKET, "Key": TEST_OBJECT_KEY},
            ExpiresIn=300,
        )
        if url and "http" in url:
            # Try to fetch it (would need requests library)
            return True
        return f"Invalid presigned URL: {url}"

    run_test("Presign URL (GET)", test_presigned_url_get)

    def test_presigned_url_put():
        url = s3.generate_presigned_url(
            "put_object",
            Params={"Bucket": TEST_BUCKET, "Key": "presigned-upload.txt"},
            ExpiresIn=300,
        )
        if url and "http" in url:
            return True
        return f"Invalid presigned URL: {url}"

    run_test("Presign URL (PUT)", test_presigned_url_put)

    # ========================================================================
    # Object Resource Interface
    # ========================================================================

    def test_resource_interface():
        s3_resource = get_s3_resource()
        bucket = s3_resource.Bucket(TEST_BUCKET)
        obj = bucket.Object("resource-test.txt")
        obj.put(Body=b"resource interface test")
        return True

    run_test("Resource Interface (put)", test_resource_interface)

    def test_resource_interface_get():
        s3_resource = get_s3_resource()
        obj = s3_resource.Object(TEST_BUCKET, "resource-test.txt")
        body = obj.get()["Body"].read()
        if body == b"resource interface test":
            return True
        return f"Content mismatch: {body!r}"

    run_test("Resource Interface (get)", test_resource_interface_get)

    # ========================================================================
    # Delete Operations (cleanup)
    # ========================================================================

    def test_delete_object():
        s3.delete_object(Bucket=TEST_BUCKET, Key=TEST_OBJECT_KEY)
        return True

    run_test("DeleteObject", test_delete_object)

    def test_delete_object_verify():
        try:
            s3.head_object(Bucket=TEST_BUCKET, Key=TEST_OBJECT_KEY)
            return "Object still exists"
        except ClientError as e:
            if e.response["Error"]["Code"] in ("404", "NoSuchKey"):
                return True
            raise

    run_test("DeleteObject (verify deletion)", test_delete_object_verify)

    # Clean up all objects
    response = s3.list_objects_v2(Bucket=TEST_BUCKET)
    for obj in response.get("Contents", []):
        s3.delete_object(Bucket=TEST_BUCKET, Key=obj["Key"])

    # Handle pagination for cleanup
    while response.get("IsTruncated"):
        response = s3.list_objects_v2(
            Bucket=TEST_BUCKET,
            ContinuationToken=response["NextContinuationToken"],
        )
        for obj in response.get("Contents", []):
            s3.delete_object(Bucket=TEST_BUCKET, Key=obj["Key"])

    def test_delete_bucket():
        s3.delete_bucket(Bucket=TEST_BUCKET)
        return True

    run_test("DeleteBucket", test_delete_bucket)

    # ========================================================================
    # Summary
    # ========================================================================

    success = print_summary()
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
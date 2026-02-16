#!/usr/bin/env python3
"""Test framework for Python-based S3 client integration tests.

Provides structured test execution with JSON output and dual display modes.
"""

import json
import sys
import time
import hashlib
import functools
from typing import List, Dict, Callable, Optional, Tuple
from dataclasses import dataclass, asdict


@dataclass
class TestResult:
    """Represents the result of a single test execution."""
    feature: str
    category: str
    status: str  # "pass", "fail", "skip"
    duration_ms: int
    message: str
    details: Dict[str, str]


@dataclass
class TestMeta:
    """Metadata about the test run."""
    client: str
    version: str
    test_run_id: str
    duration_ms: int


@dataclass
class TestSummary:
    """Summary statistics for the test run."""
    total: int
    passed: int
    failed: int
    skipped: int


class TestSkipException(Exception):
    """Exception raised to skip a test."""
    pass


class TestRunner:
    """Manages test execution and result collection."""

    def __init__(self, client: str, version: str):
        """Initialize the test runner.

        Args:
            client: Client name (e.g., "boto3", "awscli")
            version: Client version string
        """
        self.client = client
        self.version = version
        self.test_run_id = str(int(time.time()))
        self.start_time = time.time()
        self.results: List[TestResult] = []
        self.tests: List[Tuple[str, str, str, Callable]] = []

    def register_test(self, feature: str, category: str,
                     validation_type: str, test_func: Callable):
        """Register a test function to be executed.

        Args:
            feature: Feature name (e.g., "PutObject")
            category: Category name (e.g., "object_operations")
            validation_type: Type of validation (e.g., "content_integrity")
            test_func: Test function to execute
        """
        self.tests.append((feature, category, validation_type, test_func))

    def run_test(self, feature: str, category: str,
                 validation_type: str, test_func: Callable) -> TestResult:
        """Execute a single test and return the result.

        Args:
            feature: Feature name
            category: Category name
            validation_type: Type of validation
            test_func: Test function to execute

        Returns:
            TestResult object with test outcome
        """
        start_time = time.time()
        status = "pass"
        message = ""

        try:
            test_func()
            status = "pass"
            print(f"PASS: {feature}", file=sys.stderr)
        except TestSkipException as e:
            status = "skip"
            message = str(e) if str(e) else "Test skipped"
            print(f"SKIP: {feature}", file=sys.stderr)
        except Exception as e:
            status = "fail"
            message = str(e)
            print(f"FAIL: {feature}", file=sys.stderr)
            print(f"  Error: {message}", file=sys.stderr)

        end_time = time.time()
        duration_ms = int((end_time - start_time) * 1000)

        return TestResult(
            feature=feature,
            category=category,
            status=status,
            duration_ms=duration_ms,
            message=message,
            details={"validation_type": validation_type}
        )

    def run_all_tests(self):
        """Execute all registered tests."""
        for feature, category, validation_type, test_func in self.tests:
            result = self.run_test(feature, category, validation_type, test_func)
            self.results.append(result)

    def get_summary(self) -> TestSummary:
        """Calculate summary statistics from results.

        Returns:
            TestSummary object with counts
        """
        total = len(self.results)
        passed = sum(1 for r in self.results if r.status == "pass")
        failed = sum(1 for r in self.results if r.status == "fail")
        skipped = sum(1 for r in self.results if r.status == "skip")

        return TestSummary(
            total=total,
            passed=passed,
            failed=failed,
            skipped=skipped
        )

    def output_json(self):
        """Output complete JSON results to stdout."""
        end_time = time.time()
        duration_ms = int((end_time - self.start_time) * 1000)

        meta = TestMeta(
            client=self.client,
            version=self.version,
            test_run_id=self.test_run_id,
            duration_ms=duration_ms
        )

        summary = self.get_summary()

        output = {
            "meta": asdict(meta),
            "results": [asdict(r) for r in self.results],
            "summary": asdict(summary)
        }

        print(json.dumps(output, indent=2))


def compute_hash(file_path: str) -> str:
    """Compute MD5 hash of a file.

    Args:
        file_path: Path to the file

    Returns:
        MD5 hash as hexadecimal string
    """
    md5 = hashlib.md5()
    with open(file_path, 'rb') as f:
        while chunk := f.read(8192):
            md5.update(chunk)
    return md5.hexdigest()


def test_case(feature: str, category: str, validation_type: str):
    """Decorator to mark a function as a test case.

    This decorator is optional but provides a convenient way to mark tests.
    The TestRunner can also directly register test functions.

    Args:
        feature: Feature name
        category: Category name
        validation_type: Type of validation

    Returns:
        Decorated function with test metadata
    """
    def decorator(func: Callable) -> Callable:
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            return func(*args, **kwargs)

        # Attach metadata to the function
        wrapper._test_metadata = {
            'feature': feature,
            'category': category,
            'validation_type': validation_type
        }
        return wrapper
    return decorator


def skip_test(reason: str = ""):
    """Skip the current test with an optional reason.

    Args:
        reason: Reason for skipping (optional)

    Raises:
        TestSkipException: Always raised to skip the test
    """
    raise TestSkipException(reason)

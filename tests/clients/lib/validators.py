#!/usr/bin/env python3
"""Validation functions for S3 client integration tests.

Provides standardized validation across all Python-based test scripts.
All validators return (success: bool, error_message: str) tuples.
"""

import os
from typing import Tuple, Dict
from test_framework import compute_hash


def validate_content_integrity(original_file: str, downloaded_file: str) -> Tuple[bool, str]:
    """Validate content integrity by comparing MD5 hashes.

    Args:
        original_file: Path to the original file
        downloaded_file: Path to the downloaded file

    Returns:
        Tuple of (success: bool, error_message: str)
    """
    if not os.path.exists(original_file):
        return False, f"Original file not found: {original_file}"

    if not os.path.exists(downloaded_file):
        return False, f"Downloaded file not found: {downloaded_file}"

    try:
        original_hash = compute_hash(original_file)
        downloaded_hash = compute_hash(downloaded_file)

        if original_hash != downloaded_hash:
            return False, f"Content integrity check failed: hash mismatch (expected: {original_hash}, got: {downloaded_hash})"

        return True, ""
    except Exception as e:
        return False, f"Failed to compute hash: {str(e)}"


def validate_partial_content(file_path: str, expected_size: int) -> Tuple[bool, str]:
    """Validate partial content by checking file size.

    Args:
        file_path: Path to the file
        expected_size: Expected size in bytes

    Returns:
        Tuple of (success: bool, error_message: str)
    """
    if not os.path.exists(file_path):
        return False, f"File not found: {file_path}"

    try:
        actual_size = os.path.getsize(file_path)

        if actual_size != expected_size:
            return False, f"Partial content size mismatch: expected {expected_size} bytes, got {actual_size} bytes"

        return True, ""
    except Exception as e:
        return False, f"Failed to check file size: {str(e)}"


def validate_metadata(metadata: Dict[str, str], key: str, expected_value: str) -> Tuple[bool, str]:
    """Validate metadata presence and value (case-insensitive key matching).

    Args:
        metadata: Dictionary of metadata key-value pairs
        key: Expected metadata key (case-insensitive)
        expected_value: Expected metadata value (case-sensitive)

    Returns:
        Tuple of (success: bool, error_message: str)
    """
    # Convert all keys to lowercase for case-insensitive lookup
    metadata_lower = {k.lower(): v for k, v in metadata.items()}
    key_lower = key.lower()

    if key_lower not in metadata_lower:
        return False, f"Metadata key not found: {key}"

    actual_value = metadata_lower[key_lower]

    if actual_value != expected_value:
        return False, f"Metadata value mismatch for key '{key}': expected '{expected_value}', got '{actual_value}'"

    return True, ""


def validate_file_exists(file_path: str) -> Tuple[bool, str]:
    """Validate that a file exists.

    Args:
        file_path: Path to the file

    Returns:
        Tuple of (success: bool, error_message: str)
    """
    if not os.path.exists(file_path):
        return False, f"File does not exist: {file_path}"

    return True, ""


def validate_file_not_exists(file_path: str) -> Tuple[bool, str]:
    """Validate that a file does not exist.

    Args:
        file_path: Path to the file

    Returns:
        Tuple of (success: bool, error_message: str)
    """
    if os.path.exists(file_path):
        return False, f"File exists but should not: {file_path}"

    return True, ""


def validate_response_code(actual_code: int, expected_code: int) -> Tuple[bool, str]:
    """Validate HTTP response code.

    Args:
        actual_code: Actual response code
        expected_code: Expected response code

    Returns:
        Tuple of (success: bool, error_message: str)
    """
    if actual_code != expected_code:
        return False, f"Response code mismatch: expected {expected_code}, got {actual_code}"

    return True, ""


def validate_not_empty(value: str, field_name: str = "value") -> Tuple[bool, str]:
    """Validate that a value is not empty.

    Args:
        value: Value to check
        field_name: Name of the field (for error message)

    Returns:
        Tuple of (success: bool, error_message: str)
    """
    if not value or len(value.strip()) == 0:
        return False, f"{field_name} is empty"

    return True, ""


def validate_contains(haystack: str, needle: str, case_sensitive: bool = True) -> Tuple[bool, str]:
    """Validate that a string contains a substring.

    Args:
        haystack: String to search in
        needle: String to search for
        case_sensitive: Whether to perform case-sensitive search

    Returns:
        Tuple of (success: bool, error_message: str)
    """
    if not case_sensitive:
        haystack = haystack.lower()
        needle = needle.lower()

    if needle not in haystack:
        return False, f"String does not contain expected substring: '{needle}'"

    return True, ""

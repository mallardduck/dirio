#!/usr/bin/env python3
"""Aggregate client test results and generate markdown report.

Usage:
    python3 aggregate_results.py results/*.json > results/REPORT.md
"""

import json
import sys
from datetime import datetime
from pathlib import Path
from typing import List, Dict, Any
from collections import defaultdict


def load_json_file(file_path: str) -> Dict[str, Any]:
    """Load and parse a JSON results file.

    Args:
        file_path: Path to the JSON file

    Returns:
        Parsed JSON data as dictionary

    Raises:
        Exception if file cannot be loaded or parsed
    """
    try:
        with open(file_path, 'r') as f:
            return json.load(f)
    except Exception as e:
        print(f"Error loading {file_path}: {e}", file=sys.stderr)
        raise


def format_duration(duration_ms: int) -> str:
    """Format duration in milliseconds to human-readable string.

    Args:
        duration_ms: Duration in milliseconds

    Returns:
        Formatted string (e.g., "5.30s")
    """
    seconds = duration_ms / 1000
    return f"{seconds:.2f}s"


def generate_summary_table(results: List[Dict[str, Any]]) -> str:
    """Generate summary table showing overall statistics per client.

    Args:
        results: List of parsed JSON results from all clients

    Returns:
        Markdown table as string
    """
    lines = []
    lines.append("## Summary\n")
    lines.append("| Client | Total | Passed | Failed | Skipped | Duration |")
    lines.append("|--------|-------|--------|--------|---------|----------|")

    for result in results:
        meta = result['meta']
        summary = result['summary']
        lines.append(
            f"| {meta['client']:<6} | {summary['total']:>5} | "
            f"{summary['passed']:>6} | {summary['failed']:>6} | "
            f"{summary['skipped']:>7} | {format_duration(meta['duration_ms']):>8} |"
        )

    return '\n'.join(lines)


def generate_feature_matrix(results: List[Dict[str, Any]]) -> str:
    """Generate feature support matrix showing per-feature results.

    Args:
        results: List of parsed JSON results from all clients

    Returns:
        Markdown table as string
    """
    # Collect all unique features and their categories
    features_by_category = defaultdict(list)
    feature_to_category = {}

    for result in results:
        for test in result['results']:
            feature = test['feature']
            category = test['category']
            if feature not in feature_to_category:
                feature_to_category[feature] = category
                features_by_category[category].append(feature)

    # Build feature status map: feature -> client -> status
    feature_status = defaultdict(dict)
    for result in results:
        client = result['meta']['client']
        for test in result['results']:
            feature = test['feature']
            status = test['status']
            feature_status[feature][client] = status

    # Get list of clients
    clients = [r['meta']['client'] for r in results]

    # Generate table
    lines = []
    lines.append("\n## Feature Support Matrix\n")

    # Header
    header = "| Feature | " + " | ".join(clients) + " |"
    separator = "|---------|" + "|".join(["-------"] * len(clients)) + "|"
    lines.append(header)
    lines.append(separator)

    # Status symbols
    STATUS_SYMBOLS = {
        'pass': '✅',
        'fail': '❌',
        'skip': '⏭️'
    }

    # Group features by category
    categories = [
        'bucket_operations',
        'object_operations',
        'listing_operations',
        'metadata_operations',
        'advanced_features'
    ]

    for category in categories:
        if category not in features_by_category:
            continue

        # Add category header (visual separator)
        category_title = category.replace('_', ' ').title()
        lines.append(f"| **{category_title}** | " + " | ".join([""] * len(clients)) + " |")

        # Add feature rows
        for feature in features_by_category[category]:
            row_cells = [feature]
            for client in clients:
                status = feature_status[feature].get(client, 'N/A')
                symbol = STATUS_SYMBOLS.get(status, '➖')
                row_cells.append(symbol)

            lines.append("| " + " | ".join(row_cells) + " |")

    return '\n'.join(lines)


def generate_failed_tests_section(results: List[Dict[str, Any]]) -> str:
    """Generate detailed section showing all failed tests with error messages.

    Args:
        results: List of parsed JSON results from all clients

    Returns:
        Markdown section as string
    """
    lines = []
    lines.append("\n## Failed Tests\n")

    has_failures = False

    for result in results:
        client = result['meta']['client']
        failed_tests = [t for t in result['results'] if t['status'] == 'fail']

        if failed_tests:
            has_failures = True
            lines.append(f"\n### {client}\n")
            for test in failed_tests:
                lines.append(f"- **{test['feature']}**: {test['message']}")

    if not has_failures:
        lines.append("*No failed tests! All tests passed or were skipped.*")

    return '\n'.join(lines)


def generate_report(results: List[Dict[str, Any]]) -> str:
    """Generate complete markdown report from results.

    Args:
        results: List of parsed JSON results from all clients

    Returns:
        Complete markdown report as string
    """
    lines = []

    # Title and timestamp
    lines.append("# Client Test Results\n")
    lines.append(f"*Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}*\n")

    # Summary table
    lines.append(generate_summary_table(results))

    # Feature matrix
    lines.append(generate_feature_matrix(results))

    # Failed tests
    lines.append(generate_failed_tests_section(results))

    # Legend
    lines.append("\n---\n")
    lines.append("**Legend:** ✅ Pass | ❌ Fail | ⏭️ Skip | ➖ N/A\n")

    return '\n'.join(lines)


def main():
    """Main entry point for aggregation script."""
    if len(sys.argv) < 2:
        print("Usage: aggregate_results.py <json_file1> [json_file2] ...", file=sys.stderr)
        print("Example: aggregate_results.py results/*.json", file=sys.stderr)
        sys.exit(1)

    json_files = sys.argv[1:]

    # Load all JSON files
    results = []
    for file_path in json_files:
        try:
            data = load_json_file(file_path)
            results.append(data)
        except Exception as e:
            print(f"Skipping {file_path} due to error: {e}", file=sys.stderr)
            continue

    if not results:
        print("Error: No valid JSON files loaded", file=sys.stderr)
        sys.exit(1)

    # Sort results by client name for consistent ordering
    results.sort(key=lambda r: r['meta']['client'])

    # Generate and output report
    report = generate_report(results)
    print(report)


if __name__ == '__main__':
    main()

#!/usr/bin/env bash
# sync-test-deps.sh — keep test framework versions uniform across all workspace modules.
#
# Reads the canonical version of each shared test dependency from the root go.mod
# and ensures every sub-module requires that exact version.
#
# Usage: scripts/sync-test-deps.sh [--check]
#   --check  Exit non-zero if any module is out-of-sync (CI mode, no writes)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECK_ONLY=false
[[ "${1:-}" == "--check" ]] && CHECK_ONLY=true

# ── Test dependencies to sync (all sourced from root go.mod) ─────────────────
#
# Add new entries here as shared test deps grow.
DEPS=(
  "github.com/stretchr/testify"
)

# ── Sub-modules that participate in the workspace ─────────────────────────────
#
# "tidy" = can run go mod tidy safely (no workspace-only deps)
# "edit" = workspace-only imports make tidy unsafe; use go mod edit instead
declare -A MODULES=(
  [api]="tidy"
  [common]="tidy"
  [sdk]="tidy"
  [console]="edit"
)

cd "$REPO_ROOT"

any_changed=false

for dep in "${DEPS[@]}"; do
  # Extract version from root go.mod  (require block, direct or indirect)
  canonical=$(grep -E "^\s+${dep} v" go.mod | awk '{print $2}' | head -1)
  if [[ -z "$canonical" ]]; then
    echo "WARN: $dep not found in root go.mod — skipping" >&2
    continue
  fi

  echo "Syncing $dep@$canonical across sub-modules..."

  for mod in "${!MODULES[@]}"; do
    strategy="${MODULES[$mod]}"
    modfile="$mod/go.mod"

    # Check current version (direct or indirect)
    current=$(grep -E "^\s+${dep} v" "$modfile" 2>/dev/null | awk '{print $2}' | head -1)

    if [[ "$current" == "$canonical" ]]; then
      echo "  $mod: already at $canonical ✓"
      continue
    fi

    if $CHECK_ONLY; then
      echo "  $mod: OUT OF SYNC — has '${current:-missing}', want $canonical" >&2
      any_changed=true
      continue
    fi

    echo "  $mod: updating ${current:-missing} → $canonical"
    any_changed=true

    if [[ "$strategy" == "tidy" ]]; then
      go get -C "$mod" "${dep}@${canonical}"
      go mod tidy -C "$mod"
    else
      # edit strategy: update go.mod directly, update go.sum via download
      go mod edit -C "$mod" -require "${dep}@${canonical}"
      # Remove the // indirect comment if present (promotes to direct)
      sed -i "s|${dep} ${canonical} // indirect|${dep} ${canonical}|g" "$modfile"
      go mod download -C "$mod" "${dep}@${canonical}"
    fi
  done
done

if $CHECK_ONLY && $any_changed; then
  echo ""
  echo "Run 'task sync-test-deps' to fix." >&2
  exit 1
fi

echo ""
echo "Done."

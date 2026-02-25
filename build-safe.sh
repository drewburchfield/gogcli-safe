#!/usr/bin/env bash
#
# build-safe.sh - Build a safety-profiled gog binary.
#
# Reads a safety-profile.yaml, generates Go source files with only
# the enabled commands, and compiles with -tags safety_profile.
# The resulting binary version is tagged with "-safe" suffix.
#
# Usage:
#   ./build-safe.sh safety-profile.example.yaml          # Uses the example profile
#   ./build-safe.sh safety-profiles/readonly.yaml        # Uses a preset
#   ./build-safe.sh safety-profiles/agent-safe.yaml -o /usr/local/bin/gog-safe
#
set -euo pipefail

if [[ -z "${1:-}" ]] || [[ "$1" == -* ]]; then
  echo "Usage: $0 <profile.yaml> [-o output]" >&2
  echo "" >&2
  echo "Examples:" >&2
  echo "  $0 safety-profile.example.yaml" >&2
  echo "  $0 safety-profiles/readonly.yaml" >&2
  echo "  $0 safety-profiles/agent-safe.yaml -o /usr/local/bin/gog-safe" >&2
  exit 1
fi

PROFILE="$1"
shift

# Parse optional flags
OUTPUT=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o|--output)
      if [[ -z "${2:-}" ]]; then
        echo "Error: -o requires an output path" >&2
        exit 1
      fi
      OUTPUT="$2"
      shift 2
      ;;
    *)
      echo "Unknown flag: $1" >&2
      exit 1
      ;;
  esac
done

if [[ -z "$OUTPUT" ]]; then
  OUTPUT="bin/gog-safe"
fi

if [[ ! -f "$PROFILE" ]]; then
  echo "Error: profile not found: $PROFILE" >&2
  exit 1
fi

echo "Safety profile: $PROFILE"
echo "Output binary:  $OUTPUT"
echo ""

# Step 1: Clean previous generated files to avoid stale leftovers
rm -f internal/cmd/*_cmd_gen.go

# Step 2: Generate Go files from the safety profile
echo "Generating command structs from profile..."
go run ./cmd/gen-safety "$PROFILE"

# Step 3: Build with the safety_profile tag
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev-safe")
COMMIT=$(git rev-parse --short=12 HEAD 2>/dev/null || echo "")
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-X github.com/steipete/gogcli/internal/cmd.version=${VERSION}-safe -X github.com/steipete/gogcli/internal/cmd.commit=${COMMIT} -X github.com/steipete/gogcli/internal/cmd.date=${DATE}"

mkdir -p "$(dirname "$OUTPUT")"

echo "Building with -tags safety_profile..."
go build -tags safety_profile -ldflags "$LDFLAGS" -o "$OUTPUT" ./cmd/gog/

echo ""
echo "Built: $OUTPUT"
echo "Profile: $PROFILE"
if ! "$OUTPUT" --version 2>&1; then
  echo "WARNING: built binary failed to run --version" >&2
  exit 1
fi

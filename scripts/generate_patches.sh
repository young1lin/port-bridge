#!/usr/bin/env bash
# generate_patches.sh — Generate bsdiff delta patches for a new release.
#
# Usage: bash scripts/generate_patches.sh <new_tag>
# Example: bash scripts/generate_patches.sh v0.1.0
#
# Generates delta patches from the last 5 releases to the new release.
# Users on older versions fall back to full binary download.
#
# Prerequisites:
#   - bsdiff installed (apt: bsdiff, brew: bsdiff)
#   - gh CLI authenticated (for uploading)
#
# Artifact naming convention (must match platformSuffix() in updater.go):
#   Linux amd64:   port-bridge_linux-amd64
#   Linux arm64:   port-bridge_linux-arm64
#   Windows amd64: port-bridge_windows-amd64.exe
#   Windows arm64: port-bridge_windows-arm64.exe
#   macOS amd64:   port-bridge_darwin-amd64
#   macOS arm64:   port-bridge_darwin-arm64

set -euo pipefail

NEW_TAG="${1:?Usage: $0 <new_tag>}"
REPO_OWNER="young1lin"
REPO_NAME="port-bridge"
MAX_PREV_VERSIONS=5

echo "=== Generating delta patches for ${NEW_TAG} ==="

# Get the last N release tags (excluding the new one just created)
PREV_TAGS=$(gh release list --repo "${REPO_OWNER}/${REPO_NAME}" --limit $((MAX_PREV_VERSIONS + 1)) --json tagName -q '.[].tagName' | grep -v "^${NEW_TAG}$" | head -n "${MAX_PREV_VERSIONS}" || true)

if [ -z "$PREV_TAGS" ]; then
    echo "No previous releases found. Skipping delta patch generation."
    exit 0
fi

PREV_COUNT=$(echo "$PREV_TAGS" | wc -l)
echo "Generating patches from ${PREV_COUNT} previous releases to ${NEW_TAG}"

# Create temp directory
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

# Platform mapping: os-arch:extension
# Naming matches platformSuffix() = runtime.GOOS + "-" + runtime.GOARCH
PLATFORMS=(
    "windows-amd64:exe"
    "windows-arm64:exe"
    "linux-amd64:"
    "linux-arm64:"
    "darwin-amd64:"
    "darwin-arm64:"
)

# Locate the new binary for each platform.
# GoReleaser (format:binary) outputs to dist/ as port-bridge_<os>-<arch>[.exe].
# macOS binaries are downloaded from GitHub Actions artifacts to
# dist/port-bridge-darwin-<arch>/port-bridge before this script runs.
new_binary_path() {
    local platform="$1" ext="$2"
    case "${platform}" in
        darwin-*)
            local arch="${platform#darwin-}"
            echo "dist/port-bridge-darwin-${arch}/port-bridge"
            ;;
        *)
            [ -n "${ext}" ] && echo "dist/port-bridge_${platform}.${ext}" || echo "dist/port-bridge_${platform}"
            ;;
    esac
}

for PREV_TAG in $PREV_TAGS; do
    echo ""
    echo "--- ${PREV_TAG} -> ${NEW_TAG} ---"

    for platform_spec in "${PLATFORMS[@]}"; do
        IFS=':' read -r platform ext <<< "$platform_spec"
        [ -n "${ext}" ] && suffix=".${ext}" || suffix=""
        bin_name="port-bridge_${platform}${suffix}"

        # Download previous release binary from GitHub
        prev_artifact="${TMPDIR}/${PREV_TAG}-${bin_name}"
        prev_url="https://github.com/${REPO_OWNER}/${REPO_NAME}/releases/download/${PREV_TAG}/${bin_name}"

        if ! curl -sL -f -o "$prev_artifact" "$prev_url"; then
            echo "  skip ${platform} (previous binary not in ${PREV_TAG} release)"
            continue
        fi

        # Find new binary in local artifacts
        new_artifact="$(new_binary_path "${platform}" "${ext}")"
        if [ ! -f "${new_artifact}" ]; then
            echo "  skip ${platform} (new binary not found at ${new_artifact})"
            continue
        fi

        # Generate patch
        patch_name="port-bridge_${PREV_TAG}-to-${NEW_TAG}_${platform}${suffix}.patch"
        patch_path="${TMPDIR}/${patch_name}"

        bsdiff "$prev_artifact" "$new_artifact" "$patch_path"

        echo "  generated ${patch_name}"

        # Upload to release
        gh release upload "${NEW_TAG}" "$patch_path" --repo "${REPO_OWNER}/${REPO_NAME}" --clobber
    done
done

echo ""
echo "=== Done. ${PREV_COUNT} source versions × ${#PLATFORMS[@]} platforms ==="

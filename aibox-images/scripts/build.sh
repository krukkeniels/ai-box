#!/usr/bin/env bash
# =============================================================================
# AI-Box Image Build Script
# =============================================================================
# Builds a Containerfile variant using Buildah, applies stable and date tags,
# and pushes the image to the specified registry.
#
# Usage:
#   ./scripts/build.sh <variant> <stable_tag> <registry> [--no-cache]
#
# Arguments:
#   variant     - Image variant to build: base, java, node, dotnet, full
#   stable_tag  - Stable tag to apply (e.g. 24.04, 21-24.04)
#   registry    - Registry URL (e.g. harbor.internal/aibox)
#   --no-cache  - Optional: build without layer cache (for fresh base layers)
#
# Environment:
#   DATE_TAG    - Date-stamped tag (e.g. 24.04-20260218). Defaults to
#                 <stable_tag>-YYYYMMDD if not set.
# =============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
VARIANTS=(base java node dotnet full)

# ---------------------------------------------------------------------------
# Usage
# ---------------------------------------------------------------------------
usage() {
    cat <<EOF
Usage: $(basename "$0") <variant> <stable_tag> <registry> [--no-cache]

Arguments:
  variant     Image variant: base, java, node, dotnet, full
  stable_tag  Stable tag (e.g. 24.04, 21-24.04)
  registry    Registry URL (e.g. harbor.internal/aibox)
  --no-cache  Build without layer cache

Environment:
  DATE_TAG    Date-stamped tag. Defaults to <stable_tag>-YYYYMMDD.

Examples:
  $(basename "$0") base 24.04 harbor.internal/aibox
  $(basename "$0") java 21-24.04 harbor.internal/aibox --no-cache
EOF
    exit 1
}

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
log() { echo "[build] $(date '+%Y-%m-%d %H:%M:%S') $*"; }
err() { echo "[build] ERROR: $*" >&2; }

# ---------------------------------------------------------------------------
# Validate variant
# ---------------------------------------------------------------------------
is_valid_variant() {
    local v="$1"
    for valid in "${VARIANTS[@]}"; do
        if [[ "$v" == "$valid" ]]; then
            return 0
        fi
    done
    return 1
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    if [[ $# -lt 3 ]]; then
        err "Missing required arguments."
        usage
    fi

    local variant="$1"
    local stable_tag="$2"
    local registry="$3"
    local no_cache=""

    if [[ "${4:-}" == "--no-cache" ]]; then
        no_cache="--no-cache"
    fi

    if ! is_valid_variant "$variant"; then
        err "Invalid variant '${variant}'. Must be one of: ${VARIANTS[*]}"
        exit 1
    fi

    local containerfile="${REPO_ROOT}/${variant}/Containerfile"
    if [[ ! -f "$containerfile" ]]; then
        err "Containerfile not found: ${containerfile}"
        exit 1
    fi

    # Compute date tag
    local date_tag="${DATE_TAG:-${stable_tag}-$(date +%Y%m%d)}"

    local full_image="${registry}/${variant}"
    local context_dir="${REPO_ROOT}/${variant}"

    log "Building image: ${full_image}"
    log "  Containerfile: ${containerfile}"
    log "  Stable tag:    ${stable_tag}"
    log "  Date tag:      ${date_tag}"
    log "  No-cache:      ${no_cache:-no}"

    # Build
    buildah bud \
        ${no_cache} \
        -t "${full_image}:${stable_tag}" \
        -f "${containerfile}" \
        "${context_dir}"

    # Apply date tag
    buildah tag "${full_image}:${stable_tag}" "${full_image}:${date_tag}"

    log "Pushing ${full_image}:${stable_tag}"
    buildah push "${full_image}:${stable_tag}"

    log "Pushing ${full_image}:${date_tag}"
    buildah push "${full_image}:${date_tag}"

    log "Build and push complete for ${variant}"
}

main "$@"

#!/usr/bin/env bash
# =============================================================================
# AI-Box Containerfile Lint Script (hadolint)
# =============================================================================
# Finds all Containerfiles in the repository and lints them with hadolint.
#
# Usage:
#   ./scripts/lint.sh
#
# Exit codes:
#   0 - All Containerfiles pass linting
#   1 - One or more Containerfiles have lint errors
# =============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
HADOLINT_VERSION="v2.12.0"

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
log() { echo "[lint] $(date '+%Y-%m-%d %H:%M:%S') $*"; }
err() { echo "[lint] ERROR: $*" >&2; }

# ---------------------------------------------------------------------------
# Install hadolint if not present
# ---------------------------------------------------------------------------
ensure_hadolint() {
    if command -v hadolint &>/dev/null; then
        log "hadolint found: $(hadolint --version 2>&1 || true)"
        return 0
    fi

    log "hadolint not found. Downloading ${HADOLINT_VERSION}..."
    local bin_dir="${REPO_ROOT}/.bin"
    mkdir -p "$bin_dir"

    curl -sSL \
        "https://github.com/hadolint/hadolint/releases/download/${HADOLINT_VERSION}/hadolint-Linux-x86_64" \
        -o "${bin_dir}/hadolint"
    chmod +x "${bin_dir}/hadolint"

    export PATH="${bin_dir}:${PATH}"
    log "hadolint installed to ${bin_dir}/hadolint"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    ensure_hadolint

    log "Scanning for Containerfiles in ${REPO_ROOT}..."

    local exit_code=0
    local count=0

    while IFS= read -r -d '' containerfile; do
        count=$((count + 1))
        local relative_path="${containerfile#"${REPO_ROOT}/"}"
        log "Linting: ${relative_path}"

        if ! hadolint "$containerfile"; then
            err "Lint errors in: ${relative_path}"
            exit_code=1
        fi
    done < <(find "${REPO_ROOT}" -name "Containerfile" -print0)

    if [[ "$count" -eq 0 ]]; then
        err "No Containerfiles found in ${REPO_ROOT}"
        exit 1
    fi

    echo ""
    if [[ "$exit_code" -eq 0 ]]; then
        log "All ${count} Containerfile(s) passed linting."
    else
        err "${count} Containerfile(s) checked. Some had lint errors (see above)."
    fi

    exit "$exit_code"
}

main "$@"

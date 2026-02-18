#!/usr/bin/env bash
# =============================================================================
# AI-Box Image Signing Script (Cosign)
# =============================================================================
# Signs a container image using Cosign and verifies the signature.
#
# Usage:
#   ./scripts/sign.sh <image_ref> [--key <path>]
#
# Arguments:
#   image_ref  - Full image reference (e.g. harbor.internal/aibox/base:24.04)
#   --key      - Path to Cosign private key. Overrides COSIGN_KEY env var.
#
# Environment:
#   COSIGN_KEY - Cosign private key (path or content). Used if --key is not set.
# =============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
log() { echo "[sign] $(date '+%Y-%m-%d %H:%M:%S') $*"; }
err() { echo "[sign] ERROR: $*" >&2; }

# ---------------------------------------------------------------------------
# Usage
# ---------------------------------------------------------------------------
usage() {
    cat <<EOF
Usage: $(basename "$0") <image_ref> [--key <path>]

Arguments:
  image_ref  Full image reference (e.g. harbor.internal/aibox/base:24.04)
  --key      Path to Cosign private key. Overrides COSIGN_KEY env var.

Environment:
  COSIGN_KEY  Path to the Cosign private key file. Used when --key is not given.

Examples:
  $(basename "$0") harbor.internal/aibox/base:24.04
  $(basename "$0") harbor.internal/aibox/java:21-24.04 --key /secrets/cosign.key
EOF
    exit 1
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    if [[ $# -lt 1 ]]; then
        err "Missing required argument: image_ref"
        usage
    fi

    local image_ref="$1"
    shift

    local key_path=""

    # Parse optional --key flag
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --key)
                if [[ $# -lt 2 ]]; then
                    err "--key requires a path argument"
                    exit 1
                fi
                key_path="$2"
                shift 2
                ;;
            *)
                err "Unknown argument: $1"
                usage
                ;;
        esac
    done

    # Resolve key path
    if [[ -z "$key_path" ]]; then
        if [[ -z "${COSIGN_KEY:-}" ]]; then
            err "No signing key provided. Set COSIGN_KEY or use --key <path>."
            exit 1
        fi
        key_path="${COSIGN_KEY}"
    fi

    if [[ ! -f "$key_path" ]]; then
        err "Signing key not found: ${key_path}"
        exit 1
    fi

    # Check cosign is available
    if ! command -v cosign &>/dev/null; then
        err "cosign is not installed or not in PATH"
        exit 1
    fi

    # Sign the image
    log "Signing image: ${image_ref}"
    log "  Key: ${key_path}"

    cosign sign \
        --key "${key_path}" \
        --yes \
        "${image_ref}"

    log "Signature applied. Verifying..."

    # Derive the public key path (same name with .pub extension)
    local pub_key_path="${key_path%.key}.pub"
    if [[ ! -f "$pub_key_path" ]]; then
        # Try replacing any extension with .pub
        pub_key_path="${key_path%.*}.pub"
    fi

    if [[ -f "$pub_key_path" ]]; then
        cosign verify \
            --key "${pub_key_path}" \
            "${image_ref}" >/dev/null 2>&1

        log "Signature verified successfully for ${image_ref}"
    else
        log "WARNING: Public key not found at ${pub_key_path}. Skipping verification."
        log "Signing completed but could not verify. Ensure the public key is available."
    fi
}

main "$@"

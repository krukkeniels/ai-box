#!/usr/bin/env bash
# verify-image.sh - Verify Cosign signature on a container image
#
# Usage:
#   ./verify-image.sh <image-reference>
#
# Examples:
#   ./verify-image.sh harbor.internal/aibox/base:24.04
#   ./verify-image.sh harbor.internal/aibox/java:21-24.04
#   ./verify-image.sh harbor.internal/aibox/node:20-24.04
#
# Options:
#   --key PATH   Path to public key (default: /etc/aibox/cosign.pub)
#
# Exit codes:
#   0 - Signature verification passed
#   1 - Signature verification failed or error
set -euo pipefail

PUBLIC_KEY="/etc/aibox/cosign.pub"

# ── Parse arguments ──────────────────────────────────────────────────
IMAGE=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --key)
            PUBLIC_KEY="$2"
            shift 2
            ;;
        -h|--help)
            sed -n '2,/^set /{ /^#/s/^# \?//p }' "$0"
            exit 0
            ;;
        -*)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
        *)
            IMAGE="$1"
            shift
            ;;
    esac
done

if [[ -z "$IMAGE" ]]; then
    echo "ERROR: No image reference provided." >&2
    echo "Usage: $0 <image-reference>" >&2
    echo "Example: $0 harbor.internal/aibox/base:24.04" >&2
    exit 1
fi

# ── Preflight checks ────────────────────────────────────────────────
if ! command -v cosign &>/dev/null; then
    echo "ERROR: cosign is not installed or not in PATH." >&2
    exit 1
fi

if [[ ! -f "$PUBLIC_KEY" ]]; then
    echo "ERROR: Public key not found at: $PUBLIC_KEY" >&2
    echo "Specify a different path with --key PATH" >&2
    exit 1
fi

# ── Verify signature ────────────────────────────────────────────────
echo "Verifying signature for: $IMAGE"
echo "Using public key: $PUBLIC_KEY"
echo ""

if cosign verify --key "$PUBLIC_KEY" "$IMAGE" 2>&1; then
    echo ""
    echo "============================================================"
    echo "  PASS: Signature verification succeeded"
    echo "============================================================"
    echo "  Image : $IMAGE"
    echo "  Key   : $PUBLIC_KEY"
    echo "============================================================"
    exit 0
else
    echo ""
    echo "============================================================"
    echo "  FAIL: Signature verification failed"
    echo "============================================================"
    echo "  Image : $IMAGE"
    echo "  Key   : $PUBLIC_KEY"
    echo ""
    echo "  Possible causes:"
    echo "    - Image is not signed"
    echo "    - Image was signed with a different key"
    echo "    - Signature has been tampered with"
    echo "    - Registry is unreachable"
    echo "============================================================"
    exit 1
fi

#!/usr/bin/env bash
# install-policy.sh - Install Cosign verification policy on a developer machine
#
# Usage:
#   sudo ./install-policy.sh [--source-dir DIR]
#
# Installs the following files:
#   /etc/containers/policy.json       - Podman image verification policy
#   /etc/containers/registries.d/harbor.yaml - Registry sigstore config
#   /etc/aibox/cosign.pub             - Cosign public key for verification
#
# Options:
#   --source-dir DIR   Directory containing policy files (default: script directory)
#   --key PATH         Path to cosign.pub to install (default: <source-dir>/cosign.pub)
#   --dry-run          Show what would be done without making changes
#
# This script requires root privileges (or sudo) to write to /etc/.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SOURCE_DIR="$SCRIPT_DIR"
KEY_PATH=""
DRY_RUN=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --source-dir) SOURCE_DIR="$2"; shift 2 ;;
        --key)        KEY_PATH="$2";   shift 2 ;;
        --dry-run)    DRY_RUN=true;    shift   ;;
        -h|--help)
            sed -n '2,/^set /{ /^#/s/^# \?//p }' "$0"
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

[[ -z "$KEY_PATH" ]] && KEY_PATH="${SOURCE_DIR}/cosign.pub"

# ── Preflight checks ────────────────────────────────────────────────
if [[ "$EUID" -ne 0 && "$DRY_RUN" == false ]]; then
    echo "ERROR: This script must be run as root (use sudo)." >&2
    exit 1
fi

# Verify source files exist
ERRORS=0
for f in "${SOURCE_DIR}/policy.json" "${SOURCE_DIR}/registries.d/harbor.yaml" "$KEY_PATH"; do
    if [[ ! -f "$f" ]]; then
        echo "ERROR: Source file not found: $f" >&2
        ERRORS=$((ERRORS + 1))
    fi
done
if [[ "$ERRORS" -gt 0 ]]; then
    exit 1
fi

# ── Helper ───────────────────────────────────────────────────────────
install_file() {
    local src="$1"
    local dest="$2"
    local mode="${3:-644}"

    if [[ "$DRY_RUN" == true ]]; then
        echo "[DRY RUN] Would install: $src -> $dest (mode $mode)"
        return
    fi

    local dest_dir
    dest_dir="$(dirname "$dest")"
    if [[ ! -d "$dest_dir" ]]; then
        echo "Creating directory: $dest_dir"
        mkdir -p "$dest_dir"
    fi

    # Back up existing file if present
    if [[ -f "$dest" ]]; then
        local backup="${dest}.bak.$(date -u +%Y%m%dT%H%M%SZ)"
        echo "Backing up existing file: $dest -> $backup"
        cp "$dest" "$backup"
    fi

    cp "$src" "$dest"
    chmod "$mode" "$dest"
    echo "Installed: $dest (mode $mode)"
}

# ── Install files ────────────────────────────────────────────────────
echo ""
echo "Installing AI-Box Cosign verification policy..."
echo "Source directory: $SOURCE_DIR"
echo ""

install_file "${SOURCE_DIR}/policy.json" "/etc/containers/policy.json" 644
install_file "${SOURCE_DIR}/registries.d/harbor.yaml" "/etc/containers/registries.d/harbor.yaml" 644
install_file "$KEY_PATH" "/etc/aibox/cosign.pub" 644

# ── Validate installation ────────────────────────────────────────────
echo ""
if [[ "$DRY_RUN" == true ]]; then
    echo "[DRY RUN] Skipping validation."
    exit 0
fi

echo "Validating installation..."
VALID=true

# Check policy.json is valid JSON
if command -v jq &>/dev/null; then
    if jq empty /etc/containers/policy.json 2>/dev/null; then
        echo "  policy.json: valid JSON"
    else
        echo "  policy.json: INVALID JSON" >&2
        VALID=false
    fi
fi

# Check files exist and have content
for f in /etc/containers/policy.json /etc/containers/registries.d/harbor.yaml /etc/aibox/cosign.pub; do
    if [[ -f "$f" && -s "$f" ]]; then
        echo "  $f: present ($(stat -c%s "$f") bytes)"
    else
        echo "  $f: MISSING or empty" >&2
        VALID=false
    fi
done

echo ""
if [[ "$VALID" == true ]]; then
    echo "============================================================"
    echo "  Installation complete"
    echo "============================================================"
    echo ""
    echo "  Installed files:"
    echo "    /etc/containers/policy.json"
    echo "    /etc/containers/registries.d/harbor.yaml"
    echo "    /etc/aibox/cosign.pub"
    echo ""
    echo "  Podman will now verify Cosign signatures for all images"
    echo "  pulled from harbor.internal."
    echo ""
    echo "  Test with:"
    echo "    podman pull harbor.internal/aibox/base:24.04"
    echo "============================================================"
else
    echo "WARNING: Installation completed with validation errors." >&2
    echo "Please check the files listed above." >&2
    exit 1
fi

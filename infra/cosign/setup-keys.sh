#!/usr/bin/env bash
# setup-keys.sh - Generate Cosign key pair for AI-Box image signing
#
# Usage:
#   ./setup-keys.sh [--output-dir DIR]
#
# Generates a Cosign key pair (cosign.key + cosign.pub) and creates a
# timestamped backup of the public key. The private key must be stored
# securely after generation (see docs/cosign-key-management.md).
#
# Options:
#   --output-dir DIR   Directory for key output (default: current directory)
#
# Prerequisites:
#   - cosign must be installed (https://docs.sigstore.dev/cosign/installation/)
set -euo pipefail

OUTPUT_DIR="."

while [[ $# -gt 0 ]]; do
    case "$1" in
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
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

# ── Preflight checks ────────────────────────────────────────────────
if ! command -v cosign &>/dev/null; then
    echo "ERROR: cosign is not installed or not in PATH." >&2
    echo "Install from: https://docs.sigstore.dev/cosign/installation/" >&2
    exit 1
fi

mkdir -p "$OUTPUT_DIR"

PRIVATE_KEY="${OUTPUT_DIR}/cosign.key"
PUBLIC_KEY="${OUTPUT_DIR}/cosign.pub"

# ── Safety check: don't overwrite existing keys without confirmation ─
if [[ -f "$PRIVATE_KEY" || -f "$PUBLIC_KEY" ]]; then
    echo "WARNING: Existing key files detected:"
    [[ -f "$PRIVATE_KEY" ]] && echo "  - $PRIVATE_KEY"
    [[ -f "$PUBLIC_KEY" ]]  && echo "  - $PUBLIC_KEY"
    echo ""
    read -r -p "Overwrite existing keys? This is IRREVERSIBLE. [y/N] " confirm
    if [[ "${confirm,,}" != "y" ]]; then
        echo "Aborted. Existing keys were not modified."
        exit 0
    fi
    echo ""
fi

# ── Generate key pair ────────────────────────────────────────────────
echo "Generating Cosign key pair..."
echo "You will be prompted for a password to encrypt the private key."
echo ""

# cosign generate-key-pair writes cosign.key and cosign.pub to the
# current directory, so we cd into the output directory.
(cd "$OUTPUT_DIR" && cosign generate-key-pair)

if [[ ! -f "$PUBLIC_KEY" ]]; then
    echo "ERROR: Key generation failed -- cosign.pub not found." >&2
    exit 1
fi

# ── Restrict private key permissions ─────────────────────────────────
chmod 600 "$PRIVATE_KEY"
echo "Private key permissions set to 600 (owner read/write only)."

# ── Create timestamped backup of public key ──────────────────────────
TIMESTAMP=$(date -u +%Y%m%dT%H%M%SZ)
BACKUP="${OUTPUT_DIR}/cosign.pub.${TIMESTAMP}.bak"
cp "$PUBLIC_KEY" "$BACKUP"
echo "Public key backup created: $BACKUP"

# ── Summary ──────────────────────────────────────────────────────────
echo ""
echo "============================================================"
echo "  Cosign key pair generated successfully"
echo "============================================================"
echo ""
echo "  Private key : $PRIVATE_KEY"
echo "  Public key  : $PUBLIC_KEY"
echo "  Backup      : $BACKUP"
echo ""
echo "NEXT STEPS -- store the private key securely:"
echo ""
echo "  Option 1 (CI Secrets):"
echo "    Store cosign.key as a CI secret variable (e.g., COSIGN_KEY)."
echo "    Set COSIGN_PASSWORD as a separate secret."
echo "    Simplest option; acceptable for initial deployment."
echo ""
echo "  Option 2 (HashiCorp Vault):"
echo "    vault kv put secret/aibox/cosign \\"
echo "      private_key=@${PRIVATE_KEY} \\"
echo "      password=<key-password>"
echo "    Recommended for production. Provides audit logging."
echo ""
echo "  Option 3 (HSM / KMS):"
echo "    Import the key into your HSM or cloud KMS."
echo "    Most secure. Required for high-security environments."
echo ""
echo "  After storing the private key, DELETE the local copy:"
echo "    shred -u ${PRIVATE_KEY}"
echo ""
echo "  The public key (cosign.pub) should be committed to the"
echo "  repository and distributed to all developer machines."
echo "============================================================"

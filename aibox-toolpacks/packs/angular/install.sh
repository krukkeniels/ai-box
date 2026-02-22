#!/bin/bash
set -euo pipefail

PACK_DIR="/opt/toolpacks/angular"
ANGULAR_VERSION="18"

echo "==> Installing Angular tool pack (Angular CLI ${ANGULAR_VERSION}.x)"

mkdir -p "${PACK_DIR}"

# Verify Node.js is available (should be installed via dependency).
if ! command -v node &>/dev/null; then
    echo "ERROR: Node.js is required but not installed."
    echo "Install node@20 first: aibox install node@20"
    exit 1
fi

# Install Angular CLI globally.
if ! command -v ng &>/dev/null; then
    echo "  Installing Angular CLI @${ANGULAR_VERSION}..."
    npm install -g "@angular/cli@${ANGULAR_VERSION}"
    echo "  Angular CLI installed."
else
    echo "  Angular CLI already available: $(ng version 2>/dev/null | grep 'Angular CLI' || echo 'installed')"
fi

echo "==> Angular tool pack installed successfully."

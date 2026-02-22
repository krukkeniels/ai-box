#!/bin/bash
set -euo pipefail

PACK_DIR="/opt/toolpacks/bazel"
BAZELISK_VERSION="1.19.0"

echo "==> Installing Bazel tool pack (Bazel 7.x via Bazelisk)"

mkdir -p "${PACK_DIR}/bin"

# Install Bazelisk, which manages Bazel versions.
if [ ! -f "${PACK_DIR}/bin/bazelisk" ]; then
    echo "  Installing Bazelisk ${BAZELISK_VERSION}..."
    apt-get update -qq
    apt-get install -y -qq wget ca-certificates

    ARCH=$(dpkg --print-architecture)
    wget -q "https://github.com/bazelbuild/bazelisk/releases/download/v${BAZELISK_VERSION}/bazelisk-linux-${ARCH}" \
        -O "${PACK_DIR}/bin/bazelisk"
    chmod +x "${PACK_DIR}/bin/bazelisk"

    # Symlink as bazel.
    ln -sf "${PACK_DIR}/bin/bazelisk" /usr/local/bin/bazel
    ln -sf "${PACK_DIR}/bin/bazelisk" /usr/local/bin/bazelisk

    echo "  Bazelisk ${BAZELISK_VERSION} installed."
fi

# Pin to Bazel 7.x via .bazelversion if not already set.
if [ ! -f "${HOME}/.bazelversion" ]; then
    echo "7.x" > "${HOME}/.bazelversion"
fi

# Configure Bazelisk to use Nexus mirror for Bazel downloads (if available).
export BAZELISK_BASE_URL="${BAZELISK_BASE_URL:-https://nexus.internal/repository/bazel-releases}"

# Ensure cache directories exist.
mkdir -p "${HOME}/.cache/bazel"
mkdir -p "${HOME}/.cache/bazelisk"

echo "==> Bazel tool pack installed successfully."

#!/bin/bash
set -euo pipefail

PACK_DIR="/opt/toolpacks/node"
NODE_VERSION="20"

echo "==> Installing Node.js tool pack (Node ${NODE_VERSION} LTS, npm, yarn)"

mkdir -p "${PACK_DIR}"

# Install Node.js via NodeSource.
if ! command -v node &>/dev/null; then
    echo "  Installing Node.js ${NODE_VERSION}..."
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates
    curl -fsSL "https://deb.nodesource.com/setup_${NODE_VERSION}.x" | bash -
    apt-get install -y -qq nodejs
    echo "  Node.js $(node --version) installed."
else
    echo "  Node.js already available: $(node --version)"
fi

# Install Yarn globally.
if ! command -v yarn &>/dev/null; then
    echo "  Installing Yarn..."
    npm install -g yarn@1.22.x
    echo "  Yarn $(yarn --version) installed."
fi

# Configure npm to use Nexus mirror.
npm config set registry https://nexus.internal/repository/npm-public/

# Ensure cache directories exist.
mkdir -p "${HOME}/.npm"
mkdir -p "${HOME}/.yarn/cache"

echo "==> Node.js tool pack installed successfully."

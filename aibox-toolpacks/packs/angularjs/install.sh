#!/bin/bash
set -euo pipefail

PACK_DIR="/opt/toolpacks/angularjs"
NODE_VERSION="18"

echo "==> Installing AngularJS tool pack (legacy AngularJS 1.x build tools)"

mkdir -p "${PACK_DIR}"

# AngularJS requires Node 18 (not Node 20).
# Install nvm to manage multiple Node versions side-by-side.
NVM_DIR="${PACK_DIR}/nvm"
if [ ! -d "${NVM_DIR}" ]; then
    echo "  Installing nvm for Node ${NODE_VERSION} (AngularJS compatibility)..."
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates
    curl -fsSL https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | NVM_DIR="${NVM_DIR}" bash
fi

# Source nvm and install Node 18.
export NVM_DIR="${NVM_DIR}"
. "${NVM_DIR}/nvm.sh"

if ! nvm ls "${NODE_VERSION}" &>/dev/null; then
    echo "  Installing Node.js ${NODE_VERSION} via nvm..."
    nvm install "${NODE_VERSION}"
fi
nvm use "${NODE_VERSION}"

# Install AngularJS build tools globally.
echo "  Installing AngularJS build tools..."
npm install -g bower grunt-cli gulp-cli karma-cli protractor 2>/dev/null || true

# Configure npm to use Nexus mirror.
npm config set registry https://nexus.internal/repository/npm-public/

# Create shell helper for AngularJS development.
cat > "${PACK_DIR}/activate.sh" << 'ACTIVATE'
#!/bin/bash
# Source this to activate the AngularJS development environment.
# Usage: source /opt/toolpacks/angularjs/activate.sh
export NVM_DIR="/opt/toolpacks/angularjs/nvm"
. "${NVM_DIR}/nvm.sh"
nvm use 18
echo "AngularJS development environment activated (Node $(node --version))"
ACTIVATE
chmod +x "${PACK_DIR}/activate.sh"

# Ensure cache directories exist.
mkdir -p "${HOME}/.npm"
mkdir -p "${HOME}/.cache/bower"

echo "==> AngularJS tool pack installed successfully."
echo "  NOTE: Run 'source /opt/toolpacks/angularjs/activate.sh' to switch to Node 18."

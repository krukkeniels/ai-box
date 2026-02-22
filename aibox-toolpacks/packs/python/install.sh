#!/bin/bash
set -euo pipefail

PACK_DIR="/opt/toolpacks/python"
PYTHON_VERSION="3.12"

echo "==> Installing Python tool pack (Python ${PYTHON_VERSION}, pip, venv)"

mkdir -p "${PACK_DIR}"

# Install Python 3.12 from Ubuntu packages.
if ! command -v python${PYTHON_VERSION} &>/dev/null; then
    echo "  Installing Python ${PYTHON_VERSION}..."
    apt-get update -qq
    apt-get install -y -qq software-properties-common
    add-apt-repository -y ppa:deadsnakes/ppa
    apt-get update -qq
    apt-get install -y -qq python${PYTHON_VERSION} python${PYTHON_VERSION}-venv python${PYTHON_VERSION}-dev
    echo "  Python ${PYTHON_VERSION} installed."
else
    echo "  Python already available: $(python${PYTHON_VERSION} --version)"
fi

# Ensure pip is available.
if ! python${PYTHON_VERSION} -m pip --version &>/dev/null; then
    echo "  Installing pip..."
    apt-get install -y -qq python3-pip
fi

# Install setuptools.
python${PYTHON_VERSION} -m pip install --quiet setuptools wheel

# Configure pip to use Nexus mirror.
PIP_CONF_DIR="${HOME}/.config/pip"
mkdir -p "${PIP_CONF_DIR}"
if [ ! -f "${PIP_CONF_DIR}/pip.conf" ]; then
    cat > "${PIP_CONF_DIR}/pip.conf" << 'EOF'
[global]
index-url = https://nexus.internal/repository/pypi-public/simple
trusted-host = nexus.internal
EOF
fi

# Ensure cache directory exists.
mkdir -p "${HOME}/.cache/pip"

# Set python3.12 as default python3 if no default exists.
if ! command -v python3 &>/dev/null; then
    update-alternatives --install /usr/bin/python3 python3 /usr/bin/python${PYTHON_VERSION} 1
fi

echo "==> Python tool pack installed successfully."

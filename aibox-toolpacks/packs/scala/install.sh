#!/bin/bash
set -euo pipefail

PACK_DIR="/opt/toolpacks/scala"

echo "==> Installing Scala tool pack (Scala 3.x, sbt, Metals LSP)"

mkdir -p "${PACK_DIR}/bin"

# Install Coursier (Scala installer).
if [ ! -f "${PACK_DIR}/bin/cs" ]; then
    echo "  Installing Coursier..."
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates

    curl -fLo "${PACK_DIR}/bin/cs" "https://github.com/coursier/coursier/releases/latest/download/cs-x86_64-pc-linux.gz"
    # If compressed, decompress; otherwise rename.
    if file "${PACK_DIR}/bin/cs" | grep -q gzip; then
        mv "${PACK_DIR}/bin/cs" "${PACK_DIR}/bin/cs.gz"
        gunzip "${PACK_DIR}/bin/cs.gz"
    fi
    chmod +x "${PACK_DIR}/bin/cs"
    ln -sf "${PACK_DIR}/bin/cs" /usr/local/bin/cs
fi

# Use Coursier to install Scala tools.
echo "  Installing Scala 3, sbt, and Metals..."
"${PACK_DIR}/bin/cs" install \
    scala:3 \
    scalac:3 \
    sbt \
    metals \
    --install-dir "${PACK_DIR}/bin" 2>/dev/null || true

# Symlink to /usr/local/bin.
for tool in scala scalac sbt metals; do
    if [ -f "${PACK_DIR}/bin/${tool}" ]; then
        ln -sf "${PACK_DIR}/bin/${tool}" "/usr/local/bin/${tool}"
    fi
done

# Configure Coursier repositories to use Nexus mirror.
COURSIER_DIR="${HOME}/.config/coursier"
mkdir -p "${COURSIER_DIR}"
if [ ! -f "${COURSIER_DIR}/mirror.properties" ]; then
    cat > "${COURSIER_DIR}/mirror.properties" << 'EOF'
central.from=https://repo1.maven.org/maven2
central.to=https://nexus.internal/repository/maven-public
EOF
fi

# Ensure cache directories exist.
mkdir -p "${HOME}/.cache/coursier"
mkdir -p "${HOME}/.sbt"

echo "==> Scala tool pack installed successfully."

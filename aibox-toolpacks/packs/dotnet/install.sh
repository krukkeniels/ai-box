#!/bin/bash
set -euo pipefail

PACK_DIR="/opt/toolpacks/dotnet"

echo "==> Installing .NET tool pack (.NET SDK 8, NuGet CLI, MSBuild)"

mkdir -p "${PACK_DIR}"

# Install .NET SDK 8 via Microsoft install script.
if ! command -v dotnet &>/dev/null; then
    echo "  Installing .NET SDK 8..."
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates libicu-dev

    curl -fsSL https://dot.net/v1/dotnet-install.sh -o /tmp/dotnet-install.sh
    chmod +x /tmp/dotnet-install.sh
    /tmp/dotnet-install.sh --channel 8.0 --install-dir /usr/share/dotnet
    ln -sf /usr/share/dotnet/dotnet /usr/local/bin/dotnet
    rm /tmp/dotnet-install.sh
    echo "  .NET SDK 8 installed."
else
    echo "  .NET SDK already available: $(dotnet --version)"
fi

# Configure NuGet to use Nexus mirror.
NUGET_CONFIG_DIR="${HOME}/.nuget/NuGet"
mkdir -p "${NUGET_CONFIG_DIR}"
if [ ! -f "${NUGET_CONFIG_DIR}/NuGet.Config" ]; then
    cat > "${NUGET_CONFIG_DIR}/NuGet.Config" << 'EOF'
<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <clear />
    <add key="nexus" value="https://nexus.internal/repository/nuget-public/index.json" />
  </packageSources>
</configuration>
EOF
fi

# Ensure cache directories exist.
mkdir -p "${HOME}/.nuget/packages"
mkdir -p "${HOME}/.dotnet/tools"

# Add .NET tools to PATH.
export PATH="${HOME}/.dotnet/tools:${PATH}"

echo "==> .NET tool pack installed successfully."

#!/bin/bash
set -euo pipefail

PACK_DIR="/opt/toolpacks/powershell"

echo "==> Installing PowerShell tool pack (PowerShell Core 7.x)"

mkdir -p "${PACK_DIR}"

# Install PowerShell Core 7.x.
if ! command -v pwsh &>/dev/null; then
    echo "  Installing PowerShell Core 7..."
    apt-get update -qq
    apt-get install -y -qq curl ca-certificates apt-transport-https

    # Add Microsoft repository.
    . /etc/os-release
    curl -fsSL "https://packages.microsoft.com/config/ubuntu/${VERSION_ID}/packages-microsoft-prod.deb" -o /tmp/packages-microsoft-prod.deb
    dpkg -i /tmp/packages-microsoft-prod.deb
    rm /tmp/packages-microsoft-prod.deb
    apt-get update -qq
    apt-get install -y -qq powershell
    echo "  PowerShell Core installed."
else
    echo "  PowerShell already available: $(pwsh --version)"
fi

# Install useful PowerShell modules.
echo "  Installing PowerShell modules..."
pwsh -NoProfile -Command "
    Set-PSRepository -Name 'PSGallery' -InstallationPolicy Trusted
    \$modules = @('PSReadLine', 'Pester', 'PSScriptAnalyzer')
    foreach (\$mod in \$modules) {
        if (-not (Get-Module -ListAvailable -Name \$mod)) {
            Install-Module -Name \$mod -Scope CurrentUser -Force -AllowClobber
        }
    }
" 2>/dev/null || echo "  Module installation skipped (gallery may not be reachable)."

# Ensure directories exist.
mkdir -p "${HOME}/.cache/powershell"
mkdir -p "${HOME}/.local/share/powershell/Modules"

echo "==> PowerShell tool pack installed successfully."

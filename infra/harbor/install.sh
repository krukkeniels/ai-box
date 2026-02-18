#!/usr/bin/env bash
# install.sh -- Main Harbor installation script for AI-Box.
#
# Downloads the Harbor offline installer, extracts it, applies the
# harbor.yml configuration, installs Harbor with Trivy enabled,
# and runs post-install RBAC setup.
#
# Usage:
#   HARBOR_PASS=<admin-password> \
#   HARBOR_VERSION=v2.11.2 \
#     ./install.sh
#
# Prerequisites:
#   - Docker Engine 20.10+ or Podman with docker-compose compatibility
#   - docker-compose v2 or docker compose plugin
#   - 4 CPU cores, 16 GB RAM, 2 TB disk recommended
#   - Ports 80 and 443 available
#   - TLS certificate and key at /etc/harbor/tls/{cert,key}.pem
#   - Run as root or with sudo privileges

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
HARBOR_VERSION="${HARBOR_VERSION:-v2.11.2}"
HARBOR_INSTALLER_DIR="${HARBOR_INSTALLER_DIR:-/opt/harbor}"
HARBOR_DATA_DIR="${HARBOR_DATA_DIR:-/data/harbor}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HARBOR_YML="${SCRIPT_DIR}/harbor.yml"
HARBOR_PASS="${HARBOR_PASS:-}"
SKIP_DOWNLOAD="${SKIP_DOWNLOAD:-false}"

# Remove leading 'v' for the download URL.
VERSION_NUM="${HARBOR_VERSION#v}"
INSTALLER_URL="https://github.com/goharbor/harbor/releases/download/${HARBOR_VERSION}/harbor-offline-installer-${VERSION_NUM}.tgz"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
log()  { printf '[INFO]  %s\n' "$*"; }
warn() { printf '[WARN]  %s\n' "$*" >&2; }
err()  { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || return 1
}

check_root() {
  if [[ $EUID -ne 0 ]]; then
    err "This script must be run as root (or with sudo)."
  fi
}

# ---------------------------------------------------------------------------
# Prerequisite checks
# ---------------------------------------------------------------------------
preflight() {
  log "Running prerequisite checks..."

  check_root

  # --- Container runtime ---
  if require_cmd docker; then
    CONTAINER_RUNTIME="docker"
    log "Found container runtime: docker"
  elif require_cmd podman; then
    CONTAINER_RUNTIME="podman"
    log "Found container runtime: podman"
  else
    err "Neither Docker nor Podman is installed. Install one before proceeding."
  fi

  # --- docker-compose ---
  if docker compose version &>/dev/null; then
    COMPOSE_CMD="docker compose"
    log "Found docker compose plugin."
  elif require_cmd docker-compose; then
    COMPOSE_CMD="docker-compose"
    log "Found docker-compose standalone."
  else
    err "docker-compose is not installed. Install docker-compose v2+."
  fi

  # --- Required tools ---
  for cmd in curl tar openssl; do
    require_cmd "$cmd" || err "Required command not found: ${cmd}"
  done

  # --- Disk space (warn if < 50 GB free on /data or /) ---
  local data_mount="/"
  if mountpoint -q /data 2>/dev/null; then
    data_mount="/data"
  fi
  local free_gb
  free_gb=$(df -BG --output=avail "$data_mount" | tail -1 | tr -d ' G')
  if [[ "$free_gb" -lt 50 ]]; then
    warn "Only ${free_gb} GB free on ${data_mount}. Recommended: 2 TB for production."
  else
    log "Disk space: ${free_gb} GB free on ${data_mount}."
  fi

  # --- Ports ---
  for port in 80 443; do
    if ss -tlnp 2>/dev/null | grep -q ":${port} "; then
      warn "Port ${port} is already in use. Harbor needs this port."
    fi
  done

  # --- TLS certificates ---
  if [[ ! -f /etc/harbor/tls/cert.pem ]]; then
    warn "TLS certificate not found at /etc/harbor/tls/cert.pem."
    warn "Harbor HTTPS will fail without a valid certificate."
  fi
  if [[ ! -f /etc/harbor/tls/key.pem ]]; then
    warn "TLS private key not found at /etc/harbor/tls/key.pem."
  fi

  # --- harbor.yml ---
  if [[ ! -f "$HARBOR_YML" ]]; then
    err "harbor.yml not found at ${HARBOR_YML}."
  fi

  log "Prerequisite checks passed."
}

# ---------------------------------------------------------------------------
# Download Harbor offline installer
# ---------------------------------------------------------------------------
download_harbor() {
  if [[ "$SKIP_DOWNLOAD" == "true" ]]; then
    log "SKIP_DOWNLOAD is set. Skipping download."
    return 0
  fi

  local tarball="/tmp/harbor-offline-installer-${VERSION_NUM}.tgz"

  if [[ -f "$tarball" ]]; then
    log "Installer tarball already exists at ${tarball}. Skipping download."
  else
    log "Downloading Harbor ${HARBOR_VERSION} offline installer..."
    curl -fSL -o "$tarball" "$INSTALLER_URL"
    log "Download complete."
  fi

  log "Extracting installer to ${HARBOR_INSTALLER_DIR}..."
  mkdir -p "$(dirname "$HARBOR_INSTALLER_DIR")"
  tar -xzf "$tarball" -C "$(dirname "$HARBOR_INSTALLER_DIR")"

  # The tarball extracts to a "harbor" directory.
  if [[ "$(dirname "$HARBOR_INSTALLER_DIR")/harbor" != "$HARBOR_INSTALLER_DIR" ]]; then
    if [[ -d "$HARBOR_INSTALLER_DIR" ]]; then
      warn "Removing existing ${HARBOR_INSTALLER_DIR} before move."
      rm -rf "$HARBOR_INSTALLER_DIR"
    fi
    mv "$(dirname "$HARBOR_INSTALLER_DIR")/harbor" "$HARBOR_INSTALLER_DIR"
  fi

  log "Harbor extracted to ${HARBOR_INSTALLER_DIR}."
}

# ---------------------------------------------------------------------------
# Apply configuration
# ---------------------------------------------------------------------------
apply_config() {
  log "Copying harbor.yml to ${HARBOR_INSTALLER_DIR}/harbor.yml..."
  cp "$HARBOR_YML" "${HARBOR_INSTALLER_DIR}/harbor.yml"

  # Copy docker-compose override if present.
  local override="${SCRIPT_DIR}/docker-compose.override.yml"
  if [[ -f "$override" ]]; then
    log "Copying docker-compose.override.yml..."
    cp "$override" "${HARBOR_INSTALLER_DIR}/docker-compose.override.yml"
  fi

  # Replace placeholder passwords if HARBOR_PASS is set.
  if [[ -n "$HARBOR_PASS" ]]; then
    log "Setting Harbor admin password from HARBOR_PASS..."
    sed -i "s/^harbor_admin_password:.*/harbor_admin_password: ${HARBOR_PASS}/" \
      "${HARBOR_INSTALLER_DIR}/harbor.yml"
  else
    warn "HARBOR_PASS not set. The default CHANGE_ME password will be used."
  fi

  # Create data directories.
  mkdir -p "$HARBOR_DATA_DIR"
  mkdir -p "${HARBOR_DATA_DIR}/database"
  mkdir -p "${HARBOR_DATA_DIR}/trivy-cache"

  # Create internal TLS directory.
  mkdir -p /etc/harbor/tls/internal

  log "Configuration applied."
}

# ---------------------------------------------------------------------------
# Install Harbor
# ---------------------------------------------------------------------------
install_harbor() {
  log "Running Harbor installer with Trivy enabled..."
  cd "$HARBOR_INSTALLER_DIR"

  # Harbor's install.sh prepares docker-compose configs and starts services.
  bash ./install.sh --with-trivy

  log "Harbor installation complete."
}

# ---------------------------------------------------------------------------
# Wait for Harbor to be ready
# ---------------------------------------------------------------------------
wait_for_harbor() {
  local url="https://harbor.internal/api/v2.0/systeminfo"
  local max_wait=120
  local elapsed=0

  log "Waiting for Harbor to become ready (max ${max_wait}s)..."

  while [[ $elapsed -lt $max_wait ]]; do
    if curl -sSf -k "$url" &>/dev/null; then
      log "Harbor is ready."
      return 0
    fi
    sleep 5
    elapsed=$((elapsed + 5))
    log "  Waiting... (${elapsed}s)"
  done

  err "Harbor did not become ready within ${max_wait} seconds."
}

# ---------------------------------------------------------------------------
# Post-install: RBAC and GC
# ---------------------------------------------------------------------------
post_install() {
  local admin_pass="${HARBOR_PASS:-CHANGE_ME}"

  log "Running post-install configuration..."

  # RBAC setup.
  if [[ -x "${SCRIPT_DIR}/rbac-setup.sh" ]]; then
    log "Running RBAC setup..."
    HARBOR_URL="https://harbor.internal" \
    HARBOR_USER="admin" \
    HARBOR_PASS="$admin_pass" \
      bash "${SCRIPT_DIR}/rbac-setup.sh"
  else
    warn "rbac-setup.sh not found or not executable. Skipping RBAC setup."
  fi

  # GC schedule.
  if [[ -x "${SCRIPT_DIR}/gc-schedule.sh" ]]; then
    log "Configuring GC schedule..."
    HARBOR_URL="https://harbor.internal" \
    HARBOR_USER="admin" \
    HARBOR_PASS="$admin_pass" \
      bash "${SCRIPT_DIR}/gc-schedule.sh"
  else
    warn "gc-schedule.sh not found or not executable. Skipping GC setup."
  fi

  log "Post-install configuration complete."
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
  log "================================================================"
  log " AI-Box Harbor Registry Installer"
  log " Harbor version: ${HARBOR_VERSION}"
  log "================================================================"

  preflight
  download_harbor
  apply_config
  install_harbor
  wait_for_harbor
  post_install

  log ""
  log "================================================================"
  log " Harbor installation complete!"
  log ""
  log " Web UI:    https://harbor.internal"
  log " Admin:     admin / <HARBOR_PASS>"
  log " Metrics:   https://harbor.internal:9090/metrics"
  log ""
  log " Next steps:"
  log "   1. Change the admin password if you used the default."
  log "   2. Configure replication: ./replication-setup.sh"
  log "   3. Distribute TLS CA cert to developer machines."
  log "   4. Test: podman login harbor.internal"
  log "================================================================"
}

main "$@"

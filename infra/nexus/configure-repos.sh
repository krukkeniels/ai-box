#!/usr/bin/env bash
# configure-repos.sh -- Create all proxy, hosted, and group repositories in Nexus 3.x
#
# This script is idempotent: it checks whether each repository exists before
# creating it.  Re-running the script on an already-configured Nexus is safe.
#
# Prerequisites:
#   - Nexus is running and healthy on NEXUS_URL
#   - curl and jq are available
#
# Usage:
#   NEXUS_URL=http://nexus.internal:8081 \
#   NEXUS_USER=admin \
#   NEXUS_PASS=<admin-password> \
#   ./configure-repos.sh

set -euo pipefail

###############################################################################
# Configuration
###############################################################################
NEXUS_URL="${NEXUS_URL:-http://localhost:8081}"
NEXUS_USER="${NEXUS_USER:-admin}"
NEXUS_PASS="${NEXUS_PASS:-admin123}"
BLOB_STORE="${BLOB_STORE:-default}"

###############################################################################
# Helpers
###############################################################################
api() {
  local method="$1" path="$2"
  shift 2
  curl -sf -u "${NEXUS_USER}:${NEXUS_PASS}" \
    -H "Content-Type: application/json" \
    -X "${method}" \
    "${NEXUS_URL}/service/rest/v1/${path}" \
    "$@"
}

repo_exists() {
  local name="$1"
  api GET "repositories" | jq -e --arg n "$name" '.[] | select(.name == $n)' >/dev/null 2>&1
}

create_repo() {
  local format="$1" type="$2" name="$3" payload="$4"
  if repo_exists "$name"; then
    echo "[SKIP] ${name} already exists"
  else
    api POST "repositories/${format}/${type}" -d "$payload"
    echo "[CREATE] ${name}"
  fi
}

###############################################################################
# Enable anonymous access (realm)
###############################################################################
enable_anonymous_access() {
  echo "--- Enabling anonymous access ---"
  api PUT "security/anonymous" -d '{
    "enabled": true,
    "userId": "anonymous",
    "realmName": "NexusAuthorizingRealm"
  }' || true
  echo "[OK] Anonymous access enabled"
}

###############################################################################
# npm repositories
###############################################################################
create_npm_repos() {
  echo ""
  echo "=== npm ==="

  create_repo npm proxy npm-proxy "$(cat <<'JSON'
{
  "name": "npm-proxy",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true },
  "proxy": { "remoteUrl": "https://registry.npmjs.org", "contentMaxAge": 1440, "metadataMaxAge": 1440 },
  "negativeCache": { "enabled": true, "timeToLive": 1440 },
  "httpClient": { "blocked": false, "autoBlock": true }
}
JSON
  )"

  create_repo npm hosted npm-hosted "$(cat <<'JSON'
{
  "name": "npm-hosted",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true, "writePolicy": "ALLOW_ONCE" }
}
JSON
  )"

  create_repo npm group npm-group "$(cat <<'JSON'
{
  "name": "npm-group",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true },
  "group": { "memberNames": ["npm-hosted", "npm-proxy"] }
}
JSON
  )"
}

###############################################################################
# Maven repositories
###############################################################################
create_maven_repos() {
  echo ""
  echo "=== Maven ==="

  create_repo maven proxy maven-central-proxy "$(cat <<'JSON'
{
  "name": "maven-central-proxy",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true },
  "proxy": { "remoteUrl": "https://repo1.maven.org/maven2/", "contentMaxAge": 1440, "metadataMaxAge": 1440 },
  "negativeCache": { "enabled": true, "timeToLive": 1440 },
  "httpClient": { "blocked": false, "autoBlock": true },
  "maven": { "versionPolicy": "RELEASE", "layoutPolicy": "STRICT", "contentDisposition": "INLINE" }
}
JSON
  )"

  create_repo maven hosted maven-hosted "$(cat <<'JSON'
{
  "name": "maven-hosted",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true, "writePolicy": "ALLOW_ONCE" },
  "maven": { "versionPolicy": "MIXED", "layoutPolicy": "STRICT", "contentDisposition": "INLINE" }
}
JSON
  )"

  create_repo maven group maven-group "$(cat <<'JSON'
{
  "name": "maven-group",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true },
  "group": { "memberNames": ["maven-hosted", "maven-central-proxy"] },
  "maven": { "versionPolicy": "MIXED", "layoutPolicy": "STRICT", "contentDisposition": "INLINE" }
}
JSON
  )"
}

###############################################################################
# Gradle Plugin Portal (Maven format proxy)
###############################################################################
create_gradle_plugin_repos() {
  echo ""
  echo "=== Gradle Plugin Portal ==="

  create_repo maven proxy gradle-plugins-proxy "$(cat <<'JSON'
{
  "name": "gradle-plugins-proxy",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true },
  "proxy": { "remoteUrl": "https://plugins.gradle.org/m2/", "contentMaxAge": 1440, "metadataMaxAge": 1440 },
  "negativeCache": { "enabled": true, "timeToLive": 1440 },
  "httpClient": { "blocked": false, "autoBlock": true },
  "maven": { "versionPolicy": "RELEASE", "layoutPolicy": "STRICT", "contentDisposition": "INLINE" }
}
JSON
  )"
}

###############################################################################
# PyPI repositories
###############################################################################
create_pypi_repos() {
  echo ""
  echo "=== PyPI ==="

  create_repo pypi proxy pypi-proxy "$(cat <<'JSON'
{
  "name": "pypi-proxy",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true },
  "proxy": { "remoteUrl": "https://pypi.org/", "contentMaxAge": 1440, "metadataMaxAge": 1440 },
  "negativeCache": { "enabled": true, "timeToLive": 1440 },
  "httpClient": { "blocked": false, "autoBlock": true }
}
JSON
  )"

  create_repo pypi hosted pypi-hosted "$(cat <<'JSON'
{
  "name": "pypi-hosted",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true, "writePolicy": "ALLOW_ONCE" }
}
JSON
  )"

  create_repo pypi group pypi-group "$(cat <<'JSON'
{
  "name": "pypi-group",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true },
  "group": { "memberNames": ["pypi-hosted", "pypi-proxy"] }
}
JSON
  )"
}

###############################################################################
# NuGet repositories
###############################################################################
create_nuget_repos() {
  echo ""
  echo "=== NuGet ==="

  create_repo nuget proxy nuget-proxy "$(cat <<'JSON'
{
  "name": "nuget-proxy",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true },
  "proxy": { "remoteUrl": "https://api.nuget.org/v3/index.json", "contentMaxAge": 1440, "metadataMaxAge": 1440 },
  "negativeCache": { "enabled": true, "timeToLive": 1440 },
  "httpClient": { "blocked": false, "autoBlock": true },
  "nugetProxy": { "queryCacheItemMaxAge": 3600, "nugetVersion": "V3" }
}
JSON
  )"

  create_repo nuget group nuget-group "$(cat <<'JSON'
{
  "name": "nuget-group",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": true },
  "group": { "memberNames": ["nuget-proxy"] }
}
JSON
  )"
}

###############################################################################
# Go (raw proxy -- Nexus supports Go via raw format)
###############################################################################
create_go_repos() {
  echo ""
  echo "=== Go modules ==="

  create_repo raw proxy go-proxy "$(cat <<'JSON'
{
  "name": "go-proxy",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": false },
  "proxy": { "remoteUrl": "https://proxy.golang.org/", "contentMaxAge": 1440, "metadataMaxAge": 1440 },
  "negativeCache": { "enabled": true, "timeToLive": 1440 },
  "httpClient": { "blocked": false, "autoBlock": true }
}
JSON
  )"
}

###############################################################################
# Cargo / crates.io
# NOTE: Nexus 3.x does NOT natively support the Cargo registry protocol.
# This is documented as a gap.  A raw proxy is created to cache .crate files
# but the Cargo index protocol (sparse or git) is not supported by Nexus.
# See docs/nexus-mirror-urls.md for workarounds.
###############################################################################
create_cargo_repos() {
  echo ""
  echo "=== Cargo / crates.io (limited -- see docs) ==="
  echo "[NOTE] Nexus 3.x does not natively support the Cargo registry protocol."
  echo "[NOTE] Creating a raw proxy for crate file caching only."

  create_repo raw proxy cargo-proxy "$(cat <<'JSON'
{
  "name": "cargo-proxy",
  "online": true,
  "storage": { "blobStoreName": "default", "strictContentTypeValidation": false },
  "proxy": { "remoteUrl": "https://static.crates.io/", "contentMaxAge": 1440, "metadataMaxAge": 1440 },
  "negativeCache": { "enabled": true, "timeToLive": 1440 },
  "httpClient": { "blocked": false, "autoBlock": true }
}
JSON
  )"
}

###############################################################################
# Main
###############################################################################
main() {
  echo "Configuring Nexus at ${NEXUS_URL}"
  echo "Waiting for Nexus to be ready..."

  for i in $(seq 1 30); do
    if api GET "status" >/dev/null 2>&1; then
      echo "Nexus is ready."
      break
    fi
    if [ "$i" -eq 30 ]; then
      echo "ERROR: Nexus did not become ready within 5 minutes."
      exit 1
    fi
    sleep 10
  done

  enable_anonymous_access
  create_npm_repos
  create_maven_repos
  create_gradle_plugin_repos
  create_pypi_repos
  create_nuget_repos
  create_go_repos
  create_cargo_repos

  echo ""
  echo "=== Repository configuration complete ==="
  echo ""
  echo "Repositories created:"
  api GET "repositories" | jq -r '.[] | "  \(.format)\t\(.type)\t\(.name)"' | sort
}

main "$@"

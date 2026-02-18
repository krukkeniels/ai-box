#!/usr/bin/env bash
# cleanup-policies.sh -- Configure cleanup policies and compact blob store tasks
#
# Creates a cleanup policy that evicts cached proxy artifacts unused for 90 days,
# applies it to all proxy repositories, and schedules a compact blob store task.
#
# This script is idempotent.
#
# Prerequisites:
#   - Nexus is running and healthy on NEXUS_URL
#   - configure-repos.sh has been run
#   - curl and jq are available
#
# Usage:
#   NEXUS_URL=http://nexus.internal:8081 \
#   NEXUS_USER=admin \
#   NEXUS_PASS=<admin-password> \
#   ./cleanup-policies.sh

set -euo pipefail

###############################################################################
# Configuration
###############################################################################
NEXUS_URL="${NEXUS_URL:-http://localhost:8081}"
NEXUS_USER="${NEXUS_USER:-admin}"
NEXUS_PASS="${NEXUS_PASS:-admin123}"

POLICY_NAME="evict-unused-proxy-90d"
RETENTION_DAYS=90

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

###############################################################################
# Create cleanup policy
###############################################################################
create_cleanup_policy() {
  echo "--- Creating cleanup policy: ${POLICY_NAME} ---"

  # Check if policy already exists
  if api GET "lifecycle/cleanup/policy/${POLICY_NAME}" >/dev/null 2>&1; then
    echo "[SKIP] Cleanup policy '${POLICY_NAME}' already exists"
    return 0
  fi

  api POST "lifecycle/cleanup/policy" -d "$(cat <<JSON
{
  "name": "${POLICY_NAME}",
  "notes": "Evict cached proxy artifacts not downloaded in ${RETENTION_DAYS} days",
  "criteriaLastDownloaded": "${RETENTION_DAYS}",
  "format": "ALL_FORMATS"
}
JSON
  )"

  echo "[CREATE] Cleanup policy '${POLICY_NAME}'"
}

###############################################################################
# Apply cleanup policy to all proxy repositories
###############################################################################
apply_policy_to_proxy_repos() {
  echo ""
  echo "--- Applying cleanup policy to proxy repositories ---"

  local proxy_repos
  proxy_repos=$(api GET "repositories" | jq -r '.[] | select(.type == "proxy") | .name')

  if [ -z "$proxy_repos" ]; then
    echo "[WARN] No proxy repositories found"
    return 0
  fi

  for repo in $proxy_repos; do
    local format type
    format=$(api GET "repositories" | jq -r --arg n "$repo" '.[] | select(.name == $n) | .format')
    type="proxy"

    # Fetch current repo config
    local config
    config=$(api GET "repositories/${format}/${type}/${repo}" 2>/dev/null || echo "")

    if [ -z "$config" ]; then
      echo "[WARN] Could not fetch config for ${repo}, skipping"
      continue
    fi

    # Update the repo to include cleanup policy
    local updated
    updated=$(echo "$config" | jq --arg p "$POLICY_NAME" '.cleanup.policyNames = [$p]')

    api PUT "repositories/${format}/${type}/${repo}" -d "$updated" >/dev/null 2>&1 || true
    echo "[APPLY] ${POLICY_NAME} -> ${repo}"
  done
}

###############################################################################
# Schedule compact blob store task
###############################################################################
schedule_compact_task() {
  echo ""
  echo "--- Scheduling compact blob store task ---"

  local task_name="Compact default blob store"

  # Check if a compact task already exists
  local existing
  existing=$(api GET "tasks" | jq -r --arg n "$task_name" '.items[]? | select(.name == $n) | .id' 2>/dev/null || echo "")

  if [ -n "$existing" ]; then
    echo "[SKIP] Compact blob store task already exists (id: ${existing})"
    return 0
  fi

  api POST "tasks" -d "$(cat <<JSON
{
  "action": "blobstore.compact",
  "type": "blobstore.compact",
  "name": "${task_name}",
  "message": "Compact the default blob store to reclaim disk space",
  "currentState": "WAITING",
  "schedule": {
    "type": "weekly",
    "startDate": "2026-02-22T03:00:00",
    "timeZone": "UTC",
    "daysToRun": ["SUN"]
  },
  "taskProperties": {
    "blobstoreName": "default"
  },
  "enabled": true
}
JSON
  )" >/dev/null 2>&1 || true

  echo "[CREATE] Compact blob store task scheduled (weekly, Sunday 03:00 UTC)"
}

###############################################################################
# Schedule cleanup task
###############################################################################
schedule_cleanup_task() {
  echo ""
  echo "--- Scheduling cleanup task ---"

  local task_name="Cleanup unused proxy artifacts"

  local existing
  existing=$(api GET "tasks" | jq -r --arg n "$task_name" '.items[]? | select(.name == $n) | .id' 2>/dev/null || echo "")

  if [ -n "$existing" ]; then
    echo "[SKIP] Cleanup task already exists (id: ${existing})"
    return 0
  fi

  api POST "tasks" -d "$(cat <<JSON
{
  "action": "repository.cleanup",
  "type": "repository.cleanup",
  "name": "${task_name}",
  "message": "Run cleanup policies on all repositories",
  "currentState": "WAITING",
  "schedule": {
    "type": "weekly",
    "startDate": "2026-02-22T01:00:00",
    "timeZone": "UTC",
    "daysToRun": ["SUN"]
  },
  "enabled": true
}
JSON
  )" >/dev/null 2>&1 || true

  echo "[CREATE] Cleanup task scheduled (weekly, Sunday 01:00 UTC)"
}

###############################################################################
# Main
###############################################################################
main() {
  echo "Configuring cleanup policies on Nexus at ${NEXUS_URL}"
  echo ""

  create_cleanup_policy
  apply_policy_to_proxy_repos
  schedule_cleanup_task
  schedule_compact_task

  echo ""
  echo "=== Cleanup configuration complete ==="
  echo ""
  echo "Summary:"
  echo "  Policy: ${POLICY_NAME} (evict after ${RETENTION_DAYS} days unused)"
  echo "  Cleanup task: weekly, Sunday 01:00 UTC"
  echo "  Compact task: weekly, Sunday 03:00 UTC"
}

main "$@"

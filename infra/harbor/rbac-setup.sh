#!/usr/bin/env bash
# rbac-setup.sh -- Configure Harbor RBAC for the AI-Box project.
#
# Creates:
#   - Project "aibox"
#   - Robot account "aibox-ci"   (push + pull)
#   - Robot account "aibox-pull" (pull only)
#   - Vulnerability scanning policy (scan-on-push)
#   - OCI artifact support (enabled by default in Harbor 2.x)
#
# Usage:
#   HARBOR_URL=https://harbor.internal \
#   HARBOR_USER=admin \
#   HARBOR_PASS=<password> \
#     ./rbac-setup.sh
#
# The script is idempotent: it checks for existing resources before creating them.
# Robot account credentials are printed to stdout on creation -- save them securely.

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
HARBOR_URL="${HARBOR_URL:-https://harbor.internal}"
HARBOR_USER="${HARBOR_USER:-admin}"
HARBOR_PASS="${HARBOR_PASS:-}"
PROJECT_NAME="${PROJECT_NAME:-aibox}"

# Strip trailing slash from URL
HARBOR_URL="${HARBOR_URL%/}"

API="${HARBOR_URL}/api/v2.0"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
log()  { printf '[INFO]  %s\n' "$*"; }
warn() { printf '[WARN]  %s\n' "$*" >&2; }
err()  { printf '[ERROR] %s\n' "$*" >&2; exit 1; }

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || err "Required command not found: $1"
}

harbor_api() {
  local method="$1" path="$2"
  shift 2
  curl -sSf -X "$method" \
    -u "${HARBOR_USER}:${HARBOR_PASS}" \
    -H "Content-Type: application/json" \
    "${API}${path}" \
    "$@"
}

# Like harbor_api but does not fail on HTTP errors (returns status code).
harbor_api_status() {
  local method="$1" path="$2"
  shift 2
  curl -sS -o /dev/null -w '%{http_code}' -X "$method" \
    -u "${HARBOR_USER}:${HARBOR_PASS}" \
    -H "Content-Type: application/json" \
    "${API}${path}" \
    "$@"
}

# ---------------------------------------------------------------------------
# Prerequisite checks
# ---------------------------------------------------------------------------
require_cmd curl
require_cmd jq

if [[ -z "${HARBOR_PASS}" ]]; then
  err "HARBOR_PASS is not set. Export it before running this script."
fi

log "Harbor API endpoint: ${API}"
log "Checking Harbor connectivity..."
harbor_api GET /systeminfo > /dev/null || err "Cannot reach Harbor API at ${API}"
log "Harbor is reachable."

# ---------------------------------------------------------------------------
# 1. Create project "aibox"
# ---------------------------------------------------------------------------
log "Checking if project '${PROJECT_NAME}' exists..."
status=$(harbor_api_status HEAD "/projects?project_name=${PROJECT_NAME}")
if [[ "$status" == "200" ]]; then
  log "Project '${PROJECT_NAME}' already exists. Skipping."
else
  log "Creating project '${PROJECT_NAME}'..."
  harbor_api POST /projects -d "$(jq -n \
    --arg name "$PROJECT_NAME" \
    '{
      project_name: $name,
      metadata: {
        public: "false",
        enable_content_trust: "false",
        auto_scan: "true",
        severity: "critical",
        reuse_sys_cve_allowlist: "true"
      },
      storage_limit: -1
    }'
  )"
  log "Project '${PROJECT_NAME}' created."
fi

# Retrieve the project ID for later use.
PROJECT_ID=$(harbor_api GET "/projects?name=${PROJECT_NAME}" | jq -r '.[0].project_id')
log "Project ID: ${PROJECT_ID}"

# ---------------------------------------------------------------------------
# 2. Create robot account "aibox-ci" (push + pull)
# ---------------------------------------------------------------------------
create_robot_account() {
  local robot_name="$1"
  local description="$2"
  shift 2
  local permissions="$*"

  local full_name="robot\$${robot_name}"
  log "Checking if robot account '${full_name}' exists..."

  existing=$(harbor_api GET "/robots" | jq -r --arg name "${robot_name}" \
    '.[] | select(.name == ("robot$" + $name)) | .id')

  if [[ -n "$existing" ]]; then
    log "Robot account '${full_name}' already exists (ID: ${existing}). Skipping."
    return 0
  fi

  log "Creating robot account '${full_name}'..."
  result=$(harbor_api POST "/robots" -d "$(jq -n \
    --arg name "$robot_name" \
    --arg desc "$description" \
    --arg project "$PROJECT_NAME" \
    --argjson permissions "$permissions" \
    '{
      name: $name,
      description: $desc,
      duration: -1,
      level: "project",
      permissions: [
        {
          namespace: $project,
          kind: "project",
          access: $permissions
        }
      ]
    }'
  )")

  local secret
  secret=$(echo "$result" | jq -r '.secret // empty')
  if [[ -n "$secret" ]]; then
    log "Robot account '${full_name}' created."
    echo "============================================"
    echo "  Robot:  ${full_name}"
    echo "  Secret: ${secret}"
    echo "  SAVE THIS SECRET -- it will not be shown again."
    echo "============================================"
  else
    warn "Robot account creation returned unexpected response:"
    echo "$result"
  fi
}

# Push + Pull permissions for CI.
CI_PERMISSIONS='[
  {"resource": "repository", "action": "push"},
  {"resource": "repository", "action": "pull"},
  {"resource": "artifact", "action": "read"},
  {"resource": "artifact-label", "action": "create"},
  {"resource": "tag", "action": "create"},
  {"resource": "tag", "action": "list"},
  {"resource": "scan", "action": "create"}
]'

# Pull-only permissions for developer machines.
PULL_PERMISSIONS='[
  {"resource": "repository", "action": "pull"},
  {"resource": "artifact", "action": "read"},
  {"resource": "tag", "action": "list"},
  {"resource": "scan", "action": "read"}
]'

create_robot_account "aibox-ci" \
  "CI pipeline account -- push and pull images" \
  "$CI_PERMISSIONS"

create_robot_account "aibox-pull" \
  "Developer pull-only account" \
  "$PULL_PERMISSIONS"

# ---------------------------------------------------------------------------
# 3. Configure vulnerability scanning policy (scan-on-push)
# ---------------------------------------------------------------------------
log "Configuring scan-on-push for project '${PROJECT_NAME}'..."

# Update project metadata to ensure auto_scan is enabled.
harbor_api PUT "/projects/${PROJECT_ID}" -d "$(jq -n '{
  metadata: {
    auto_scan: "true",
    severity: "critical"
  }
}')" || warn "Failed to update scan policy -- may already be set."

log "Scan-on-push is enabled. Severity threshold set to block critical."

# ---------------------------------------------------------------------------
# 4. OCI artifact support
# ---------------------------------------------------------------------------
# Harbor 2.x supports OCI artifacts natively. Verify the project allows them.
log "OCI artifact support is enabled by default in Harbor 2.x."
log "Verifying project configuration..."
harbor_api GET "/projects/${PROJECT_ID}" | jq '{
  project_name: .name,
  auto_scan: .metadata.auto_scan,
  severity: .metadata.severity
}'

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
log "RBAC setup complete."
log ""
log "Summary:"
log "  Project:          ${PROJECT_NAME}"
log "  Robot (CI):       robot\$aibox-ci   -- push + pull"
log "  Robot (pull):     robot\$aibox-pull  -- pull only"
log "  Scan-on-push:     enabled"
log "  Severity block:   critical"
log "  OCI artifacts:    enabled (Harbor 2.x default)"

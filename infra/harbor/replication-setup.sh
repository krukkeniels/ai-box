#!/usr/bin/env bash
# replication-setup.sh -- Configure Harbor replication rules.
#
# Sets up a push-based replication rule to replicate the "aibox" project
# from this Harbor instance to a secondary (DR / air-gapped) Harbor instance.
#
# Usage:
#   HARBOR_URL=https://harbor.internal \
#   HARBOR_USER=admin \
#   HARBOR_PASS=<password> \
#   TARGET_URL=https://harbor-dr.internal \
#   TARGET_USER=admin \
#   TARGET_PASS=<password> \
#     ./replication-setup.sh
#
# The script is idempotent: it checks for existing endpoints and rules.

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
HARBOR_URL="${HARBOR_URL:-https://harbor.internal}"
HARBOR_USER="${HARBOR_USER:-admin}"
HARBOR_PASS="${HARBOR_PASS:-}"

TARGET_URL="${TARGET_URL:-https://harbor-dr.internal}"
TARGET_USER="${TARGET_USER:-admin}"
TARGET_PASS="${TARGET_PASS:-}"
TARGET_NAME="${TARGET_NAME:-harbor-dr}"
TARGET_INSECURE="${TARGET_INSECURE:-false}"

PROJECT_NAME="${PROJECT_NAME:-aibox}"
RULE_NAME="${RULE_NAME:-aibox-replication-to-dr}"

# Replication trigger: manual, event_based, or scheduled.
REPLICATION_TRIGGER="${REPLICATION_TRIGGER:-event_based}"
# Cron schedule (only used when REPLICATION_TRIGGER=scheduled).
REPLICATION_CRON="${REPLICATION_CRON:-0 0 2 * * *}"

# Strip trailing slashes.
HARBOR_URL="${HARBOR_URL%/}"
TARGET_URL="${TARGET_URL%/}"

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

[[ -z "$HARBOR_PASS" ]] && err "HARBOR_PASS is not set."
[[ -z "$TARGET_PASS" ]] && err "TARGET_PASS is not set."

log "Source Harbor: ${HARBOR_URL}"
log "Target Harbor: ${TARGET_URL}"
log "Project:       ${PROJECT_NAME}"

harbor_api GET /systeminfo > /dev/null || err "Cannot reach source Harbor API."

# ---------------------------------------------------------------------------
# 1. Create or find the registry endpoint for the target
# ---------------------------------------------------------------------------
log "Checking if registry endpoint '${TARGET_NAME}' exists..."

ENDPOINT_ID=$(harbor_api GET "/registries" | jq -r \
  --arg name "$TARGET_NAME" '.[] | select(.name == $name) | .id')

if [[ -n "$ENDPOINT_ID" ]]; then
  log "Registry endpoint '${TARGET_NAME}' already exists (ID: ${ENDPOINT_ID})."
else
  log "Creating registry endpoint '${TARGET_NAME}'..."
  harbor_api POST "/registries" -d "$(jq -n \
    --arg name "$TARGET_NAME" \
    --arg url "$TARGET_URL" \
    --arg user "$TARGET_USER" \
    --arg pass "$TARGET_PASS" \
    --argjson insecure "$TARGET_INSECURE" \
    '{
      name: $name,
      url: $url,
      type: "harbor",
      credential: {
        type: "basic",
        access_key: $user,
        access_secret: $pass
      },
      insecure: $insecure
    }'
  )"

  ENDPOINT_ID=$(harbor_api GET "/registries" | jq -r \
    --arg name "$TARGET_NAME" '.[] | select(.name == $name) | .id')
  log "Registry endpoint created (ID: ${ENDPOINT_ID})."
fi

# ---------------------------------------------------------------------------
# 2. Test endpoint connectivity
# ---------------------------------------------------------------------------
log "Testing connectivity to target endpoint..."
ping_status=$(harbor_api_status POST "/registries/ping" -d "$(jq -n \
  --argjson id "$ENDPOINT_ID" '{ id: $id }')")

if [[ "$ping_status" == "200" ]]; then
  log "Target endpoint is reachable."
else
  warn "Target endpoint ping returned HTTP ${ping_status}. Replication may fail until the target is reachable."
fi

# ---------------------------------------------------------------------------
# 3. Create replication rule
# ---------------------------------------------------------------------------
log "Checking if replication rule '${RULE_NAME}' exists..."

RULE_ID=$(harbor_api GET "/replication/policies" | jq -r \
  --arg name "$RULE_NAME" '.[] | select(.name == $name) | .id')

if [[ -n "$RULE_ID" ]]; then
  log "Replication rule '${RULE_NAME}' already exists (ID: ${RULE_ID}). Skipping."
else
  log "Creating replication rule '${RULE_NAME}'..."

  # Build trigger JSON based on type.
  case "$REPLICATION_TRIGGER" in
    event_based)
      TRIGGER_JSON='{"type": "event_based"}'
      ;;
    scheduled)
      TRIGGER_JSON=$(jq -n --arg cron "$REPLICATION_CRON" \
        '{ type: "scheduled", trigger_settings: { cron: $cron } }')
      ;;
    manual)
      TRIGGER_JSON='{"type": "manual"}'
      ;;
    *)
      err "Unknown REPLICATION_TRIGGER: ${REPLICATION_TRIGGER}"
      ;;
  esac

  harbor_api POST "/replication/policies" -d "$(jq -n \
    --arg name "$RULE_NAME" \
    --arg project "$PROJECT_NAME" \
    --argjson endpoint_id "$ENDPOINT_ID" \
    --argjson trigger "$TRIGGER_JSON" \
    '{
      name: $name,
      description: "Replicate aibox project images to DR/secondary Harbor instance",
      src_registry: null,
      dest_registry: { id: $endpoint_id },
      dest_namespace_replace_count: 0,
      trigger: $trigger,
      filters: [
        {
          type: "name",
          value: ($project + "/**")
        }
      ],
      enabled: true,
      deletion: false,
      override: true,
      speed: -1
    }'
  )"

  RULE_ID=$(harbor_api GET "/replication/policies" | jq -r \
    --arg name "$RULE_NAME" '.[] | select(.name == $name) | .id')
  log "Replication rule created (ID: ${RULE_ID})."
fi

# ---------------------------------------------------------------------------
# 4. Optionally trigger an initial replication
# ---------------------------------------------------------------------------
if [[ "${TRIGGER_INITIAL:-false}" == "true" && -n "$RULE_ID" ]]; then
  log "Triggering initial replication execution..."
  harbor_api POST "/replication/executions" -d "$(jq -n \
    --argjson id "$RULE_ID" '{ policy_id: $id }')"
  log "Replication execution started."
fi

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
log "Replication setup complete."
log ""
log "Summary:"
log "  Target endpoint: ${TARGET_NAME} (${TARGET_URL})"
log "  Rule name:       ${RULE_NAME}"
log "  Trigger:         ${REPLICATION_TRIGGER}"
log "  Filter:          ${PROJECT_NAME}/**"
log ""
log "To trigger a manual replication:"
log "  curl -X POST -u admin:<pass> ${API}/replication/executions -H 'Content-Type: application/json' -d '{\"policy_id\": ${RULE_ID}}'"

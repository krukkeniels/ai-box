#!/usr/bin/env bash
# gc-schedule.sh -- Configure Harbor garbage collection schedule.
#
# Schedules weekly garbage collection to reclaim storage from
# deleted/untagged image layers.
#
# Usage:
#   HARBOR_URL=https://harbor.internal \
#   HARBOR_USER=admin \
#   HARBOR_PASS=<password> \
#     ./gc-schedule.sh
#
# Options (environment variables):
#   GC_CRON      -- Cron expression for GC schedule (default: weekly Sunday 03:00 UTC)
#   GC_DELETE_UNTAGGED -- Delete untagged artifacts (default: true)
#   GC_DRY_RUN   -- Run in dry-run mode first (default: false)

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
HARBOR_URL="${HARBOR_URL:-https://harbor.internal}"
HARBOR_USER="${HARBOR_USER:-admin}"
HARBOR_PASS="${HARBOR_PASS:-}"

# Weekly GC: Sunday 03:00 UTC.
# Harbor uses a 6-field cron: second minute hour day month weekday
GC_CRON="${GC_CRON:-0 0 3 * * 0}"
GC_DELETE_UNTAGGED="${GC_DELETE_UNTAGGED:-true}"
GC_DRY_RUN="${GC_DRY_RUN:-false}"
GC_WORKERS="${GC_WORKERS:-1}"

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

# ---------------------------------------------------------------------------
# Prerequisite checks
# ---------------------------------------------------------------------------
require_cmd curl
require_cmd jq

[[ -z "$HARBOR_PASS" ]] && err "HARBOR_PASS is not set."

log "Harbor API: ${API}"
harbor_api GET /systeminfo > /dev/null || err "Cannot reach Harbor API."

# ---------------------------------------------------------------------------
# 1. Show current GC schedule (if any)
# ---------------------------------------------------------------------------
log "Current GC schedule:"
current=$(harbor_api GET "/system/gc/schedule" 2>/dev/null || echo '{}')
echo "$current" | jq '.' 2>/dev/null || echo "$current"

# ---------------------------------------------------------------------------
# 2. Optionally run a dry-run first
# ---------------------------------------------------------------------------
if [[ "$GC_DRY_RUN" == "true" ]]; then
  log "Running GC dry-run to estimate reclaimable space..."
  harbor_api POST "/system/gc/schedule" -d "$(jq -n \
    --argjson delete_untagged "$GC_DELETE_UNTAGGED" \
    --argjson workers "$GC_WORKERS" \
    '{
      schedule: { type: "Manual" },
      parameters: {
        delete_untagged: $delete_untagged,
        dry_run: true,
        workers: $workers
      }
    }'
  )"
  log "Dry-run triggered. Check Harbor UI or /system/gc for results."
  log "Waiting 10 seconds for dry-run to start..."
  sleep 10

  # Show latest GC result.
  harbor_api GET "/system/gc" | jq '.[0] | {
    id, job_status, creation_time,
    delete_untagged: .job_parameters.delete_untagged,
    dry_run: .job_parameters.dry_run
  }' 2>/dev/null || true
fi

# ---------------------------------------------------------------------------
# 3. Set the weekly GC schedule
# ---------------------------------------------------------------------------
log "Setting GC schedule: ${GC_CRON}"
log "Delete untagged: ${GC_DELETE_UNTAGGED}"

harbor_api PUT "/system/gc/schedule" -d "$(jq -n \
  --arg cron "$GC_CRON" \
  --argjson delete_untagged "$GC_DELETE_UNTAGGED" \
  --argjson workers "$GC_WORKERS" \
  '{
    schedule: {
      type: "Custom",
      cron: $cron
    },
    parameters: {
      delete_untagged: $delete_untagged,
      dry_run: false,
      workers: $workers
    }
  }'
)"

log "GC schedule configured."

# ---------------------------------------------------------------------------
# 4. Verify the schedule was applied
# ---------------------------------------------------------------------------
log "Verifying GC schedule..."
harbor_api GET "/system/gc/schedule" | jq '{
  type: .schedule.type,
  cron: .schedule.cron,
  delete_untagged: .parameters.delete_untagged,
  workers: .parameters.workers
}'

# ---------------------------------------------------------------------------
# 5. Show recent GC history
# ---------------------------------------------------------------------------
log "Recent GC history (last 5 runs):"
harbor_api GET "/system/gc?page=1&page_size=5" | jq '[.[] | {
  id,
  status: .job_status,
  started: .creation_time,
  finished: .update_time,
  dry_run: .job_parameters.dry_run
}]' 2>/dev/null || log "No GC history yet."

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
log ""
log "GC schedule set successfully."
log "  Schedule:         ${GC_CRON} (weekly Sunday 03:00 UTC by default)"
log "  Delete untagged:  ${GC_DELETE_UNTAGGED}"
log "  Workers:          ${GC_WORKERS}"
log ""
log "To trigger GC manually:"
log "  curl -X POST -u admin:<pass> ${API}/system/gc/schedule \\"
log "    -H 'Content-Type: application/json' \\"
log "    -d '{\"schedule\":{\"type\":\"Manual\"},\"parameters\":{\"delete_untagged\":true,\"dry_run\":false,\"workers\":1}}'"

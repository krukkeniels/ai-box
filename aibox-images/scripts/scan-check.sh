#!/usr/bin/env bash
# =============================================================================
# AI-Box Trivy Scan Script
# =============================================================================
# Scans a container image with Trivy and enforces severity thresholds.
# CRITICAL findings cause a failure (exit 1). HIGH findings are warned.
# MEDIUM/LOW findings are logged as informational.
#
# Usage:
#   ./scripts/scan-check.sh <image_ref> [--report <path>]
#
# Arguments:
#   image_ref   - Full image reference (e.g. harbor.internal/aibox/base:24.04)
#   --report    - Optional: save full JSON report to the given path
#
# Exit codes:
#   0 - No critical CVEs found (pass)
#   1 - Critical CVEs found (fail)
# =============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
log()  { echo "[scan] $(date '+%Y-%m-%d %H:%M:%S') $*"; }
warn() { echo "[scan] WARNING: $*" >&2; }
err()  { echo "[scan] ERROR: $*" >&2; }

# ---------------------------------------------------------------------------
# Usage
# ---------------------------------------------------------------------------
usage() {
    cat <<EOF
Usage: $(basename "$0") <image_ref> [--report <path>]

Arguments:
  image_ref   Full image reference (e.g. harbor.internal/aibox/base:24.04)
  --report    Save full JSON report to the given file path

Exit codes:
  0  No critical CVEs found (pass)
  1  Critical CVEs found (fail)

Examples:
  $(basename "$0") harbor.internal/aibox/base:24.04
  $(basename "$0") harbor.internal/aibox/java:21-24.04 --report /tmp/trivy-java.json
EOF
    exit 1
}

# ---------------------------------------------------------------------------
# Parse results and print summary table
# ---------------------------------------------------------------------------
print_summary() {
    local json_file="$1"

    local critical high medium low unknown
    critical=$(jq '[.Results[]?.Vulnerabilities[]? | select(.Severity == "CRITICAL")] | length' "$json_file")
    high=$(jq '[.Results[]?.Vulnerabilities[]? | select(.Severity == "HIGH")] | length' "$json_file")
    medium=$(jq '[.Results[]?.Vulnerabilities[]? | select(.Severity == "MEDIUM")] | length' "$json_file")
    low=$(jq '[.Results[]?.Vulnerabilities[]? | select(.Severity == "LOW")] | length' "$json_file")
    unknown=$(jq '[.Results[]?.Vulnerabilities[]? | select(.Severity == "UNKNOWN")] | length' "$json_file")

    echo ""
    echo "============================================"
    echo " Trivy Scan Summary"
    echo "============================================"
    printf " %-12s %s\n" "CRITICAL:" "$critical"
    printf " %-12s %s\n" "HIGH:" "$high"
    printf " %-12s %s\n" "MEDIUM:" "$medium"
    printf " %-12s %s\n" "LOW:" "$low"
    printf " %-12s %s\n" "UNKNOWN:" "$unknown"
    echo "============================================"
    echo ""

    # Log details for CRITICAL and HIGH
    if [[ "$critical" -gt 0 ]]; then
        err "CRITICAL vulnerabilities found:"
        jq -r '.Results[]?.Vulnerabilities[]? | select(.Severity == "CRITICAL") | "  \(.VulnerabilityID) - \(.PkgName) \(.InstalledVersion) -> \(.FixedVersion // "no fix") : \(.Title // "no title")"' "$json_file"
    fi

    if [[ "$high" -gt 0 ]]; then
        warn "HIGH vulnerabilities found:"
        jq -r '.Results[]?.Vulnerabilities[]? | select(.Severity == "HIGH") | "  \(.VulnerabilityID) - \(.PkgName) \(.InstalledVersion) -> \(.FixedVersion // "no fix") : \(.Title // "no title")"' "$json_file"
    fi

    # Return critical count for exit code decision
    echo "$critical"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    if [[ $# -lt 1 ]]; then
        err "Missing required argument: image_ref"
        usage
    fi

    local image_ref="$1"
    shift

    local report_path=""

    # Parse optional flags
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --report)
                if [[ $# -lt 2 ]]; then
                    err "--report requires a file path argument"
                    exit 1
                fi
                report_path="$2"
                shift 2
                ;;
            *)
                err "Unknown argument: $1"
                usage
                ;;
        esac
    done

    # Check trivy is available
    if ! command -v trivy &>/dev/null; then
        err "trivy is not installed or not in PATH"
        exit 1
    fi

    log "Scanning image: ${image_ref}"

    # Create temp file for JSON output
    local tmp_report
    tmp_report=$(mktemp /tmp/trivy-report-XXXXXX.json)
    trap 'rm -f "$tmp_report"' EXIT

    # Run trivy scan
    trivy image \
        --format json \
        --output "$tmp_report" \
        --severity CRITICAL,HIGH,MEDIUM,LOW \
        "${image_ref}"

    # Save report if requested
    if [[ -n "$report_path" ]]; then
        cp "$tmp_report" "$report_path"
        log "Full report saved to: ${report_path}"
    fi

    # Print summary and capture critical count (last line of output)
    local summary_output
    summary_output=$(print_summary "$tmp_report")

    # Print everything except the last line (which is the critical count)
    echo "$summary_output" | head -n -1

    # Extract critical count from the last line
    local critical_count
    critical_count=$(echo "$summary_output" | tail -n 1)

    if [[ "$critical_count" -gt 0 ]]; then
        err "FAIL: ${critical_count} critical CVE(s) found. Blocking push."
        exit 1
    fi

    log "PASS: No critical CVEs found."
    exit 0
}

main "$@"

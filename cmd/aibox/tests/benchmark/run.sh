#!/usr/bin/env bash
# =============================================================================
# AI-Box Performance Benchmark Script
# =============================================================================
# Measures cold and warm start times for aibox containers.
# Requires: hyperfine, aibox CLI, podman/docker.
#
# Usage:
#   ./run.sh                   # Run all benchmarks
#   ./run.sh --json            # Output JSON for CI
#   ./run.sh --runs 5          # Custom number of runs
# =============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
BINARY="${PROJECT_DIR}/bin/aibox"
WORKSPACE="/tmp/aibox-benchmark-workspace"
RUNS="${RUNS:-10}"
OUTPUT_FORMAT="text"
RESULTS_DIR="${SCRIPT_DIR}/results"

# Parse arguments.
while [[ $# -gt 0 ]]; do
    case "$1" in
        --json) OUTPUT_FORMAT="json"; shift ;;
        --runs) RUNS="$2"; shift 2 ;;
        *) echo "Unknown argument: $1"; exit 1 ;;
    esac
done

# Check prerequisites.
check_prereqs() {
    if ! command -v hyperfine &>/dev/null; then
        echo "ERROR: hyperfine not found. Install it: https://github.com/sharkdp/hyperfine"
        exit 1
    fi

    if [[ ! -x "$BINARY" ]]; then
        echo "Building aibox CLI..."
        (cd "$PROJECT_DIR" && make build)
    fi

    if ! command -v podman &>/dev/null && ! command -v docker &>/dev/null; then
        echo "ERROR: No container runtime (podman/docker) found."
        exit 1
    fi

    mkdir -p "$WORKSPACE" "$RESULTS_DIR"
}

# Ensure the base image is cached.
ensure_image() {
    local rt
    rt=$(command -v podman || command -v docker)
    local image
    image=$("$BINARY" --help 2>/dev/null | head -1 || echo "")

    echo "Ensuring base image is cached..."
    # Use the default image from config.
    "$BINARY" update 2>/dev/null || true
}

# Benchmark: Warm start (image cached, container stopped).
bench_warm_start() {
    echo "=== Warm Start Benchmark (${RUNS} runs) ==="
    echo "  Image cached, container stopped -> started"
    echo ""

    local json_flag=""
    if [[ "$OUTPUT_FORMAT" == "json" ]]; then
        json_flag="--export-json ${RESULTS_DIR}/warm-start.json"
    fi

    # Cleanup function between runs.
    hyperfine \
        --warmup 1 \
        --runs "$RUNS" \
        --prepare "$BINARY stop 2>/dev/null || true; sleep 1" \
        --cleanup "$BINARY stop 2>/dev/null || true" \
        $json_flag \
        "$BINARY start --workspace $WORKSPACE"

    echo ""
    echo "Target: p95 < 15 seconds"
}

# Benchmark: Cold start (no container, image cached).
bench_cold_start() {
    echo "=== Cold Start Benchmark (${RUNS} runs) ==="
    echo "  No existing container, image cached"
    echo ""

    local rt
    rt=$(command -v podman || command -v docker)

    local json_flag=""
    if [[ "$OUTPUT_FORMAT" == "json" ]]; then
        json_flag="--export-json ${RESULTS_DIR}/cold-start.json"
    fi

    # Full cleanup between runs: stop and remove container.
    hyperfine \
        --warmup 1 \
        --runs "$RUNS" \
        --prepare "$BINARY stop 2>/dev/null || true; $rt ps -a --filter label=aibox=true --format '{{.Names}}' | xargs -r $rt rm -f 2>/dev/null || true; sleep 1" \
        --cleanup "$BINARY stop 2>/dev/null || true" \
        $json_flag \
        "$BINARY start --workspace $WORKSPACE"

    echo ""
    echo "Target: p95 < 90 seconds"
}

# Main.
main() {
    check_prereqs

    echo "======================================"
    echo "  AI-Box Performance Benchmarks"
    echo "======================================"
    echo "Runs per benchmark: $RUNS"
    echo "Workspace: $WORKSPACE"
    echo "Binary: $BINARY"
    echo ""

    bench_warm_start
    echo ""
    bench_cold_start

    echo ""
    echo "======================================"
    echo "  Benchmarks complete"
    echo "======================================"

    if [[ "$OUTPUT_FORMAT" == "json" ]] && [[ -d "$RESULTS_DIR" ]]; then
        echo "JSON results saved to: $RESULTS_DIR/"
        ls -la "$RESULTS_DIR"/*.json 2>/dev/null || true
    fi

    # Cleanup workspace.
    rm -rf "$WORKSPACE"
}

main "$@"

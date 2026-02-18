#!/usr/bin/env bash
# test-verification.sh - Test Cosign signing and verification setup
#
# Usage:
#   ./test-verification.sh [--key PATH] [--registry REGISTRY]
#
# Runs a suite of verification tests against the Cosign signing
# infrastructure and client-side policy configuration.
#
# Options:
#   --key PATH          Path to public key (default: /etc/aibox/cosign.pub)
#   --registry REGISTRY Harbor registry hostname (default: harbor.internal)
#   --image IMAGE       Signed image to test with (default: <registry>/aibox/base:24.04)
#   --skip-pull         Skip tests that require pulling images (offline mode)
#
# Exit codes:
#   0 - All tests passed
#   1 - One or more tests failed
set -euo pipefail

# ── Configuration ────────────────────────────────────────────────────
PUBLIC_KEY="/etc/aibox/cosign.pub"
REGISTRY="harbor.internal"
IMAGE=""
SKIP_PULL=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --key)        PUBLIC_KEY="$2"; shift 2 ;;
        --registry)   REGISTRY="$2";  shift 2 ;;
        --image)      IMAGE="$2";     shift 2 ;;
        --skip-pull)  SKIP_PULL=true; shift   ;;
        -h|--help)
            sed -n '2,/^set /{ /^#/s/^# \?//p }' "$0"
            exit 0
            ;;
        *)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
    esac
done

[[ -z "$IMAGE" ]] && IMAGE="${REGISTRY}/aibox/base:24.04"

# ── Test harness ─────────────────────────────────────────────────────
PASS=0
FAIL=0
SKIP=0
TOTAL=0

run_test() {
    local test_num="$1"
    local description="$2"
    TOTAL=$((TOTAL + 1))
    echo ""
    echo "--- Test ${test_num}: ${description} ---"
}

pass() {
    PASS=$((PASS + 1))
    echo "ok ${TOTAL} - $1"
}

fail() {
    FAIL=$((FAIL + 1))
    echo "not ok ${TOTAL} - $1"
}

skip() {
    SKIP=$((SKIP + 1))
    echo "ok ${TOTAL} - SKIP: $1"
}

# ── TAP header ───────────────────────────────────────────────────────
echo "TAP version 13"
echo "# Cosign verification test suite"
echo "# Registry : $REGISTRY"
echo "# Key      : $PUBLIC_KEY"
echo "# Image    : $IMAGE"
echo "# Date     : $(date -u +%Y-%m-%dT%H:%M:%SZ)"

# ── Test 1: Verify a signed image ────────────────────────────────────
run_test 1 "Verify signed image with cosign"

if [[ "$SKIP_PULL" == true ]]; then
    skip "Pull tests skipped (--skip-pull)"
elif ! command -v cosign &>/dev/null; then
    skip "cosign not installed"
else
    if cosign verify --key "$PUBLIC_KEY" "$IMAGE" &>/dev/null; then
        pass "Signed image verification succeeded: $IMAGE"
    else
        fail "Signed image verification failed: $IMAGE"
    fi
fi

# ── Test 2: Reject image with wrong/missing signature ────────────────
run_test 2 "Reject image with invalid signature"

if [[ "$SKIP_PULL" == true ]]; then
    skip "Pull tests skipped (--skip-pull)"
elif ! command -v cosign &>/dev/null; then
    skip "cosign not installed"
else
    # Generate a throwaway key to simulate a wrong signer
    TMPDIR_KEYS=$(mktemp -d)
    trap 'rm -rf "$TMPDIR_KEYS"' EXIT

    # Generate a different key pair (non-interactive with empty password)
    COSIGN_PASSWORD="" cosign generate-key-pair --output-key-prefix="${TMPDIR_KEYS}/wrong" &>/dev/null 2>&1 || true
    WRONG_KEY="${TMPDIR_KEYS}/wrong.pub"

    if [[ -f "$WRONG_KEY" ]]; then
        if cosign verify --key "$WRONG_KEY" "$IMAGE" &>/dev/null 2>&1; then
            fail "Image should NOT verify with wrong key but did"
        else
            pass "Image correctly rejected with wrong key"
        fi
    else
        skip "Could not generate throwaway key for negative test"
    fi
fi

# ── Test 3: Verify policy.json is correctly installed ────────────────
run_test 3 "Verify policy.json is installed at /etc/containers/policy.json"

POLICY_FILE="/etc/containers/policy.json"
if [[ ! -f "$POLICY_FILE" ]]; then
    fail "policy.json not found at $POLICY_FILE"
else
    # Check that the policy requires sigstoreSigned for harbor.internal
    if command -v jq &>/dev/null; then
        HARBOR_TYPE=$(jq -r '.transports.docker["harbor.internal"][0].type // empty' "$POLICY_FILE" 2>/dev/null)
        HARBOR_KEY=$(jq -r '.transports.docker["harbor.internal"][0].keyPath // empty' "$POLICY_FILE" 2>/dev/null)

        if [[ "$HARBOR_TYPE" == "sigstoreSigned" ]]; then
            echo "  policy type for harbor.internal: $HARBOR_TYPE"
            echo "  keyPath: $HARBOR_KEY"
            if [[ "$HARBOR_KEY" == "/etc/aibox/cosign.pub" ]]; then
                pass "policy.json correctly requires sigstoreSigned for $REGISTRY"
            else
                fail "policy.json keyPath is '$HARBOR_KEY', expected '/etc/aibox/cosign.pub'"
            fi
        else
            fail "policy.json type for harbor.internal is '$HARBOR_TYPE', expected 'sigstoreSigned'"
        fi
    else
        # Fallback: simple grep check
        if grep -q "sigstoreSigned" "$POLICY_FILE" && grep -q "harbor.internal" "$POLICY_FILE"; then
            pass "policy.json contains sigstoreSigned rule for harbor.internal (jq not available for deep check)"
        else
            fail "policy.json does not contain expected sigstoreSigned rule"
        fi
    fi
fi

# ── Test 4: Verify registries.d configuration ────────────────────────
run_test 4 "Verify registries.d/harbor.yaml is installed"

REGISTRIES_FILE="/etc/containers/registries.d/harbor.yaml"
if [[ ! -f "$REGISTRIES_FILE" ]]; then
    fail "harbor.yaml not found at $REGISTRIES_FILE"
else
    if grep -q "use-sigstore-attachments: true" "$REGISTRIES_FILE" && \
       grep -q "harbor.internal" "$REGISTRIES_FILE"; then
        pass "registries.d/harbor.yaml has sigstore-attachments enabled for $REGISTRY"
    else
        fail "registries.d/harbor.yaml missing expected configuration"
    fi
fi

# ── Test 5: Verify public key is installed ───────────────────────────
run_test 5 "Verify cosign public key is installed"

if [[ ! -f "$PUBLIC_KEY" ]]; then
    fail "Public key not found at $PUBLIC_KEY"
else
    # Basic check: cosign public keys start with a PEM header
    if head -1 "$PUBLIC_KEY" | grep -q "BEGIN PUBLIC KEY"; then
        pass "Public key is present and has valid PEM header"
    else
        fail "Public key exists but does not appear to be a valid PEM file"
    fi
fi

# ── Test 6: Verify podman respects policy (pull signed image) ────────
run_test 6 "Podman pull of signed image respects policy"

if [[ "$SKIP_PULL" == true ]]; then
    skip "Pull tests skipped (--skip-pull)"
elif ! command -v podman &>/dev/null; then
    skip "podman not installed"
elif [[ ! -f "$POLICY_FILE" ]]; then
    skip "policy.json not installed"
else
    if podman pull "$IMAGE" &>/dev/null; then
        pass "podman pull of signed image succeeded"
        # Clean up pulled image
        podman rmi "$IMAGE" &>/dev/null || true
    else
        fail "podman pull of signed image failed"
    fi
fi

# ── Summary ──────────────────────────────────────────────────────────
echo ""
echo "1..${TOTAL}"
echo ""
echo "============================================================"
echo "  Test Summary"
echo "============================================================"
echo "  Total  : $TOTAL"
echo "  Passed : $PASS"
echo "  Failed : $FAIL"
echo "  Skipped: $SKIP"
echo "============================================================"

if [[ "$FAIL" -gt 0 ]]; then
    echo "  RESULT: FAIL"
    echo "============================================================"
    exit 1
else
    echo "  RESULT: PASS"
    echo "============================================================"
    exit 0
fi

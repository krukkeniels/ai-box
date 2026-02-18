#!/usr/bin/env bash
# test-mirrors.sh -- Verify that all Nexus mirror repositories are functional
#
# Tests each package format by performing a real install through the Nexus proxy.
# Reports pass/fail for each format.
#
# Prerequisites:
#   - Nexus is running with repositories configured (configure-repos.sh)
#   - npm, mvn, pip, and go are available on the test machine
#   - curl and jq are available
#
# Usage:
#   NEXUS_URL=http://nexus.internal:8081 ./test-mirrors.sh
#
# Environment variables:
#   NEXUS_URL   Base URL of Nexus (default: http://localhost:8081)
#   SKIP_NPM    Set to 1 to skip npm test
#   SKIP_MAVEN  Set to 1 to skip Maven test
#   SKIP_PYPI   Set to 1 to skip PyPI test
#   SKIP_GO     Set to 1 to skip Go test
#   SKIP_NUGET  Set to 1 to skip NuGet test

set -euo pipefail

###############################################################################
# Configuration
###############################################################################
NEXUS_URL="${NEXUS_URL:-http://localhost:8081}"
WORK_DIR=$(mktemp -d)
PASS=0
FAIL=0
SKIP=0

###############################################################################
# Helpers
###############################################################################
cleanup() {
  rm -rf "$WORK_DIR"
}
trap cleanup EXIT

report() {
  local status="$1" format="$2" detail="${3:-}"
  case "$status" in
    PASS) echo "[PASS] ${format}: ${detail}"; PASS=$((PASS + 1)) ;;
    FAIL) echo "[FAIL] ${format}: ${detail}"; FAIL=$((FAIL + 1)) ;;
    SKIP) echo "[SKIP] ${format}: ${detail}"; SKIP=$((SKIP + 1)) ;;
  esac
}

###############################################################################
# Test: Nexus is reachable
###############################################################################
test_nexus_health() {
  echo "=== Testing Nexus connectivity ==="
  if curl -sf "${NEXUS_URL}/service/rest/v1/status" >/dev/null 2>&1; then
    report PASS "nexus-health" "Nexus is reachable at ${NEXUS_URL}"
  else
    report FAIL "nexus-health" "Cannot reach Nexus at ${NEXUS_URL}"
    echo "ERROR: Nexus is not reachable. Aborting."
    exit 1
  fi
  echo ""
}

###############################################################################
# Test: npm
###############################################################################
test_npm() {
  echo "=== Testing npm mirror ==="
  if [ "${SKIP_NPM:-0}" = "1" ]; then
    report SKIP "npm" "Skipped by SKIP_NPM=1"
    return
  fi

  if ! command -v npm >/dev/null 2>&1; then
    report SKIP "npm" "npm not installed"
    return
  fi

  local npm_dir="${WORK_DIR}/npm-test"
  mkdir -p "$npm_dir"

  # Write .npmrc for this test
  cat > "${npm_dir}/.npmrc" <<EOF
registry=${NEXUS_URL}/repository/npm-group/
strict-ssl=false
EOF

  cd "$npm_dir"
  if npm init -y >/dev/null 2>&1 && \
     npm install --registry="${NEXUS_URL}/repository/npm-group/" express >/dev/null 2>&1; then
    report PASS "npm" "npm install express succeeded via npm-group"
  else
    report FAIL "npm" "npm install express failed"
  fi
}

###############################################################################
# Test: Maven
###############################################################################
test_maven() {
  echo "=== Testing Maven mirror ==="
  if [ "${SKIP_MAVEN:-0}" = "1" ]; then
    report SKIP "maven" "Skipped by SKIP_MAVEN=1"
    return
  fi

  if ! command -v mvn >/dev/null 2>&1; then
    report SKIP "maven" "mvn not installed"
    return
  fi

  local mvn_dir="${WORK_DIR}/maven-test"
  mkdir -p "$mvn_dir"

  # Write a test settings.xml
  cat > "${mvn_dir}/settings.xml" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<settings xmlns="http://maven.apache.org/SETTINGS/1.2.0"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
          xsi:schemaLocation="http://maven.apache.org/SETTINGS/1.2.0
                              http://maven.apache.org/xsd/settings-1.2.0.xsd">
  <mirrors>
    <mirror>
      <id>nexus-maven</id>
      <mirrorOf>central</mirrorOf>
      <url>${NEXUS_URL}/repository/maven-group/</url>
    </mirror>
  </mirrors>
</settings>
EOF

  # Write a minimal pom.xml
  cat > "${mvn_dir}/pom.xml" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0
                             http://maven.apache.org/xsd/maven-4.0.0.xsd">
  <modelVersion>4.0.0</modelVersion>
  <groupId>com.aibox.test</groupId>
  <artifactId>mirror-test</artifactId>
  <version>1.0.0</version>
  <dependencies>
    <dependency>
      <groupId>com.google.guava</groupId>
      <artifactId>guava</artifactId>
      <version>33.0.0-jre</version>
    </dependency>
  </dependencies>
</project>
EOF

  cd "$mvn_dir"
  if mvn -s settings.xml dependency:resolve -q >/dev/null 2>&1; then
    report PASS "maven" "mvn dependency:resolve succeeded via maven-group"
  else
    report FAIL "maven" "mvn dependency:resolve failed"
  fi
}

###############################################################################
# Test: PyPI
###############################################################################
test_pypi() {
  echo "=== Testing PyPI mirror ==="
  if [ "${SKIP_PYPI:-0}" = "1" ]; then
    report SKIP "pypi" "Skipped by SKIP_PYPI=1"
    return
  fi

  if ! command -v pip >/dev/null 2>&1 && ! command -v pip3 >/dev/null 2>&1; then
    report SKIP "pypi" "pip/pip3 not installed"
    return
  fi

  local pip_cmd="pip"
  command -v pip >/dev/null 2>&1 || pip_cmd="pip3"

  local pip_dir="${WORK_DIR}/pypi-test"
  mkdir -p "$pip_dir"

  # Write pip.conf for reference
  cat > "${pip_dir}/pip.conf" <<EOF
[global]
index-url = ${NEXUS_URL}/repository/pypi-group/simple/
trusted-host = $(echo "${NEXUS_URL}" | sed 's|https\?://||' | cut -d: -f1)
EOF

  if $pip_cmd install \
    --index-url="${NEXUS_URL}/repository/pypi-group/simple/" \
    --trusted-host="$(echo "${NEXUS_URL}" | sed 's|https\?://||' | cut -d: -f1)" \
    --target="${pip_dir}/lib" \
    requests >/dev/null 2>&1; then
    report PASS "pypi" "pip install requests succeeded via pypi-group"
  else
    report FAIL "pypi" "pip install requests failed"
  fi
}

###############################################################################
# Test: Go modules
###############################################################################
test_go() {
  echo "=== Testing Go module mirror ==="
  if [ "${SKIP_GO:-0}" = "1" ]; then
    report SKIP "go" "Skipped by SKIP_GO=1"
    return
  fi

  if ! command -v go >/dev/null 2>&1; then
    report SKIP "go" "go not installed"
    return
  fi

  local go_dir="${WORK_DIR}/go-test"
  mkdir -p "$go_dir"

  cd "$go_dir"
  export GOPATH="${go_dir}/gopath"
  export GONOSUMCHECK="*"
  export GOFLAGS="-insecure"
  export GOPROXY="${NEXUS_URL}/repository/go-proxy/"
  export GONOSUMDB="*"

  if go mod init test.local/mirror-test >/dev/null 2>&1 && \
     go get golang.org/x/text >/dev/null 2>&1; then
    report PASS "go" "go get golang.org/x/text succeeded via go-proxy"
  else
    report FAIL "go" "go get failed (Go proxy via raw format may have limitations)"
  fi
}

###############################################################################
# Test: NuGet
###############################################################################
test_nuget() {
  echo "=== Testing NuGet mirror ==="
  if [ "${SKIP_NUGET:-0}" = "1" ]; then
    report SKIP "nuget" "Skipped by SKIP_NUGET=1"
    return
  fi

  if ! command -v dotnet >/dev/null 2>&1; then
    report SKIP "nuget" "dotnet not installed"
    return
  fi

  local nuget_dir="${WORK_DIR}/nuget-test"
  mkdir -p "$nuget_dir"

  cd "$nuget_dir"
  if dotnet new console -n NugetTest >/dev/null 2>&1; then
    cd NugetTest
    if dotnet add package Newtonsoft.Json \
      --source "${NEXUS_URL}/repository/nuget-group/index.json" >/dev/null 2>&1; then
      report PASS "nuget" "dotnet add package Newtonsoft.Json succeeded via nuget-group"
    else
      report FAIL "nuget" "dotnet add package failed"
    fi
  else
    report SKIP "nuget" "Could not create dotnet test project"
  fi
}

###############################################################################
# Test: Cargo (limited support)
###############################################################################
test_cargo() {
  echo "=== Testing Cargo mirror (limited) ==="
  echo "[NOTE] Nexus 3.x does not natively support Cargo registry protocol."
  echo "[NOTE] cargo-proxy is a raw proxy for crate file caching only."
  report SKIP "cargo" "Native Cargo registry protocol not supported by Nexus 3.x"
}

###############################################################################
# Main
###############################################################################
main() {
  echo "============================================"
  echo " Nexus Mirror Test Suite"
  echo " Target: ${NEXUS_URL}"
  echo " Date:   $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "============================================"
  echo ""

  test_nexus_health
  test_npm
  test_maven
  test_pypi
  test_go
  test_nuget
  test_cargo

  echo ""
  echo "============================================"
  echo " Results: ${PASS} passed, ${FAIL} failed, ${SKIP} skipped"
  echo "============================================"

  if [ "$FAIL" -gt 0 ]; then
    exit 1
  fi
}

main "$@"

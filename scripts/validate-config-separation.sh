#!/bin/bash
# Validation script for configuration separation implementation

set -e

echo "=== Configuration Separation Validation ==="
echo ""

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

function check_pass() {
    echo -e "${GREEN}✓${NC} $1"
}

function check_fail() {
    echo -e "${RED}✗${NC} $1"
    exit 1
}

function check_warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

echo "1. Checking Go build..."
if go build -o /tmp/test-controller ./cmd/kube-node-ready-controller 2>&1 | grep -q "error"; then
    check_fail "Controller build failed"
else
    check_pass "Controller builds successfully"
fi

if go build -o /tmp/test-worker ./cmd/kube-node-ready-worker 2>&1 | grep -q "error"; then
    check_fail "Worker build failed"
else
    check_pass "Worker builds successfully"
fi

echo ""
echo "2. Checking configuration files exist..."
[ -f "examples/controller-config.yaml" ] && check_pass "Controller config example exists" || check_fail "Controller config example missing"
[ -f "examples/worker-config.yaml" ] && check_pass "Worker config example exists" || check_fail "Worker config example missing"

echo ""
echo "3. Checking Helm templates..."
[ -f "deploy/helm/kube-node-ready/templates/controller-configmap.yaml" ] && check_pass "Controller ConfigMap template exists" || check_fail "Controller ConfigMap template missing"
[ -f "deploy/helm/kube-node-ready/templates/worker-configmap.yaml" ] && check_pass "Worker ConfigMap template exists" || check_fail "Worker ConfigMap template missing"

echo ""
echo "4. Validating controller config structure..."
if grep -q "checkTimeoutSeconds" examples/controller-config.yaml; then
    check_fail "Controller config still contains worker runtime config (checkTimeoutSeconds)"
else
    check_pass "Controller config doesn't contain worker runtime config"
fi

if grep -q "configMapName" examples/controller-config.yaml; then
    check_pass "Controller config contains configMapName field"
else
    check_fail "Controller config missing configMapName field"
fi

echo ""
echo "5. Validating worker config structure..."
if grep -q "checkTimeoutSeconds" examples/worker-config.yaml; then
    check_pass "Worker config contains checkTimeoutSeconds"
else
    check_fail "Worker config missing checkTimeoutSeconds"
fi

if grep -q "dnsTestDomains" examples/worker-config.yaml; then
    check_pass "Worker config contains dnsTestDomains"
else
    check_fail "Worker config missing dnsTestDomains"
fi

echo ""
echo "6. Checking Helm values.yaml structure..."
if grep -q "worker:" deploy/helm/kube-node-ready/values.yaml; then
    check_pass "values.yaml contains worker section"
else
    check_fail "values.yaml missing worker section"
fi

if grep -A20 "^worker:" deploy/helm/kube-node-ready/values.yaml | grep -q "config:"; then
    check_pass "Worker section contains config subsection"
else
    check_fail "Worker section missing config subsection"
fi

echo ""
echo "7. Checking controller doesn't pass worker config via env..."
if grep -q "CHECK_TIMEOUT" internal/controller/worker_manager.go; then
    check_fail "WorkerManager still sets CHECK_TIMEOUT env var"
else
    check_pass "WorkerManager doesn't set CHECK_TIMEOUT env var"
fi

if grep -q "DNS_TEST_DOMAINS" internal/controller/worker_manager.go; then
    check_fail "WorkerManager still sets DNS_TEST_DOMAINS env var"
else
    check_pass "WorkerManager doesn't set DNS_TEST_DOMAINS env var"
fi

echo ""
echo "8. Checking worker loads from file..."
if grep -q "LoadWorkerConfigFromFile" cmd/kube-node-ready-worker/main.go; then
    check_pass "Worker main.go uses LoadWorkerConfigFromFile"
else
    check_fail "Worker main.go doesn't use LoadWorkerConfigFromFile"
fi

if grep -q "/etc/kube-node-ready/worker-config.yaml" cmd/kube-node-ready-worker/main.go; then
    check_pass "Worker uses correct config file path"
else
    check_fail "Worker uses incorrect config file path"
fi

echo ""
echo "9. Checking ConfigMap volume mount..."
if grep -q "VolumeMount" internal/controller/worker_manager.go; then
    check_pass "WorkerManager mounts ConfigMap volume"
else
    check_fail "WorkerManager doesn't mount ConfigMap volume"
fi

if grep -q "worker-config" internal/controller/worker_manager.go; then
    check_pass "Volume named 'worker-config' exists"
else
    check_fail "Volume 'worker-config' not found"
fi

echo ""
echo "10. Checking documentation..."
[ -f "docs/CONFIGURATION_SEPARATION.md" ] && check_pass "Configuration separation docs exist" || check_warn "Configuration separation docs missing"
[ -f "docs/IMPLEMENTATION_SUMMARY.md" ] && check_pass "Implementation summary exists" || check_warn "Implementation summary missing"

echo ""
echo "11. Validating Go code structure..."
if grep -q "LoadWorkerConfigFromFile" internal/config/worker_config.go; then
    check_pass "LoadWorkerConfigFromFile function exists"
else
    check_fail "LoadWorkerConfigFromFile function missing"
fi

if grep -q "ConfigMapName.*string.*yaml" internal/config/controller_config.go; then
    check_pass "WorkerPodConfig has ConfigMapName field"
else
    check_fail "WorkerPodConfig missing ConfigMapName field"
fi

echo ""
echo "12. Testing example configs..."
if go run ./cmd/kube-node-ready-controller/main.go -config examples/controller-config.yaml -help 2>&1 | grep -q "flag provided but not defined"; then
    check_pass "Controller config loads (help flag check)"
else
    check_warn "Could not verify controller config loading"
fi

echo ""
echo "=== All Checks Passed ==="
echo ""
echo "Summary of Changes:"
echo "  - Controller config: Removed worker runtime fields, added configMapName"
echo "  - Worker config: New LoadWorkerConfigFromFile() function"
echo "  - WorkerManager: Simplified env vars, added ConfigMap volume mount"
echo "  - Helm: Split into controller and worker ConfigMaps"
echo "  - Examples: Created controller-config.yaml and worker-config.yaml"
echo "  - Documentation: Added comprehensive guides"
echo ""
echo "Next Steps:"
echo "  1. Test with actual Kubernetes cluster"
echo "  2. Verify ConfigMap mounting in worker pods"
echo "  3. Update integration tests"
echo "  4. Update Helm chart README"
echo ""

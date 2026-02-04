#!/bin/bash
# Example script to run the kube-node-ready controller locally
# This script sets up the environment and starts the controller

set -e

echo "Starting kube-node-ready controller..."
echo "======================================="

# Configuration file path
CONFIG_FILE="${CONFIG_FILE:-./examples/controller-config.yaml}"

# Controller flags
LEADER_ELECT="${LEADER_ELECT:-false}"
METRICS_PORT="${METRICS_PORT:-8080}"
HEALTH_PORT="${HEALTH_PORT:-8081}"

# Print configuration
echo ""
echo "Configuration:"
echo "  CONFIG_FILE: ${CONFIG_FILE}"
echo "  LEADER_ELECT: ${LEADER_ELECT}"
echo "  METRICS_PORT: ${METRICS_PORT}"
echo "  HEALTH_PORT: ${HEALTH_PORT}"
echo ""

# Check if config file exists
if [ ! -f "${CONFIG_FILE}" ]; then
    echo "Error: Configuration file not found at ${CONFIG_FILE}"
    echo "Please create a config file or set CONFIG_FILE environment variable"
    echo "Example: cp examples/controller-config.yaml /tmp/controller-config.yaml"
    exit 1
fi

# Check if binary exists
if [ ! -f "./bin/kube-node-ready-controller" ]; then
    echo "Error: Controller binary not found at ./bin/kube-node-ready-controller"
    echo "Please run 'make build-controller' first"
    exit 1
fi

# Check if binary is executable
if [ ! -x "./bin/kube-node-ready-controller" ]; then
    echo "Error: Controller binary is not executable"
    echo "Making binary executable..."
    chmod +x ./bin/kube-node-ready-controller
fi

# Check if kubectl is configured
if ! kubectl cluster-info &> /dev/null; then
    echo "Error: kubectl is not configured or cannot connect to cluster"
    echo "Please configure kubectl first"
    exit 1
fi

echo "Kubernetes cluster info:"
kubectl cluster-info
echo ""

# Check permissions (optional)
echo "Checking RBAC permissions..."
RBAC_ISSUES=0

if kubectl auth can-i list nodes &> /dev/null; then
    echo "✓ Can list nodes"
else
    echo "✗ Cannot list nodes - RBAC may not be configured"
    RBAC_ISSUES=$((RBAC_ISSUES + 1))
fi

if kubectl auth can-i watch nodes &> /dev/null; then
    echo "✓ Can watch nodes"
else
    echo "✗ Cannot watch nodes - RBAC may not be configured"
    RBAC_ISSUES=$((RBAC_ISSUES + 1))
fi

if kubectl auth can-i patch nodes &> /dev/null; then
    echo "✓ Can patch nodes"
else
    echo "✗ Cannot patch nodes - RBAC may not be configured"
    RBAC_ISSUES=$((RBAC_ISSUES + 1))
fi

if kubectl auth can-i create pods &> /dev/null; then
    echo "✓ Can create pods"
else
    echo "✗ Cannot create pods - RBAC may not be configured"
    RBAC_ISSUES=$((RBAC_ISSUES + 1))
fi

if kubectl auth can-i delete pods &> /dev/null; then
    echo "✓ Can delete pods"
else
    echo "✗ Cannot delete pods - RBAC may not be configured"
    RBAC_ISSUES=$((RBAC_ISSUES + 1))
fi

if kubectl auth can-i list pods &> /dev/null; then
    echo "✓ Can list pods"
else
    echo "✗ Cannot list pods - RBAC may not be configured"
    RBAC_ISSUES=$((RBAC_ISSUES + 1))
fi

if [ $RBAC_ISSUES -gt 0 ]; then
    echo ""
    echo "⚠️  Warning: Found $RBAC_ISSUES RBAC permission issues"
    echo "The controller may not work correctly without proper permissions"
    echo "Press Ctrl+C to abort or wait 5 seconds to continue..."
    sleep 5
fi

echo ""
echo "Starting controller..."
echo "Config file: ${CONFIG_FILE}"
echo "Health endpoint: http://localhost:${HEALTH_PORT}/healthz"
echo "Metrics endpoint: http://localhost:${METRICS_PORT}/metrics"
echo ""
echo "Press Ctrl+C to stop"
echo "======================================="
echo ""

# Build command line arguments
CONTROLLER_ARGS=(
    "--config=${CONFIG_FILE}"
    "--leader-elect=${LEADER_ELECT}"
    "--metrics-bind-address=:${METRICS_PORT}"
    "--health-probe-bind-address=:${HEALTH_PORT}"
    "--zap-devel=true"
    "--zap-log-level=info"
)

# Run the controller
exec ./bin/kube-node-ready-controller "${CONTROLLER_ARGS[@]}"

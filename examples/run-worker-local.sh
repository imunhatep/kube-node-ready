#!/bin/bash
# Example script to run the kube-node-ready worker locally
# This simulates how the worker will run when created by the controller

set -e

echo "Starting kube-node-ready worker..."
echo "===================================="

# Check if binary exists
if [ ! -f "./bin/kube-node-ready-worker" ]; then
    echo "Error: Worker binary not found at ./bin/kube-node-ready-worker"
    echo "Please run 'make build-worker' first"
    exit 1
fi

# Check if binary is executable
if [ ! -x "./bin/kube-node-ready-worker" ]; then
    echo "Error: Worker binary is not executable"
    echo "Making binary executable..."
    chmod +x ./bin/kube-node-ready-worker
fi

# Get node name (use first node if not specified)
if [ -z "$NODE_NAME" ]; then
    NODE_NAME=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    if [ -z "$NODE_NAME" ]; then
        echo "Error: Could not determine node name"
        echo "Please set NODE_NAME environment variable or ensure kubectl is configured"
        exit 1
    fi
    echo "Using node: $NODE_NAME"
fi

# Set required environment variables
export NODE_NAME="${NODE_NAME}"
export POD_NAMESPACE="${POD_NAMESPACE:-kube-system}"
export CHECK_TIMEOUT="${CHECK_TIMEOUT:-10s}"
export DNS_TEST_DOMAINS="${DNS_TEST_DOMAINS:-kubernetes.default.svc.cluster.local,google.com}"
export KUBERNETES_SERVICE_HOST="${KUBERNETES_SERVICE_HOST:-kubernetes.default.svc.cluster.local}"
export KUBERNETES_SERVICE_PORT="${KUBERNETES_SERVICE_PORT:-443}"
export LOG_LEVEL="${LOG_LEVEL:-info}"
export LOG_FORMAT="${LOG_FORMAT:-console}"

# Print configuration
echo ""
echo "Configuration:"
echo "  NODE_NAME: ${NODE_NAME}"
echo "  POD_NAMESPACE: ${POD_NAMESPACE}"
echo "  CHECK_TIMEOUT: ${CHECK_TIMEOUT}"
echo "  DNS_TEST_DOMAINS: ${DNS_TEST_DOMAINS}"
echo "  KUBERNETES_SERVICE_HOST: ${KUBERNETES_SERVICE_HOST}"
echo "  KUBERNETES_SERVICE_PORT: ${KUBERNETES_SERVICE_PORT}"
echo "  LOG_LEVEL: ${LOG_LEVEL}"
echo ""

# Check if kubectl is configured
if ! kubectl cluster-info &> /dev/null; then
    echo "Warning: kubectl is not configured or cannot connect to cluster"
    echo "Worker will attempt to run but may fail"
    echo ""
fi

echo "Running worker..."
echo "===================================="
echo ""

# Run the worker
./bin/kube-node-ready-worker

# Capture exit code
EXIT_CODE=$?

echo ""
echo "===================================="
echo "Worker completed with exit code: $EXIT_CODE"

# Interpret exit code
case $EXIT_CODE in
    0)
        echo "✅ SUCCESS: All verification checks passed"
        ;;
    1)
        echo "❌ FAILED: One or more verification checks failed"
        ;;
    2)
        echo "⚙️  CONFIG ERROR: Configuration validation failed"
        ;;
    3)
        echo "🔌 CLIENT ERROR: Failed to create Kubernetes client"
        ;;
    10)
        echo "⚠️  UNEXPECTED ERROR: An unexpected error occurred"
        ;;
    *)
        echo "❓ UNKNOWN: Unknown exit code"
        ;;
esac

exit $EXIT_CODE

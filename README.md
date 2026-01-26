# kube-node-ready

A lightweight Kubernetes DaemonSet that verifies node networking before allowing workloads to be scheduled. Designed to work seamlessly with Karpenter and other node autoscalers.

## Overview

When new nodes join a Kubernetes cluster, they may have networking issues such as:
- DNS resolution failures
- Inability to connect to the Kubernetes API
- No connectivity within the cluster
- Broken service discovery

**kube-node-ready** solves this by:
1. Running verification checks on new nodes before they accept workloads
2. Removing the taint only when all checks pass
3. Adding a label to mark the node as verified
4. Automatically terminating after successful verification (zero ongoing cost)

## Features

- ✅ **One-time verification** - Runs once per node, then terminates
- ✅ **Zero ongoing resource consumption** - Pods removed after verification
- ✅ **Comprehensive checks** - DNS, Kubernetes API, network connectivity, service discovery
- ✅ **Retry logic** - Exponential backoff handles transient failures
- ✅ **Label-based lifecycle** - Uses nodeAffinity to auto-remove pods
- ✅ **Karpenter-ready** - Works seamlessly with node autoscaling
- ✅ **Multi-architecture** - Supports both amd64 and arm64 (AWS Graviton, GCP Tau, Azure Ampere)
- ✅ **Production-ready** - Security hardened, minimal permissions

## Quick Start

### Prerequisites

- Kubernetes 1.24+
- Helm 3.0+
- Nodes with initial taint (optional but recommended)

### Installation

```bash
# Install with Helm
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace

# Verify installation
kubectl get daemonset -n kube-system kube-node-ready
kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-node-ready
```

### Configuration

Key configuration options in `values.yaml`:

```yaml
config:
  # Maximum time for initial verification
  initialTimeout: "300s"
  
  # Timeout for individual checks (DNS, network, Kubernetes API)
  checkTimeout: "10s"
  
  # Number of retry attempts
  maxRetries: 5
  
  # DNS domains to test
  dnsTestDomains:
    - kubernetes.default.svc.cluster.local
    - google.com
  
  # Taint to remove on success
  taintKey: "node-ready/unverified"
  
  # Label to add on success
  verifiedLabel: "node-ready/verified"
```

## How It Works

### 1. Node Created with Taint
```bash
# New node has taint preventing workload scheduling
kubectl get node <node-name> -o yaml
# spec:
#   taints:
#   - key: node-ready/unverified
#     value: "true"
#     effect: NoSchedule
```

### 2. Verification Pod Scheduled
DaemonSet schedules pod on the new node (tolerates the taint).

### 3. Checks Execute
- DNS resolution (internal + external)
- Kubernetes API connectivity
- Network connectivity
- Service discovery

### 4. Success: Node Marked Ready
```bash
# Taint removed, label added
kubectl get node <node-name> -o yaml
# metadata:
#   labels:
#     node-ready/verified: "true"
# spec:
#   taints: []  # Taint removed
```

### 5. Pod Terminates
NodeAffinity excludes nodes with `verified` label, pod automatically terminates.

## Architecture

```
┌─────────────────────────────────────────┐
│ New Node (tainted)                      │
│   ↓                                     │
│ kube-node-ready Pod Starts              │
│   ↓                                     │
│ Run Verification Checks                 │
│   ├─ DNS Check                          │
│   ├─ Kubernetes API Check               │
│   ├─ Network Check                      │
│   └─ Service Discovery Check            │
│   ↓                                     │
│ All Pass? → Remove Taint + Add Label   │
│   ↓                                     │
│ Pod Terminates (nodeAffinity)           │
│   ↓                                     │
│ Node Ready for Workloads ✅             │
└─────────────────────────────────────────┘
```

See [ARCHITECTURE.md](docs/ARCHITECTURE.md) for detailed architecture documentation.

## Usage with Karpenter

Karpenter can be configured to add the initial taint to new nodes:

```yaml
apiVersion: karpenter.sh/v1beta1
kind: NodePool
metadata:
  name: default
spec:
  template:
    spec:
      taints:
        - key: node-ready/unverified
          value: "true"
          effect: NoSchedule
      # ... other configuration
```

## Verification Checks

### 1. DNS Resolution
- Tests: `kubernetes.default.svc.cluster.local`, external domains
- Timeout: Configurable (default: 10 seconds)
- Purpose: Verify DNS resolution works

### 2. Kubernetes API Check
- Tests: Connection to API server, authentication
- Timeout: Configurable (default: 10 seconds)
- Purpose: Verify node can communicate with control plane

### 3. Network Connectivity
- Tests: TCP connection to Kubernetes service
- Timeout: Configurable (default: 10 seconds)
- Purpose: Verify network routing works

### 4. Service Discovery
- Tests: Query Kubernetes services and endpoints
- Timeout: Configurable (default: 10 seconds)
- Purpose: Verify service mesh works

## Monitoring

### Metrics (Prometheus format)
Available at `:8080/metrics` during pod execution:

```
node_check_status{node="node-1"} 1
node_check_dns_duration_seconds{node="node-1"} 0.123
node_check_api_duration_seconds{node="node-1"} 0.456
node_check_completion_timestamp{node="node-1"} 1234567890
node_check_failures_total{node="node-1",check="dns"} 0
```

### Logs
Structured JSON logs with relevant context:

```json
{"level":"info","timestamp":"2024-01-23T10:00:00Z","msg":"Starting DNS check","domain":"kubernetes.default.svc.cluster.local"}
{"level":"info","timestamp":"2024-01-23T10:00:01Z","msg":"DNS check passed","domain":"kubernetes.default.svc.cluster.local","addresses":["10.96.0.1"],"duration":"0.123s"}
```

## Troubleshooting

### Pod Doesn't Schedule on New Node
```bash
# Check if node has the verified label already
kubectl get nodes -L node-ready/verified

# Check DaemonSet nodeAffinity
kubectl get ds -n kube-system kube-node-ready -o yaml | grep -A 10 affinity
```

### Verification Fails
```bash
# Check pod logs
kubectl logs -n kube-system -l app.kubernetes.io/name=kube-node-ready

# Check node taints
kubectl describe node <node-name> | grep Taints

# Manually test DNS from pod
kubectl exec -n kube-system <pod-name> -- nslookup kubernetes.default.svc.cluster.local
```

### Pod Doesn't Terminate After Success
```bash
# Verify label was added
kubectl get node <node-name> --show-labels | grep verified

# Force label addition (testing only)
kubectl label node <node-name> node-ready/verified=true

# Pod should terminate automatically
```

### Re-run Verification
```bash
# Remove the verified label
kubectl label node <node-name> node-ready/verified-

# New pod will be scheduled automatically
kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-node-ready -w
```

## Development

### Local Testing with Dry-Run Mode

Test the application locally without modifying cluster nodes:

```bash
# Run with default kubeconfig
go run ./cmd/kube-node-ready --dry-run --log-format=console

# Or with custom kubeconfig
go run ./cmd/kube-node-ready --dry-run --kubeconfig=/path/to/config

# With debug logging
go run ./cmd/kube-node-ready --dry-run --log-level=debug --log-format=console
```

**Dry-run mode:**
- ✅ Performs all network verification checks (DNS, network connectivity)
- ✅ Works with local kubeconfig (if available)
- ✅ Gracefully handles missing kubeconfig (skips K8s API checks only)
- ❌ Does NOT modify node taints or labels
- Perfect for development and testing


### Build

```bash
# Build using Make (automatically includes version from git)
make build

# Or build manually with version information
go build -ldflags "-X main.version=1.0.0 -X main.commitHash=$(git rev-parse --short HEAD) -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o bin/kube-node-ready ./cmd/kube-node-ready

# Build Docker image with version information
make docker-build VERSION=1.0.0

# Or manually
docker build \
  --build-arg VERSION=1.0.0 \
  --build-arg COMMIT_HASH=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t kube-node-ready:1.0.0 .

# Test locally with dry-run (uses local kubeconfig if available)
make dry-run
# Or: ./bin/kube-node-ready --dry-run --log-format=console

# Check version
./bin/kube-node-ready --version

# Test in production mode (requires in-cluster config or kubeconfig)
export NODE_NAME=test-node
export KUBERNETES_SERVICE_HOST=localhost
export KUBERNETES_SERVICE_PORT=6443
./bin/kube-node-ready
```

### Testing

```bash
# Run unit tests
go test ./...

# Test Helm chart
helm lint ./deploy/helm/kube-node-ready

# Dry-run install
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --dry-run --debug
```

## Configuration Reference

See [values.yaml](deploy/helm/kube-node-ready/values.yaml) for all available configuration options.

### Common Configurations

#### Custom Taint
```yaml
config:
  taintKey: "my-custom/unverified"
  taintValue: "true"
  taintEffect: "NoSchedule"
```

#### Different DNS Tests
```yaml
config:
  dnsTestDomains:
    - kubernetes.default.svc.cluster.local
    - cloudflare.com
    - 8.8.8.8
```

#### More Retries
```yaml
config:
  maxRetries: 10
  retryBackoff: "exponential"
  initialTimeout: "600s"
```

## Security

- Runs as non-root user (UID 1000)
- Read-only filesystem
- No privilege escalation
- Minimal RBAC permissions (nodes, services, endpoints)
- Drops all capabilities

## Resource Usage

- **During verification**: ~64Mi memory, ~50m CPU
- **After verification**: 0 (pod terminated)
- **Per node**: One-time cost only

### Cluster Impact
- 10 nodes: ~640Mi during verification, then 0
- 1000 nodes: ~64Gi during bulk scaling, then 0
- Zero ongoing cost after verification complete

## License
AGPL 3.0 License - see LICENSE file for details

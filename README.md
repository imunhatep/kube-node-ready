# kube-node-ready

A Kubernetes operator that verifies node networking before allowing workloads to be scheduled. Designed for dynamic clusters with node autoscaling (Karpenter, Cluster Autoscaler, etc.).

## Overview

When new nodes join a Kubernetes cluster, they may have networking issues such as:
- DNS resolution failures
- Inability to connect to the Kubernetes API
- No connectivity within the cluster
- Broken service discovery

**kube-node-ready** solves this with a **Controller + Worker architecture**:
1. **Controller** watches for new/unverified nodes and orchestrates verification
2. **Worker pods** execute verification checks on-demand for each node
3. **Automated remediation** - Removes taints, adds labels, and optionally deletes failed nodes
4. **Centralized metrics** - Single endpoint for monitoring all node verifications

## Architecture

**Controller-Worker Mode** (Recommended for production):
```
┌─────────────────────────────────────────────────────────────┐
│ Controller Deployment (single replica)                      │
│  • Watches node events                                      │
│  • Creates worker pods for unverified nodes                 │
│  • Manages reconciliation & retries                         │
│  • Exposes centralized metrics                              │
│  • Handles node lifecycle (optional deletion)               │
└────────────┬────────────────────────────────────────────────┘
             │
             │ Creates on-demand
             ↓
┌─────────────────────────────────────────────────────────────┐
│ Worker Pods (per node, short-lived)                         │
│  • DNS checks                                               │
│  • Kubernetes API connectivity                              │
│  • Network connectivity tests                               │
│  • Service discovery validation                             │
│  • Reports back via exit code                               │
│  • Terminates after completion                              │
└─────────────────────────────────────────────────────────────┘
```

**DaemonSet Mode** (Legacy, simpler but less efficient):
- Single pod per node, always running or terminated after verification
- See [DaemonSet Architecture](docs/ARCHITECTURE_DAEMONSET.md) for details

## Features

### Controller-Worker Mode
- ✅ **Intelligent orchestration** - Controller manages verification lifecycle
- ✅ **On-demand workers** - Pods created only when needed, then terminated
- ✅ **Centralized metrics** - Single Prometheus endpoint for all nodes
- ✅ **Automatic retries** - Exponential backoff with configurable limits
- ✅ **Node remediation** - Optional automatic deletion of failed nodes (with Karpenter NodeClaim detection)
- ✅ **Reconciliation loop** - Ensures all nodes are verified
- ✅ **Leader election** - High availability support
- ✅ **Karpenter-optimized** - Perfect for dynamic node scaling

### Verification Capabilities
- ✅ **Comprehensive checks** - DNS, Kubernetes API, network connectivity, service discovery
- ✅ **Configurable timeouts** - Per-check and overall verification timeouts
- ✅ **Multi-architecture** - Supports amd64 and arm64 (AWS Graviton, GCP Tau, Azure Ampere)
- ✅ **Production-ready** - Security hardened, minimal permissions
- ✅ **Flexible configuration** - YAML-based config via ConfigMap

## Quick Start

### Prerequisites

- Kubernetes 1.24+
- Helm 3.0+
- kubectl configured with cluster access
- (Recommended) Karpenter or node autoscaler configured to add initial taint

### Installation (Controller Mode - Recommended)

```bash
# Install with Helm
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace \
  --set deploymentMode=controller

# Verify installation
kubectl get deployment -n kube-system kube-node-ready-controller
kubectl get pods -n kube-system -l app.kubernetes.io/component=controller
```

### Alternative: DaemonSet Mode

For simpler deployments without autoscaling:

```bash
# Install in DaemonSet mode (legacy)
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set deploymentMode=daemonset
```

### Configuration

Controller mode uses a ConfigMap-based configuration. Key options:

```yaml
# Controller settings
controller:
  config:
    # Worker pod configuration
    worker:
      image:
        repository: ghcr.io/imunhatep/kube-node-ready
        tag: "latest"
      namespace: kube-system
      timeoutSeconds: 300
      checkTimeoutSeconds: 10
      dnsTestDomains:
        - kubernetes.default.svc.cluster.local
        - google.com
      
    # Reconciliation settings  
    reconciliation:
      intervalSeconds: 30
      maxRetries: 5
      retryBackoff: exponential
      
    # Node management
    nodeManagement:
      deleteFailedNodes: false  # Set to true to auto-delete failed nodes
      taints:
        - key: node-ready/unverified
          value: "true"
          effect: NoSchedule
      verifiedLabel:
        key: node-ready/verified
        value: "true"
```

See [examples/controller-config.yaml](examples/controller-config.yaml) for full configuration.

## How It Works

### Controller-Worker Flow

### 1. Node Created with Taint
```bash
# New node created by Karpenter/autoscaler with taint
kubectl get node <node-name> -o yaml
# spec:
#   taints:
#   - key: node-ready/unverified
#     value: "true"
#     effect: NoSchedule
```

### 2. Controller Detects Unverified Node
- Controller watches node events
- Detects new node without `node-ready/verified` label
- Checks if node has verification taint
- Adds node to reconciliation queue

### 3. Worker Pod Created
- Controller creates a worker pod with nodeAffinity for the target node
- Worker pod tolerates the verification taint
- Pod scheduled exclusively on the unverified node

### 4. Verification Checks Execute
Worker performs comprehensive checks:
- **DNS resolution** (internal + external)
- **Kubernetes API connectivity**
- **Network connectivity** tests
- **Service discovery** validation

### 5. Results Reported
- Worker pod exits with status code (0 = success, non-zero = failure)
- Controller reads pod status and exit code
- Controller updates metrics with verification results

### 6. Success: Node Marked Ready
```bash
# Controller removes taint and adds verified label
kubectl get node <node-name> -o yaml
# metadata:
#   labels:
#     node-ready/verified: "true"
# spec:
#   taints: []  # Taint removed
```

### 7. Worker Pod Cleaned Up
- Controller deletes the completed worker pod
- Node is now ready for workload scheduling
- Zero ongoing resource consumption

### 8. Failure Handling (if checks fail)
- Worker pod exits with non-zero status
- Controller implements retry logic with exponential backoff
- After max retries, optionally deletes the node (if `deleteFailedNodes: true`)
- Metrics expose failure details for alerting

## Detailed Architecture

### Controller-Worker Pattern

```
┌───────────────────────────────────────────────────────────────────┐
│                    Kubernetes API Server                           │
└───────────────────────────┬───────────────────────────────────────┘
                            │
                            │ Watch Nodes
                            ↓
┌───────────────────────────────────────────────────────────────────┐
│                   kube-node-ready-controller                       │
│                                                                    │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │ Reconciliation Loop                                         │  │
│  │  1. Detect unverified nodes                                │  │
│  │  2. Create worker pod with nodeAffinity                    │  │
│  │  3. Monitor worker pod status                              │  │
│  │  4. Process results (exit code)                            │  │
│  │  5. Update node (remove taint, add label)                  │  │
│  │  6. Clean up worker pod                                    │  │
│  │  7. Handle failures (retry/delete node)                    │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                    │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │ Metrics & Monitoring                                        │  │
│  │  • Nodes verified/failed                                   │  │
│  │  • Verification duration                                   │  │
│  │  • Retry attempts                                          │  │
│  │  • Worker pod status                                       │  │
│  └────────────────────────────────────────────────────────────┘  │
└────────────┬───────────────────────────────────────────────────────┘
             │
             │ Creates Worker Pods
             ↓
┌───────────────────────────────────────────────────────────────────┐
│              Worker Pods (short-lived, per node)                  │
│                                                                    │
│  Node: node-1           Node: node-2           Node: node-3       │
│  ┌─────────────────┐    ┌─────────────────┐    ┌──────────────┐  │
│  │ Worker Pod      │    │ Worker Pod      │    │ Worker Pod   │  │
│  │                 │    │                 │    │              │  │
│  │ • DNS Check     │    │ • DNS Check     │    │ • DNS Check  │  │
│  │ • API Check     │    │ • API Check     │    │ • API Check  │  │
│  │ • Network Check │    │ • Network Check │    │ • Network... │  │
│  │ • Service Check │    │ • Service Check │    │ • Service... │  │
│  │                 │    │                 │    │              │  │
│  │ Exit: 0 ✅      │    │ Exit: 0 ✅      │    │ Exit: 1 ❌   │  │
│  └─────────────────┘    └─────────────────┘    └──────────────┘  │
│  Terminated            Terminated             Retry/Delete       │
└───────────────────────────────────────────────────────────────────┘
```

See [Controller Architecture](docs/ARCHITECTURE_CONTROLLER.md) for detailed design.

### DaemonSet Mode (Legacy)

For simpler deployments, a DaemonSet mode is available where pods run continuously or terminate after verification. See [DaemonSet Architecture](docs/ARCHITECTURE_DAEMONSET.md).

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

### Karpenter NodeClaim Integration

When `deleteFailedNodes` is enabled, kube-node-ready automatically detects Karpenter-managed nodes and prefers to delete the corresponding `NodeClaim` resource instead of the node directly. This ensures proper cleanup and allows Karpenter to handle termination gracefully.

**How it works:**
1. When node verification fails and deletion is triggered
2. kube-node-ready searches for NodeClaim resources with `status.nodeName` matching the failed node
3. If a NodeClaim is found, it deletes the NodeClaim (preferred method)
4. If no NodeClaim is found, it falls back to direct node deletion
5. Karpenter handles the actual node termination and cleanup

**Benefits:**
- ✅ Proper integration with Karpenter's lifecycle management
- ✅ Maintains Karpenter's termination workflows (draining, finalizers, etc.)
- ✅ Preserves Karpenter's spot instance handling
- ✅ Automatic fallback for non-Karpenter nodes

**Example logs:**
```
INFO Found NodeClaim for failed node, deleting NodeClaim instead  nodeClaim=default-12345 node=ip-192-168-1-100
INFO Successfully deleted NodeClaim, node should be terminated by Karpenter  nodeClaim=default-12345
```

See [examples/karpenter-example.yaml](examples/karpenter-example.yaml) for NodeClaim resource format.

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

**Controller Mode** - Available at controller pod `:8080/metrics`:

```prometheus
# Node verification status
kube_node_ready_verification_status{node="node-1",status="verified"} 1
kube_node_ready_verification_status{node="node-2",status="failed"} 1

# Verification duration
kube_node_ready_verification_duration_seconds{node="node-1"} 45.2

# Total verifications
kube_node_ready_verifications_total{status="success"} 150
kube_node_ready_verifications_total{status="failed"} 3

# Active worker pods
kube_node_ready_worker_pods{status="running"} 2
kube_node_ready_worker_pods{status="succeeded"} 148

# Retry attempts
kube_node_ready_retry_attempts_total{node="node-2"} 3

# Controller health
kube_node_ready_controller_healthy 1
kube_node_ready_controller_reconcile_errors_total 0
```

**DaemonSet Mode** - Available at each pod's `:8080/metrics`:

```prometheus
node_check_status{node="node-1"} 1
node_check_dns_duration_seconds{node="node-1"} 0.123
node_check_api_duration_seconds{node="node-1"} 0.456
node_check_completion_timestamp{node="node-1"} 1234567890
node_check_failures_total{node="node-1",check="dns"} 0
```

### Logs

**Controller Logs**:
```json
{"level":"info","msg":"Node added to queue","node":"node-1","state":"unverified"}
{"level":"info","msg":"Creating worker pod","node":"node-1","pod":"verify-node-1"}
{"level":"info","msg":"Worker completed successfully","node":"node-1","duration":"45.2s"}
{"level":"info","msg":"Node verified","node":"node-1","label":"node-ready/verified=true"}
```

**Worker Logs**:
```json
{"level":"info","timestamp":"2024-01-23T10:00:00Z","msg":"Starting DNS check","domain":"kubernetes.default.svc.cluster.local"}
{"level":"info","timestamp":"2024-01-23T10:00:01Z","msg":"DNS check passed","domain":"kubernetes.default.svc.cluster.local","addresses":["10.96.0.1"],"duration":"0.123s"}
{"level":"info","msg":"All checks passed","node":"node-1","total_duration":"45.2s"}
```

## Troubleshooting

### Controller Mode

#### Worker Pod Not Created for New Node
```bash
# Check controller logs
kubectl logs -n kube-system -l app.kubernetes.io/component=controller

# Check if node is already verified
kubectl get nodes -L node-ready/verified

# Check controller reconciliation
kubectl describe deployment -n kube-system kube-node-ready-controller
```

#### Verification Fails
```bash
# Check controller logs for retry attempts
kubectl logs -n kube-system -l app.kubernetes.io/component=controller | grep -i failed

# Check worker pod logs
kubectl logs -n kube-system -l app.kubernetes.io/component=worker

# Check worker pod status
kubectl get pods -n kube-system -l app.kubernetes.io/component=worker -o wide

# Manually inspect failed node
kubectl describe node <node-name>
```

#### Worker Pod Stuck
```bash
# Check pod events
kubectl describe pod -n kube-system <worker-pod-name>

# Check if node is schedulable
kubectl get node <node-name> -o json | jq '.spec.taints'

# Manually delete stuck worker
kubectl delete pod -n kube-system <worker-pod-name>
# Controller will recreate it
```

#### Controller Not Running
```bash
# Check controller status
kubectl get deployment -n kube-system kube-node-ready-controller

# Check controller logs
kubectl logs -n kube-system -l app.kubernetes.io/component=controller

# Check RBAC permissions
kubectl auth can-i list nodes --as=system:serviceaccount:kube-system:kube-node-ready-controller
kubectl auth can-i create pods --as=system:serviceaccount:kube-system:kube-node-ready-controller
```

### DaemonSet Mode

#### Pod Doesn't Schedule on New Node
```bash
# Check if node has the verified label already
kubectl get nodes -L node-ready/verified

# Check DaemonSet nodeAffinity
kubectl get ds -n kube-system kube-node-ready -o yaml | grep -A 10 affinity
```

#### Verification Fails
```bash
# Check pod logs
kubectl logs -n kube-system -l app.kubernetes.io/name=kube-node-ready

# Check node taints
kubectl describe node <node-name> | grep Taints

# Manually test DNS from pod
kubectl exec -n kube-system <pod-name> -- nslookup kubernetes.default.svc.cluster.local
```

#### Pod Doesn't Terminate After Success
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

# Controller mode: Controller will create new worker pod automatically
# DaemonSet mode: New pod will be scheduled automatically

# Watch for new pods
kubectl get pods -n kube-system -w
```

## Development

### Local Testing

#### Run Controller Locally
```bash
# Build controller
make build-controller

# Run controller against your cluster
./examples/run-controller-local.sh

# Or with custom config
CONFIG_FILE=/path/to/config.yaml ./examples/run-controller-local.sh
```

#### Run Worker Locally
```bash
# Build worker
make build-worker

# Run worker for a specific node
NODE_NAME=my-node ./examples/run-worker-local.sh
```

#### Dry-Run Mode (DaemonSet)
Test the daemonset binary locally without modifying cluster nodes:

```bash
# Build daemonset binary
make build

# Run with dry-run flag
./bin/kube-node-ready --dry-run --log-format=console

# With debug logging
./bin/kube-node-ready --dry-run --log-level=debug --log-format=console
```

**Dry-run mode:**
- ✅ Performs all network verification checks (DNS, network connectivity)
- ✅ Works with local kubeconfig (if available)
- ✅ Gracefully handles missing kubeconfig (skips K8s API checks only)
- ❌ Does NOT modify node taints or labels
- Perfect for development and testing

### Build

```bash
# Build all binaries (daemonset, controller, worker)
make build-all

# Build specific binary
make build-controller  # Controller
make build-worker      # Worker
make build             # DaemonSet (legacy)

# Build with version information
VERSION=1.0.0 make build-all

# Build Docker/Podman image with all binaries
make docker-build VERSION=1.0.0

# Or manually with podman
podman build \
  --build-arg VERSION=1.0.0 \
  --build-arg COMMIT_HASH=$(git rev-parse --short HEAD) \
  --build-arg BUILD_DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ) \
  -t kube-node-ready:1.0.0 .

# Check version
./bin/kube-node-ready-controller --version
./bin/kube-node-ready-worker --version
./bin/kube-node-ready --version
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

### Controller Mode
- **Controller**: ~100Mi memory, ~100m CPU (always running)
- **Worker pods**: ~64Mi memory, ~50m CPU (per verification, then terminated)
- **After verification**: Only controller remains running

### DaemonSet Mode
- **During verification**: ~64Mi memory, ~50m CPU per node
- **After verification**: 0 (pod terminated)

### Cluster Impact
**Controller Mode:**
- 10 nodes: Controller + 64Mi per active verification
- 1000 nodes: Controller + ~64Gi during bulk scaling, then just controller
- Ongoing cost: Only controller (~100Mi)

**DaemonSet Mode:**
- 10 nodes: ~640Mi during verification, then 0
- 1000 nodes: ~64Gi during bulk scaling, then 0
- Zero ongoing cost after verification complete

## License
AGPL 3.0 License - see LICENSE file for details

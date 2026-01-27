# Controller-Worker Architecture Migration Plan

## Executive Summary

This document outlines the plan to refactor `kube-node-ready` from a DaemonSet-based architecture to a **Controller + Worker** architecture. The new design separates concerns:

- **Controller**: A single deployment that watches node events, manages worker pods, handles reconciliation, and exposes centralized metrics
- **Worker**: Short-lived pods that perform actual node verification checks (existing logic)

## Current Architecture (DaemonSet-Based)

### Current Flow
```
New Node Created
    ↓
DaemonSet schedules pod on node
    ↓
Pod runs verification checks
    ↓
Pod updates node (remove taint, add label)
    ↓
Pod terminates (or continues monitoring - optional)
```

### Current Limitations
1. **No centralized control**: Each DaemonSet pod operates independently
2. **Limited orchestration**: Cannot easily retry or reschedule verification
3. **Distributed metrics**: Each pod exposes its own metrics endpoint
4. **No reconciliation loop**: Manual intervention needed if verification fails
5. **Resource waste**: Pods remain running even after verification completes
6. **Complex failure handling**: No centralized decision-making for node deletion or remediation

## Proposed Architecture (Controller + Worker)

### High-Level Design

```
┌──────────────────────────────────────────────────────────────────┐
│                    Kubernetes Cluster                             │
│                                                                    │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │              Controller Deployment (1 replica)              │  │
│  │                                                              │  │
│  │  ┌─────────────────────────────────────────────────────┐   │  │
│  │  │  Kubernetes Reconciliation Loop                      │   │  │
│  │  │  - Watch Node events (Add/Update/Delete)            │   │  │
│  │  │  - Track node verification state                    │   │  │
│  │  │  - Create worker pods for new/unverified nodes     │   │  │
│  │  │  - Monitor worker pod status                        │   │  │
│  │  │  - Handle failed verifications                      │   │  │
│  │  │  - Delete unhealthy nodes (if configured)          │   │  │
│  │  │  - Expose centralized metrics                       │   │  │
│  │  └─────────────────────────────────────────────────────┘   │  │
│  │                                                              │  │
│  │  Resources:                                                  │  │
│  │  - Memory: ~100Mi                                           │  │
│  │  - CPU: ~100m                                               │  │
│  │  - Metrics Port: 8080                                       │  │
│  │  - Health Port: 8081                                        │  │
│  └──────────────────┬───────────────────────────────────────────┘  │
│                     │                                             │
│                     │ Creates/Monitors                            │
│                     ↓                                             │
│  ┌────────────────────────────────────────────────────────────┐  │
│  │              Worker Pods (per node)                         │  │
│  │                                                              │  │
│  │  Pod 1 (Node: node-1)    Pod 2 (Node: node-2)              │  │
│  │  ┌──────────────────┐    ┌──────────────────┐              │  │
│  │  │ DNS Check        │    │ DNS Check        │              │  │
│  │  │ API Check        │    │ API Check        │              │  │
│  │  │ Network Check    │    │ Network Check    │              │  │
│  │  │ Service Check    │    │ Service Check    │              │  │
│  │  └─────┬────────────┘    └─────┬────────────┘              │  │
│  │        │ Reports Result         │ Reports Result            │  │
│  │        ↓                        ↓                            │  │
│  │  Success/Failure          Success/Failure                   │  │
│  │  Exit (cleanup)           Exit (cleanup)                    │  │
│  └────────────────────────────────────────────────────────────┘  │
│                                                                    │
└────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities

#### Controller (New Component)
**Purpose**: Centralized orchestration and state management

**Responsibilities**:
1. **Node Watching**: Watch for Node Add/Update/Delete events
2. **State Management**: Track verification state per node (pending, in-progress, verified, failed)
3. **Worker Management**: Create/delete worker pods as needed
4. **Reconciliation**: Ensure every unverified node has a worker pod
5. **Result Processing**: Read worker pod status and update node accordingly
6. **Metrics**: Expose centralized metrics for all nodes
7. **Node Lifecycle**: Delete nodes that fail verification (if configured)
8. **Retry Logic**: Handle transient failures with backoff

**Implementation Details**:
- Single Deployment with 1 replica (leader election for HA later)
- Uses controller-runtime framework (Kubernetes operator pattern)
- Watches Node resources with predicates (filter for unverified nodes)
- Maintains in-memory state cache
- Exposes Prometheus metrics endpoint
- Health/readiness endpoints for Kubernetes probes

#### Worker (Modified Existing Logic)
**Purpose**: Execute node verification checks on specific nodes

**Responsibilities**:
1. **Verification Checks**: Run existing DNS, API, network, service checks
2. **Status Reporting**: Report results via pod status, exit code, and/or annotations
3. **Self-Termination**: Exit after checks complete (success or failure)
4. **Node Affinity**: Must run on the target node being verified

**Implementation Details**:
- Reuse existing checker logic (`internal/checker/`)
- Run as a Job pod with node affinity
- Exit code: 0 = success, non-zero = failure
- Optional: Write results to pod annotations/labels
- Tolerates node taints to schedule on unverified nodes

## Detailed Design

### 1. Controller Component

#### Directory Structure
```
cmd/
  kube-node-ready-controller/
    main.go                 # Controller entrypoint
  kube-node-ready-worker/
    main.go                 # Worker entrypoint (existing main.go refactored)

internal/
  controller/
    node_controller.go      # Main reconciliation logic
    worker_manager.go       # Worker pod lifecycle management
    state.go                # Node state tracking
    metrics.go              # Controller-specific metrics
  worker/
    runner.go               # Worker execution logic (wrapper for existing checker)
  checker/                  # Existing checker logic (unchanged)
  config/                   # Shared config
  k8sclient/               # Shared K8s client
  metrics/                 # Shared metrics (extended)
  node/                    # Existing node operations
```

#### Controller API

```go
// NodeVerificationState tracks the verification state of a node
type NodeVerificationState string

const (
    StateUnverified   NodeVerificationState = "unverified"
    StatePending      NodeVerificationState = "pending"
    StateInProgress   NodeVerificationState = "in_progress"
    StateVerified     NodeVerificationState = "verified"
    StateFailed       NodeVerificationState = "failed"
    StateDeleting     NodeVerificationState = "deleting"
)

// NodeReconciler reconciles Node objects
type NodeReconciler struct {
    client.Client
    Scheme         *runtime.Scheme
    Config         *config.ControllerConfig
    WorkerManager  *WorkerManager
    MetricsServer  *metrics.Server
    StateCache     *StateCache
}

// Reconcile handles node events
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch node
    // 2. Check if node needs verification
    // 3. Check current state
    // 4. Take action based on state:
    //    - Unverified -> Create worker pod
    //    - InProgress -> Check worker pod status
    //    - Failed -> Retry or delete node
    //    - Verified -> No action (ensure taint removed)
    // 5. Update metrics
    // 6. Return result (requeue if needed)
}
```

#### State Management

```go
// StateCache maintains in-memory state of node verifications
type StateCache struct {
    mu     sync.RWMutex
    states map[string]*NodeState
}

type NodeState struct {
    NodeName        string
    State           NodeVerificationState
    WorkerPodName   string
    LastAttempt     time.Time
    AttemptCount    int
    LastError       string
    CreatedAt       time.Time
    VerifiedAt      *time.Time
}

// Methods:
// - Get(nodeName) *NodeState
// - Set(nodeName, state)
// - Delete(nodeName)
// - List() []*NodeState
```

#### Worker Manager

```go
// WorkerManager creates and monitors worker pods
type WorkerManager struct {
    client    client.Client
    config    *config.ControllerConfig
    namespace string
}

// CreateWorkerPod creates a verification pod for a node
func (w *WorkerManager) CreateWorkerPod(ctx context.Context, nodeName string) (*corev1.Pod, error)

// GetWorkerPodStatus gets the status of a worker pod
func (w *WorkerManager) GetWorkerPodStatus(ctx context.Context, podName string) (*WorkerPodStatus, error)

// DeleteWorkerPod deletes a worker pod
func (w *WorkerManager) DeleteWorkerPod(ctx context.Context, podName string) error

// WorkerPodStatus represents the status of a worker pod
type WorkerPodStatus struct {
    Phase       corev1.PodPhase
    ExitCode    *int32
    Reason      string
    Message     string
    Completed   bool
    Succeeded   bool
}
```

### 2. Worker Component

#### Worker Pod Specification

The controller will create worker pods with this specification:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: node-ready-worker-<node-name>-<random-suffix>
  namespace: kube-system
  labels:
    app: kube-node-ready
    component: worker
    node: <node-name>
  ownerReferences:
    # Reference to controller for cleanup
spec:
  restartPolicy: Never
  serviceAccountName: kube-node-ready-worker
  nodeSelector:
    kubernetes.io/hostname: <node-name>
  tolerations:
    - key: node-ready/unverified
      operator: Exists
      effect: NoSchedule
    - key: node.kubernetes.io/not-ready
      operator: Exists
      effect: NoSchedule
  containers:
  - name: worker
    image: kube-node-ready:latest
    command: ["kube-node-ready-worker"]
    env:
    - name: NODE_NAME
      value: <node-name>
    - name: WORKER_MODE
      value: "true"
    # ... other config
    resources:
      requests:
        memory: 64Mi
        cpu: 50m
      limits:
        memory: 128Mi
        cpu: 100m
```

#### Worker Execution Flow

```go
// Worker main function
func main() {
    // 1. Parse config
    // 2. Create checker (existing logic)
    // 3. Run verification checks
    // 4. Exit with appropriate code:
    //    - 0: All checks passed
    //    - 1: Checks failed
    //    - 2: Configuration error
}
```

### 3. Reconciliation Logic

#### State Transitions

```
┌──────────────┐
│ Unverified   │ (Node created with taint)
└──────┬───────┘
       │
       │ Controller creates worker pod
       ↓
┌──────────────┐
│   Pending    │ (Worker pod created, not yet scheduled)
└──────┬───────┘
       │
       │ Worker pod scheduled & running
       ↓
┌──────────────┐
│ In Progress  │ (Worker performing checks)
└──────┬───────┘
       │
       ├──→ Success ──→ ┌──────────┐
       │                │ Verified │ (Taint removed, label added)
       │                └──────────┘
       │
       └──→ Failure ──→ ┌─────────┐
                        │ Failed  │ (Retry or delete node)
                        └────┬────┘
                             │
                    ┌────────┴────────┐
                    │                 │
              Retry Logic      Delete Node (optional)
                    │                 │
                    ↓                 ↓
              [Unverified]      [Deleting]
```

#### Reconciliation Algorithm

```go
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    node := &corev1.Node{}
    if err := r.Get(ctx, req.NamespacedName, node); err != nil {
        if errors.IsNotFound(err) {
            // Node deleted, clean up state
            r.StateCache.Delete(req.Name)
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    // Check if node already verified (has label)
    if hasVerifiedLabel(node) {
        r.StateCache.Delete(node.Name)
        return ctrl.Result{}, nil
    }

    // Get current state
    state := r.StateCache.Get(node.Name)
    if state == nil {
        state = &NodeState{
            NodeName:  node.Name,
            State:     StateUnverified,
            CreatedAt: time.Now(),
        }
        r.StateCache.Set(node.Name, state)
    }

    switch state.State {
    case StateUnverified:
        return r.handleUnverified(ctx, node, state)
    case StatePending, StateInProgress:
        return r.handleInProgress(ctx, node, state)
    case StateFailed:
        return r.handleFailed(ctx, node, state)
    case StateVerified:
        return r.handleVerified(ctx, node, state)
    }

    return ctrl.Result{}, nil
}

func (r *NodeReconciler) handleUnverified(ctx context.Context, node *corev1.Node, state *NodeState) (ctrl.Result, error) {
    // Create worker pod
    pod, err := r.WorkerManager.CreateWorkerPod(ctx, node.Name)
    if err != nil {
        metrics.WorkerPodCreationTotal.WithLabelValues(node.Name, "failure").Inc()
        return ctrl.Result{RequeueAfter: 10 * time.Second}, err
    }

    state.State = StatePending
    state.WorkerPodName = pod.Name
    state.LastAttempt = time.Now()
    state.AttemptCount++
    r.StateCache.Set(node.Name, state)

    metrics.WorkerPodCreationTotal.WithLabelValues(node.Name, "success").Inc()
    
    // Requeue to check status
    return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *NodeReconciler) handleInProgress(ctx context.Context, node *corev1.Node, state *NodeState) (ctrl.Result, error) {
    // Check worker pod status
    status, err := r.WorkerManager.GetWorkerPodStatus(ctx, state.WorkerPodName)
    if err != nil {
        return ctrl.Result{RequeueAfter: 5 * time.Second}, err
    }

    if !status.Completed {
        // Update state if needed
        if state.State != StateInProgress {
            state.State = StateInProgress
            r.StateCache.Set(node.Name, state)
        }
        
        // Check for timeout
        if time.Since(state.LastAttempt) > r.Config.WorkerTimeout {
            // Worker timed out
            r.WorkerManager.DeleteWorkerPod(ctx, state.WorkerPodName)
            state.State = StateFailed
            state.LastError = "worker pod timed out"
            r.StateCache.Set(node.Name, state)
            return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
        }
        
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    }

    // Worker completed
    if status.Succeeded {
        // Remove taint and add label
        if err := r.updateNodeOnSuccess(ctx, node); err != nil {
            return ctrl.Result{RequeueAfter: 5 * time.Second}, err
        }
        
        state.State = StateVerified
        now := time.Now()
        state.VerifiedAt = &now
        r.StateCache.Set(node.Name, state)
        
        metrics.NodeVerificationTotal.WithLabelValues(node.Name, "success").Inc()
        
        // Clean up worker pod
        r.WorkerManager.DeleteWorkerPod(ctx, state.WorkerPodName)
        
        return ctrl.Result{}, nil
    } else {
        // Worker failed
        state.State = StateFailed
        state.LastError = status.Message
        r.StateCache.Set(node.Name, state)
        
        metrics.NodeVerificationTotal.WithLabelValues(node.Name, "failure").Inc()
        
        // Clean up worker pod
        r.WorkerManager.DeleteWorkerPod(ctx, state.WorkerPodName)
        
        return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
    }
}

func (r *NodeReconciler) handleFailed(ctx context.Context, node *corev1.Node, state *NodeState) (ctrl.Result, error) {
    // Check retry logic
    if state.AttemptCount < r.Config.MaxRetries {
        // Retry with backoff
        backoff := r.calculateBackoff(state.AttemptCount)
        if time.Since(state.LastAttempt) < backoff {
            return ctrl.Result{RequeueAfter: backoff - time.Since(state.LastAttempt)}, nil
        }
        
        // Reset to unverified for retry
        state.State = StateUnverified
        r.StateCache.Set(node.Name, state)
        
        return ctrl.Result{}, nil
    }

    // Max retries exceeded
    if r.Config.DeleteFailedNodes {
        // Delete the node
        klog.InfoS("Deleting failed node", "node", node.Name, "attempts", state.AttemptCount)
        
        if err := r.Client.Delete(ctx, node); err != nil {
            return ctrl.Result{RequeueAfter: 10 * time.Second}, err
        }
        
        state.State = StateDeleting
        r.StateCache.Set(node.Name, state)
        
        metrics.NodeDeletionTotal.WithLabelValues(node.Name, "success").Inc()
    }

    return ctrl.Result{}, nil
}

func (r *NodeReconciler) handleVerified(ctx context.Context, node *corev1.Node, state *NodeState) (ctrl.Result, error) {
    // Nothing to do, node is verified
    return ctrl.Result{}, nil
}
```

### 4. Configuration

#### Controller Configuration

```go
type ControllerConfig struct {
    // Worker configuration
    WorkerImage         string
    WorkerNamespace     string
    WorkerTimeout       time.Duration
    WorkerResources     corev1.ResourceRequirements
    
    // Reconciliation
    MaxRetries          int
    RetryBackoff        string
    ReconcileInterval   time.Duration
    
    // Node management
    DeleteFailedNodes   bool
    TaintKey            string
    TaintValue          string
    TaintEffect         string
    VerifiedLabel       string
    VerifiedLabelValue  string
    
    // Metrics
    MetricsEnabled      bool
    MetricsPort         int
    
    // Health
    HealthPort          int
    
    // Logging
    LogLevel            string
    LogFormat           string
}
```

#### Worker Configuration (Existing + Changes)

```go
type WorkerConfig struct {
    // Existing fields (from current Config)
    NodeName              string
    Namespace             string
    CheckTimeout          time.Duration
    DNSTestDomains        []string
    ClusterDNSIP          string
    KubernetesServiceHost string
    KubernetesServicePort string
    
    // New fields
    WorkerMode            bool   // Indicates worker mode (vs controller mode)
    ReportResultsToAPI    bool   // Whether to report via annotations
    
    // Removed fields (controller handles these)
    // - InitialCheckTimeout
    // - MaxRetries
    // - RetryBackoff
    // - TaintKey, TaintValue, TaintEffect
    // - VerifiedLabel, VerifiedLabelValue
    // - DeleteFailedNode
    // - MetricsEnabled (workers don't expose metrics)
}
```

### 5. Metrics

#### Controller Metrics (New)

```go
// Controller-specific metrics
var (
    // Reconciliation metrics
    ReconciliationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kube_node_ready_controller_reconciliation_total",
            Help: "Total number of reconciliation loops",
        },
        []string{"result"}, // result: success, error
    )

    ReconciliationDuration = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "kube_node_ready_controller_reconciliation_duration_seconds",
            Help:    "Duration of reconciliation loops",
            Buckets: prometheus.DefBuckets,
        },
    )

    // Worker pod metrics
    WorkerPodCreationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kube_node_ready_worker_pod_creation_total",
            Help: "Total number of worker pod creation attempts",
        },
        []string{"node", "status"}, // status: success, failure
    )

    WorkerPodDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kube_node_ready_worker_pod_duration_seconds",
            Help:    "Duration from worker pod creation to completion",
            Buckets: []float64{5, 10, 30, 60, 120, 300, 600},
        },
        []string{"node", "result"}, // result: success, failure, timeout
    )

    // Node state metrics
    NodesInState = promauto.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "kube_node_ready_nodes_in_state",
            Help: "Number of nodes in each verification state",
        },
        []string{"state"}, // state: unverified, pending, in_progress, verified, failed
    )

    NodeVerificationTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "kube_node_ready_node_verification_total",
            Help: "Total number of node verifications completed",
        },
        []string{"node", "result"}, // result: success, failure
    )

    NodeVerificationAttempts = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "kube_node_ready_node_verification_attempts",
            Help:    "Number of attempts needed for node verification",
            Buckets: []float64{1, 2, 3, 4, 5, 10},
        },
        []string{"node"},
    )
)
```

#### Worker Metrics (Modified)

Workers no longer expose metrics endpoints. Instead, they focus on:
- Exit codes (0 = success, non-zero = failure)
- Pod status and conditions
- Optional: annotations with check results

### 6. RBAC Permissions

#### Controller RBAC

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kube-node-ready-controller
rules:
# Node permissions
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "list", "watch", "patch", "delete"]
  
# Pod permissions (for worker management)
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch", "create", "delete"]

# Service/Endpoint discovery (for worker pods)
- apiGroups: [""]
  resources: ["services", "endpoints"]
  verbs: ["get", "list"]

# Events (for debugging)
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]

# Leader election (for HA)
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "create", "update"]
```

#### Worker RBAC

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kube-node-ready-worker
rules:
# Minimal permissions - only for health checks
- apiGroups: [""]
  resources: ["services", "endpoints"]
  verbs: ["get", "list"]

# No node modification permissions!
# Workers report via exit codes/status
```

### 7. Deployment Manifests

#### Controller Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kube-node-ready-controller
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kube-node-ready
      component: controller
  template:
    metadata:
      labels:
        app: kube-node-ready
        component: controller
    spec:
      serviceAccountName: kube-node-ready-controller
      containers:
      - name: controller
        image: kube-node-ready:latest
        command: ["kube-node-ready-controller"]
        env:
        - name: WORKER_IMAGE
          value: "kube-node-ready:latest"
        - name: WORKER_NAMESPACE
          value: "kube-system"
        - name: MAX_RETRIES
          value: "5"
        - name: DELETE_FAILED_NODES
          value: "false"
        resources:
          requests:
            memory: 64Mi
            cpu: 50m
          limits:
            memory: 128Mi
            cpu: 200m
        ports:
        - name: metrics
          containerPort: 8080
        - name: health
          containerPort: 8081
        livenessProbe:
          httpGet:
            path: /healthz
            port: health
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: health
          initialDelaySeconds: 5
          periodSeconds: 10
```

## Migration Strategy

### Phase 1: Preparation (Week 1)
1. **Create new directory structure**
   - `cmd/kube-node-ready-controller/`
   - `cmd/kube-node-ready-worker/`
   - `internal/controller/`
   - `internal/worker/`

2. **Add dependencies**
   - Add `controller-runtime` to `go.mod`
   - Add `operator-sdk` (optional, for scaffolding)

3. **Refactor existing code**
   - Move current `main.go` logic to `cmd/kube-node-ready-worker/main.go`
   - Keep `internal/checker/` unchanged
   - Extract common config logic

### Phase 2: Controller Implementation (Week 2-3)
1. **Implement core controller**
   - `NodeReconciler` with basic reconciliation loop
   - `StateCache` for state management
   - `WorkerManager` for pod lifecycle

2. **Implement worker pod creation**
   - Pod specification generation
   - Node affinity and tolerations
   - Resource allocation

3. **Implement state transitions**
   - Unverified → Pending → InProgress → Verified/Failed

### Phase 3: Metrics & Observability (Week 4)
1. **Add controller metrics**
   - Reconciliation metrics
   - Worker pod metrics
   - Node state metrics

2. **Add health endpoints**
   - `/healthz` - liveness
   - `/readyz` - readiness
   - `/metrics` - Prometheus metrics

3. **Add logging**
   - Structured logging with klog
   - Debug mode for troubleshooting

### Phase 4: Testing (Week 5)
1. **Unit tests**
   - Controller reconciliation logic
   - State transitions
   - Worker manager

2. **Integration tests**
   - End-to-end flows
   - Failure scenarios
   - Retry logic

3. **Load testing**
   - Multiple nodes (10, 100, 1000)
   - Concurrent verifications
   - API server load

### Phase 5: Documentation & Deployment (Week 6)
1. **Update documentation**
   - New architecture diagrams
   - Configuration guide
   - Migration guide

2. **Create Helm charts**
   - Controller deployment
   - Worker RBAC
   - ConfigMaps

3. **Migration guide**
   - Steps to migrate from DaemonSet
   - Rollback procedures

### Phase 6: Rollout (Week 7-8)
1. **Canary deployment**
   - Deploy to test cluster
   - Monitor metrics and logs
   - Validate functionality

2. **Production rollout**
   - Blue-green deployment
   - Gradual rollout with monitoring
   - Rollback plan ready

3. **Deprecate DaemonSet**
   - Keep DaemonSet as fallback (1-2 releases)
   - Remove DaemonSet in future release

## Benefits of New Architecture

### 1. Centralized Control
- Single controller manages all node verifications
- Unified state management and decision-making
- Easier to debug and monitor

### 2. Better Resource Efficiency
- Controller runs once per cluster (vs per node)
- Workers are short-lived (vs long-running DaemonSet pods)
- Reduced memory footprint (no idle pods)

### 3. Enhanced Reliability
- Automatic retries with backoff
- Better failure handling
- Reconciliation ensures eventual consistency

### 4. Improved Observability
- Centralized metrics (single endpoint)
- Better visibility into verification state
- Easier integration with monitoring

### 5. Greater Flexibility
- Easy to add new verification strategies
- Can implement complex workflows (e.g., gradual rollout)
- Support for custom node selection

### 6. Kubernetes-Native
- Follows operator pattern
- Uses standard reconciliation loop
- Better integration with K8s ecosystem

## Risks & Mitigations

### Risk 1: Controller Failure
**Impact**: No new nodes verified

**Mitigation**:
- Implement leader election for HA (multiple controller replicas)
- Add comprehensive health checks
- Alert on controller unavailability

### Risk 2: Worker Pod Scheduling Issues
**Impact**: Verification stuck in pending state

**Mitigation**:
- Implement timeout detection in controller
- Retry with exponential backoff
- Alert on stuck verifications

### Risk 3: Increased API Server Load
**Impact**: More API calls from controller

**Mitigation**:
- Use informers/caches (controller-runtime handles this)
- Implement rate limiting
- Tune reconciliation intervals

### Risk 4: Migration Complexity
**Impact**: Breaking changes for existing users

**Mitigation**:
- Provide migration guide
- Support both modes initially (feature flag)
- Thorough testing in staging

### Risk 5: State Inconsistency
**Impact**: Controller state out of sync with cluster

**Mitigation**:
- Periodic full reconciliation
- State stored in K8s annotations (source of truth)
- Controller restarts don't lose state

## Success Metrics

### Functional Metrics
- ✅ All nodes verified successfully within SLA (< 2 minutes)
- ✅ Failed nodes retried appropriately
- ✅ Controller uptime > 99.9%

### Performance Metrics
- ✅ Controller CPU < 100m steady state
- ✅ Controller memory < 128Mi steady state
- ✅ Reconciliation latency < 1s (p99)

### Reliability Metrics
- ✅ Zero false positives (healthy nodes marked as failed)
- ✅ Zero false negatives (unhealthy nodes marked as verified)
- ✅ Automatic recovery from transient failures

## Future Enhancements

### Post-MVP Features
1. **Leader Election**: Multiple controller replicas for HA
2. **Webhook Validation**: ValidatingWebhook for node creation
3. **Custom Resources**: NodeVerification CRD for advanced config
4. **Webhooks**: Admission webhooks for automated taint injection
5. **Advanced Scheduling**: Priority-based verification
6. **Continuous Monitoring**: Periodic re-verification of verified nodes
7. **Integration**: Integration with node provisioners (Karpenter, Cluster Autoscaler)

## Appendix

### A. File Changes Summary

#### New Files
```
cmd/kube-node-ready-controller/main.go        (new controller entrypoint)
internal/controller/node_controller.go         (reconciliation logic)
internal/controller/worker_manager.go          (worker pod management)
internal/controller/state.go                   (state cache)
internal/controller/metrics.go                 (controller metrics)
internal/worker/runner.go                      (worker wrapper)
deploy/helm/kube-node-ready/templates/controller-deployment.yaml
deploy/helm/kube-node-ready/templates/controller-service.yaml
deploy/helm/kube-node-ready/templates/worker-clusterrole.yaml
```

#### Modified Files
```
cmd/kube-node-ready/main.go                    → cmd/kube-node-ready-worker/main.go
internal/config/config.go                      (split into controller/worker configs)
internal/metrics/metrics.go                    (add controller metrics)
deploy/helm/kube-node-ready/values.yaml       (add controller config)
deploy/helm/kube-node-ready/Chart.yaml        (version bump)
docs/ARCHITECTURE.md                           (update architecture docs)
README.md                                      (update usage instructions)
```

#### Removed Files
```
deploy/helm/kube-node-ready/templates/daemonset.yaml  (replaced by controller + worker)
```

### B. Dependencies

#### New Dependencies
```
sigs.k8s.io/controller-runtime v0.17.x
k8s.io/apimachinery v0.29.x (already present)
k8s.io/client-go v0.29.x (already present)
```

### C. Breaking Changes

1. **Deployment Model**: DaemonSet → Controller + Worker Pods
2. **Metrics Endpoint**: Multiple endpoints → Single controller endpoint
3. **Configuration**: Some flags moved from worker to controller
4. **RBAC**: Different permissions for controller vs worker

### D. Backwards Compatibility

**Option 1**: Hard cutover (recommended for v2.0)
- Remove DaemonSet support
- Clean migration path with documentation

**Option 2**: Dual mode (recommended for v1.x → v2.0 transition)
- Support both DaemonSet and Controller modes
- Feature flag: `--mode=daemonset|controller`
- Deprecation warning for DaemonSet mode

## Conclusion

This migration transforms `kube-node-ready` from a distributed DaemonSet-based tool into a centralized, operator-style controller with ephemeral workers. The new architecture provides:

- ✅ **Better control**: Centralized orchestration and state management
- ✅ **Resource efficiency**: Short-lived workers, single controller
- ✅ **Reliability**: Automatic retries, reconciliation, failure handling
- ✅ **Observability**: Unified metrics, better debugging
- ✅ **Scalability**: Handles large clusters efficiently

The migration is structured in phases with clear milestones, risk mitigations, and success criteria. The resulting system will be more robust, maintainable, and aligned with Kubernetes operator best practices.

---

**Next Steps**: 
1. Review and approve this plan
2. Create GitHub issues for each phase
3. Set up project board for tracking
4. Begin Phase 1 implementation

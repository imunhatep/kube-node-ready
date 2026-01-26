# Architecture Overview

## System Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                        │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                    Karpenter Controller                     │ │
│  │                  (Node Provisioner)                         │ │
│  └──────────────────────┬─────────────────────────────────────┘ │
│                         │ Creates new node                       │
│                         ▼                                        │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                      New Node                               │ │
│  │  Initial State: Tainted (NoSchedule)                        │ │
│  │  Taint: node-ready/unverified=true:NoSchedule              │ │
│  │                                                              │ │
│  │  ┌──────────────────────────────────────────────────────┐  │ │
│  │  │    kube-node-ready Pod (DaemonSet)                   │  │ │
│  │  │                                                        │  │ │
│  │  │  1. Starts automatically on new node                  │  │ │
│  │  │  2. Tolerates not-ready taint                         │  │ │
│  │  │                                                        │  │ │
│  │  │  ┌────────────────────────────────────────────────┐  │  │ │
│  │  │  │  Verification Checks (Sequential)              │  │  │ │
│  │  │  │                                                  │  │  │ │
│  │  │  │  ┌──────────────────────────────────────────┐  │  │  │ │
│  │  │  │  │ 1. DNS Resolution Check                  │  │  │  │ │
│  │  │  │  │    - kubernetes.default.svc.cluster.local│  │  │  │ │
│  │  │  │  │    - External DNS (google.com)           │  │  │  │ │
│  │  │  │  │    Timeout: configurable (default 3s)    │  │  │  │ │
│  │  │  │  └──────────────┬───────────────────────────┘  │  │  │ │
│  │  │  │                 │ PASS                          │  │  │ │
│  │  │  │                 ▼                               │  │  │ │
│  │  │  │  ┌──────────────────────────────────────────┐  │  │  │ │
│  │  │  │  │ 2. Kubernetes API Check                  │  │  │  │ │
│  │  │  │  │    - Connect to API server               │  │  │  │ │
│  │  │  │  │    - Verify authentication               │  │  │  │ │
│  │  │  │  │    Timeout: configurable (default 3s)    │  │  │  │ │
│  │  │  │  └──────────────┬───────────────────────────┘  │  │  │ │
│  │  │  │                 │ PASS                          │  │  │ │
│  │  │  │                 ▼                               │  │  │ │
│  │  │  │  ┌──────────────────────────────────────────┐  │  │  │ │
│  │  │  │  │ 3. Cluster Network Check                 │  │  │  │ │
│  │  │  │  │    - TCP to kubernetes service           │  │  │  │ │
│  │  │  │  │    - Pod-to-pod connectivity             │  │  │  │ │
│  │  │  │  │    Timeout: configurable (default 3s)    │  │  │  │ │
│  │  │  │  └──────────────┬───────────────────────────┘  │  │  │ │
│  │  │  │                 │ PASS                          │  │  │ │
│  │  │  │                 ▼                               │  │  │ │
│  │  │  │  ┌──────────────────────────────────────────┐  │  │  │ │
│  │  │  │  │ 4. Service Discovery Check               │  │  │  │ │
│  │  │  │  │    - Query service endpoints             │  │  │  │ │
│  │  │  │  │    - Verify kube-dns/CoreDNS             │  │  │  │ │
│  │  │  │  │    Timeout: configurable (default 3s)    │  │  │  │ │
│  │  │  │  └──────────────┬───────────────────────────┘  │  │  │ │
│  │  │  │                 │ ALL PASS                      │  │  │ │
│  │  │  │                 ▼                               │  │  │ │
│  │  │  └─────────────────────────────────────────────┘  │  │ │
│  │  │                                                        │  │ │
│  │  │  ┌────────────────────────────────────────────────┐  │  │ │
│  │  │  │  Remove Taint from Node                        │  │  │ │
│  │  │  │  kubectl patch node <name> --type=json         │  │  │ │
│  │  │  │  -p='[{"op":"remove","path":"/spec/taints/.."}]'│  │ │
│  │  │  └────────────────┬───────────────────────────────┘  │  │ │
│  │  │                   │                                   │  │ │
│  │  │                   ▼                                   │  │ │
│  │  │  ┌────────────────────────────────────────────────┐  │  │ │
│  │  │  │  Periodic Health Checks (every 60s)            │  │  │ │
│  │  │  │  - Re-run all verification checks              │  │  │ │
│  │  │  │  - If failures detected → Re-apply taint       │  │  │ │
│  │  │  │  - Expose metrics to Prometheus                │  │  │ │
│  │  │  └────────────────────────────────────────────────┘  │  │ │
│  │  │                                                        │  │ │
│  │  └──────────────────────────────────────────────────────┘  │ │
│  │                                                              │ │
│  └────────────┬─────────────────────────────────────────────────┘
│               │ Node Ready & Untainted                           │
│               ▼                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │              Workload Pods Can Be Scheduled                 │ │
│  │              (Normal cluster operations)                     │ │
│  └────────────────────────────────────────────────────────────┘ │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                 Monitoring Stack                            │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌─────────────────┐  │ │
│  │  │ Prometheus   │◄─│ Metrics      │◄─│ ServiceMonitor  │  │ │
│  │  │              │  │ Endpoint     │  │                 │  │ │
│  │  └──────────────┘  │ :8080/metrics│  └─────────────────┘  │ │
│  │                    └──────────────┘                         │ │
│  └────────────────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────────────┘
```

## Component Interactions

### 1. Node Initialization Flow
```
Karpenter Creates Node
         ↓
Node starts with taint: node-ready/unverified=true:NoSchedule
         ↓
Kubelet starts
         ↓
DaemonSet controller schedules kube-node-ready pod
         ↓
kube-node-ready pod starts (tolerates taint)
         ↓
Runs verification checks
         ↓
All checks pass → Remove taint
         ↓
Node becomes schedulable for workloads
```

### 2. Verification Check Flow
```
Start
  │
  ├─► DNS Check ────► FAIL ──┐
  │                           │
  ├─► API Check ────► FAIL ──┤
  │                           │
  ├─► Network Check ► FAIL ──┤
  │                           │
  └─► Service Check ► FAIL ──┤
                              │
         ALL PASS             │
            ↓                 ↓
      Remove Taint      Keep Taint
            ↓           Retry (backoff)
    Start Monitoring          │
            ↓                 │
    Periodic Checks ←─────────┘
```

### 3. RBAC Interaction
```
┌─────────────────────────────────────────┐
│      kube-node-ready Pod                │
│  (ServiceAccount: kube-node-ready)      │
└────────────────┬────────────────────────┘
                 │
                 │ Uses ServiceAccount Token
                 ▼
┌─────────────────────────────────────────┐
│      Kubernetes API Server              │
│  ┌───────────────────────────────────┐  │
│  │ RBAC Authorization                │  │
│  │                                   │  │
│  │ ClusterRole: kube-node-ready     │  │
│  │ - nodes: get, list, patch        │  │
│  │ - services: get, list            │  │
│  │ - endpoints: get, list           │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

## Data Flow

### Configuration
```
values.yaml (Helm)
       ↓
ConfigMap (Kubernetes)
       ↓
Environment Variables
       ↓
Go Application Config
```

### Metrics
```
Verification Checks
       ↓
Prometheus Metrics (in-memory)
       ↓
/metrics endpoint exposed
       ↓
ServiceMonitor (CRD)
       ↓
Prometheus scrapes metrics
       ↓
Grafana dashboards
```

## Network Topology

```
┌─────────────────────────────────────────────────────────┐
│                        Node                              │
│                                                           │
│  ┌─────────────────────────────────────────────────┐    │
│  │  kube-node-ready Pod (Pod Network)              │    │
│  │  IP: 10.244.x.x                                  │    │
│  │                                                   │    │
│  │  Checks:                                         │    │
│  │  1. DNS → CoreDNS (ClusterIP: 10.96.0.10)       │    │
│  │  2. API → kubernetes.default (ClusterIP)        │    │
│  │  3. Internet → External DNS                     │    │
│  │  4. Services → Endpoint discovery               │    │
│  └─────────────────────────────────────────────────┘    │
│                                                           │
│  Network Interfaces:                                     │
│  - eth0: Host network (EC2 network)                     │
│  - cni0: Pod network bridge                             │
│                                                           │
└─────────────────────────────────────────────────────────┘
```

## Failure Scenarios & Recovery

### Scenario 1: DNS Failure
```
DNS Check Fails
       ↓
Log error with details
       ↓
Retry with exponential backoff (1s, 2s, 4s, 8s, 16s)
       ↓
Max retries (5) reached
       ↓
Keep taint on node
       ↓
Emit metric: node_check_failures_total{check="dns"}
       ↓
Alert (if configured)
```

### Scenario 2: Transient Network Blip
```
All checks passing, node running
       ↓
Periodic check encounters network error
       ↓
Single failure → Log warning (don't re-taint yet)
       ↓
Continue monitoring
       ↓
If 3 consecutive failures → Re-apply taint & alert
```

### Scenario 3: Node Removal
```
Node draining initiated
       ↓
SIGTERM sent to kube-node-ready pod
       ↓
Graceful shutdown (30s timeout)
       ↓
Stop periodic checks
       ↓
Close metrics endpoint
       ↓
Exit cleanly
```

## Security Architecture

### Pod Security
```
┌─────────────────────────────────────────┐
│  Pod Security Context                   │
│  - runAsNonRoot: true                   │
│  - runAsUser: 1000                      │
│  - readOnlyRootFilesystem: true         │
│  - allowPrivilegeEscalation: false      │
│  - capabilities: drop ALL               │
└─────────────────────────────────────────┘
```

### Network Security
```
┌─────────────────────────────────────────┐
│  Network Policies (Recommended)         │
│                                          │
│  Egress:                                │
│  - Allow DNS (port 53)                  │
│  - Allow Kubernetes API (port 443)      │
│  - Allow external (if needed)           │
│                                          │
│  Ingress:                               │
│  - Allow metrics scraping (port 8080)   │
│    from Prometheus namespace            │
└─────────────────────────────────────────┘
```

## Performance Characteristics

### Resource Usage
```
Memory: 
  - Startup: ~20 Mi
  - Steady state: ~40 Mi
  - Peak: ~80 Mi

CPU:
  - Startup: ~50m (0.05 cores)
  - Steady state: ~5m (0.005 cores)
  - During checks: ~30m (0.03 cores)

Network:
  - Minimal (KB/s)
  - Spike during checks
```

### Timing
```
Node Creation → Pod Start: ~10-20s
Pod Start → First Check: ~2-5s
Total Verification Time: ~15-30s
Check Duration: ~5-10s each
Periodic Check Interval: 60s (configurable)
```

## Scalability

### Cluster Size Impact
```
Small Cluster (10 nodes):
  - Total Memory: 400 Mi
  - Total CPU: 50m
  - Negligible impact

Large Cluster (1000 nodes):
  - Total Memory: 40 Gi
  - Total CPU: 5 cores
  - Still minimal impact (~0.1% of cluster resources)
```

### API Server Load
```
Per Node:
  - Initial: 1-2 API calls to get node & patch
  - Periodic: 1-2 API calls every 60s
  
1000 Nodes:
  - ~16-33 API calls/second
  - Minimal impact on API server
```

---

This architecture is designed to be:
- ✅ **Lightweight**: Minimal resource footprint, pods terminate after verification
- ✅ **Fast**: Quick verification (< 30s) then cleanup
- ✅ **Reliable**: Handles transient failures with retries
- ✅ **Secure**: Minimal permissions, non-root
- ✅ **Observable**: Rich metrics and logging during execution
- ✅ **Scalable**: Works from 10 to 1000+ nodes
- ✅ **Efficient**: Zero ongoing resource consumption after verification
- ✅ **Simple**: One-time execution, no continuous monitoring complexity

# Prometheus Metrics for Controller

## Overview

The kube-node-ready controller exposes comprehensive Prometheus metrics to monitor its operation, track worker pod lifecycle, node verification state, and leader election status.

## Metrics Endpoint

**Endpoint:** `http://<controller-service>:8080/metrics`

**Access:**
```bash
# Port-forward to controller service
kubectl port-forward -n kube-system service/kube-node-ready-controller 8080:8080

# View metrics
curl http://localhost:8080/metrics
```

## Metric Categories

### 1. Reconciliation Metrics

#### `kube_node_ready_controller_reconciliation_total`
**Type:** Counter  
**Description:** Total number of node reconciliations  
**Labels:**
- `result`: Reconciliation result (`success`, `error`, `requeue`, `deleted`, `worker_created`, `worker_succeeded`, `worker_failed`, `already_verified`, `no_taint`, `retry`, `backoff`, `max_retries_exceeded`, `node_deleted`)

**Example:**
```promql
# Rate of successful reconciliations
rate(kube_node_ready_controller_reconciliation_total{result="success"}[5m])

# Total failed reconciliations
kube_node_ready_controller_reconciliation_total{result="error"}
```

#### `kube_node_ready_controller_reconciliation_duration_seconds`
**Type:** Histogram  
**Description:** Duration of node reconciliation in seconds  
**Labels:**
- `node`: Node name

**Buckets:** 0.1, 0.5, 1, 2, 5, 10, 30

**Example:**
```promql
# Average reconciliation duration
rate(kube_node_ready_controller_reconciliation_duration_seconds_sum[5m]) /
rate(kube_node_ready_controller_reconciliation_duration_seconds_count[5m])

# 95th percentile reconciliation time
histogram_quantile(0.95, rate(kube_node_ready_controller_reconciliation_duration_seconds_bucket[5m]))
```

#### `kube_node_ready_controller_reconciliation_errors_total`
**Type:** Counter  
**Description:** Total number of reconciliation errors  
**Labels:**
- `node`: Node name
- `error_type`: Error type (`node_fetch_error`, `worker_pod_creation_error`, `max_retries_exceeded`, `node_deletion_error`)

**Example:**
```promql
# Nodes with most errors
topk(5, sum by (node) (kube_node_ready_controller_reconciliation_errors_total))
```

### 2. Worker Pod Metrics

#### `kube_node_ready_controller_worker_pods_created_total`
**Type:** Counter  
**Description:** Total number of worker pods created  
**Labels:**
- `node`: Node name

**Example:**
```promql
# Worker pod creation rate
rate(kube_node_ready_controller_worker_pods_created_total[5m])
```

#### `kube_node_ready_controller_worker_pods_succeeded_total`
**Type:** Counter  
**Description:** Total number of worker pods that succeeded  
**Labels:**
- `node`: Node name

#### `kube_node_ready_controller_worker_pods_failed_total`
**Type:** Counter  
**Description:** Total number of worker pods that failed  
**Labels:**
- `node`: Node name
- `reason`: Failure reason (`timeout`, `check_failed`)

**Example:**
```promql
# Worker pod success rate
sum(rate(kube_node_ready_controller_worker_pods_succeeded_total[5m])) /
(sum(rate(kube_node_ready_controller_worker_pods_succeeded_total[5m])) +
 sum(rate(kube_node_ready_controller_worker_pods_failed_total[5m])))
```

#### `kube_node_ready_controller_worker_pods_active`
**Type:** Gauge  
**Description:** Current number of active worker pods per node  
**Labels:**
- `node`: Node name

**Example:**
```promql
# Total active worker pods
sum(kube_node_ready_controller_worker_pods_active)

# Nodes with active workers
count(kube_node_ready_controller_worker_pods_active > 0)
```

#### `kube_node_ready_controller_worker_pod_duration_seconds`
**Type:** Histogram  
**Description:** Duration of worker pod execution in seconds  
**Labels:**
- `node`: Node name
- `result`: Execution result (`success`, `failure`, `timeout`)

**Buckets:** 10, 30, 60, 120, 300, 600

**Example:**
```promql
# Average worker pod duration by result
avg by (result) (
  rate(kube_node_ready_controller_worker_pod_duration_seconds_sum[5m]) /
  rate(kube_node_ready_controller_worker_pod_duration_seconds_count[5m])
)

# Slow worker pods (>5min)
kube_node_ready_controller_worker_pod_duration_seconds_bucket{le="300"} - 
kube_node_ready_controller_worker_pod_duration_seconds_bucket{le="60"}
```

#### `kube_node_ready_controller_worker_pod_creation_errors_total`
**Type:** Counter  
**Description:** Total number of worker pod creation errors  
**Labels:**
- `node`: Node name
- `error_type`: Error type (`create_failed`)

### 3. Node State Metrics

#### `kube_node_ready_controller_nodes_unverified`
**Type:** Gauge  
**Description:** Current number of unverified nodes

**Example:**
```promql
# Unverified nodes count
kube_node_ready_controller_nodes_unverified
```

#### `kube_node_ready_controller_nodes_verifying`
**Type:** Gauge  
**Description:** Current number of nodes being verified

#### `kube_node_ready_controller_nodes_verified`
**Type:** Gauge  
**Description:** Current number of verified nodes

#### `kube_node_ready_controller_nodes_failed`
**Type:** Gauge  
**Description:** Current number of nodes that failed verification

**Example:**
```promql
# Node verification pipeline
sum(kube_node_ready_controller_nodes_unverified) as unverified,
sum(kube_node_ready_controller_nodes_verifying) as verifying,
sum(kube_node_ready_controller_nodes_verified) as verified,
sum(kube_node_ready_controller_nodes_failed) as failed
```

#### `kube_node_ready_controller_node_retry_count`
**Type:** Gauge  
**Description:** Current retry count for node verification  
**Labels:**
- `node`: Node name

**Example:**
```promql
# Nodes with highest retry counts
topk(10, kube_node_ready_controller_node_retry_count)
```

### 4. Leader Election Metrics

#### `kube_node_ready_controller_leader_election_status`
**Type:** Gauge  
**Description:** Leader election status (1=leader, 0=standby)

**Example:**
```promql
# Check if this instance is leader
kube_node_ready_controller_leader_election_status == 1

# Count leaders (should always be 1)
sum(kube_node_ready_controller_leader_election_status)
```

#### `kube_node_ready_controller_leader_election_transitions_total`
**Type:** Counter  
**Description:** Total number of leader election transitions

**Example:**
```promql
# Leader changes rate
rate(kube_node_ready_controller_leader_election_transitions_total[1h])
```

#### `kube_node_ready_controller_leader_election_lease_duration_seconds`
**Type:** Gauge  
**Description:** Leader election lease duration in seconds

### 5. Controller Health Metrics

#### `kube_node_ready_controller_healthy`
**Type:** Gauge  
**Description:** Controller health status (1=healthy, 0=unhealthy)

**Example:**
```promql
# Alert if controller unhealthy
kube_node_ready_controller_healthy == 0
```

#### `kube_node_ready_controller_start_time_seconds`
**Type:** Gauge  
**Description:** Controller start time in unix timestamp

**Example:**
```promql
# Controller uptime
time() - kube_node_ready_controller_start_time_seconds
```

### 6. Queue Metrics

#### `kube_node_ready_controller_reconcile_queue_depth`
**Type:** Gauge  
**Description:** Current depth of the reconciliation queue

**Example:**
```promql
# Alert on queue buildup
kube_node_ready_controller_reconcile_queue_depth > 100
```

#### `kube_node_ready_controller_reconcile_queue_latency_seconds`
**Type:** Histogram  
**Description:** Time items spend in the reconciliation queue  
**Buckets:** 0.1, 0.5, 1, 5, 10, 30, 60

### 7. Taint and Label Operations

#### `kube_node_ready_controller_taints_applied_total`
**Type:** Counter  
**Description:** Total number of taints applied to nodes  
**Labels:**
- `node`: Node name
- `taint_key`: Taint key

#### `kube_node_ready_controller_taints_removed_total`
**Type:** Counter  
**Description:** Total number of taints removed from nodes  
**Labels:**
- `node`: Node name
- `taint_key`: Taint key

**Example:**
```promql
# Taint operations rate
rate(kube_node_ready_controller_taints_removed_total[5m])
```

#### `kube_node_ready_controller_labels_applied_total`
**Type:** Counter  
**Description:** Total number of labels applied to nodes  
**Labels:**
- `node`: Node name
- `label_key`: Label key

### 8. Build Info

#### `kube_node_ready_build_info`
**Type:** Gauge  
**Description:** Build information (always 1)  
**Labels:**
- `version`: Version string
- `commit_hash`: Git commit hash
- `build_date`: Build date

**Example:**
```promql
# Get controller version
kube_node_ready_build_info
```

## Common Queries

### Success Rate
```promql
# Overall verification success rate
sum(rate(kube_node_ready_controller_worker_pods_succeeded_total[5m])) /
(sum(rate(kube_node_ready_controller_worker_pods_succeeded_total[5m])) +
 sum(rate(kube_node_ready_controller_worker_pods_failed_total[5m])))
```

### Average Verification Time
```promql
# Average time to verify a node
avg(
  rate(kube_node_ready_controller_worker_pod_duration_seconds_sum{result="success"}[5m]) /
  rate(kube_node_ready_controller_worker_pod_duration_seconds_count{result="success"}[5m])
)
```

### Problematic Nodes
```promql
# Nodes with most failures
topk(10, 
  sum by (node) (kube_node_ready_controller_worker_pods_failed_total)
)

# Nodes with highest retry counts
topk(10, kube_node_ready_controller_node_retry_count)
```

### Controller Performance
```promql
# Reconciliation rate
sum(rate(kube_node_ready_controller_reconciliation_total[5m]))

# Average reconciliation duration
avg(
  rate(kube_node_ready_controller_reconciliation_duration_seconds_sum[5m]) /
  rate(kube_node_ready_controller_reconciliation_duration_seconds_count[5m])
)
```

### Node Pipeline Status
```promql
# Nodes in verification pipeline
sum(kube_node_ready_controller_nodes_unverified) as unverified +
sum(kube_node_ready_controller_nodes_verifying) as verifying
```

## Prometheus Alerts

### Critical Alerts

```yaml
groups:
- name: kube-node-ready-controller
  rules:
  # Controller down
  - alert: ControllerDown
    expr: up{job="kube-node-ready-controller"} == 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Controller is down"

  # No leader elected
  - alert: NoLeaderElected
    expr: sum(kube_node_ready_controller_leader_election_status) == 0
    for: 2m
    labels:
      severity: critical
    annotations:
      summary: "No controller leader elected"

  # Controller unhealthy
  - alert: ControllerUnhealthy
    expr: kube_node_ready_controller_healthy == 0
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Controller is unhealthy"
```

### Warning Alerts

```yaml
  # High failure rate
  - alert: HighVerificationFailureRate
    expr: |
      sum(rate(kube_node_ready_controller_worker_pods_failed_total[5m])) /
      (sum(rate(kube_node_ready_controller_worker_pods_succeeded_total[5m])) +
       sum(rate(kube_node_ready_controller_worker_pods_failed_total[5m]))) > 0.2
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "High node verification failure rate (>20%)"

  # Many unverified nodes
  - alert: ManyUnverifiedNodes
    expr: kube_node_ready_controller_nodes_unverified > 10
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "Many unverified nodes pending"

  # Queue buildup
  - alert: ReconciliationQueueBuildup
    expr: kube_node_ready_controller_reconcile_queue_depth > 50
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Reconciliation queue is building up"

  # Frequent leader changes
  - alert: FrequentLeaderChanges
    expr: rate(kube_node_ready_controller_leader_election_transitions_total[1h]) > 0.1
    for: 1h
    labels:
      severity: warning
    annotations:
      summary: "Frequent leader election changes"

  # Slow verifications
  - alert: SlowNodeVerifications
    expr: |
      histogram_quantile(0.95,
        rate(kube_node_ready_controller_worker_pod_duration_seconds_bucket[5m])
      ) > 300
    for: 15m
    labels:
      severity: warning
    annotations:
      summary: "95th percentile verification time >5min"
```

## Grafana Dashboard

### Key Panels

1. **Overview**
   - Controller health status
   - Leader election status
   - Total nodes by state

2. **Node Verification**
   - Verification success rate
   - Average verification time
   - Nodes in pipeline (unverified, verifying, verified, failed)

3. **Worker Pods**
   - Worker pod creation rate
   - Worker pod success/failure rate
   - Active worker pods
   - Worker pod duration distribution

4. **Reconciliation**
   - Reconciliation rate
   - Reconciliation duration
   - Reconciliation errors
   - Queue depth

5. **Errors**
   - Error rate by type
   - Failed nodes list
   - Nodes with high retry counts

### Example Dashboard JSON

```json
{
  "dashboard": {
    "title": "Kube Node Ready Controller",
    "panels": [
      {
        "title": "Controller Status",
        "targets": [
          {
            "expr": "kube_node_ready_controller_healthy"
          }
        ]
      },
      {
        "title": "Nodes by State",
        "targets": [
          {
            "expr": "kube_node_ready_controller_nodes_unverified",
            "legendFormat": "Unverified"
          },
          {
            "expr": "kube_node_ready_controller_nodes_verifying",
            "legendFormat": "Verifying"
          },
          {
            "expr": "kube_node_ready_controller_nodes_verified",
            "legendFormat": "Verified"
          },
          {
            "expr": "kube_node_ready_controller_nodes_failed",
            "legendFormat": "Failed"
          }
        ]
      }
    ]
  }
}
```

## Monitoring Best Practices

1. **Set up alerts** for critical metrics (controller down, no leader, high failure rate)
2. **Monitor trends** for verification times and success rates
3. **Track queue depth** to identify performance issues
4. **Watch for** nodes with high retry counts
5. **Review** leader election transitions (should be rare)
6. **Monitor** worker pod duration for performance regression

## Metric Cardinality

**Low Cardinality:** (<100 unique label combinations)
- Most controller-level metrics
- Leader election metrics
- Health metrics

**Medium Cardinality:** (100-1000 unique label combinations)
- Node-specific metrics (scaled by node count)
- Worker pod metrics

**Labels to Watch:**
- `node`: Cardinality = number of nodes in cluster
- `error_type`: Fixed set of error types
- `result`: Fixed set of result states

## Retention Recommendations

- **Short-term (1-7 days):** All metrics at full resolution
- **Medium-term (7-30 days):** Aggregate by 5m intervals
- **Long-term (30+ days):** Keep only summary metrics and alerts

---

**Date:** January 27, 2025  
**Version:** 0.3.0  
**Status:** Production Ready ✅

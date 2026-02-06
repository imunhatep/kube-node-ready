# kube-node-ready Helm Chart

Helm chart for deploying kube-node-ready, a Kubernetes node network verification system using the Controller + Worker architecture.

## Architecture

kube-node-ready uses a **Controller + Worker Job** pattern:

- **Controller**: Manages node lifecycle, creates worker jobs, monitors verification status, and updates node metadata (removes taints, adds labels)
- **Worker Jobs**: Short-lived pods that run verification checks (DNS, network, Kubernetes API) on each node and exit with success/failure code
- **Separation of Concerns**: Workers only verify; Controller manages node state based on verification results

### Security & RBAC

The system follows the **principle of least privilege**:

- **Worker Pods**: Minimal permissions (only `get` access to `services` and `endpoints` for verification checks)
- **Controller**: Full node management permissions (create jobs, update nodes, optional Karpenter integration)
- No privileged containers or host access required

### Karpenter Integration

The controller automatically detects and uses Karpenter NodeClaims when available:
- Failed nodes: Deletes NodeClaim instead of node directly (graceful termination by Karpenter)
- Standard nodes: Falls back to direct node deletion when NodeClaim not found
- No additional configuration needed - detection is automatic

## Installation

### Install kube-node-ready
```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace
```

### Install with Custom Values
```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --values custom-values.yaml
```

## Monitoring

### Prometheus Metrics

The controller exposes Prometheus metrics on port `8080` at `/metrics` endpoint.

**25+ metrics available:**
- Reconciliation metrics (rate, duration, errors)
- Worker pod metrics (created, succeeded, failed, duration)
- Node state metrics (unverified, verifying, verified, failed)
- Leader election metrics
- Queue and performance metrics

See [PROMETHEUS_METRICS.md](../../../docs/PROMETHEUS_METRICS.md) for complete metric reference.

### ServiceMonitor (Prometheus Operator)

Enable automatic metrics discovery with Prometheus Operator:

```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set controller.metrics.serviceMonitor.enabled=true \
  --set controller.metrics.serviceMonitor.labels.prometheus=kube-prometheus
```

**Configuration:**
```yaml
controller:
  metrics:
    serviceMonitor:
      enabled: true
      namespace: monitoring  # Optional: deploy to monitoring namespace
      interval: 30s
      scrapeTimeout: 10s
      labels:
        prometheus: kube-prometheus  # Match Prometheus selector
```

## Configuration

### Common Configuration

| Parameter | Description | Default           |
|-----------|-------------|-------------------|
| `image.repository` | Container image repository | `kube-node-ready` |
| `image.tag` | Container image tag | `latest`          |
| `image.pullPolicy` | Image pull policy | `IfNotPresent`    |
| `imagePullSecrets` | Image pull secrets | `[]`              |

### Controller Configuration (Optional)

| Parameter | Description | Default                             |
|-----------|-------------|-------------------------------------|
| `controller.replicas` | Number of controller replicas | `1`                                 |
| `controller.image.repository` | Controller image repository | `ghcr.io/imunhatep/kube-node-ready` |
| `controller.image.tag` | Controller image tag | `latest`                            |
| `controller.image.pullPolicy` | Image pull policy | `IfNotPresent`                      |

#### Controller Configuration File

The controller reads configuration from a ConfigMap mounted as `/etc/kube-node-ready/config.yaml`:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.config.workerImage` | Image for worker pods | `ghcr.io/imunhatep/kube-node-ready:0.2.6` |
| `controller.config.workerNamespace` | Namespace for worker pods (auto-detected if not set) | Auto-detected from controller |
| `controller.config.workerTimeout` | Worker pod timeout | `"300s"` |
| `controller.config.maxRetries` | Retry attempts for failed nodes | `5` |
| `controller.config.retryBackoff` | Backoff strategy (exponential/linear) | `"exponential"` |
| `controller.config.reconcileInterval` | Reconciliation loop interval | `"30s"` |
| `controller.config.deleteFailedNodes` | Delete nodes that fail verification (⚠️ DANGEROUS) | `false` |
| `controller.config.taintKey` | Taint key for unverified nodes | `"node-ready/unverified"` |
| `controller.config.taintValue` | Taint value | `"true"` |
| `controller.config.taintEffect` | Taint effect | `"NoSchedule"` |
| `controller.config.verifiedLabel` | Label for verified nodes | `"node-ready/verified"` |
| `controller.config.verifiedLabelValue` | Label value | `"true"` |
| `controller.config.metricsEnabled` | Enable metrics endpoint | `true` |
| `controller.config.metricsPort` | Metrics port | `8080` |
| `controller.config.healthPort` | Health check port | `8081` |
| `controller.config.logLevel` | Log level (debug/info/warn/error) | `"info"` |
| `controller.config.logFormat` | Log format (json/console) | `"json"` |
| `controller.config.kubeconfigPath` | Kubeconfig path (empty for in-cluster) | `""` |

#### Worker Configuration

Worker pods can be configured with tolerations and custom init containers:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.config.worker.tolerations` | Additional tolerations for worker pods (beyond managed taints) | `[]` |
| `controller.config.worker.initContainers` | Custom init containers for extended checks | `[]` |

**Worker Tolerations**: By default, workers tolerate the verification taints and standard Kubernetes taints (`not-ready`, `unreachable`). Additional tolerations can be configured for custom node taints that should not be removed on verification success.

**Custom Init Containers**: Extend verification checks by adding custom init containers that run before the main worker. If any init container fails, the entire verification fails. See examples below.

#### Controller Resources and Scheduling

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.resources.requests.cpu` | CPU request | `50m` |
| `controller.resources.requests.memory` | Memory request | `64Mi` |
| `controller.resources.limits.cpu` | CPU limit (optional) | `200m` |
| `controller.resources.limits.memory` | Memory limit (optional) | `128Mi` |
| `controller.serviceAccount.create` | Create service account | `true` |
| `controller.serviceAccount.annotations` | Service account annotations | `{}` |
| `controller.serviceAccount.name` | Service account name | `""` (auto-generated) |
| `controller.podAnnotations` | Pod annotations | `{}` |
| `controller.nodeSelector` | Node selector | `{}` |
| `controller.tolerations` | Pod tolerations | `[]` |
| `controller.affinity` | Pod affinity rules | `{}` |

#### Controller Service

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.service.type` | Service type | `ClusterIP` |
| `controller.service.metricsPort` | Metrics port | `8080` |
| `controller.service.healthPort` | Health check port | `8081` |

### Service Account

| Parameter | Description | Default |
|-----------|-------------|---------|
| `serviceAccount.create` | Create service account | `true` |
| `serviceAccount.annotations` | Service account annotations | `{}` |
| `serviceAccount.name` | Service account name | `""` (auto-generated) |

### Verification Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.initialTimeout` | Maximum time for verification | `"300s"` |
| `config.checkTimeout` | Timeout for individual checks (DNS, network, K8s API) | `"10s"` |
| `config.maxRetries` | Number of retry attempts | `5` |
| `config.retryBackoff` | Backoff strategy (exponential/linear) | `"exponential"` |
| `config.dnsTestDomains` | DNS domains to test | `[kubernetes.default.svc.cluster.local, google.com]` |
| `config.taintKey` | Taint key to remove | `"node-ready/unverified"` |
| `config.taintValue` | Taint value | `"true"` |
| `config.taintEffect` | Taint effect | `"NoSchedule"` |
| `config.verifiedLabel` | Label to add on success | `"node-ready/verified"` |
| `config.verifiedLabelValue` | Label value | `"true"` |

### Logging and Metrics

| Parameter | Description | Default |
|-----------|-------------|---------|
| `config.logLevel` | Log level (debug/info/warn/error) | `"info"` |
| `config.logFormat` | Log format (json/console) | `"json"` |
| `config.enableMetrics` | Enable Prometheus metrics | `true` |
| `config.metricsPort` | Metrics port | `8080` |
| `metrics.enabled` | Enable metrics endpoint | `true` |
| `metrics.port` | Metrics port | `8080` |

### Resources

| Parameter | Description | Default |
|-----------|-------------|---------|
| `resources.requests.cpu` | CPU request | `50m` |
| `resources.requests.memory` | Memory request | `64Mi` |
| `resources.limits.cpu` | CPU limit (optional, omit in loaded clusters) | `100m` |
| `resources.limits.memory` | Memory limit (optional) | `128Mi` |

**Note**: Resource limits, especially CPU limits, are optional. In loaded clusters, it's recommended to omit CPU limits to avoid throttling during node verification checks. Memory limits can be kept to prevent OOM issues.

### Security

| Parameter | Description | Default |
|-----------|-------------|---------|
| `podSecurityContext.runAsNonRoot` | Run as non-root | `true` |
| `podSecurityContext.runAsUser` | User ID | `1000` |
| `podSecurityContext.fsGroup` | Filesystem group | `1000` |
| `securityContext.allowPrivilegeEscalation` | Allow privilege escalation | `false` |
| `securityContext.readOnlyRootFilesystem` | Read-only filesystem | `true` |
| `securityContext.capabilities.drop` | Dropped capabilities | `[ALL]` |

### Scheduling

| Parameter | Description | Default |
|-----------|-------------|---------|
| `nodeAffinity` | Node affinity rules | Exclude verified nodes |
| `tolerations` | Pod tolerations | Allow unverified nodes |
| `nodeSelector` | Node selector | `{}` |
| `priorityClassName` | Priority class name | `"system-node-critical"` |

## Examples

### Basic Installation

Install with default controller configuration:

```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system
```

### Controller Configuration

Configure the controller component:

```yaml
controller:
  config:
    worker:
      image:
        repository: "ghcr.io/imunhatep/kube-node-ready"
        tag: "latest"
      timeoutSeconds: 300
    reconciliation:
      maxRetries: 5
      intervalSeconds: 30
    logging:
      level: "info"
```

### Custom DNS Tests
```yaml
worker:
  config:
    dnsTestDomains:
      - kubernetes.default.svc.cluster.local
      - cloudflare.com
      - 1.1.1.1
```

### Increased Retries
```yaml
controller:
  config:
    reconciliation:
      maxRetries: 10
    worker:
      timeoutSeconds: 600
```

### Custom Taint and Labels
```yaml
controller:
  config:
    nodeManagement:
      taints:
        - key: "custom/unverified"
          value: "pending"
          effect: "NoSchedule"
      verifiedLabel:
        key: "custom/verified"
        value: "ready"
```

### Development Logging
```yaml
controller:
  config:
    logging:
      level: "debug"
      format: "console"
```

### Worker Tolerations

Configure workers to tolerate additional node taints (e.g., for nodes with custom taints that should not be removed):

```yaml
controller:
  config:
    worker:
      tolerations:
        - key: "custom-taint"
          operator: "Exists"
          effect: "NoSchedule"
        - key: "gpu"
          operator: "Equal"
          value: "true"
          effect: "NoSchedule"
```

**Note**: To tolerate all taints (like DaemonSets), use:
```yaml
controller:
  config:
    worker:
      tolerations:
        - operator: "Exists"
```

### Custom Init Containers

Extend verification checks with custom init containers:

```yaml
controller:
  config:
    worker:
      initContainers:
        - name: "check-gpu"
          image: "nvidia/cuda:11.8.0-base-ubuntu22.04"
          command: ["nvidia-smi"]
        - name: "check-storage"
          image: "busybox:1.36"
          command: ["sh", "-c"]
          args:
            - |
              df -h /host-root
              [ $(df -P /host-root | tail -1 | awk '{print $5}' | sed 's/%//') -lt 90 ]
          volumeMounts:
            - name: host-root
              mountPath: /host-root
              readOnly: true
```

Common use cases:
- **Hardware checks**: Verify GPU availability, disk space, network interfaces
- **Custom network tests**: Check connectivity to internal services or databases
- **Configuration validation**: Verify required files or settings exist on the node
- **Security scans**: Run custom security checks before node activation

### Resource Limits
```yaml
controller:
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      memory: 256Mi
```

## Upgrade

```bash
helm upgrade kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --values custom-values.yaml
```

## Uninstall

```bash
helm uninstall kube-node-ready --namespace kube-system
```

**Note**: Nodes will retain the `verified` label after uninstallation. To clean up:

```bash
# Remove labels from all nodes
kubectl label nodes --all node-ready/verified-
```

## Troubleshooting

### View Controller
```bash
kubectl get deployment -n kube-system -l app.kubernetes.io/component=controller
kubectl get pods -n kube-system -l app.kubernetes.io/component=controller
```

### View Worker Pods
```bash
kubectl get pods -n kube-system -l app.kubernetes.io/component=worker
```

### Check Logs
```bash
# Controller logs
kubectl logs -n kube-system -l app.kubernetes.io/component=controller --tail=100

# Worker logs
kubectl logs -n kube-system -l app.kubernetes.io/component=worker --tail=100
```

### Verify RBAC
```bash
# Controller permissions: nodes, jobs, pods, leases, nodeclaims (Karpenter)
kubectl get clusterrole kube-node-ready-controller -o yaml

# Worker permissions: services, endpoints (read-only)
kubectl get clusterrole kube-node-ready-worker -o yaml

# Verify bindings
kubectl get clusterrolebinding kube-node-ready-controller -o yaml
kubectl get clusterrolebinding kube-node-ready-worker -o yaml
```

**RBAC Security**:
- Workers have minimal permissions (only `get` on services/endpoints)
- Workers cannot modify nodes (principle of least privilege)
- Controller has full node management capabilities
- Karpenter integration requires nodeclaims permissions on controller

### Test on Specific Node
```bash
# Remove verified label to force re-verification
kubectl label node <node-name> node-ready/verified-

# Watch controller create worker pod
kubectl get pods -n kube-system -l app.kubernetes.io/component=worker -w
```

## Values File Reference

See [values.yaml](values.yaml) for the complete list of configurable parameters.

# kube-node-ready Helm Chart

Helm chart for deploying kube-node-ready, a Kubernetes node network verification system.

## Deployment Modes

This chart supports **two deployment modes**:

| Mode | Description | Use Case |
|------|-------------|----------|
| **`daemonset`** (default) | DaemonSet runs continuously on each unverified node | Static clusters, continuous monitoring |
| **`controller`** | Controller creates worker pods on-demand | Dynamic clusters with autoscaling (Karpenter, etc.) |

**Quick Mode Selection:**
```bash
# DaemonSet mode (default)
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system

# Controller mode
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set deploymentMode=controller
```

See [Deployment Modes Guide](../../../docs/HELM_DEPLOYMENT_MODES.md) for detailed comparison.

## Installation

### Install DaemonSet Mode (Default)
```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace
```

### Install Controller Mode
```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace \
  --set deploymentMode=controller
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
  --set deploymentMode=controller \
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

See [SERVICEMONITOR.md](../../../docs/SERVICEMONITOR.md) for detailed configuration.

## Configuration

### Deployment Mode Selection

| Parameter | Description | Default |
|-----------|-------------|---------|
| `deploymentMode` | Deployment mode: `daemonset` or `controller` | `daemonset` |

### Common Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Container image repository | `kube-node-ready` |
| `image.tag` | Container image tag | `0.1.0` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `imagePullSecrets` | Image pull secrets | `[]` |

### Controller Configuration (Optional)

Enable with `controller.enabled=true` to deploy the controller component.

| Parameter | Description | Default |
|-----------|-------------|---------|
| `controller.enabled` | Enable controller deployment | `false` |
| `controller.replicas` | Number of controller replicas | `1` |
| `controller.image.repository` | Controller image repository | `ghcr.io/imunhatep/kube-node-ready` |
| `controller.image.tag` | Controller image tag | `0.2.6` |
| `controller.image.pullPolicy` | Image pull policy | `IfNotPresent` |

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

### DaemonSet

| Parameter | Description | Default |
|-----------|-------------|---------|
| `updateStrategy.type` | Update strategy | `RollingUpdate` |
| `updateStrategy.rollingUpdate.maxUnavailable` | Max unavailable | `1` |
| `podAnnotations` | Pod annotations | `{}` |

## Examples

### Deploy with Controller

Enable the controller component for centralized node management:

```yaml
controller:
  enabled: true
  config:
    workerImage: "ghcr.io/imunhatep/kube-node-ready:0.2.6"
    maxRetries: 5
    reconcileInterval: "30s"
    logLevel: "info"
```

Install with controller:

```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set controller.enabled=true
```

### Custom DNS Tests
```yaml
config:
  dnsTestDomains:
    - kubernetes.default.svc.cluster.local
    - cloudflare.com
    - 1.1.1.1
```

### Increased Retries
```yaml
config:
  maxRetries: 10
  initialTimeout: "600s"
```

### Custom Taint
```yaml
config:
  taintKey: "custom/unverified"
  taintValue: "pending"
  verifiedLabel: "custom/verified"
```

### Development Logging
```yaml
config:
  logLevel: "debug"
  logFormat: "console"
```

### Resource Limits
```yaml
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 200m
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

### Check DaemonSet Status
```bash
kubectl get daemonset -n kube-system kube-node-ready
kubectl describe daemonset -n kube-system kube-node-ready
```

### View Pods
```bash
kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-node-ready
```

### Check Logs
```bash
kubectl logs -n kube-system -l app.kubernetes.io/name=kube-node-ready --tail=100
```

### Verify RBAC
```bash
kubectl get clusterrole kube-node-ready -o yaml
kubectl get clusterrolebinding kube-node-ready -o yaml
```

### Test on Specific Node
```bash
# Label a node to force re-verification
kubectl label node <node-name> node-ready/verified-

# Watch pod get scheduled
kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-node-ready -w
```

## Values File Reference

See [values.yaml](values.yaml) for the complete list of configurable parameters.

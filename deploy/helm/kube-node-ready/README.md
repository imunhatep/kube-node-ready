# kube-node-ready Helm Chart

Helm chart for deploying kube-node-ready, a Kubernetes node network verification DaemonSet.

## Installation

### Add Repository (if published)
```bash
helm repo add kube-node-ready https://your-repo-url
helm repo update
```

### Install from Local Chart
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

## Configuration

The following table lists the configurable parameters and their default values.

### Image Configuration

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.repository` | Container image repository | `kube-node-ready` |
| `image.tag` | Container image tag | `0.1.0` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `imagePullSecrets` | Image pull secrets | `[]` |

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
| `resources.limits.cpu` | CPU limit | `100m` |
| `resources.limits.memory` | Memory limit | `128Mi` |

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

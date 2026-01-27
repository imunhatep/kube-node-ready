# Deployment Mode Quick Start

## Choose Your Mode

kube-node-ready supports two deployment modes:

### 🔄 DaemonSet Mode (Default)
**Best for:** Static clusters, continuous monitoring
```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready --namespace kube-system
```

### 🎮 Controller Mode  
**Best for:** Dynamic clusters with autoscaling (Karpenter, Cluster Autoscaler)
```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set deploymentMode=controller
```

## Quick Comparison

| Feature | DaemonSet | Controller |
|---------|-----------|------------|
| **Architecture** | Pod on each node | Controller + on-demand workers |
| **Resource Usage** | Higher (always running) | Lower (pods created as needed) |
| **Complexity** | Simple | More complex |
| **Best For** | Static clusters | Dynamic clusters |
| **Monitoring** | Continuous | On-demand |
| **Ideal Use Case** | Traditional K8s | Karpenter/autoscaling |

## Installation Examples

### DaemonSet Mode
```bash
# Basic installation
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace

# With custom DNS domains
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set config.dnsTestDomains[0]=kubernetes.default.svc.cluster.local \
  --set config.dnsTestDomains[1]=internal.company.com
```

### Controller Mode
```bash
# Basic installation
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace \
  --set deploymentMode=controller

# With custom configuration
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set deploymentMode=controller \
  --set controller.config.reconciliation.maxRetries=10 \
  --set controller.config.worker.checkTimeoutSeconds=30
```

## Switching Modes

```bash
# Switch to controller mode
helm upgrade kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set deploymentMode=controller

# Switch to daemonset mode
helm upgrade kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set deploymentMode=daemonset
```

## Verification

### DaemonSet Mode
```bash
# Check status
kubectl get daemonset kube-node-ready -n kube-system

# View pods
kubectl get pods -l app.kubernetes.io/component=daemonset -n kube-system

# View logs
kubectl logs -l app.kubernetes.io/component=daemonset -n kube-system -f
```

### Controller Mode
```bash
# Check controller
kubectl get deployment kube-node-ready-controller -n kube-system

# View controller logs
kubectl logs -l app.kubernetes.io/component=controller -n kube-system -f

# View worker pods
kubectl get pods -l app.kubernetes.io/component=worker -n kube-system

# Watch for new workers
kubectl get pods -l app.kubernetes.io/component=worker -n kube-system -w
```

## Which Mode Should I Use?

### Use DaemonSet Mode If:
- ✅ Your cluster has mostly static nodes
- ✅ You want continuous node monitoring
- ✅ You prefer simpler architecture
- ✅ Resource usage is not a concern
- ✅ You're using traditional Kubernetes setup

### Use Controller Mode If:
- ✅ You use Karpenter or Cluster Autoscaler
- ✅ Nodes are frequently added/removed
- ✅ You want to minimize resource usage
- ✅ You only need one-time verification per node
- ✅ Your cluster is highly dynamic

## Troubleshooting

### Check Current Mode
```bash
helm get values kube-node-ready -n kube-system | grep deploymentMode
```

### View All Resources
```bash
kubectl get all -l app.kubernetes.io/name=kube-node-ready -A
```

### Debug Installation
```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set deploymentMode=controller \
  --dry-run --debug
```

## Next Steps

- 📖 Read [Deployment Modes Guide](HELM_DEPLOYMENT_MODES.md) for detailed comparison
- 🔧 Check [Configuration Reference](../examples/controller-config.yaml) for all options
- 🏗️ See [Architecture Documentation](CONTROLLER_WORKER_ARCHITECTURE_PLAN.md) for technical details

## Need Help?

After installation, Helm shows mode-specific instructions. You can view them again:
```bash
helm get notes kube-node-ready -n kube-system
```

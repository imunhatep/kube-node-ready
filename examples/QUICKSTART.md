# Quick Start Guide

This guide will help you get kube-node-ready up and running in minutes.

## Prerequisites

- Kubernetes cluster (1.24+)
- Helm 3.0+
- `kubectl` configured to access your cluster
- (Optional) Karpenter for node autoscaling

## Step 1: Install kube-node-ready

### Option A: Using Helm (Recommended)

```bash
# Install the DaemonSet
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace

# Verify installation
kubectl get daemonset -n kube-system kube-node-ready
kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-node-ready
```

### Option B: Using kubectl

```bash
# Generate manifests
helm template kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system > kube-node-ready.yaml

# Apply manifests
kubectl apply -f kube-node-ready.yaml
```

## Step 2: Configure Karpenter (if using)

Apply the taint to new nodes created by Karpenter:

```bash
# Edit your NodePool
kubectl edit nodepool default

# Add the taint:
spec:
  template:
    spec:
      taints:
        - key: node-ready/unverified
          value: "true"
          effect: NoSchedule
```

Or apply the example configuration:

```bash
# Update CLUSTER_NAME first
export CLUSTER_NAME=my-cluster
envsubst < examples/karpenter-nodepool.yaml | kubectl apply -f -
```

## Step 3: Test the Verification

### Manual Test (without Karpenter)

```bash
# Pick an existing node
NODE_NAME=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')

# Add the taint manually
kubectl taint node $NODE_NAME node-ready/unverified=true:NoSchedule

# kube-node-ready pod should schedule automatically
kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-node-ready -w

# Watch verification complete
kubectl logs -n kube-system -l app.kubernetes.io/name=kube-node-ready -f

# After ~30 seconds, taint should be removed and label added
kubectl get node $NODE_NAME -o yaml | grep -A 5 "labels:"
kubectl describe node $NODE_NAME | grep Taints
```

### With Karpenter

```bash
# Scale up a deployment to trigger node creation
kubectl create deployment test-app --image=nginx --replicas=10

# Watch for new nodes
kubectl get nodes -w

# Watch verification pods
kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-node-ready -w

# Check logs
kubectl logs -n kube-system -l app.kubernetes.io/name=kube-node-ready --tail=50
```

## Step 4: Verify It's Working

### Check Node Labels

```bash
# All verified nodes should have the label
kubectl get nodes -L node-ready/verified

# Example output:
# NAME                          STATUS   ROLES    AGE   VERSION   VERIFIED
# ip-10-0-1-100.ec2.internal   Ready    <none>   5m    v1.28.0   true
# ip-10-0-1-101.ec2.internal   Ready    <none>   3m    v1.28.0   true
```

### Check Pod Status

```bash
# Should see no running pods on verified nodes
kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-node-ready -o wide

# If all nodes are verified, you'll see:
# No resources found in kube-system namespace.
```

### Check Metrics (if Prometheus is installed)

```bash
# Port-forward to a verification pod (if any running)
kubectl port-forward -n kube-system <pod-name> 8080:8080

# Query metrics
curl http://localhost:8080/metrics | grep node_check
```

## Step 5: Monitor and Maintain

### View Logs

```bash
# Recent logs
kubectl logs -n kube-system -l app.kubernetes.io/name=kube-node-ready --tail=100

# Follow logs
kubectl logs -n kube-system -l app.kubernetes.io/name=kube-node-ready -f

# Logs from all pods
kubectl logs -n kube-system -l app.kubernetes.io/name=kube-node-ready --all-containers=true
```

### Force Re-verification

```bash
# Remove verified label from a node
kubectl label node <node-name> node-ready/verified-

# Pod will be scheduled automatically
kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-node-ready -w
```

### Check DaemonSet Health

```bash
# DaemonSet status
kubectl get daemonset -n kube-system kube-node-ready

# Detailed info
kubectl describe daemonset -n kube-system kube-node-ready

# Check RBAC
kubectl get clusterrole kube-node-ready -o yaml
kubectl get clusterrolebinding kube-node-ready -o yaml
```

## Common Issues

### Issue: Pod not scheduling on new nodes

**Cause**: Node might already have the verified label

**Solution**:
```bash
# Check node labels
kubectl get nodes -L node-ready/verified

# If needed, remove the label
kubectl label node <node-name> node-ready/verified-
```

### Issue: Verification fails

**Cause**: Actual network issues or misconfiguration

**Solution**:
```bash
# Check logs for specific error
kubectl logs -n kube-system <pod-name>

# Test DNS manually
kubectl exec -n kube-system <pod-name> -- nslookup kubernetes.default.svc.cluster.local

# Check node network
kubectl describe node <node-name>
```

### Issue: Pod doesn't terminate after success

**Cause**: Label wasn't added or nodeAffinity not working

**Solution**:
```bash
# Check if label was added
kubectl get node <node-name> --show-labels | grep verified

# Check DaemonSet affinity
kubectl get daemonset -n kube-system kube-node-ready -o yaml | grep -A 10 affinity

# Manually add label (testing)
kubectl label node <node-name> node-ready/verified=true
```

## Customization

### Change DNS Test Domains

```bash
helm upgrade kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set config.dnsTestDomains="{kubernetes.default.svc.cluster.local,cloudflare.com}"
```

### Increase Retries

```bash
helm upgrade kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set config.maxRetries=10 \
  --set config.initialTimeout=600s
```

### Use Custom Taint

```bash
helm upgrade kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set config.taintKey=my-custom/unverified \
  --set config.verifiedLabel=my-custom/verified
```

## Uninstall

```bash
# Remove the Helm release
helm uninstall kube-node-ready --namespace kube-system

# Clean up labels (optional)
kubectl label nodes --all node-ready/verified-
```

## Next Steps

- Read the [Architecture Documentation](../docs/ARCHITECTURE.md)
- Configure [Prometheus monitoring](../README.md#monitoring)
- Set up alerts for failed verifications
- Integrate with your CI/CD pipeline

## Local Development and Testing

### Running the Controller Locally

For development and testing, you can run the controller locally against your Kubernetes cluster:

```bash
# Build the controller binary
make build-controller

# Run the controller with default config
./examples/run-controller-local.sh

# Or with a custom config file
CONFIG_FILE=/path/to/my-config.yaml ./examples/run-controller-local.sh

# Disable leader election for local testing (already default)
LEADER_ELECT=false ./examples/run-controller-local.sh
```

The script will:
1. Check if the config file exists (default: `./examples/controller-config.yaml`)
2. Verify the binary exists and is executable
3. Check kubectl connectivity
4. Verify RBAC permissions for the controller
5. Start the controller with proper flags

**Configuration**: Edit `examples/controller-config.yaml` to customize:
- Worker image and namespace
- Reconciliation interval and retries
- Node taints and labels
- Metrics and logging settings

### Running the Worker Locally

To test worker functionality independently:

```bash
# Build the worker binary
make build-worker

# Run the worker against a specific node
NODE_NAME=my-node ./examples/run-worker-local.sh
```

### Prerequisites for Local Running

- Go 1.25+
- Access to a Kubernetes cluster
- kubectl configured with admin permissions
- Controller needs: list/watch/patch nodes, create/delete pods
- Worker needs: access to update node labels/taints

## Getting Help

- Check logs: `kubectl logs -n kube-system -l app.kubernetes.io/name=kube-node-ready`
- Review [Troubleshooting Guide](../README.md#troubleshooting)
- Open an issue on GitHub

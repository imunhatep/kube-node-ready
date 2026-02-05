# Quick Start Guide

This guide will help you get kube-node-ready up and running in minutes.

## Prerequisites

- Kubernetes cluster (1.24+)
- Helm 3.0+
- `kubectl` configured to access your cluster
- (Optional) Karpenter for node autoscaling

## Step 1: Install kube-node-ready

### Option A: Controller Mode (Recommended)

```bash
# Install the Controller + Worker architecture
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace \
  --set deploymentMode=controller

# Verify installation
kubectl get deployment -n kube-system kube-node-ready-controller
kubectl get pods -n kube-system -l app.kubernetes.io/component=controller
kubectl get configmap -n kube-system kube-node-ready-controller
```

### Option B: DaemonSet Mode (Legacy)

```bash
# Install the DaemonSet (legacy mode)
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace \
  --set deploymentMode=daemonset

# Verify installation
kubectl get daemonset -n kube-system kube-node-ready
kubectl get pods -n kube-system -l app.kubernetes.io/name=kube-node-ready
```

### Option C: Using kubectl

```bash
# Generate manifests for controller mode
helm template kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set deploymentMode=controller > kube-node-ready-controller.yaml

# Apply manifests
kubectl apply -f kube-node-ready-controller.yaml
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

### With Controller Mode

```bash
# Pick an existing node
NODE_NAME=$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')

# Add the taint manually to trigger verification
kubectl taint node $NODE_NAME node-ready/unverified=true:NoSchedule

# Controller should detect and create a worker pod
kubectl get pods -n kube-system -l app.kubernetes.io/component=worker -w

# Watch controller logs
kubectl logs -n kube-system -l app.kubernetes.io/component=controller -f

# Watch worker pod logs
kubectl logs -n kube-system -l app.kubernetes.io/component=worker -f

# After ~30 seconds, taint should be removed and label added
kubectl get node $NODE_NAME -o yaml | grep -A 5 "labels:"
kubectl describe node $NODE_NAME | grep Taints

# Worker pod should terminate after success
kubectl get pods -n kube-system -l app.kubernetes.io/component=worker
```

### Manual Test (DaemonSet Mode)

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

**Controller Mode:**
```bash
# Controller logs
kubectl logs -n kube-system -l app.kubernetes.io/component=controller --tail=100

# Worker logs (if any active)
kubectl logs -n kube-system -l app.kubernetes.io/component=worker --tail=100

# Follow logs
kubectl logs -n kube-system -l app.kubernetes.io/component=controller -f
```

**DaemonSet Mode:**
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

# Controller mode: Controller will create new worker pod automatically
# DaemonSet mode: Pod will be scheduled automatically
kubectl get pods -n kube-system -w
```

### Check Health

**Controller Mode:**
```bash
# Controller deployment status
kubectl get deployment -n kube-system kube-node-ready-controller

# Controller metrics (if enabled)
kubectl port-forward -n kube-system deployment/kube-node-ready-controller 8080:8080
curl http://localhost:8080/metrics | grep kube_node_ready

# Detailed controller info
kubectl describe deployment -n kube-system kube-node-ready-controller
```

**DaemonSet Mode:**
```bash
# DaemonSet status
kubectl get daemonset -n kube-system kube-node-ready

# Detailed info
kubectl describe daemonset -n kube-system kube-node-ready
```

### Check RBAC
```bash
kubectl get clusterrole kube-node-ready -o yaml
kubectl get clusterrolebinding kube-node-ready -o yaml

# For controller mode, also check:
kubectl get clusterrole kube-node-ready-controller -o yaml
kubectl get clusterrole kube-node-ready-worker -o yaml
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

### Basic Configuration

**Change DNS Test Domains:**
```bash
helm upgrade kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set controller.config.worker.dnsTestDomains="{kubernetes.default.svc.cluster.local,cloudflare.com}"
```

**Increase Retries:**
```bash
helm upgrade kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set controller.config.reconciliation.maxRetries=10 \
  --set controller.config.worker.timeoutSeconds=600
```

**Use Custom Taint:**
```bash
helm upgrade kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set controller.config.nodeManagement.taints[0].key=my-custom/unverified \
  --set controller.config.nodeManagement.verifiedLabel.key=my-custom/verified
```

### Advanced Configuration: Custom Init Containers

Add custom validation logic to worker pods with init containers:

**Quick Start with Simple Init Containers:**
```bash
# Use the simple init container example
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace \
  --values examples/values-simple-init.yaml
```

**Advanced Setup with Comprehensive Validation:**
```bash
# Use the advanced custom configuration  
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace \
  --values examples/values-custom.yaml
```

**Create a values.yaml file:**

For simple network and DNS validation, see `examples/values-simple-init.yaml`.

For comprehensive validation including storage, GPU, and API checks, see `examples/values-custom.yaml`.

You can also create your own custom configuration:

```yaml
# values-custom.yaml
deploymentMode: controller

controller:
  config:
    worker:
      # Custom init containers for additional verification
      initContainers:
        # Network connectivity check
        - name: network-check
          image: busybox:latest
          command: ["/bin/sh", "-c"]
          args: ["ping -c 3 internal-api.company.com"]
          
        # Storage driver validation
        - name: storage-check
          image: busybox:latest
          command: ["/bin/sh", "-c"]
          args: ["test -d /host/var/lib/kubelet && echo 'Storage OK'"]
          volumeMounts:
            - name: host-kubelet
              mountPath: /host/var/lib/kubelet
              readOnly: true
          securityContext:
            privileged: false
            readOnlyRootFilesystem: true
            
        # GPU validation (for GPU nodes)
        - name: gpu-check
          image: nvidia/cuda:11.0-base
          command: ["nvidia-smi"]
          resources:
            limits:
              nvidia.com/gpu: 1
              
        # Custom API validation
        - name: api-health-check
          image: curlimages/curl:latest
          command: ["curl"]
          args: ["-f", "--max-time", "10", "http://internal-health.company.com/status"]
          env:
            - name: HEALTH_ENDPOINT
              value: "http://internal-health.company.com/status"

      # Tolerate all taints (similar to DaemonSet behavior)
      tolerations:
        - operator: "Exists"
          effect: ""

    nodeManagement:
      # Custom verification label
      verifiedLabel:
        key: "company.com/node-verified"
        value: "true"
      # Custom taint for unverified nodes
      taints:
        - key: "company.com/node-unverified" 
          value: "true"
          effect: "NoSchedule"
```

**Apply the custom configuration:**
```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --create-namespace \
  --values values-custom.yaml
```

### Configure Worker Tolerations

**Tolerate all taints (DaemonSet-like behavior):**
```bash
helm upgrade kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set 'controller.config.worker.tolerations[0].operator=Exists' \
  --set 'controller.config.worker.tolerations[0].effect='
```

**Tolerate specific taints:**
```bash
helm upgrade kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system \
  --set 'controller.config.worker.tolerations[0].key=spot-instance' \
  --set 'controller.config.worker.tolerations[0].operator=Exists' \
  --set 'controller.config.worker.tolerations[1].key=workload-type' \
  --set 'controller.config.worker.tolerations[1].operator=Equal' \
  --set 'controller.config.worker.tolerations[1].value=batch' \
  --set 'controller.config.worker.tolerations[1].effect=NoExecute' \
  --set 'controller.config.worker.tolerations[1].tolerationSeconds=300'
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
CONFIG_FILE=./examples/controller-config.yaml ./examples/run-controller-local.sh

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
- Custom init containers for extended validation
- Additional worker pod tolerations
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

- Go 1.21+
- Access to a Kubernetes cluster
- kubectl configured with admin permissions
- Controller needs: list/watch/patch nodes, create/delete jobs/pods
- Worker needs: access to update node labels/taints

### Testing Init Containers

When developing custom init containers, test them independently first:

```bash
# Test a custom init container
kubectl run test-init --rm -it --restart=Never \
  --image=busybox:latest \
  --command -- /bin/sh -c "ping -c 3 google.com"

# Test with node affinity (replace NODE_NAME)
kubectl run test-init --rm -it --restart=Never \
  --image=busybox:latest \
  --overrides='{"spec":{"nodeSelector":{"kubernetes.io/hostname":"NODE_NAME"}}}' \
  --command -- /bin/sh -c "ping -c 3 google.com"
```

## Getting Help

- Check logs: `kubectl logs -n kube-system -l app.kubernetes.io/name=kube-node-ready`
- Review [Troubleshooting Guide](../README.md#troubleshooting)
- Open an issue on GitHub

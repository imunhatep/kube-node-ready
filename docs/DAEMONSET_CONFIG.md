# DaemonSet Configuration Reference

## Key DaemonSet Configuration

This document shows the critical configuration that enables the one-time verification pattern.

## DaemonSet Template (Simplified)

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: kube-node-ready
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app: kube-node-ready
  template:
    metadata:
      labels:
        app: kube-node-ready
    spec:
      # Critical: Exclude verified nodes
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: node-ready/verified
                operator: DoesNotExist

      # Allow scheduling on tainted nodes
      tolerations:
      - key: node-ready/unverified
        operator: Exists
        effect: NoSchedule
      - key: node.kubernetes.io/not-ready
        operator: Exists
        effect: NoSchedule

      # Use service account with node patch permissions
      serviceAccountName: kube-node-ready
      
      # Security context
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      
      containers:
      - name: node-ready-checkee
        image: kube-node-ready:latest
        imagePullPolicy: IfNotPresent
        
        # Security
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
        
        # Resource limits
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 100m
            memory: 128Mi
        
        # Environment variables
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace
        
        # Configuration from ConfigMap
        envFrom:
        - configMapRef:
            name: kube-node-ready-config
        
        # Health probes (during execution only)
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 2
          periodSeconds: 5
          timeoutSeconds: 3
```

## How the NodeAffinity Works

### Step 1: New Node Created
```yaml
# Node has no labels
apiVersion: v1
kind: Node
metadata:
  name: ip-10-0-1-100.ec2.internal
  labels:
    node.kubernetes.io/instance-type: m5.large
spec:
  taints:
  - key: node-ready/unverified
    value: "true"
    effect: NoSchedule
```

**DaemonSet matches:** ✅ (no `node-ready/verified` label)
**Pod scheduled:** ✅

### Step 2: Verification Succeeds
```yaml
# Application patches node
apiVersion: v1
kind: Node
metadata:
  name: ip-10-0-1-100.ec2.internal
  labels:
    node.kubernetes.io/instance-type: m5.large
    node-ready/verified: "true"  # ← Added by verification pod
spec:
  taints: []  # ← Taint removed
```

**DaemonSet matches:** ❌ (has `node-ready/verified` label)
**Pod action:** Terminates (affinity no longer satisfied)

## ConfigMap Configuration

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kube-node-ready-config
  namespace: kube-system
data:
  INITIAL_TIMEOUT: "300s"
  MAX_RETRIES: "5"
  RETRY_BACKOFF: "exponential"
  DNS_TEST_DOMAINS: "kubernetes.default.svc.cluster.local,google.com"
  TAINT_KEY: "node-ready/unverified"
  TAINT_VALUE: "true"
  TAINT_EFFECT: "NoSchedule"
  VERIFIED_LABEL: "node-ready/verified"
  VERIFIED_LABEL_VALUE: "true"
  ENABLE_METRICS: "true"
  METRICS_PORT: "8080"
  LOG_LEVEL: "info"
  LOG_FORMAT: "json"
```

## RBAC Configuration

```yaml
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: kube-node-ready
  namespace: kube-system

---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kube-node-ready
rules:
# Node operations (get info, remove taint, add label)
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "list", "patch"]

# Service discovery
- apiGroups: [""]
  resources: ["services", "endpoints"]
  verbs: ["get", "list"]

---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kube-node-ready
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kube-node-ready
subjects:
- kind: ServiceAccount
  name: kube-node-ready
  namespace: kube-system
```

## Node Patch Operation

The Go application performs this operation on successful verification:

```go
// Pseudo-code for node patching
func UpdateNodeAfterVerification(ctx context.Context, nodeName string) error {
    // Create JSON patch to:
    // 1. Remove taint
    // 2. Add verified label
    
    patch := []byte(`[
        {
            "op": "remove",
            "path": "/spec/taints",
            "value": [
                {
                    "key": "node-ready/unverified",
                    "value": "true",
                    "effect": "NoSchedule"
                }
            ]
        },
        {
            "op": "add",
            "path": "/metadata/labels/node-ready~1verified",
            "value": "true"
        }
    ]`)
    
    _, err := clientset.CoreV1().Nodes().Patch(
        ctx,
        nodeName,
        types.JSONPatchType,
        patch,
        metav1.PatchOptions{},
    )
    
    return err
}
```

## Verification Flow Visualization

```
┌─────────────────────────────────────────────────────────┐
│ Timeline: Node Lifecycle with Verification              │
├─────────────────────────────────────────────────────────┤
│                                                          │
│ T+0s    Karpenter creates node                          │
│         └─ Taint: node-ready/unverified=true           │
│                                                          │
│ T+10s   Kubelet ready                                   │
│         DaemonSet controller sees unverified node       │
│         └─ Schedules kube-node-ready pod                │
│                                                          │
│ T+15s   Pod starts                                      │
│         └─ Tolerates the taint                          │
│                                                          │
│ T+17s   Begin verification checks                       │
│         ├─ DNS check (5s)                               │
│         ├─ API check (10s)                              │
│         ├─ Network check (10s)                          │
│         └─ Service check (5s)                           │
│                                                          │
│ T+47s   All checks PASS ✅                              │
│         └─ Patch node:                                  │
│            ├─ Remove taint                              │
│            └─ Add label: verified=true                  │
│                                                          │
│ T+48s   DaemonSet controller sees label                 │
│         └─ nodeAffinity no longer matches               │
│         └─ Initiates pod termination                    │
│                                                          │
│ T+50s   Pod terminates gracefully                       │
│         └─ Resources freed                              │
│                                                          │
│ T+50s+  Node ready for workloads                        │
│         └─ No verification pod running                  │
│         └─ Zero ongoing resource consumption            │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## Testing the Configuration

### 1. Deploy the DaemonSet
```bash
helm install kube-node-ready ./deploy/helm/kube-node-ready \
  --namespace kube-system
```

### 2. Watch for Verification
```bash
# Watch pods
kubectl get pods -n kube-system -l app=kube-node-ready -w

# Watch node labels
kubectl get nodes --show-labels -w

# Check specific node
kubectl describe node <node-name>
```

### 3. Verify Labels
```bash
# Should see verified=true on ready nodes
kubectl get nodes -L node-ready/verified

# Example output:
# NAME                          STATUS   ROLES    AGE     VERSION   VERIFIED
# ip-10-0-1-100.ec2.internal   Ready    <none>   5m      v1.28.0   true
# ip-10-0-1-101.ec2.internal   Ready    <none>   3m      v1.28.0   true
```

### 4. Force Re-verification
```bash
# Remove the label
kubectl label node <node-name> node-ready/verified-

# Watch pod get scheduled again
kubectl get pods -n kube-system -l app=kube-node-ready -w
```

## Troubleshooting

### Pod Keeps Running (Doesn't Terminate)
**Cause:** Node wasn't labeled properly
```bash
# Check if label exists
kubectl get node <node-name> --show-labels | grep verified

# Manually add label if needed (testing only)
kubectl label node <node-name> node-ready/verified=true
```

### Pod Doesn't Schedule on New Node
**Cause:** Node might already have verified label or wrong tolerations
```bash
# Check node labels
kubectl get node <node-name> --show-labels

# Check pod tolerations
kubectl get ds kube-node-ready -n kube-system -o yaml | grep -A 10 tolerations
```

### Verification Fails
**Cause:** Actual network issues or misconfiguration
```bash
# Check pod logs
kubectl logs -n kube-system -l app=kube-node-ready --tail=100

# Check DNS resolution from pod
kubectl exec -n kube-system <pod-name> -- nslookup kubernetes.default.svc.cluster.local

# Check node events
kubectl describe node <node-name>
```

## Performance Tuning

### Faster Verification (Development)
```yaml
# In ConfigMap
INITIAL_TIMEOUT: "60s"
MAX_RETRIES: "3"
DNS_TEST_DOMAINS: "kubernetes.default.svc.cluster.local"  # Skip external
```

### More Thorough Verification (Production)
```yaml
# In ConfigMap
INITIAL_TIMEOUT: "300s"
MAX_RETRIES: "5"
DNS_TEST_DOMAINS: "kubernetes.default.svc.cluster.local,google.com,1.1.1.1"
ENABLE_CONNECTIVITY_MATRIX: "true"  # Future: test pod-to-pod
```

---

## Summary

The key to this design is the **nodeAffinity** configuration:

```yaml
nodeAffinity:
  requiredDuringSchedulingIgnoredDuringExecution:
    nodeSelectorTerms:
    - matchExpressions:
      - key: node-ready/verified
        operator: DoesNotExist
```

This single configuration:
- ✅ Ensures pods only run on unverified nodes
- ✅ Automatically triggers pod removal when node is labeled
- ✅ Enables zero ongoing resource consumption
- ✅ Makes re-verification as simple as removing a label

Combined with proper RBAC and the application logic, this creates an elegant, efficient node verification system.

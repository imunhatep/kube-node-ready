# Controller-Worker Architecture

## Overview

The `kube-node-ready` system uses a **Controller + Worker** architecture that separates concerns between orchestration and execution:

- **Controller**: A single deployment that watches Node and Job resources via controller-runtime, manages worker pods, handles reconciliation, and exposes centralized metrics
- **Worker**: Short-lived pods that perform actual node verification checks on specific nodes

## Architecture

### High-Level Flow

```
New Node Created (with taint)
    ↓
Controller detects unverified node
    ↓
Controller creates worker pod for node
    ↓
Worker pod runs verification checks
    ↓
Worker reports success/failure via exit code
    ↓
Controller processes result:
  - Success: Remove taint, add verified label
  - Failure: Retry or delete node (if configured)
    ↓
Worker pod terminates and is cleaned up
```

### Component Responsibilities

#### Controller
- **Resource Watching**: Monitor Node and Job resources using controller-runtime (watches for create, update, delete operations)
- **State Management**: Track verification state per node (pending, in-progress, verified, failed)
- **Worker Orchestration**: Create and monitor worker pods (as Jobs) for unverified nodes
- **Result Processing**: Handle worker completion and update nodes accordingly
- **Retry Logic**: Implement backoff and retry for failed verifications
- **Metrics**: Expose centralized metrics for all node operations
- **Node Lifecycle**: Delete failed nodes when configured

#### Worker
- **Verification Execution**: Run DNS, API, network, and service discovery checks
- **Status Reporting**: Report results via pod exit codes and status
- **Node-Specific**: Execute on the specific node being verified
- **Short-Lived**: Terminate after completing verification checks

## Reconciliation Logic

### Node Verification States

- **Unverified**: Node created with verification taint, needs worker pod
- **Pending**: Worker pod created but not yet scheduled or running  
- **In Progress**: Worker pod running verification checks
- **Verified**: Checks passed, taint removed, verified label added
- **Failed**: Checks failed, retry logic or node deletion applies

### State Transitions

```
Unverified → Pending → In Progress → Verified
     ↑                      ↓
     └── Retry ←── Failed ←──┘
              ↓
         Delete Node (optional)
```

### Controller Reconciliation Flow

1. **Watch Node Events**: Controller monitors all node additions and updates
2. **Filter Unverified Nodes**: Focus on nodes that have verification taints but lack verified labels
3. **State Assessment**: Determine current verification state for each node
4. **Worker Management**: 
   - Create worker pods for unverified nodes
   - Monitor existing worker pod progress
   - Clean up completed worker pods
5. **Result Processing**:
   - On success: Remove taint, add verified label, update metrics
   - On failure: Implement retry logic with backoff or delete node if configured
6. **Continuous Reconciliation**: Ensure eventual consistency and handle edge cases

## Worker Pod Configuration

### Worker Pod Scheduling
- **Node Affinity**: Worker pods are scheduled specifically on the target node being verified
- **Tolerations**: Include tolerations for unverified taints and not-ready conditions
- **Service Account**: Uses dedicated worker service account with minimal permissions
- **Resource Limits**: Lightweight pods with configurable resource requests and limits

### Worker Execution
- **Exit Codes**: Workers communicate results via pod exit codes (0 = success, non-zero = failure)
- **Timeout Handling**: Controller monitors worker pod execution time and handles timeouts
- **Cleanup**: Worker pods are automatically cleaned up after completion

## RBAC Requirements

### Controller Permissions
- **Nodes**: Get, list, watch, patch, delete (for node lifecycle management)
- **Pods**: Get, list, watch, create, delete (for worker pod management)  
- **Services/Endpoints**: Get, list (for worker pod service discovery checks)
- **Events**: Create, patch (for debugging and observability)
- **Leases**: Get, create, update (for leader election in HA deployments)

### Worker Permissions
- **Services/Endpoints**: Get, list (minimal permissions for health checks)
- **No node modification permissions**: Workers only report via exit codes

## Benefits of Controller Architecture

### Centralized Control
- Single controller manages all node verifications across the cluster
- Unified state management and decision-making
- Easier debugging and troubleshooting from one place

### Resource Efficiency  
- Controller runs once per cluster rather than per node
- Workers are ephemeral and only consume resources during active verification
- No idle pods consuming resources after verification completes

### Enhanced Reliability
- Automatic retry logic with configurable backoff strategies
- Reconciliation loop ensures eventual consistency
- Better handling of edge cases and transient failures

### Improved Observability
- Centralized metrics endpoint for monitoring all node verifications
- Better visibility into verification states and progress
- Unified logging and debugging experience

### Greater Flexibility
- Easy to implement complex verification workflows
- Support for different retry strategies and failure handling
- Can easily add features like priority-based verification or gradual rollouts

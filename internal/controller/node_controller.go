package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/imunhatep/kube-node-ready/internal/k8s"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/imunhatep/kube-node-ready/internal/config"
	"github.com/imunhatep/kube-node-ready/internal/metrics"
)

// NodeReconciler reconciles Node objects
type NodeReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Config        *config.ControllerConfig
	StateCache    *k8s.NodeStateCache
	WorkerManager *WorkerManager
	NodeManager   *k8s.NodeManager
}

// NewNodeReconciler creates a new NodeReconciler
func NewNodeReconciler(client client.Client, scheme *runtime.Scheme, cfg *config.ControllerConfig, nodeManager *k8s.NodeManager) *NodeReconciler {
	return &NodeReconciler{
		Client:        client,
		Scheme:        scheme,
		Config:        cfg,
		StateCache:    k8s.NewNodeStateCache(),
		WorkerManager: NewWorkerManager(client, cfg),
		NodeManager:   nodeManager,
	}
}

// Reconcile handles node events and manages verification state
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	startTime := time.Now()
	log := klog.FromContext(ctx)

	log.Info("NodeReconciler: new reconcile invoked")

	// Fetch the nodeEntity
	nodeEntity := &corev1.Node{}
	if err := r.Get(ctx, req.NamespacedName, nodeEntity); err != nil {
		if errors.IsNotFound(err) {
			// Node deleted, clean up state
			log.Info("Node deleted, cleaning up state", "nodeEntity", req.Name)
			r.StateCache.Delete(req.Name)
			metrics.SetWorkerPodsActive(req.Name, 0)
			metrics.SetNodeRetryCount(req.Name, 0)
			metrics.RecordReconciliation("deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get nodeEntity", "nodeEntity", req.Name)
		metrics.RecordReconciliationError(req.Name, "node_fetch_error")
		metrics.RecordReconciliation("error")
		return ctrl.Result{}, err
	}

	// Record reconciliation duration at the end
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.RecordReconciliationDuration(nodeEntity.Name, duration)
	}()

	// Check if nodeEntity already has the verified label
	if hasVerifiedLabel(nodeEntity, r.Config.NodeManagement.VerifiedLabel.Key) {
		log.V(4).Info("Node already verified, skipping", "nodeEntity", nodeEntity.Name)
		r.StateCache.Delete(nodeEntity.Name)
		metrics.RecordReconciliation("already_verified")
		return ctrl.Result{}, nil
	}

	// Check if nodeEntity has any of the unverified taints
	if !hasAnyUnverifiedTaint(nodeEntity, r.Config.NodeManagement.Taints) {
		log.V(4).Info("Node does not have unverified taint, skipping", "nodeEntity", nodeEntity.Name)
		metrics.RecordReconciliation("no_taint")
		return ctrl.Result{}, nil
	}

	// Get or create state
	state := r.StateCache.Get(nodeEntity.Name)
	if state == nil {
		log.Info("New unverified nodeEntity detected", "nodeEntity", nodeEntity.Name)
		state = &k8s.NodeState{
			NodeName:  nodeEntity.Name,
			State:     k8s.NodeStateUnverified,
			CreatedAt: time.Now(),
		}
		r.StateCache.Set(nodeEntity.Name, state)
	}

	log.Info("Reconciling nodeEntity",
		"nodeEntity", nodeEntity.Name,
		"state", state.State,
		"attempts", state.AttemptCount,
	)

	// Handle based on current state
	switch state.State {
	case k8s.NodeStateUnverified:
		return r.handleUnverified(ctx, nodeEntity, state)
	case k8s.NodeStatePending, k8s.NodeStateInProgress:
		return r.handleInProgress(ctx, nodeEntity, state)
	case k8s.NodeStateFailed:
		return r.handleFailed(ctx, nodeEntity, state)
	case k8s.NodeStateVerified:
		return r.handleVerified(ctx, nodeEntity, state)
	}

	// No action needed, wait for events
	return ctrl.Result{}, nil
}

// handleUnverified creates a worker job for an unverified node
func (r *NodeReconciler) handleUnverified(ctx context.Context, node *corev1.Node, state *k8s.NodeState) (ctrl.Result, error) {
	klog.InfoS("Handling unverified node", "node", node.Name)

	// Create worker job
	job, err := r.WorkerManager.CreateWorkerJob(ctx, node.Name)
	if err != nil {
		klog.ErrorS(err, "Failed to create worker job", "node", node.Name)
		metrics.RecordWorkerPodCreationError(node.Name, "create_failed")
		metrics.RecordReconciliationError(node.Name, "worker_job_creation_error")
		metrics.RecordReconciliation("error")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	// Get job UID for tracking
	jobUID := string(job.UID)

	// Update state
	state.State = k8s.NodeStatePending
	state.WorkerJobName = job.Name
	state.JobUID = jobUID
	state.LastAttempt = time.Now()
	state.AttemptCount++
	r.StateCache.Set(node.Name, state)

	// Record metrics
	metrics.RecordWorkerPodCreated(node.Name)
	metrics.SetWorkerPodsActive(node.Name, 1)
	metrics.SetNodeRetryCount(node.Name, state.AttemptCount)
	metrics.RecordReconciliation("worker_created")

	klog.InfoS("Worker job created, transitioning to pending",
		"node", node.Name,
		"job", job.Name,
		"jobUID", jobUID,
		"attempt", state.AttemptCount,
	)

	// Wait for job events (no requeue needed)
	return ctrl.Result{}, nil
}

// handleInProgress monitors the worker job and processes results
func (r *NodeReconciler) handleInProgress(ctx context.Context, node *corev1.Node, state *k8s.NodeState) (ctrl.Result, error) {
	klog.InfoS("Handling in-progress node", "node", node.Name, "job", state.WorkerJobName)

	// Get worker job status
	status, err := r.WorkerManager.GetWorkerJobStatus(ctx, state.WorkerJobName)
	if err != nil {
		klog.ErrorS(err, "Failed to get worker job status", "node", node.Name, "job", state.WorkerJobName)
		// Job might have been deleted externally, reset to unverified
		state.State = k8s.NodeStateUnverified
		state.WorkerJobName = ""
		state.JobUID = ""
		r.StateCache.Set(node.Name, state)
		// Requeue to create new job
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Update state if job is now running
	if state.State == k8s.NodeStatePending && status.Active > 0 {
		klog.InfoS("Worker job is running", "node", node.Name, "job", state.WorkerJobName)
		state.State = k8s.NodeStateInProgress
		r.StateCache.Set(node.Name, state)
		metrics.RecordReconciliation("worker_running")
	}

	// Check if job has completed (either successfully or failed)
	if !status.Completed {
		// Job still running, wait for job completion event
		return ctrl.Result{}, nil
	}

	// Calculate worker job duration
	var duration float64
	if status.StartTime != nil && status.FinishTime != nil {
		duration = status.FinishTime.Sub(*status.StartTime).Seconds()
	} else {
		duration = time.Since(state.LastAttempt).Seconds()
	}

	// Process completion
	klog.InfoS("Worker job completed",
		"node", node.Name,
		"job", state.WorkerJobName,
		"succeeded", status.Succeeded,
		"failed", status.Failed,
		"exitCode", status.ExitCode,
		"duration", duration,
	)

	// Clean up worker job (jobs can auto-cleanup with TTL, but we clean up manually for control)
	//_ = r.WorkerManager.DeleteWorkerJob(ctx, state.WorkerJobName)
	metrics.SetWorkerPodsActive(node.Name, 0)

	if status.Succeeded > 0 && (status.ExitCode == nil || *status.ExitCode == 0) {
		// Success - mark as verified
		klog.InfoS("Verification successful, marking node as verified", "node", node.Name)

		// Use NodeManager to remove taints and add labels
		taintsToRemove := []corev1.Taint{}
		for _, taint := range r.Config.NodeManagement.Taints {
			taintsToRemove = append(taintsToRemove, corev1.Taint{
				Key:    taint.Key,
				Value:  taint.Value,
				Effect: corev1.TaintEffect(taint.Effect),
			})
		}

		labelsToAdd := map[string]string{
			r.Config.NodeManagement.VerifiedLabel.Key: r.Config.NodeManagement.VerifiedLabel.Value,
		}

		if err := r.NodeManager.UpdateNodeMetadata(ctx, node, taintsToRemove, labelsToAdd); err != nil {
			klog.ErrorS(err, "Failed to update node after successful verification", "node", node.Name)
			metrics.RecordReconciliationError(node.Name, "node_update_error")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}

		state.State = k8s.NodeStateVerified
		now := time.Now()
		state.VerifiedAt = &now
		r.StateCache.Set(node.Name, state)

		// Record metrics
		metrics.RecordWorkerPodSucceeded(node.Name)
		metrics.RecordWorkerPodDuration(node.Name, "success", duration)
		metrics.RecordReconciliation("worker_succeeded")

		// Node is now verified, no further action needed
		return ctrl.Result{}, nil
	} else {
		// Failed
		errMsg := status.Message
		if status.ExitCode != nil {
			errMsg = fmt.Sprintf("exit code %d: %s", *status.ExitCode, status.Message)
		}
		if errMsg == "" {
			errMsg = status.Reason
		}

		// Record metrics
		metrics.RecordWorkerPodFailed(node.Name, "check_failed")
		metrics.RecordWorkerPodDuration(node.Name, "failure", duration)
		metrics.RecordReconciliation("worker_failed")

		klog.InfoS("Verification failed", "node", node.Name, "error", errMsg)

		state.State = k8s.NodeStateFailed
		state.LastError = errMsg
		r.StateCache.Set(node.Name, state)

		// Trigger immediate reconcile to handle retry logic
		return ctrl.Result{Requeue: true}, nil
	}
}

// handleFailed handles nodes that failed verification
func (r *NodeReconciler) handleFailed(ctx context.Context, node *corev1.Node, state *k8s.NodeState) (ctrl.Result, error) {
	klog.InfoS("Handling failed node", "node", node.Name, "attempts", state.AttemptCount)

	// Check if we should retry
	if state.AttemptCount < r.Config.Reconciliation.MaxRetries {
		// Calculate backoff
		backoff := r.calculateBackoff(state.AttemptCount)
		timeSinceLastAttempt := time.Since(state.LastAttempt)

		if timeSinceLastAttempt < backoff {
			// Still in backoff period
			remainingBackoff := backoff - timeSinceLastAttempt
			klog.InfoS("In backoff period",
				"node", node.Name,
				"remaining", remainingBackoff,
				"attempt", state.AttemptCount,
			)
			metrics.RecordReconciliation("backoff")
			return ctrl.Result{RequeueAfter: remainingBackoff}, nil
		}

		// Retry
		klog.InfoS("Retrying verification", "node", node.Name, "attempt", state.AttemptCount+1)
		state.State = k8s.NodeStateUnverified
		state.WorkerJobName = ""
		state.JobUID = ""
		r.StateCache.Set(node.Name, state)

		metrics.RecordReconciliation("retry")
		return ctrl.Result{}, nil
	}

	// Max retries exceeded
	klog.InfoS("Max retries exceeded", "node", node.Name, "attempts", state.AttemptCount)
	metrics.RecordReconciliationError(node.Name, "max_retries_exceeded")
	metrics.RecordReconciliation("max_retries_exceeded")

	if r.Config.NodeManagement.DeleteFailedNodes {
		klog.InfoS("Deleting failed node", "node", node.Name)

		if err := r.NodeManager.DeleteNode(ctx, node); err != nil {
			klog.ErrorS(err, "Failed to delete node", "node", node.Name)
			metrics.RecordReconciliationError(node.Name, "node_deletion_error")
			metrics.RecordReconciliation("error")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}

		state.State = k8s.NodeStateDeleting
		r.StateCache.Set(node.Name, state)

		klog.InfoS("Node deleted successfully", "node", node.Name)
		metrics.RecordReconciliation("node_deleted")
	} else {
		metrics.RecordReconciliation("verification_failed_permanent")
	}

	// Don't requeue, we're done
	return ctrl.Result{}, nil
}

// handleVerified processes verified nodes (should not normally be called due to predicate filtering)
func (r *NodeReconciler) handleVerified(_ context.Context, node *corev1.Node, _ *k8s.NodeState) (ctrl.Result, error) {
	klog.InfoS("Node is verified", "node", node.Name)

	metrics.RecordReconciliation("already_verified")
	// Nothing to do
	return ctrl.Result{}, nil
}

// calculateBackoff calculates the backoff duration based on attempt count
func (r *NodeReconciler) calculateBackoff(attempt int) time.Duration {
	if r.Config.Reconciliation.RetryBackoff == "exponential" {
		// Exponential backoff: 1s, 2s, 4s, 8s, 16s, capped at 30s
		backoff := time.Duration(1<<uint(attempt-1)) * time.Second
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
		return backoff
	}

	// Linear backoff: 5s each time
	return 5 * time.Second
}

// SetupWithManager sets up the controller with the NodeManager
func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager, log klog.Logger) error {
	// Create predicates for node events
	nodePredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			n, ok := e.Object.(*corev1.Node)
			if !ok {
				return false
			}

			// Watch new nodes that have unverified taints and no verified label
			hasVerified := hasVerifiedLabel(n, r.Config.NodeManagement.VerifiedLabel.Key)
			hasTaint := hasAnyUnverifiedTaint(n, r.Config.NodeManagement.Taints)

			log.V(1).Info("Node created",
				"node", n.Name,
				"hasVerified", hasVerified,
				"hasTaint", hasTaint,
				"reconcile", !hasVerified && hasTaint,
			)

			return !hasVerified && hasTaint
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode, ok := e.ObjectOld.(*corev1.Node)
			if !ok {
				return false
			}
			newNode, ok := e.ObjectNew.(*corev1.Node)
			if !ok {
				return false
			}

			// Check if label or taint status changed
			oldHasVerified := hasVerifiedLabel(oldNode, r.Config.NodeManagement.VerifiedLabel.Key)
			newHasVerified := hasVerifiedLabel(newNode, r.Config.NodeManagement.VerifiedLabel.Key)
			oldHasTaint := hasAnyUnverifiedTaint(oldNode, r.Config.NodeManagement.Taints)
			newHasTaint := hasAnyUnverifiedTaint(newNode, r.Config.NodeManagement.Taints)

			// Reconcile if:
			// 1. Verified label was removed (oldHasVerified && !newHasVerified)
			// 2. Unverified taint was added (!oldHasTaint && newHasTaint)
			// 3. Node now needs verification (!newHasVerified && newHasTaint)
			labelChanged := oldHasVerified != newHasVerified
			taintChanged := oldHasTaint != newHasTaint
			needsVerification := !newHasVerified && newHasTaint

			shouldReconcile := (labelChanged || taintChanged) && needsVerification

			if shouldReconcile {
				log.V(1).Info("Node updated - requires reconciliation",
					"node", newNode.Name,
					"oldHasVerified", oldHasVerified,
					"newHasVerified", newHasVerified,
					"oldHasTaint", oldHasTaint,
					"newHasTaint", newHasTaint,
				)
			}

			return shouldReconcile
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			// Always handle delete to clean up state
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			// Ignore generic events
			return false
		},
	}

	// Create predicates for job events - only watch for completion
	// NOTE: For optimal performance, configure the manager's cache to watch only
	// the worker namespace. This predicate provides additional filtering by label.
	jobPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// Don't reconcile on job creation
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			job, ok := e.ObjectNew.(*batchv1.Job)
			if !ok {
				return false
			}

			// Filter by namespace (defensive check even if cache is namespace-scoped)
			if job.Namespace != r.Config.GetNamespace() {
				return false
			}

			// Filter by label (only process our worker jobs)
			if _, exists := job.Labels["node-ready.io/node"]; !exists {
				return false
			}

			// Only reconcile when job completes (succeeded or failed)
			completed := job.Status.Succeeded > 0 || job.Status.Failed > 0

			if completed {
				log.V(1).Info("Worker job completed",
					"job", job.Name,
					"namespace", job.Namespace,
					"succeeded", job.Status.Succeeded,
					"failed", job.Status.Failed,
				)
			}

			return completed
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			job, ok := e.Object.(*batchv1.Job)
			if !ok {
				return false
			}

			// Filter by namespace
			if job.Namespace != r.Config.GetNamespace() {
				return false
			}

			// Filter by label (only care about our worker jobs)
			if _, exists := job.Labels["node-ready.io/node"]; !exists {
				return false
			}

			log.V(1).Info("Worker job deleted",
				"job", job.Name,
				"namespace", job.Namespace,
			)

			// Reconcile if our worker job was deleted (might need to recreate)
			return true
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		WithEventFilter(nodePredicate).
		Watches(
			&batchv1.Job{},
			handler.EnqueueRequestsFromMapFunc(r.jobToNodeMapper),
			builder.WithPredicates(jobPredicate),
		).
		Complete(r)
}

// jobToNodeMapper maps Job events to Node reconciliation requests
func (r *NodeReconciler) jobToNodeMapper(_ context.Context, obj client.Object) []reconcile.Request {
	job, ok := obj.(*batchv1.Job)
	if !ok {
		return nil
	}

	// Extract node name from job labels
	nodeName, exists := job.Labels["node-ready.io/node"]
	if !exists {
		return nil
	}

	klog.V(2).InfoS("Mapping job to node reconciliation",
		"job", job.Name,
		"node", nodeName,
	)

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Name: nodeName,
			},
		},
	}
}

// hasVerifiedLabel checks if a node has the verified label
func hasVerifiedLabel(node *corev1.Node, labelKey string) bool {
	if node.Labels == nil {
		return false
	}
	_, exists := node.Labels[labelKey]
	return exists
}

// hasAnyUnverifiedTaint checks if a node has any of the configured unverified taints
func hasAnyUnverifiedTaint(node *corev1.Node, taints []config.TaintConfig) bool {
	if len(taints) == 0 {
		return true
	}

	for _, configTaint := range taints {
		for _, nodeTaint := range node.Spec.Taints {
			if nodeTaint.Key == configTaint.Key {
				return true
			}
		}
	}
	return false
}

package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/imunhatep/kube-node-ready/internal/config"
	"github.com/imunhatep/kube-node-ready/internal/metrics"
)

// NodeReconciler reconciles Node objects
type NodeReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Config        *config.ControllerConfig
	StateCache    *StateCache
	WorkerManager *WorkerManager
}

// NewNodeReconciler creates a new NodeReconciler
func NewNodeReconciler(client client.Client, scheme *runtime.Scheme, cfg *config.ControllerConfig) *NodeReconciler {
	return &NodeReconciler{
		Client:        client,
		Scheme:        scheme,
		Config:        cfg,
		StateCache:    NewStateCache(),
		WorkerManager: NewWorkerManager(client, cfg),
	}
}

// Reconcile handles node events and manages verification state
func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	startTime := time.Now()
	log := klog.FromContext(ctx)

	// Fetch the node
	node := &corev1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		if errors.IsNotFound(err) {
			// Node deleted, clean up state
			log.Info("Node deleted, cleaning up state", "node", req.Name)
			r.StateCache.Delete(req.Name)
			metrics.SetWorkerPodsActive(req.Name, 0)
			metrics.SetNodeRetryCount(req.Name, 0)
			metrics.RecordReconciliation("deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get node", "node", req.Name)
		metrics.RecordReconciliationError(req.Name, "node_fetch_error")
		metrics.RecordReconciliation("error")
		return ctrl.Result{}, err
	}

	// Record reconciliation duration at the end
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.RecordReconciliationDuration(node.Name, duration)
	}()

	// Check if node already has the verified label
	if hasVerifiedLabel(node, r.Config.NodeManagement.VerifiedLabel.Key) {
		log.V(4).Info("Node already verified, skipping", "node", node.Name)
		r.StateCache.Delete(node.Name)
		metrics.RecordReconciliation("already_verified")
		return ctrl.Result{}, nil
	}

	// Check if node has any of the unverified taints
	if !hasAnyUnverifiedTaint(node, r.Config.NodeManagement.Taints) {
		log.V(4).Info("Node does not have unverified taint, skipping", "node", node.Name)
		metrics.RecordReconciliation("no_taint")
		return ctrl.Result{}, nil
	}

	// Get or create state
	state := r.StateCache.Get(node.Name)
	if state == nil {
		log.Info("New unverified node detected", "node", node.Name)
		state = &NodeState{
			NodeName:  node.Name,
			State:     StateUnverified,
			CreatedAt: time.Now(),
		}
		r.StateCache.Set(node.Name, state)
	}

	log.Info("Reconciling node",
		"node", node.Name,
		"state", state.State,
		"attempts", state.AttemptCount,
	)

	// Handle based on current state
	switch state.State {
	case StateUnverified:
		return r.handleUnverified(ctx, node, state)
	case StatePending, StateInProgress:
		return r.handleInProgress(ctx, node, state)
	case StateFailed:
		return r.handleFailed(ctx, node, state)
	case StateVerified:
		return r.handleVerified(ctx, node, state)
	}

	return ctrl.Result{RequeueAfter: r.Config.Reconciliation.GetInterval()}, nil
}

// handleUnverified creates a worker pod for an unverified node
func (r *NodeReconciler) handleUnverified(ctx context.Context, node *corev1.Node, state *NodeState) (ctrl.Result, error) {
	klog.InfoS("Handling unverified node", "node", node.Name)

	// Create worker pod
	pod, err := r.WorkerManager.CreateWorkerPod(ctx, node.Name)
	if err != nil {
		klog.ErrorS(err, "Failed to create worker pod", "node", node.Name)
		metrics.RecordWorkerPodCreationError(node.Name, "create_failed")
		metrics.RecordReconciliationError(node.Name, "worker_pod_creation_error")
		metrics.RecordReconciliation("error")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, err
	}

	// Update state
	state.State = StatePending
	state.WorkerPodName = pod.Name
	state.LastAttempt = time.Now()
	state.AttemptCount++
	r.StateCache.Set(node.Name, state)

	// Record metrics
	metrics.RecordWorkerPodCreated(node.Name)
	metrics.SetWorkerPodsActive(node.Name, 1)
	metrics.SetNodeRetryCount(node.Name, state.AttemptCount)
	metrics.RecordReconciliation("worker_created")

	klog.InfoS("Worker pod created, transitioning to pending",
		"node", node.Name,
		"pod", pod.Name,
		"attempt", state.AttemptCount,
	)

	// Requeue to check status
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// handleInProgress monitors the worker pod and processes results
func (r *NodeReconciler) handleInProgress(ctx context.Context, node *corev1.Node, state *NodeState) (ctrl.Result, error) {
	klog.InfoS("Handling in-progress node", "node", node.Name, "pod", state.WorkerPodName)

	// Get worker pod status
	status, err := r.WorkerManager.GetWorkerPodStatus(ctx, state.WorkerPodName)
	if err != nil {
		klog.ErrorS(err, "Failed to get worker pod status", "node", node.Name, "pod", state.WorkerPodName)
		// Pod might have been deleted externally, reset to unverified
		state.State = StateUnverified
		state.WorkerPodName = ""
		r.StateCache.Set(node.Name, state)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Update state if needed
	if state.State == StatePending && status.Phase == corev1.PodRunning {
		klog.InfoS("Worker pod is running", "node", node.Name, "pod", state.WorkerPodName)
		state.State = StateInProgress
		r.StateCache.Set(node.Name, state)
		metrics.RecordReconciliation("worker_running")
	}

	// Check for timeout
	if time.Since(state.LastAttempt) > r.Config.Worker.GetTimeout() {
		duration := time.Since(state.LastAttempt).Seconds()
		klog.InfoS("Worker pod timed out", "node", node.Name, "pod", state.WorkerPodName)
		// Clean up pod
		_ = r.WorkerManager.DeleteWorkerPod(ctx, state.WorkerPodName)

		state.State = StateFailed
		state.LastError = "worker pod timed out"
		r.StateCache.Set(node.Name, state)

		// Record metrics
		metrics.RecordWorkerPodFailed(node.Name, "timeout")
		metrics.RecordWorkerPodDuration(node.Name, "timeout", duration)
		metrics.SetWorkerPodsActive(node.Name, 0)
		metrics.RecordReconciliation("worker_timeout")

		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	// Check if pod has completed
	if !status.Completed {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Calculate worker pod duration
	duration := time.Since(state.LastAttempt).Seconds()

	// Process completion
	klog.InfoS("Worker pod completed",
		"node", node.Name,
		"pod", state.WorkerPodName,
		"exitCode", status.ExitCode,
		"succeeded", status.Succeeded,
		"duration", duration,
	)

	// Clean up worker pod
	_ = r.WorkerManager.DeleteWorkerPod(ctx, state.WorkerPodName)
	metrics.SetWorkerPodsActive(node.Name, 0)

	if status.Succeeded && status.ExitCode != nil && *status.ExitCode == 0 {
		// Success - mark as verified
		klog.InfoS("Verification successful, marking node as verified", "node", node.Name)

		state.State = StateVerified
		now := time.Now()
		state.VerifiedAt = &now
		r.StateCache.Set(node.Name, state)

		// Record metrics
		metrics.RecordWorkerPodSucceeded(node.Name)
		metrics.RecordWorkerPodDuration(node.Name, "success", duration)
		metrics.RecordReconciliation("worker_succeeded")

		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	} else {
		// Failed
		errMsg := status.Message
		if status.ExitCode != nil {
			errMsg = fmt.Sprintf("exit code %d: %s", *status.ExitCode, status.Message)
		}

		// Record metrics
		metrics.RecordWorkerPodFailed(node.Name, "check_failed")
		metrics.RecordWorkerPodDuration(node.Name, "failure", duration)
		metrics.RecordReconciliation("worker_failed")

		klog.InfoS("Verification failed", "node", node.Name, "error", errMsg)

		state.State = StateFailed
		state.LastError = errMsg
		r.StateCache.Set(node.Name, state)

		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}
}

// handleFailed handles nodes that failed verification
func (r *NodeReconciler) handleFailed(ctx context.Context, node *corev1.Node, state *NodeState) (ctrl.Result, error) {
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
		state.State = StateUnverified
		state.WorkerPodName = ""
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

		if err := r.Client.Delete(ctx, node); err != nil {
			klog.ErrorS(err, "Failed to delete node", "node", node.Name)
			metrics.RecordReconciliationError(node.Name, "node_deletion_error")
			metrics.RecordReconciliation("error")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}

		state.State = StateDeleting
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
func (r *NodeReconciler) handleVerified(ctx context.Context, node *corev1.Node, state *NodeState) (ctrl.Result, error) {
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

// SetupWithManager sets up the controller with the Manager
func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			node, ok := object.(*corev1.Node)
			if !ok {
				return false
			}

			// Only watch nodes that:
			// 1. Don't have the verified label, AND
			// 2. Have any of the unverified taints
			hasVerified := hasVerifiedLabel(node, r.Config.NodeManagement.VerifiedLabel.Key)
			hasTaint := hasAnyUnverifiedTaint(node, r.Config.NodeManagement.Taints)

			return !hasVerified && hasTaint
		})).
		Complete(r)
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
	for _, configTaint := range taints {
		for _, nodeTaint := range node.Spec.Taints {
			if nodeTaint.Key == configTaint.Key {
				return true
			}
		}
	}
	return false
}

// hasUnverifiedTaint checks if a node has the unverified taint (legacy function kept for compatibility)
func hasUnverifiedTaint(node *corev1.Node, taintKey string) bool {
	for _, taint := range node.Spec.Taints {
		if taint.Key == taintKey {
			return true
		}
	}
	return false
}

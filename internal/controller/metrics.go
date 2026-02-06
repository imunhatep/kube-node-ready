package controller

import (
	"time"

	"github.com/imunhatep/kube-node-ready/internal/k8s"
	"github.com/imunhatep/kube-node-ready/internal/metrics"
)

// StartMetricsUpdater starts a goroutine that periodically updates aggregate metrics
func (r *NodeReconciler) StartMetricsUpdater(stopCh <-chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Update immediately on start
	r.updateAggregateMetrics()

	for {
		select {
		case <-ticker.C:
			r.updateAggregateMetrics()
		case <-stopCh:
			return
		}
	}
}

// updateAggregateMetrics updates aggregate metrics based on current state cache
func (r *NodeReconciler) updateAggregateMetrics() {
	states := r.StateCache.GetAll()

	unverified := 0
	verifying := 0
	verified := 0
	failed := 0

	for _, state := range states {
		switch state.State {
		case k8s.NodeStateUnverified:
			unverified++
		case k8s.NodeStatePending, k8s.NodeStateInProgress:
			verifying++
		case k8s.NodeStateVerified:
			verified++
		case k8s.NodeStateFailed:
			failed++
		}
	}

	metrics.SetNodesUnverified(unverified)
	metrics.SetNodesVerifying(verifying)
	metrics.SetNodesVerified(verified)
	metrics.SetNodesFailed(failed)
}

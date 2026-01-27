package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// DNS check metrics
	DNSCheckTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_dns_check_total",
			Help: "Total number of DNS checks performed",
		},
		[]string{"domain", "status"}, // status: success, failure
	)

	DNSCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kube_node_ready_dns_check_duration_seconds",
			Help:    "Duration of DNS checks in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"domain"},
	)

	// Network check metrics
	NetworkCheckTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_network_check_total",
			Help: "Total number of network checks performed",
		},
		[]string{"address", "status"}, // status: success, failure
	)

	NetworkCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kube_node_ready_network_check_duration_seconds",
			Help:    "Duration of network checks in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"address"},
	)

	// Kubernetes API check metrics
	KubernetesCheckTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_kubernetes_check_total",
			Help: "Total number of Kubernetes API checks performed",
		},
		[]string{"check_type", "status"}, // check_type: api, service_discovery
	)

	KubernetesCheckDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kube_node_ready_kubernetes_check_duration_seconds",
			Help:    "Duration of Kubernetes API checks in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"check_type"},
	)

	// Overall verification metrics
	VerificationAttemptsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_verification_attempts_total",
			Help: "Total number of verification attempts",
		},
		[]string{"node", "status"}, // status: success, failure
	)

	VerificationRetriesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "kube_node_ready_verification_retries_total",
			Help: "Total number of verification retries",
		},
	)

	VerificationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kube_node_ready_verification_duration_seconds",
			Help:    "Duration of complete verification in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300}, // Custom buckets for verification
		},
		[]string{"node"},
	)

	// Node controller metrics
	NodeTaintRemovalTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_node_taint_removal_total",
			Help: "Total number of node taint removal operations",
		},
		[]string{"node", "status"}, // status: success, failure
	)

	NodeLabelAddTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_node_label_add_total",
			Help: "Total number of node label addition operations",
		},
		[]string{"node", "status"}, // status: success, failure
	)

	NodeUpdateDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kube_node_ready_node_update_duration_seconds",
			Help:    "Duration of node update operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"node", "operation"}, // operation: taint_removal, label_add
	)

	NodeDeletionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_node_deletion_total",
			Help: "Total number of node deletion operations",
		},
		[]string{"node", "status"}, // status: success, failure
	)

	// Application info
	BuildInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_build_info",
			Help: "Build information",
		},
		[]string{"version", "commit_hash", "build_date"},
	)

	// Node readiness status
	NodeReadyStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_node_status",
			Help: "Node readiness status (1=ready, 0=not ready)",
		},
		[]string{"node"},
	)

	// Controller-specific metrics

	// Reconciliation metrics
	ReconciliationTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_controller_reconciliation_total",
			Help: "Total number of node reconciliations",
		},
		[]string{"result"}, // result: success, error, requeue
	)

	ReconciliationDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kube_node_ready_controller_reconciliation_duration_seconds",
			Help:    "Duration of node reconciliation in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"node"},
	)

	ReconciliationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_controller_reconciliation_errors_total",
			Help: "Total number of reconciliation errors",
		},
		[]string{"node", "error_type"},
	)

	// Worker pod metrics
	WorkerPodsCreated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_controller_worker_pods_created_total",
			Help: "Total number of worker pods created",
		},
		[]string{"node"},
	)

	WorkerPodsFailed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_controller_worker_pods_failed_total",
			Help: "Total number of worker pods that failed",
		},
		[]string{"node", "reason"},
	)

	WorkerPodsSucceeded = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_controller_worker_pods_succeeded_total",
			Help: "Total number of worker pods that succeeded",
		},
		[]string{"node"},
	)

	WorkerPodsActive = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_controller_worker_pods_active",
			Help: "Current number of active worker pods",
		},
		[]string{"node"},
	)

	WorkerPodDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "kube_node_ready_controller_worker_pod_duration_seconds",
			Help:    "Duration of worker pod execution in seconds",
			Buckets: []float64{10, 30, 60, 120, 300, 600},
		},
		[]string{"node", "result"}, // result: success, failure, timeout
	)

	WorkerPodCreationErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_controller_worker_pod_creation_errors_total",
			Help: "Total number of worker pod creation errors",
		},
		[]string{"node", "error_type"},
	)

	// Node state metrics
	NodesUnverified = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_controller_nodes_unverified",
			Help: "Current number of unverified nodes",
		},
	)

	NodesVerifying = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_controller_nodes_verifying",
			Help: "Current number of nodes being verified",
		},
	)

	NodesVerified = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_controller_nodes_verified",
			Help: "Current number of verified nodes",
		},
	)

	NodesFailed = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_controller_nodes_failed",
			Help: "Current number of nodes that failed verification",
		},
	)

	NodeRetryCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_controller_node_retry_count",
			Help: "Current retry count for node verification",
		},
		[]string{"node"},
	)

	// Leader election metrics
	LeaderElectionStatus = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_controller_leader_election_status",
			Help: "Leader election status (1=leader, 0=standby)",
		},
	)

	LeaderElectionTransitions = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "kube_node_ready_controller_leader_election_transitions_total",
			Help: "Total number of leader election transitions",
		},
	)

	LeaderElectionLeaseDuration = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_controller_leader_election_lease_duration_seconds",
			Help: "Leader election lease duration in seconds",
		},
	)

	// Controller health metrics
	ControllerHealthy = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_controller_healthy",
			Help: "Controller health status (1=healthy, 0=unhealthy)",
		},
	)

	ControllerStartTime = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_controller_start_time_seconds",
			Help: "Controller start time in unix timestamp",
		},
	)

	// Queue metrics
	ReconcileQueueDepth = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "kube_node_ready_controller_reconcile_queue_depth",
			Help: "Current depth of the reconciliation queue",
		},
	)

	ReconcileQueueLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "kube_node_ready_controller_reconcile_queue_latency_seconds",
			Help:    "Time items spend in the reconciliation queue",
			Buckets: []float64{0.1, 0.5, 1, 5, 10, 30, 60},
		},
	)

	// Taint and label operations
	TaintsApplied = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_controller_taints_applied_total",
			Help: "Total number of taints applied to nodes",
		},
		[]string{"node", "taint_key"},
	)

	TaintsRemoved = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_controller_taints_removed_total",
			Help: "Total number of taints removed from nodes",
		},
		[]string{"node", "taint_key"},
	)

	LabelsApplied = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kube_node_ready_controller_labels_applied_total",
			Help: "Total number of labels applied to nodes",
		},
		[]string{"node", "label_key"},
	)
)

// SetBuildInfo sets the build information metric
func SetBuildInfo(version, commitHash, buildDate string) {
	BuildInfo.WithLabelValues(version, commitHash, buildDate).Set(1)
}

// SetNodeStatus sets the node readiness status
func SetNodeStatus(nodeName string, ready bool) {
	value := 0.0
	if ready {
		value = 1.0
	}
	NodeReadyStatus.WithLabelValues(nodeName).Set(value)
}

// Controller metric helpers

// RecordReconciliation records a reconciliation attempt
func RecordReconciliation(result string) {
	ReconciliationTotal.WithLabelValues(result).Inc()
}

// RecordReconciliationDuration records the duration of a reconciliation
func RecordReconciliationDuration(nodeName string, duration float64) {
	ReconciliationDuration.WithLabelValues(nodeName).Observe(duration)
}

// RecordReconciliationError records a reconciliation error
func RecordReconciliationError(nodeName, errorType string) {
	ReconciliationErrors.WithLabelValues(nodeName, errorType).Inc()
}

// RecordWorkerPodCreated records a worker pod creation
func RecordWorkerPodCreated(nodeName string) {
	WorkerPodsCreated.WithLabelValues(nodeName).Inc()
}

// RecordWorkerPodFailed records a worker pod failure
func RecordWorkerPodFailed(nodeName, reason string) {
	WorkerPodsFailed.WithLabelValues(nodeName, reason).Inc()
}

// RecordWorkerPodSucceeded records a successful worker pod
func RecordWorkerPodSucceeded(nodeName string) {
	WorkerPodsSucceeded.WithLabelValues(nodeName).Inc()
}

// SetWorkerPodsActive sets the current number of active worker pods for a node
func SetWorkerPodsActive(nodeName string, count int) {
	WorkerPodsActive.WithLabelValues(nodeName).Set(float64(count))
}

// RecordWorkerPodDuration records the duration of a worker pod execution
func RecordWorkerPodDuration(nodeName, result string, duration float64) {
	WorkerPodDuration.WithLabelValues(nodeName, result).Observe(duration)
}

// RecordWorkerPodCreationError records a worker pod creation error
func RecordWorkerPodCreationError(nodeName, errorType string) {
	WorkerPodCreationErrors.WithLabelValues(nodeName, errorType).Inc()
}

// SetNodesUnverified sets the current number of unverified nodes
func SetNodesUnverified(count int) {
	NodesUnverified.Set(float64(count))
}

// SetNodesVerifying sets the current number of nodes being verified
func SetNodesVerifying(count int) {
	NodesVerifying.Set(float64(count))
}

// SetNodesVerified sets the current number of verified nodes
func SetNodesVerified(count int) {
	NodesVerified.Set(float64(count))
}

// SetNodesFailed sets the current number of nodes that failed verification
func SetNodesFailed(count int) {
	NodesFailed.Set(float64(count))
}

// SetNodeRetryCount sets the retry count for a specific node
func SetNodeRetryCount(nodeName string, count int) {
	NodeRetryCount.WithLabelValues(nodeName).Set(float64(count))
}

// SetLeaderElectionStatus sets the leader election status
func SetLeaderElectionStatus(isLeader bool) {
	value := 0.0
	if isLeader {
		value = 1.0
	}
	LeaderElectionStatus.Set(value)
}

// RecordLeaderElectionTransition records a leader election transition
func RecordLeaderElectionTransition() {
	LeaderElectionTransitions.Inc()
}

// SetLeaderElectionLeaseDuration sets the lease duration
func SetLeaderElectionLeaseDuration(seconds float64) {
	LeaderElectionLeaseDuration.Set(seconds)
}

// SetControllerHealthy sets the controller health status
func SetControllerHealthy(healthy bool) {
	value := 0.0
	if healthy {
		value = 1.0
	}
	ControllerHealthy.Set(value)
}

// SetControllerStartTime sets the controller start time
func SetControllerStartTime(timestamp float64) {
	ControllerStartTime.Set(timestamp)
}

// SetReconcileQueueDepth sets the current reconciliation queue depth
func SetReconcileQueueDepth(depth int) {
	ReconcileQueueDepth.Set(float64(depth))
}

// RecordReconcileQueueLatency records the time an item spent in the queue
func RecordReconcileQueueLatency(seconds float64) {
	ReconcileQueueLatency.Observe(seconds)
}

// RecordTaintApplied records a taint application
func RecordTaintApplied(nodeName, taintKey string) {
	TaintsApplied.WithLabelValues(nodeName, taintKey).Inc()
}

// RecordTaintRemoved records a taint removal
func RecordTaintRemoved(nodeName, taintKey string) {
	TaintsRemoved.WithLabelValues(nodeName, taintKey).Inc()
}

// RecordLabelApplied records a label application
func RecordLabelApplied(nodeName, labelKey string) {
	LabelsApplied.WithLabelValues(nodeName, labelKey).Inc()
}

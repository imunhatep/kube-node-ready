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

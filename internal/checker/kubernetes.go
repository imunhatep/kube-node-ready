package checker

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/imunhatep/kube-node-ready/internal/metrics"
)

// KubernetesChecker performs Kubernetes API checks
type KubernetesChecker struct {
	clientset *kubernetes.Clientset
	timeout   time.Duration
}

// NewKubernetesChecker creates a new Kubernetes API checker
func NewKubernetesChecker(clientset *kubernetes.Clientset, timeout time.Duration) *KubernetesChecker {
	return &KubernetesChecker{
		clientset: clientset,
		timeout:   timeout,
	}
}

// Check performs Kubernetes API connectivity check
func (k *KubernetesChecker) Check(ctx context.Context) error {
	start := time.Now()
	klog.Info("Starting Kubernetes API check")

	// Create a context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()

	// Try to get server version as a simple API check
	version, err := k.clientset.Discovery().ServerVersion()
	duration := time.Since(start)

	// Record metrics
	metrics.KubernetesCheckDuration.WithLabelValues("api").Observe(duration.Seconds())

	// Ensure context is respected
	if checkCtx.Err() != nil {
		metrics.KubernetesCheckTotal.WithLabelValues("api", "failure").Inc()
		return fmt.Errorf("Kubernetes API check cancelled: %w", checkCtx.Err())
	}

	if err != nil {
		metrics.KubernetesCheckTotal.WithLabelValues("api", "failure").Inc()
		klog.ErrorS(err, "Kubernetes API check failed",
			"duration", duration,
		)
		return fmt.Errorf("Kubernetes API check failed: %w", err)
	}

	metrics.KubernetesCheckTotal.WithLabelValues("api", "success").Inc()
	klog.InfoS("Kubernetes API check passed",
		"version", version.GitVersion,
		"duration", duration,
	)

	return nil
}

// CheckServiceDiscovery verifies service and endpoint discovery
func (k *KubernetesChecker) CheckServiceDiscovery(ctx context.Context) error {
	start := time.Now()
	klog.Info("Starting service discovery check")

	// Create a context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()

	// Try to get the kubernetes service in default namespace
	svc, err := k.clientset.CoreV1().Services("default").Get(checkCtx, "kubernetes", metav1.GetOptions{})
	duration := time.Since(start)

	// Record metrics
	metrics.KubernetesCheckDuration.WithLabelValues("service_discovery").Observe(duration.Seconds())

	if err != nil {
		metrics.KubernetesCheckTotal.WithLabelValues("service_discovery", "failure").Inc()
		klog.ErrorS(err, "Service discovery check failed",
			"duration", duration,
		)
		return fmt.Errorf("service discovery failed: %w", err)
	}

	klog.InfoS("Service discovery check passed",
		"service", svc.Name,
		"clusterIP", svc.Spec.ClusterIP,
		"duration", duration,
	)

	// Check endpoints
	endpoints, err := k.clientset.CoreV1().Endpoints("default").Get(checkCtx, "kubernetes", metav1.GetOptions{})
	if err != nil {
		metrics.KubernetesCheckTotal.WithLabelValues("service_discovery", "failure").Inc()
		klog.ErrorS(err, "Endpoints check failed",
			"duration", time.Since(start),
		)
		return fmt.Errorf("endpoints check failed: %w", err)
	}

	if len(endpoints.Subsets) == 0 {
		metrics.KubernetesCheckTotal.WithLabelValues("service_discovery", "failure").Inc()
		klog.ErrorS(nil, "Endpoints check failed - no subsets",
			"duration", time.Since(start),
		)
		return fmt.Errorf("no endpoints found for kubernetes service")
	}

	metrics.KubernetesCheckTotal.WithLabelValues("service_discovery", "success").Inc()
	klog.InfoS("Endpoints check passed",
		"endpoints", len(endpoints.Subsets),
		"duration", time.Since(start),
	)

	return nil
}

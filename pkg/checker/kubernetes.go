package checker

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// KubernetesChecker performs Kubernetes API checks
type KubernetesChecker struct {
	logger    *zap.Logger
	clientset *kubernetes.Clientset
	timeout   time.Duration
}

// NewKubernetesChecker creates a new Kubernetes API checker
func NewKubernetesChecker(logger *zap.Logger, clientset *kubernetes.Clientset, timeout time.Duration) *KubernetesChecker {
	return &KubernetesChecker{
		logger:    logger,
		clientset: clientset,
		timeout:   timeout,
	}
}

// Check performs Kubernetes API connectivity check
func (k *KubernetesChecker) Check(ctx context.Context) error {
	start := time.Now()
	k.logger.Info("Starting Kubernetes API check")

	// Create a context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()

	// Try to get server version as a simple API check
	version, err := k.clientset.Discovery().ServerVersion()
	duration := time.Since(start)

	// Ensure context is respected
	if checkCtx.Err() != nil {
		return fmt.Errorf("Kubernetes API check cancelled: %w", checkCtx.Err())
	}

	if err != nil {
		k.logger.Error("Kubernetes API check failed",
			zap.Duration("duration", duration),
			zap.Error(err),
		)
		return fmt.Errorf("Kubernetes API check failed: %w", err)
	}

	k.logger.Info("Kubernetes API check passed",
		zap.String("version", version.GitVersion),
		zap.Duration("duration", duration),
	)

	return nil
}

// CheckServiceDiscovery verifies service and endpoint discovery
func (k *KubernetesChecker) CheckServiceDiscovery(ctx context.Context) error {
	start := time.Now()
	k.logger.Info("Starting service discovery check")

	// Create a context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, k.timeout)
	defer cancel()

	// Try to get the kubernetes service in default namespace
	svc, err := k.clientset.CoreV1().Services("default").Get(checkCtx, "kubernetes", metav1.GetOptions{})
	if err != nil {
		k.logger.Error("Service discovery check failed",
			zap.Duration("duration", time.Since(start)),
			zap.Error(err),
		)
		return fmt.Errorf("service discovery failed: %w", err)
	}

	k.logger.Info("Service discovery check passed",
		zap.String("service", svc.Name),
		zap.String("clusterIP", svc.Spec.ClusterIP),
		zap.Duration("duration", time.Since(start)),
	)

	// Check endpoints
	endpoints, err := k.clientset.CoreV1().Endpoints("default").Get(checkCtx, "kubernetes", metav1.GetOptions{})
	if err != nil {
		k.logger.Error("Endpoints check failed",
			zap.Duration("duration", time.Since(start)),
			zap.Error(err),
		)
		return fmt.Errorf("endpoints check failed: %w", err)
	}

	if len(endpoints.Subsets) == 0 {
		k.logger.Error("Endpoints check failed - no subsets",
			zap.Duration("duration", time.Since(start)),
		)
		return fmt.Errorf("no endpoints found for kubernetes service")
	}

	k.logger.Info("Endpoints check passed",
		zap.Int("subsets", len(endpoints.Subsets)),
		zap.Duration("duration", time.Since(start)),
	)

	return nil
}

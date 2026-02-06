package checker

import (
	"context"
	"fmt"
	"time"

	"github.com/imunhatep/kube-node-ready/internal/metrics"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// Checker orchestrates all verification checks
type Checker struct {
	config            *CheckerConfig
	dnsChecker        *DNSChecker
	kubernetesChecker *KubernetesChecker
	networkChecker    *NetworkChecker
}

// NewChecker creates a new checker orchestrator
func NewChecker(cfg *CheckerConfig, clientset *kubernetes.Clientset) *Checker {
	var kubernetesChecker *KubernetesChecker

	if clientset != nil {
		kubernetesChecker = NewKubernetesChecker(clientset, cfg.CheckTimeout)
	}

	return &Checker{
		config:            cfg,
		dnsChecker:        NewDNSChecker(cfg.CheckTimeout),
		kubernetesChecker: kubernetesChecker,
		networkChecker:    NewNetworkChecker(cfg.CheckTimeout),
	}
}

// RunAllChecks executes all verification checks
func (c *Checker) RunAllChecks(ctx context.Context) error {
	klog.Info("Starting all verification checks")

	// 1. DNS Check
	if err := c.runDNSChecks(ctx); err != nil {
		return err
	}

	// 2. Kubernetes API Check
	if err := c.runKubernetesAPICheck(ctx); err != nil {
		return err
	}

	// 3. Network Connectivity Check
	if err := c.runNetworkCheck(ctx); err != nil {
		return err
	}

	// 4. Service Discovery Check
	if err := c.runServiceDiscoveryCheck(ctx); err != nil {
		return err
	}

	klog.Info("All verification checks passed successfully")
	return nil
}

func (c *Checker) runDNSChecks(ctx context.Context) error {
	klog.InfoS("Running DNS checks", "domains", c.config.DNSTestDomains)

	if err := c.dnsChecker.CheckAll(ctx, c.config.DNSTestDomains); err != nil {
		return fmt.Errorf("DNS check failed: %w", err)
	}

	return nil
}

func (c *Checker) runKubernetesAPICheck(ctx context.Context) error {
	if c.kubernetesChecker == nil {
		klog.Info("Skipping Kubernetes API check (no client available)")
		return nil
	}

	klog.Info("Running Kubernetes API check")

	if err := c.kubernetesChecker.Check(ctx); err != nil {
		return fmt.Errorf("Kubernetes API check failed: %w", err)
	}

	return nil
}

func (c *Checker) runNetworkCheck(ctx context.Context) error {
	klog.Info("Running network connectivity check")

	// Check connectivity to Kubernetes API server
	if err := c.networkChecker.CheckKubernetesService(ctx, c.config.KubernetesServiceHost, c.config.KubernetesServicePort); err != nil {
		return fmt.Errorf("network check failed: %w", err)
	}

	return nil
}

func (c *Checker) runServiceDiscoveryCheck(ctx context.Context) error {
	if c.kubernetesChecker == nil {
		klog.Info("Skipping service discovery check (no client available)")
		return nil
	}

	klog.Info("Running service discovery check")

	if err := c.kubernetesChecker.CheckServiceDiscovery(ctx); err != nil {
		return fmt.Errorf("service discovery check failed: %w", err)
	}

	return nil
}

// RunWithRetry runs all checks with retry logic
func (c *Checker) RunWithRetry(ctx context.Context) error {
	var lastErr error
	verificationStart := time.Now()

	for attempt := 1; attempt <= c.config.MaxRetries; attempt++ {
		klog.InfoS("Verification attempt", "attempt", attempt, "maxRetries", c.config.MaxRetries)

		err := c.RunAllChecks(ctx)
		if err == nil {
			// Record success metrics
			duration := time.Since(verificationStart)
			metrics.VerificationDuration.WithLabelValues(c.config.NodeName).Observe(duration.Seconds())
			metrics.VerificationAttemptsTotal.WithLabelValues(c.config.NodeName, "success").Inc()

			if attempt > 1 {
				metrics.VerificationRetriesTotal.Add(float64(attempt - 1))
			}

			klog.InfoS("Verification successful", "attempt", attempt)

			// Set node status to ready
			metrics.SetNodeStatus(c.config.NodeName, true)

			return nil
		}

		lastErr = err

		// Record failure for this attempt
		metrics.VerificationAttemptsTotal.WithLabelValues(c.config.NodeName, "failure").Inc()

		klog.InfoS("Verification attempt failed",
			"attempt", attempt,
			"maxRetries", c.config.MaxRetries,
			"error", err,
		)

		// Don't sleep after the last attempt
		if attempt < c.config.MaxRetries {
			metrics.VerificationRetriesTotal.Inc()
			backoff := c.calculateBackoff(attempt)
			klog.InfoS("Waiting before retry", "backoff", backoff)

			select {
			case <-time.After(backoff):
				// Continue to next attempt
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	// Set node status to not ready
	metrics.SetNodeStatus(c.config.NodeName, false)

	klog.ErrorS(lastErr, "All verification attempts failed", "attempts", c.config.MaxRetries)
	return fmt.Errorf("verification failed after %d attempts: %w", c.config.MaxRetries, lastErr)
}

func (c *Checker) calculateBackoff(attempt int) time.Duration {
	if c.config.RetryBackoff == "exponential" {
		// Exponential backoff: 1s, 2s, 4s, 8s, 16s
		backoff := time.Duration(1<<uint(attempt-1)) * time.Second
		// Cap at 30 seconds
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
		return backoff
	}

	// Linear backoff: 5s each time
	return 5 * time.Second
}

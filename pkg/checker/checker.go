package checker

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"k8s.io/client-go/kubernetes"

	"github.com/imunhatep/kube-node-ready/pkg/config"
)

// Checker orchestrates all verification checks
type Checker struct {
	logger            *zap.Logger
	config            *config.Config
	dnsChecker        *DNSChecker
	kubernetesChecker *KubernetesChecker
	networkChecker    *NetworkChecker
}

// NewChecker creates a new checker orchestrator
func NewChecker(logger *zap.Logger, cfg *config.Config, clientset *kubernetes.Clientset) *Checker {
	var kubernetesChecker *KubernetesChecker
	if clientset != nil {
		kubernetesChecker = NewKubernetesChecker(logger, clientset, 10*time.Second)
	}

	return &Checker{
		logger:            logger,
		config:            cfg,
		dnsChecker:        NewDNSChecker(logger, 5*time.Second),
		kubernetesChecker: kubernetesChecker,
		networkChecker:    NewNetworkChecker(logger, 10*time.Second),
	}
}

// RunAllChecks executes all verification checks
func (c *Checker) RunAllChecks(ctx context.Context) error {
	c.logger.Info("Starting all verification checks")

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

	c.logger.Info("All verification checks passed successfully")
	return nil
}

func (c *Checker) runDNSChecks(ctx context.Context) error {
	c.logger.Info("Running DNS checks", zap.Strings("domains", c.config.DNSTestDomains))

	if err := c.dnsChecker.CheckAll(ctx, c.config.DNSTestDomains); err != nil {
		return fmt.Errorf("DNS check failed: %w", err)
	}

	return nil
}

func (c *Checker) runKubernetesAPICheck(ctx context.Context) error {
	if c.kubernetesChecker == nil {
		c.logger.Info("Skipping Kubernetes API check (no client available)")
		return nil
	}

	c.logger.Info("Running Kubernetes API check")

	if err := c.kubernetesChecker.Check(ctx); err != nil {
		return fmt.Errorf("Kubernetes API check failed: %w", err)
	}

	return nil
}

func (c *Checker) runNetworkCheck(ctx context.Context) error {
	c.logger.Info("Running network connectivity check")

	// Check connectivity to Kubernetes API server
	if err := c.networkChecker.CheckKubernetesService(ctx, c.config.KubernetesServiceHost, c.config.KubernetesServicePort); err != nil {
		return fmt.Errorf("network check failed: %w", err)
	}

	return nil
}

func (c *Checker) runServiceDiscoveryCheck(ctx context.Context) error {
	if c.kubernetesChecker == nil {
		c.logger.Info("Skipping service discovery check (no client available)")
		return nil
	}

	c.logger.Info("Running service discovery check")

	if err := c.kubernetesChecker.CheckServiceDiscovery(ctx); err != nil {
		return fmt.Errorf("service discovery check failed: %w", err)
	}

	return nil
}

// RunWithRetry runs all checks with retry logic
func (c *Checker) RunWithRetry(ctx context.Context) error {
	var lastErr error

	for attempt := 1; attempt <= c.config.MaxRetries; attempt++ {
		c.logger.Info("Verification attempt", zap.Int("attempt", attempt), zap.Int("maxRetries", c.config.MaxRetries))

		err := c.RunAllChecks(ctx)
		if err == nil {
			c.logger.Info("Verification successful", zap.Int("attempt", attempt))
			return nil
		}

		lastErr = err
		c.logger.Warn("Verification attempt failed",
			zap.Int("attempt", attempt),
			zap.Int("maxRetries", c.config.MaxRetries),
			zap.Error(err),
		)

		// Don't sleep after the last attempt
		if attempt < c.config.MaxRetries {
			backoff := c.calculateBackoff(attempt)
			c.logger.Info("Waiting before retry", zap.Duration("backoff", backoff))

			select {
			case <-time.After(backoff):
				// Continue to next attempt
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	c.logger.Error("All verification attempts failed", zap.Int("attempts", c.config.MaxRetries), zap.Error(lastErr))
	return fmt.Errorf("verification failed after %d attempts: %w", c.config.MaxRetries, lastErr)
}

func (c *Checker) calculateBackoff(attempt int) time.Duration {
	if c.config.RetryBackoff == config.RetryBackoffExponential {
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

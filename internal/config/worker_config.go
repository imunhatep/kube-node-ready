package config

import (
	"fmt"
	"time"
)

// WorkerConfig holds configuration for the worker component
type WorkerConfig struct {
	// Node identification
	NodeName  string
	Namespace string

	// Check configuration
	CheckTimeout   time.Duration
	DNSTestDomains []string
	ClusterDNSIP   string

	// Kubernetes service
	KubernetesServiceHost string
	KubernetesServicePort string

	// Logging
	LogLevel  string
	LogFormat string

	// Kubeconfig (for out-of-cluster testing)
	KubeconfigPath string

	// Worker mode specific
	WorkerMode bool // Always true for worker
}

// LoadWorkerConfigFromEnv loads worker configuration from environment variables
func LoadWorkerConfigFromEnv() (*WorkerConfig, error) {
	cfg := &WorkerConfig{
		NodeName:              getEnv("NODE_NAME", ""),
		Namespace:             getEnv("POD_NAMESPACE", "kube-system"),
		CheckTimeout:          getDurationEnv("CHECK_TIMEOUT", 10*time.Second),
		DNSTestDomains:        getSliceEnv("DNS_TEST_DOMAINS", []string{"kubernetes.default.svc.cluster.local", "google.com"}),
		ClusterDNSIP:          getEnv("CLUSTER_DNS_IP", ""),
		KubernetesServiceHost: getEnv("KUBERNETES_SERVICE_HOST", ""),
		KubernetesServicePort: getEnv("KUBERNETES_SERVICE_PORT", "443"),
		LogLevel:              getEnv("LOG_LEVEL", "info"),
		LogFormat:             getEnv("LOG_FORMAT", "json"),
		KubeconfigPath:        getEnv("KUBECONFIG", ""),
		WorkerMode:            true,
	}

	return cfg, nil
}

// Validate checks if the worker configuration is valid
func (c *WorkerConfig) Validate() error {
	if c.NodeName == "" {
		return fmt.Errorf("NODE_NAME is required")
	}

	if c.KubernetesServiceHost == "" {
		return fmt.Errorf("KUBERNETES_SERVICE_HOST is required")
	}

	if len(c.DNSTestDomains) == 0 {
		return fmt.Errorf("DNS_TEST_DOMAINS must have at least one domain")
	}

	if c.CheckTimeout < 1*time.Second {
		return fmt.Errorf("CHECK_TIMEOUT must be at least 1 second")
	}

	return nil
}

// ToConfig converts WorkerConfig to the legacy Config format for compatibility
// with existing checker code
func (c *WorkerConfig) ToConfig() *Config {
	return &Config{
		NodeName:              c.NodeName,
		Namespace:             c.Namespace,
		CheckTimeout:          c.CheckTimeout,
		DNSTestDomains:        c.DNSTestDomains,
		ClusterDNSIP:          c.ClusterDNSIP,
		KubernetesServiceHost: c.KubernetesServiceHost,
		KubernetesServicePort: c.KubernetesServicePort,
		LogLevel:              c.LogLevel,
		LogFormat:             c.LogFormat,
		KubeconfigPath:        c.KubeconfigPath,
		// Worker-specific defaults (not used in worker mode)
		InitialCheckTimeout: 300 * time.Second,
		MaxRetries:          1, // Worker runs checks once
		RetryBackoff:        RetryBackoffLinear,
		MetricsEnabled:      false, // Workers don't expose metrics
		DryRun:              false,
		DeleteFailedNode:    false,
	}
}

package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// WorkerConfig holds configuration for the worker component
type WorkerConfig struct {
	// Node identification
	NodeName string

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

// workerConfigFile represents the YAML structure of the worker config file
type workerConfigFile struct {
	CheckTimeoutSeconds int      `yaml:"checkTimeoutSeconds"`
	DNSTestDomains      []string `yaml:"dnsTestDomains"`
	ClusterDNSIP        string   `yaml:"clusterDnsIp"`
	Logging             struct {
		Level  string `yaml:"level"`
		Format string `yaml:"format"`
	} `yaml:"logging"`
}

// LoadWorkerConfigFromFile loads worker configuration from a YAML file (ConfigMap mount)
// and environment variables. File provides check configuration, environment provides node identity.
func LoadWorkerConfigFromFile(configPath string) (*WorkerConfig, error) {
	// Read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read worker config file: %w", err)
	}

	// Parse YAML
	var fileCfg workerConfigFile
	if err := yaml.Unmarshal(data, &fileCfg); err != nil {
		return nil, fmt.Errorf("failed to parse worker config file: %w", err)
	}

	// Build WorkerConfig from file + environment
	cfg := &WorkerConfig{
		// From environment (injected by controller/k8s)
		NodeName:              getEnv("NODE_NAME", ""),
		KubernetesServiceHost: getEnv("KUBERNETES_SERVICE_HOST", ""),
		KubernetesServicePort: getEnv("KUBERNETES_SERVICE_PORT", "443"),
		KubeconfigPath:        getEnv("KUBECONFIG", ""),

		// From config file
		CheckTimeout:   time.Duration(fileCfg.CheckTimeoutSeconds) * time.Second,
		DNSTestDomains: fileCfg.DNSTestDomains,
		ClusterDNSIP:   fileCfg.ClusterDNSIP,
		LogLevel:       fileCfg.Logging.Level,
		LogFormat:      fileCfg.Logging.Format,

		// Worker mode
		WorkerMode: true,
	}

	// Apply defaults if not specified in file
	if cfg.CheckTimeout == 0 {
		cfg.CheckTimeout = 10 * time.Second
	}
	if len(cfg.DNSTestDomains) == 0 {
		cfg.DNSTestDomains = []string{"kubernetes.default.svc.cluster.local", "google.com"}
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.LogFormat == "" {
		cfg.LogFormat = "json"
	}

	return cfg, nil
}

// LoadWorkerConfigFromEnv loads worker configuration from environment variables only
// This is kept for backward compatibility and testing
func LoadWorkerConfigFromEnv() (*WorkerConfig, error) {
	cfg := &WorkerConfig{
		NodeName:              getEnv("NODE_NAME", ""),
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

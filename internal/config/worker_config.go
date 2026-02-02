package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// WorkerConfig holds configuration for the worker component
type WorkerConfig struct {
	// Node identification (from environment, not YAML)
	NodeName string

	// Check configuration (from YAML)
	DNSTestDomains []string `yaml:"dnsTestDomains"`
	ClusterDNSIP   string   `yaml:"clusterDnsIp"`

	// Internal field for YAML parsing
	CheckTimeoutSeconds        int    `yaml:"checkTimeoutSeconds"`
	InitialCheckTimeoutSeconds int    `yaml:"initialCheckTimeoutSeconds"`
	MaxRetries                 int    `default:"1" yaml:"maxRetries"`
	RetryBackoff               string `default:"linear" yaml:"retryBackoff"`

	// Kubernetes service (from environment, not YAML)
	KubernetesServiceHost string
	KubernetesServicePort string

	// Logging (from YAML, nested structure)
	Logging LoggingConfig `yaml:"logging"`

	DryRun bool `yaml:"dryRun"`

	// Kubeconfig (from environment, not YAML)
	KubeconfigPath string

	// Worker mode specific (not from YAML)
	WorkerMode bool // Always true for worker
}

// LoadWorkerConfigFromFile loads worker configuration from a YAML file (ConfigMap mount)
// and environment variables. File provides check configuration, environment provides node identity.
func LoadWorkerConfigFromFile(configPath string) (*WorkerConfig, error) {
	// Read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read worker config file: %w", err)
	}

	// Parse YAML directly into WorkerConfig
	cfg := &WorkerConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse worker config file: %w", err)
	}

	// Populate environment variables (injected by controller/k8s)
	cfg.NodeName = getEnv("NODE_NAME", "")
	cfg.KubernetesServiceHost = getEnv("KUBERNETES_SERVICE_HOST", "")
	cfg.KubernetesServicePort = getEnv("KUBERNETES_SERVICE_PORT", "443")
	cfg.KubeconfigPath = getEnv("KUBECONFIG", "")
	cfg.WorkerMode = true

	// Apply defaults if not specified in file
	if len(cfg.DNSTestDomains) == 0 {
		cfg.DNSTestDomains = []string{"kubernetes.default.svc.cluster.local", "google.com"}
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

	if c.CheckTimeoutSeconds < 1 {
		return fmt.Errorf("checkTimeoutSeconds must be at least 1 second")
	}

	if c.MaxRetries < 1 {
		return fmt.Errorf("maxRetries must be at least 1")
	}

	if c.InitialCheckTimeoutSeconds < 1 {
		return fmt.Errorf("initialCheckTimeoutSeconds must be at least 1 second")
	}

	if c.RetryBackoff != "exponential" && c.RetryBackoff != "linear" {
		return fmt.Errorf("retryBackoff must be 'exponential' or 'linear'")
	}

	return nil
}

// GetCheckTimeout returns CheckTimeoutSeconds as a time.Duration
func (c *WorkerConfig) GetCheckTimeout() time.Duration {
	return time.Duration(c.CheckTimeoutSeconds) * time.Second
}

// GetInitialCheckTimeout returns InitialCheckTimeoutSeconds as a time.Duration
func (c *WorkerConfig) GetInitialCheckTimeout() time.Duration {
	return time.Duration(c.InitialCheckTimeoutSeconds) * time.Second
}

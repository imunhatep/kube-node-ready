package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// WorkerPodConfig holds worker pod configuration
type WorkerPodConfig struct {
	Image             ImageConfig     `yaml:"image"`
	Namespace         string          `yaml:"namespace"`
	TimeoutSeconds    int             `yaml:"timeoutSeconds"` // Timeout in seconds
	ServiceAccount    string          `yaml:"serviceAccount"`
	PriorityClassName string          `yaml:"priorityClassName"`
	Resources         ResourcesConfig `yaml:"resources"`

	// Worker runtime configuration (passed as environment variables)
	CheckTimeoutSeconds int      `yaml:"checkTimeoutSeconds"` // Timeout for individual checks in seconds
	DNSTestDomains      []string `yaml:"dnsTestDomains"`      // DNS domains to test
	ClusterDNSIP        string   `yaml:"clusterDnsIp"`        // Cluster DNS IP (optional)
}

// GetTimeout returns timeout as time.Duration
func (w *WorkerPodConfig) GetTimeout() time.Duration {
	return time.Duration(w.TimeoutSeconds) * time.Second
}

// GetCheckTimeout returns check timeout as time.Duration
func (w *WorkerPodConfig) GetCheckTimeout() time.Duration {
	return time.Duration(w.CheckTimeoutSeconds) * time.Second
}

// ImageConfig holds container image configuration
type ImageConfig struct {
	Repository string `yaml:"repository"`
	Tag        string `yaml:"tag"`
	PullPolicy string `yaml:"pullPolicy"`
}

// ResourcesConfig holds resource requests and limits
type ResourcesConfig struct {
	Requests ResourceRequirements `yaml:"requests"`
	Limits   ResourceRequirements `yaml:"limits"`
}

// ResourceRequirements holds CPU and memory requirements
type ResourceRequirements struct {
	CPU    string `yaml:"cpu"`
	Memory string `yaml:"memory"`
}

// ReconciliationConfig holds reconciliation settings
type ReconciliationConfig struct {
	IntervalSeconds int    `yaml:"intervalSeconds"` // Interval in seconds
	MaxRetries      int    `yaml:"maxRetries"`
	RetryBackoff    string `yaml:"retryBackoff"`
}

// GetInterval returns interval as time.Duration
func (r *ReconciliationConfig) GetInterval() time.Duration {
	return time.Duration(r.IntervalSeconds) * time.Second
}

// TaintConfig holds Kubernetes taint configuration
type TaintConfig struct {
	Key    string `yaml:"key"`
	Value  string `yaml:"value"`
	Effect string `yaml:"effect"`
}

// LabelConfig holds Kubernetes label configuration
type LabelConfig struct {
	Key   string `yaml:"key"`
	Value string `yaml:"value"`
}

// NodeManagementConfig holds node management settings
type NodeManagementConfig struct {
	DeleteFailedNodes bool          `yaml:"deleteFailedNodes"`
	Taints            []TaintConfig `yaml:"taints"`
	VerifiedLabel     LabelConfig   `yaml:"verifiedLabel"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
}

// HealthConfig holds health check configuration
type HealthConfig struct {
	Port int `yaml:"port"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// KubernetesConfig holds Kubernetes client configuration
type KubernetesConfig struct {
	KubeconfigPath string `yaml:"kubeconfigPath"`
	QPS            int    `yaml:"qps"`
	Burst          int    `yaml:"burst"`
}

// ControllerConfig holds configuration for the controller component
type ControllerConfig struct {
	Worker         WorkerPodConfig      `yaml:"worker"`
	Reconciliation ReconciliationConfig `yaml:"reconciliation"`
	NodeManagement NodeManagementConfig `yaml:"nodeManagement"`
	Metrics        MetricsConfig        `yaml:"metrics"`
	Health         HealthConfig         `yaml:"health"`
	Logging        LoggingConfig        `yaml:"logging"`
	Kubernetes     KubernetesConfig     `yaml:"kubernetes"`
}

// LoadControllerConfigFromFile loads controller configuration from a YAML file
func LoadControllerConfigFromFile(configPath string) (*ControllerConfig, error) {
	// Read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse YAML directly into ControllerConfig
	cfg := &ControllerConfig{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Validate required fields
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks if the controller configuration is valid
// Only validates absolutely required fields - defaults should be set in Helm values.yaml
func (c *ControllerConfig) Validate() error {
	// Required: worker image configuration
	if c.Worker.Image.Repository == "" {
		return fmt.Errorf("worker.image.repository is required")
	}
	if c.Worker.Image.Tag == "" {
		return fmt.Errorf("worker.image.tag is required")
	}
	if c.Worker.Namespace == "" {
		return fmt.Errorf("worker.namespace is required")
	}

	// Required: reconciliation configuration
	if c.Reconciliation.MaxRetries < 1 {
		return fmt.Errorf("reconciliation.maxRetries must be at least 1")
	}
	if c.Reconciliation.RetryBackoff != RetryBackoffExponential && c.Reconciliation.RetryBackoff != RetryBackoffLinear {
		return fmt.Errorf("reconciliation.retryBackoff must be 'exponential' or 'linear'")
	}
	if c.Reconciliation.IntervalSeconds < 1 {
		return fmt.Errorf("reconciliation.intervalSeconds must be at least 1")
	}

	// Required: timeout must be reasonable
	if c.Worker.TimeoutSeconds < 10 {
		return fmt.Errorf("worker.timeoutSeconds must be at least 10")
	}

	// Required: ports must be valid
	if c.Metrics.Port < 1 || c.Metrics.Port > 65535 {
		return fmt.Errorf("metrics.port must be between 1 and 65535")
	}
	if c.Health.Port < 1 || c.Health.Port > 65535 {
		return fmt.Errorf("health.port must be between 1 and 65535")
	}

	// Required: at least one taint must be configured
	if len(c.NodeManagement.Taints) == 0 {
		return fmt.Errorf("nodeManagement.taints must have at least one taint configured")
	}

	// Validate each taint
	for i, taint := range c.NodeManagement.Taints {
		if taint.Key == "" {
			return fmt.Errorf("nodeManagement.taints[%d].key is required", i)
		}
		validEffects := []string{"NoSchedule", "PreferNoSchedule", "NoExecute"}
		valid := false
		for _, effect := range validEffects {
			if taint.Effect == effect {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("nodeManagement.taints[%d].effect must be one of: NoSchedule, PreferNoSchedule, NoExecute", i)
		}
	}

	// Required: verified label must be configured
	if c.NodeManagement.VerifiedLabel.Key == "" {
		return fmt.Errorf("nodeManagement.verifiedLabel.key is required")
	}

	return nil
}

// GetWorkerImage returns the full worker image string (repository:tag)
func (c *ControllerConfig) GetWorkerImage() string {
	return fmt.Sprintf("%s:%s", c.Worker.Image.Repository, c.Worker.Image.Tag)
}

// Helper function to safely get environment variable with type conversion
func getEnvInt(key string, defaultValue int) (int, error) {
	if value := os.Getenv(key); value != "" {
		intVal, err := strconv.Atoi(value)
		if err != nil {
			return defaultValue, fmt.Errorf("invalid value for %s: %w", key, err)
		}
		return intVal, nil
	}
	return defaultValue, nil
}

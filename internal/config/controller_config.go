package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

const RetryBackoffExponential = "exponential"
const RetryBackoffLinear = "linear"

// JobConfig holds job-specific configuration
type JobConfig struct {
	ActiveDeadlineSeconds   *int32 `yaml:"activeDeadlineSeconds"`   // Job timeout (nil for no timeout)
	BackoffLimit            *int32 `yaml:"backoffLimit"`            // Number of retries (nil for 6)
	Completions             *int32 `yaml:"completions"`             // Number of successful completions (nil for 1)
	TTLSecondsAfterFinished *int32 `yaml:"ttlSecondsAfterFinished"` // Auto-cleanup completed jobs (nil for no cleanup)
}

// WorkerPodConfig holds worker pod configuration
// This only includes pod scheduling and lifecycle configuration.
// Worker runtime configuration (checks, DNS, etc.) is managed via separate worker ConfigMap.
type WorkerPodConfig struct {
	Image              ImageConfig           `yaml:"image"`
	Namespace          string                `yaml:"namespace"`
	ServiceAccountName string                `yaml:"serviceAccountName"`
	PriorityClassName  string                `yaml:"priorityClassName"`
	Resources          ResourcesConfig       `yaml:"resources"`
	ConfigMapName      string                `yaml:"configMapName"`  // Name of worker ConfigMap to mount
	Job                JobConfig             `yaml:"job"`            // Job-specific configuration
	Tolerations        []TolerationConfig    `yaml:"tolerations"`    // Additional tolerations for worker pods
	InitContainers     []InitContainerConfig `yaml:"initContainers"` // Custom init containers for additional verification
}

// GetTimeout returns timeout as time.Duration
// Returns 0 if ActiveDeadlineSeconds is nil (no timeout)
func (w *WorkerPodConfig) GetTimeout() time.Duration {
	if w.Job.ActiveDeadlineSeconds == nil {
		return 0
	}
	return time.Duration(*w.Job.ActiveDeadlineSeconds) * time.Second
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

// TolerationConfig holds Kubernetes toleration configuration
type TolerationConfig struct {
	Key               string `yaml:"key"`
	Operator          string `yaml:"operator,omitempty"`          // Exists (default) or Equal
	Value             string `yaml:"value,omitempty"`             // Value to match (only for Equal operator)
	Effect            string `yaml:"effect,omitempty"`            // NoSchedule, PreferNoSchedule, NoExecute, or empty for all
	TolerationSeconds *int64 `yaml:"tolerationSeconds,omitempty"` // For NoExecute effect
}

// InitContainerConfig holds Kubernetes init container configuration
type InitContainerConfig struct {
	Name            string                 `yaml:"name"`
	Image           string                 `yaml:"image"`
	Command         []string               `yaml:"command,omitempty"`
	Args            []string               `yaml:"args,omitempty"`
	Env             []EnvVarConfig         `yaml:"env,omitempty"`
	VolumeMounts    []VolumeMountConfig    `yaml:"volumeMounts,omitempty"`
	Resources       *ResourcesConfig       `yaml:"resources,omitempty"`
	SecurityContext *SecurityContextConfig `yaml:"securityContext,omitempty"`
	WorkingDir      string                 `yaml:"workingDir,omitempty"`
}

// EnvVarConfig holds environment variable configuration
type EnvVarConfig struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// VolumeMountConfig holds volume mount configuration
type VolumeMountConfig struct {
	Name      string `yaml:"name"`
	MountPath string `yaml:"mountPath"`
	ReadOnly  bool   `yaml:"readOnly,omitempty"`
	SubPath   string `yaml:"subPath,omitempty"`
}

// SecurityContextConfig holds security context configuration
type SecurityContextConfig struct {
	Privileged               *bool  `yaml:"privileged,omitempty"`
	ReadOnlyRootFilesystem   *bool  `yaml:"readOnlyRootFilesystem,omitempty"`
	RunAsNonRoot             *bool  `yaml:"runAsNonRoot,omitempty"`
	RunAsUser                *int64 `yaml:"runAsUser,omitempty"`
	RunAsGroup               *int64 `yaml:"runAsGroup,omitempty"`
	AllowPrivilegeEscalation *bool  `yaml:"allowPrivilegeEscalation,omitempty"`
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

// NodeManagementConfig holds node management configuration
type NodeManagementConfig struct {
	DeleteFailedNodes bool          `yaml:"deleteFailedNodes"`
	Taints            []TaintConfig `yaml:"taints"`
	VerifiedLabel     LabelConfig   `yaml:"verifiedLabel"`
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

	// Apply defaults for fields not specified in YAML
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
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
	// Note: worker.namespace is now optional - auto-detected from controller environment
	if c.Worker.ConfigMapName == "" {
		return fmt.Errorf("worker.configMapName is required")
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
	if c.Worker.Job.ActiveDeadlineSeconds == nil || *c.Worker.Job.ActiveDeadlineSeconds < 10 {
		return fmt.Errorf("worker.Job.ActiveDeadlineSeconds must be at least 10 seconds")
	}

	// Required: ports must be valid
	if c.Metrics.Port < 1 || c.Metrics.Port > 65535 {
		return fmt.Errorf("metrics.port must be between 1 and 65535")
	}
	if c.Health.Port < 1 || c.Health.Port > 65535 {
		return fmt.Errorf("health.port must be between 1 and 65535")
	}

	if c.Worker.ServiceAccountName == "" {
		return fmt.Errorf("worker.serviceAccountName is required")
	}

	// Required: at least one taint must be configured
	//if len(c.NodeManagement.Taints) == 0 {
	//	return fmt.Errorf("nodeManagement.taints must have at least one taint configured")
	//}

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

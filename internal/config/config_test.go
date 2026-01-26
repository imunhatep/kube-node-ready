package config

import (
	"os"
	"testing"
	"time"
)

func TestLoadFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
		want    *Config
		wantErr bool
	}{
		{
			name:    "default values",
			envVars: map[string]string{},
			want: &Config{
				NodeName:              "",
				Namespace:             "kube-system",
				InitialCheckTimeout:   300 * time.Second,
				MaxRetries:            5,
				RetryBackoff:          "exponential",
				DNSTestDomains:        []string{"kubernetes.default.svc.cluster.local", "google.com"},
				ClusterDNSIP:          "",
				KubernetesServiceHost: "",
				KubernetesServicePort: "443",
				TaintKey:              "node-ready/unverified",
				TaintValue:            "true",
				TaintEffect:           "NoSchedule",
				VerifiedLabel:         "node-ready/verified",
				VerifiedLabelValue:    "true",
				MetricsEnabled:        true,
				MetricsPort:           8080,
				LogLevel:              "info",
				LogFormat:             "json",
				DryRun:                false,
				KubeconfigPath:        "",
			},
			wantErr: false,
		},
		{
			name: "custom values",
			envVars: map[string]string{
				"NODE_NAME":               "test-node",
				"POD_NAMESPACE":           "custom-ns",
				"INITIAL_TIMEOUT":         "60s",
				"MAX_RETRIES":             "3",
				"RETRY_BACKOFF":           "linear",
				"DNS_TEST_DOMAINS":        "test1.com,test2.com",
				"KUBERNETES_SERVICE_HOST": "api.k8s.local",
				"LOG_LEVEL":               "debug",
				"LOG_FORMAT":              "console",
				"DRY_RUN":                 "true",
				"KUBECONFIG":              "/path/to/kubeconfig",
			},
			want: &Config{
				NodeName:              "test-node",
				Namespace:             "custom-ns",
				InitialCheckTimeout:   60 * time.Second,
				MaxRetries:            3,
				RetryBackoff:          "linear",
				DNSTestDomains:        []string{"test1.com", "test2.com"},
				ClusterDNSIP:          "",
				KubernetesServiceHost: "api.k8s.local",
				KubernetesServicePort: "443",
				TaintKey:              "node-ready/unverified",
				TaintValue:            "true",
				TaintEffect:           "NoSchedule",
				VerifiedLabel:         "node-ready/verified",
				VerifiedLabelValue:    "true",
				MetricsEnabled:        true,
				MetricsPort:           8080,
				LogLevel:              "debug",
				LogFormat:             "console",
				DryRun:                true,
				KubeconfigPath:        "/path/to/kubeconfig",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment
			os.Clearenv()

			// Set test environment variables
			for k, v := range tt.envVars {
				os.Setenv(k, v)
			}

			got, err := LoadFromEnv()
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadFromEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err == nil {
				if got.NodeName != tt.want.NodeName {
					t.Errorf("NodeName = %v, want %v", got.NodeName, tt.want.NodeName)
				}
				if got.Namespace != tt.want.Namespace {
					t.Errorf("Namespace = %v, want %v", got.Namespace, tt.want.Namespace)
				}
				if got.InitialCheckTimeout != tt.want.InitialCheckTimeout {
					t.Errorf("InitialCheckTimeout = %v, want %v", got.InitialCheckTimeout, tt.want.InitialCheckTimeout)
				}
				if got.MaxRetries != tt.want.MaxRetries {
					t.Errorf("MaxRetries = %v, want %v", got.MaxRetries, tt.want.MaxRetries)
				}
				if got.RetryBackoff != tt.want.RetryBackoff {
					t.Errorf("RetryBackoff = %v, want %v", got.RetryBackoff, tt.want.RetryBackoff)
				}
				if got.LogLevel != tt.want.LogLevel {
					t.Errorf("LogLevel = %v, want %v", got.LogLevel, tt.want.LogLevel)
				}
				if got.LogFormat != tt.want.LogFormat {
					t.Errorf("LogFormat = %v, want %v", got.LogFormat, tt.want.LogFormat)
				}
				if got.DryRun != tt.want.DryRun {
					t.Errorf("DryRun = %v, want %v", got.DryRun, tt.want.DryRun)
				}
				if got.KubeconfigPath != tt.want.KubeconfigPath {
					t.Errorf("KubeconfigPath = %v, want %v", got.KubeconfigPath, tt.want.KubeconfigPath)
				}
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid production config",
			config: &Config{
				NodeName:              "test-node",
				KubernetesServiceHost: "api.k8s.local",
				MaxRetries:            5,
				RetryBackoff:          "exponential",
				DNSTestDomains:        []string{"test.com"},
				DryRun:                false,
			},
			wantErr: false,
		},
		{
			name: "valid dry-run config with defaults",
			config: &Config{
				NodeName:              "",
				KubernetesServiceHost: "",
				MaxRetries:            5,
				RetryBackoff:          "exponential",
				DNSTestDomains:        []string{"test.com"},
				DryRun:                true,
			},
			wantErr: false,
		},
		{
			name: "missing NODE_NAME in production",
			config: &Config{
				NodeName:              "",
				KubernetesServiceHost: "api.k8s.local",
				MaxRetries:            5,
				RetryBackoff:          "exponential",
				DNSTestDomains:        []string{"test.com"},
				DryRun:                false,
			},
			wantErr: true,
			errMsg:  "NODE_NAME is required",
		},
		{
			name: "missing KUBERNETES_SERVICE_HOST in production",
			config: &Config{
				NodeName:              "test-node",
				KubernetesServiceHost: "",
				MaxRetries:            5,
				RetryBackoff:          "exponential",
				DNSTestDomains:        []string{"test.com"},
				DryRun:                false,
			},
			wantErr: true,
			errMsg:  "KUBERNETES_SERVICE_HOST is required",
		},
		{
			name: "invalid MaxRetries",
			config: &Config{
				NodeName:              "test-node",
				KubernetesServiceHost: "api.k8s.local",
				MaxRetries:            0,
				RetryBackoff:          "exponential",
				DNSTestDomains:        []string{"test.com"},
				DryRun:                false,
			},
			wantErr: true,
			errMsg:  "MAX_RETRIES must be at least 1",
		},
		{
			name: "invalid RetryBackoff",
			config: &Config{
				NodeName:              "test-node",
				KubernetesServiceHost: "api.k8s.local",
				MaxRetries:            5,
				RetryBackoff:          "invalid",
				DNSTestDomains:        []string{"test.com"},
				DryRun:                false,
			},
			wantErr: true,
			errMsg:  "RETRY_BACKOFF must be 'exponential' or 'linear'",
		},
		{
			name: "empty DNSTestDomains",
			config: &Config{
				NodeName:              "test-node",
				KubernetesServiceHost: "api.k8s.local",
				MaxRetries:            5,
				RetryBackoff:          "exponential",
				DNSTestDomains:        []string{},
				DryRun:                false,
			},
			wantErr: true,
			errMsg:  "DNS_TEST_DOMAINS must have at least one domain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("Config.Validate() error message = %v, want %v", err.Error(), tt.errMsg)
			}

			// Check that dry-run mode sets defaults
			if tt.config.DryRun && !tt.wantErr {
				if tt.config.NodeName == "" {
					t.Errorf("DryRun mode should set default NodeName")
				}
				if tt.config.KubernetesServiceHost == "" {
					t.Errorf("DryRun mode should set default KubernetesServiceHost")
				}
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		want         string
	}{
		{
			name:         "environment variable set",
			key:          "TEST_KEY",
			defaultValue: "default",
			envValue:     "custom",
			want:         "custom",
		},
		{
			name:         "environment variable not set",
			key:          "TEST_KEY",
			defaultValue: "default",
			envValue:     "",
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
			}

			got := getEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetIntEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue int
		envValue     string
		want         int
	}{
		{
			name:         "valid integer",
			key:          "TEST_INT",
			defaultValue: 10,
			envValue:     "42",
			want:         42,
		},
		{
			name:         "invalid integer",
			key:          "TEST_INT",
			defaultValue: 10,
			envValue:     "invalid",
			want:         10,
		},
		{
			name:         "not set",
			key:          "TEST_INT",
			defaultValue: 10,
			envValue:     "",
			want:         10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
			}

			got := getIntEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getIntEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetBoolEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue bool
		envValue     string
		want         bool
	}{
		{
			name:         "true value",
			key:          "TEST_BOOL",
			defaultValue: false,
			envValue:     "true",
			want:         true,
		},
		{
			name:         "false value",
			key:          "TEST_BOOL",
			defaultValue: true,
			envValue:     "false",
			want:         false,
		},
		{
			name:         "invalid value",
			key:          "TEST_BOOL",
			defaultValue: true,
			envValue:     "invalid",
			want:         true,
		},
		{
			name:         "not set",
			key:          "TEST_BOOL",
			defaultValue: false,
			envValue:     "",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
			}

			got := getBoolEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getBoolEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetDurationEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue time.Duration
		envValue     string
		want         time.Duration
	}{
		{
			name:         "valid duration",
			key:          "TEST_DURATION",
			defaultValue: 10 * time.Second,
			envValue:     "5m",
			want:         5 * time.Minute,
		},
		{
			name:         "invalid duration",
			key:          "TEST_DURATION",
			defaultValue: 10 * time.Second,
			envValue:     "invalid",
			want:         10 * time.Second,
		},
		{
			name:         "not set",
			key:          "TEST_DURATION",
			defaultValue: 10 * time.Second,
			envValue:     "",
			want:         10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
			}

			got := getDurationEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getDurationEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSliceEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue []string
		envValue     string
		want         []string
	}{
		{
			name:         "comma separated values",
			key:          "TEST_SLICE",
			defaultValue: []string{"default"},
			envValue:     "one,two,three",
			want:         []string{"one", "two", "three"},
		},
		{
			name:         "single value",
			key:          "TEST_SLICE",
			defaultValue: []string{"default"},
			envValue:     "single",
			want:         []string{"single"},
		},
		{
			name:         "not set",
			key:          "TEST_SLICE",
			defaultValue: []string{"default"},
			envValue:     "",
			want:         []string{"default"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
			}

			got := getSliceEnv(tt.key, tt.defaultValue)
			if len(got) != len(tt.want) {
				t.Errorf("getSliceEnv() length = %v, want %v", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getSliceEnv()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

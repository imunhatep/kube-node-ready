package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all configuration for the node checker
type Config struct {
	NodeName              string
	Namespace             string
	InitialCheckTimeout   time.Duration
	MaxRetries            int
	RetryBackoff          string
	DNSTestDomains        []string
	ClusterDNSIP          string
	KubernetesServiceHost string
	KubernetesServicePort string
	TaintKey              string
	TaintValue            string
	TaintEffect           string
	VerifiedLabel         string
	VerifiedLabelValue    string
	MetricsEnabled        bool
	MetricsPort           int
	LogLevel              string
	LogFormat             string
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		NodeName:              getEnv("NODE_NAME", ""),
		Namespace:             getEnv("POD_NAMESPACE", "kube-system"),
		InitialCheckTimeout:   getDurationEnv("INITIAL_TIMEOUT", 300*time.Second),
		MaxRetries:            getIntEnv("MAX_RETRIES", 5),
		RetryBackoff:          getEnv("RETRY_BACKOFF", "exponential"),
		DNSTestDomains:        getSliceEnv("DNS_TEST_DOMAINS", []string{"kubernetes.default.svc.cluster.local", "google.com"}),
		ClusterDNSIP:          getEnv("CLUSTER_DNS_IP", ""),
		KubernetesServiceHost: getEnv("KUBERNETES_SERVICE_HOST", ""),
		KubernetesServicePort: getEnv("KUBERNETES_SERVICE_PORT", "443"),
		TaintKey:              getEnv("TAINT_KEY", "node-ready/unverified"),
		TaintValue:            getEnv("TAINT_VALUE", "true"),
		TaintEffect:           getEnv("TAINT_EFFECT", "NoSchedule"),
		VerifiedLabel:         getEnv("VERIFIED_LABEL", "node-ready/verified"),
		VerifiedLabelValue:    getEnv("VERIFIED_LABEL_VALUE", "true"),
		MetricsEnabled:        getBoolEnv("ENABLE_METRICS", true),
		MetricsPort:           getIntEnv("METRICS_PORT", 8080),
		LogLevel:              getEnv("LOG_LEVEL", "info"),
		LogFormat:             getEnv("LOG_FORMAT", "json"),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.NodeName == "" {
		return fmt.Errorf("NODE_NAME is required")
	}

	if c.MaxRetries < 1 {
		return fmt.Errorf("MAX_RETRIES must be at least 1")
	}

	if c.RetryBackoff != "exponential" && c.RetryBackoff != "linear" {
		return fmt.Errorf("RETRY_BACKOFF must be 'exponential' or 'linear'")
	}

	if len(c.DNSTestDomains) == 0 {
		return fmt.Errorf("DNS_TEST_DOMAINS must have at least one domain")
	}

	if c.KubernetesServiceHost == "" {
		return fmt.Errorf("KUBERNETES_SERVICE_HOST is required")
	}

	return nil
}

// Helper functions for environment variable parsing

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getBoolEnv(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

func getSliceEnv(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}

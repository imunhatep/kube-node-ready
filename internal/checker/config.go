package checker

import "time"

// CheckerConfig holds configuration for the verification checker
type CheckerConfig struct {
	// Node identification
	NodeName string

	// Check timeouts
	CheckTimeout time.Duration

	// Retry configuration
	MaxRetries   int
	RetryBackoff string // "exponential" or "linear"

	// DNS check configuration
	DNSTestDomains []string

	// Kubernetes service configuration
	KubernetesServiceHost string
	KubernetesServicePort string
}

// NewCheckerConfig creates a CheckerConfig with sensible defaults
func NewCheckerConfig() *CheckerConfig {
	return &CheckerConfig{
		CheckTimeout:          10 * time.Second,
		MaxRetries:            5,
		RetryBackoff:          "exponential",
		DNSTestDomains:        []string{"kubernetes.default.svc.cluster.local", "google.com"},
		KubernetesServicePort: "443",
	}
}

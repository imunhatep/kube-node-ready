package k8sclient

// ClientConfig holds configuration for creating Kubernetes clients
type ClientConfig struct {
	// Mode configuration
	DryRun bool

	// Kubeconfig path for out-of-cluster usage
	KubeconfigPath string

	// In-cluster configuration (optional, auto-detected if empty)
	KubernetesServiceHost string
	KubernetesServicePort string
}

// NewClientConfig creates a ClientConfig with sensible defaults
func NewClientConfig() *ClientConfig {
	return &ClientConfig{
		DryRun:                false,
		KubeconfigPath:        "",
		KubernetesServiceHost: "",
		KubernetesServicePort: "443",
	}
}

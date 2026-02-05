package config

import (
	"github.com/imunhatep/kube-node-ready/internal/checker"
	"github.com/imunhatep/kube-node-ready/internal/k8sclient"
)

// NewClientConfigFromWorkerConfig converts WorkerConfig to k8sclient.ClientConfig
func NewClientConfigFromWorkerConfig(c *WorkerConfig) *k8sclient.ClientConfig {
	return &k8sclient.ClientConfig{
		DryRun:                c.DryRun,
		KubeconfigPath:        c.KubeconfigPath,
		KubernetesServiceHost: c.KubernetesServiceHost,
		KubernetesServicePort: c.KubernetesServicePort,
	}
}

// NewCheckerConfigFromWorkerConfig converts WorkerConfig to checker.CheckerConfig
func NewCheckerConfigFromWorkerConfig(c *WorkerConfig) *checker.CheckerConfig {
	return &checker.CheckerConfig{
		NodeName:              c.NodeName,
		CheckTimeout:          c.GetCheckTimeout(),
		InitialCheckTimeout:   c.GetInitialCheckTimeout(),
		MaxRetries:            c.MaxRetries,
		RetryBackoff:          c.RetryBackoff,
		DNSTestDomains:        c.DNSTestDomains,
		ClusterDNSIP:          c.ClusterDNSIP,
		KubernetesServiceHost: c.KubernetesServiceHost,
		KubernetesServicePort: c.KubernetesServicePort,
	}
}

package k8sclient

import (
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/imunhatep/kube-node-ready/internal/config"
)

// CreateClient creates a Kubernetes client based on the configuration mode
func CreateClient(cfg *config.Config) (*kubernetes.Clientset, error) {
	if cfg.DryRun {
		return createClientForDryRun(cfg)
	}

	// If kubeconfig is provided, use it (for testing in production-like mode)
	if cfg.KubeconfigPath != "" {
		clientset, err := createClientFromKubeconfig(cfg.KubeconfigPath)
		if err != nil {
			klog.ErrorS(err, "Failed to create Kubernetes client from kubeconfig")
			return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
		}
		return clientset, nil
	}

	// Otherwise, use in-cluster config (standard production deployment)
	clientset, err := createClientInCluster()
	if err != nil {
		klog.ErrorS(err, "Failed to create in-cluster Kubernetes client")
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return clientset, nil
}

// createClientForDryRun attempts to create a Kubernetes client for dry-run mode
// Returns nil without error if kubeconfig is not available (graceful degradation)
func createClientForDryRun(cfg *config.Config) (*kubernetes.Clientset, error) {
	kubeconfigPath := resolveKubeconfigPath(cfg.KubeconfigPath)

	// Check if kubeconfig exists
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		klog.InfoS("Dry-run mode: kubeconfig not found, running network checks only", "kubeconfigPath", kubeconfigPath)
		klog.Info("Kubernetes API checks will be skipped")
		return nil, nil
	}

	// Try to create client
	clientset, err := createClientFromKubeconfig(kubeconfigPath)
	if err != nil {
		klog.InfoS("Dry-run mode: failed to create Kubernetes client", "error", err, "kubeconfigPath", kubeconfigPath)
		klog.Info("Continuing with network checks only")
		return nil, nil
	}

	return clientset, nil
}

// createClientFromKubeconfig creates a client from a kubeconfig file
func createClientFromKubeconfig(kubeconfigPath string) (*kubernetes.Clientset, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}

// createClientInCluster creates a client using in-cluster configuration
func createClientInCluster() (*kubernetes.Clientset, error) {
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}

// resolveKubeconfigPath resolves the kubeconfig path from multiple sources
func resolveKubeconfigPath(configuredPath string) string {
	if configuredPath != "" {
		return configuredPath
	}

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath != "" {
		return kubeconfigPath
	}

	homeDir, _ := os.UserHomeDir()
	return homeDir + "/.kube/config"
}

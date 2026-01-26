package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"github.com/imunhatep/kube-node-ready/pkg/checker"
	"github.com/imunhatep/kube-node-ready/pkg/config"
	"github.com/imunhatep/kube-node-ready/pkg/node"
)

func main() {
	// Initialize klog to output to stderr (standard for Kubernetes)
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Set("v", "0")

	app := &cli.Command{
		Name:    "kube-node-ready",
		Usage:   "Kubernetes node readiness verification tool",
		Version: "1.0.0",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "dry-run",
				Usage:   "Run in dry-run mode (does not modify node taints/labels)",
				Value:   false,
				Sources: cli.EnvVars("DRY_RUN"),
			},
			&cli.BoolFlag{
				Name:    "skip-k8s",
				Usage:   "Skip Kubernetes client creation (for testing only, implies dry-run)",
				Value:   false,
				Sources: cli.EnvVars("SKIP_K8S"),
			},
			&cli.IntFlag{
				Name:    "max-retries",
				Usage:   "Maximum number of retry attempts for verification checks",
				Value:   0,
				Sources: cli.EnvVars("MAX_RETRIES"),
			},
			&cli.StringFlag{
				Name:    "retry-backoff",
				Usage:   "Retry backoff strategy: exponential or linear",
				Value:   config.RetryBackoffExponential,
				Sources: cli.EnvVars("RETRY_BACKOFF"),
			},
			&cli.StringFlag{
				Name:    "kubeconfig",
				Usage:   "Path to kubeconfig file (for dry-run mode)",
				Value:   "",
				Sources: cli.EnvVars("KUBECONFIG"),
			},
		},
		Action: run,
	}

	defer klog.Flush()

	if err := app.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	// Get flag values
	dryRun := cmd.Bool("dry-run")
	skipK8s := cmd.Bool("skip-k8s")
	maxRetries := cmd.Int("max-retries")
	retryBackoff := cmd.String("retry-backoff")
	kubeconfig := cmd.String("kubeconfig")

	// If skip-k8s is set, enable dry-run
	if skipK8s {
		dryRun = true
	}

	klog.Info("Starting kube-node-ready")

	// Load configuration from environment
	cfg, err := config.LoadFromEnv()
	if err != nil {
		klog.ErrorS(err, "Failed to load configuration")
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Override with command-line flags
	if dryRun {
		cfg.DryRun = true
	}
	if maxRetries > 0 {
		cfg.MaxRetries = maxRetries
	}
	if retryBackoff != "" {
		cfg.RetryBackoff = retryBackoff
	}
	if kubeconfig != "" {
		cfg.KubeconfigPath = kubeconfig
	}

	// Validate configuration after applying flags
	if err := cfg.Validate(); err != nil {
		klog.ErrorS(err, "Configuration validation failed")
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	klog.InfoS("Configuration loaded",
		"nodeName", cfg.NodeName,
		"namespace", cfg.Namespace,
		"timeout", cfg.InitialCheckTimeout,
		"maxRetries", cfg.MaxRetries,
		"retryBackoff", cfg.RetryBackoff,
		"dryRun", cfg.DryRun,
		"skipK8s", skipK8s,
	)

	// In dry-run mode without skip-k8s, still try to create client if kubeconfig is available
	// But skip client creation entirely if skip-k8s is set
	var clientset *kubernetes.Clientset

	if skipK8s {
		klog.Info("Running with --skip-k8s flag: Kubernetes client will not be created")
		klog.Info("This mode is for testing configuration only. No network checks will be performed.")
		klog.Info("Configuration validation successful")
		os.Exit(0)
	}

	// For dry-run mode, only try to create client if kubeconfig is explicitly provided or exists
	if cfg.DryRun {
		kubeconfigPath := cfg.KubeconfigPath
		if kubeconfigPath == "" {
			kubeconfigPath = os.Getenv("KUBECONFIG")
			if kubeconfigPath == "" {
				homeDir, _ := os.UserHomeDir()
				kubeconfigPath = homeDir + "/.kube/config"
			}
		}

		// Check if kubeconfig exists
		if _, statErr := os.Stat(kubeconfigPath); os.IsNotExist(statErr) {
			klog.InfoS("Dry-run mode: kubeconfig not found, skipping Kubernetes client creation",
				"kubeconfigPath", kubeconfigPath)
			klog.Info("To test with a real cluster, provide --kubeconfig or use --skip-k8s to suppress this warning")
			os.Exit(0)
		}

		clientset, err = createKubernetesClient(cfg.DryRun, cfg.KubeconfigPath)
		if err != nil {
			klog.InfoS("Dry-run mode: failed to create Kubernetes client",
				"error", err)
			klog.Info("Continuing with network checks only (DNS, connectivity)")
			klog.Info("Kubernetes API checks will be skipped")
			// clientset remains nil, checker will handle this
		}
	} else {
		// Production mode - client is required
		clientset, err = createKubernetesClient(cfg.DryRun, cfg.KubeconfigPath)
		if err != nil {
			klog.ErrorS(err, "Failed to create Kubernetes client")
			return fmt.Errorf("failed to create Kubernetes client: %w", err)
		}
	}

	if clientset != nil {
		klog.Info("Kubernetes client created successfully")
	} else {
		klog.Info("Running without Kubernetes client (network checks only)")
	}

	// Create context with timeout and cancellation
	runCtx, cancel := context.WithTimeout(ctx, cfg.InitialCheckTimeout)
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		klog.InfoS("Received signal, shutting down", "signal", sig.String())
		cancel()
	}()

	// Create checker
	chk := checker.NewChecker(cfg, clientset)

	// Run verification checks with retry
	klog.Info("Starting verification checks")
	if err := chk.RunWithRetry(runCtx); err != nil {
		klog.ErrorS(err, "Verification failed")
		return fmt.Errorf("verification failed: %w", err)
	}

	klog.Info("All verification checks passed")

	// Remove taint and add verified label (only if not in dry-run and clientset is available)
	if cfg.DryRun {
		klog.Info("Dry-run mode: skipping node taint/label updates")
		klog.Info("In production mode, node would be marked as verified")
	} else if clientset == nil {
		klog.Info("No Kubernetes client available: skipping node updates")
	} else {
		klog.Info("Updating node status")
		nodeManager := node.NewManager(cfg, clientset)
		if err := nodeManager.RemoveTaintAndAddLabel(context.Background()); err != nil {
			klog.ErrorS(err, "Failed to update node")
			return fmt.Errorf("failed to update node: %w", err)
		}
	}

	klog.InfoS("Node verification completed successfully",
		"node", cfg.NodeName,
		"label", cfg.VerifiedLabel,
	)

	// Give Kubernetes time to detect the label change before pod terminates
	time.Sleep(2 * time.Second)

	klog.Info("Exiting successfully")
	return nil
}

// createKubernetesClient creates a Kubernetes client (in-cluster or from kubeconfig)
func createKubernetesClient(dryRun bool, kubeconfigPath string) (*kubernetes.Clientset, error) {
	var restConfig *rest.Config
	var err error

	if dryRun && kubeconfigPath != "" {
		// Use kubeconfig file for dry-run mode
		restConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create config from kubeconfig: %w", err)
		}
	} else if dryRun {
		// Dry-run without kubeconfig - try default kubeconfig location
		kubeconfigPath = os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			homeDir, _ := os.UserHomeDir()
			kubeconfigPath = homeDir + "/.kube/config"
		}

		// Check if kubeconfig file exists
		if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("kubeconfig file not found at %s. For dry-run mode, either:\n  1. Provide --kubeconfig=/path/to/kubeconfig\n  2. Use --skip-k8s to test configuration only\n  3. Set up a valid kubeconfig at %s", kubeconfigPath, kubeconfigPath)
		}

		restConfig, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create config from default kubeconfig (%s): %w", kubeconfigPath, err)
		}
	} else {
		// Use in-cluster config for production mode
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
		}
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}

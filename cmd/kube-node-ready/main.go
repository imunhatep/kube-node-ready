package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/imunhatep/kube-node-ready/pkg/checker"
	"github.com/imunhatep/kube-node-ready/pkg/config"
	"github.com/imunhatep/kube-node-ready/pkg/node"
)

func main() {
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
				Name:    "log-level",
				Usage:   "Log level (debug, info, warn, error)",
				Value:   "",
				Sources: cli.EnvVars("LOG_LEVEL"),
			},
			&cli.StringFlag{
				Name:    "log-format",
				Usage:   "Log format (json, console)",
				Value:   "",
				Sources: cli.EnvVars("LOG_FORMAT"),
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
	logLevel := cmd.String("log-level")
	logFormat := cmd.String("log-format")
	kubeconfig := cmd.String("kubeconfig")

	// If skip-k8s is set, enable dry-run
	if skipK8s {
		dryRun = true
	}

	// Create logger with initial settings
	initialLogLevel := "info"
	initialLogFormat := "json"
	if logLevel != "" {
		initialLogLevel = logLevel
	}
	if logFormat != "" {
		initialLogFormat = logFormat
	}

	logger, err := createLogger(initialLogLevel, initialLogFormat)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("Starting kube-node-ready")

	// Load configuration from environment
	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
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
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}
	if logFormat != "" {
		cfg.LogFormat = logFormat
	}
	if kubeconfig != "" {
		cfg.KubeconfigPath = kubeconfig
	}

	// Validate configuration after applying flags
	if err := cfg.Validate(); err != nil {
		logger.Fatal("Configuration validation failed", zap.Error(err))
	}

	// Update logger with configured log level
	logger, err = createLogger(cfg.LogLevel, cfg.LogFormat)
	if err != nil {
		logger.Fatal("Failed to update logger", zap.Error(err))
	}
	defer logger.Sync()

	logger.Info("Configuration loaded",
		zap.String("nodeName", cfg.NodeName),
		zap.String("namespace", cfg.Namespace),
		zap.Duration("timeout", cfg.InitialCheckTimeout),
		zap.Int("maxRetries", cfg.MaxRetries),
		zap.String("retryBackoff", cfg.RetryBackoff),
		zap.Bool("dryRun", cfg.DryRun),
		zap.Bool("skipK8s", skipK8s),
	)

	// In dry-run mode without skip-k8s, still try to create client if kubeconfig is available
	// But skip client creation entirely if skip-k8s is set
	var clientset *kubernetes.Clientset

	if skipK8s {
		logger.Warn("Running with --skip-k8s flag: Kubernetes client will not be created")
		logger.Info("This mode is for testing configuration only. No network checks will be performed.")
		logger.Info("Configuration validation successful")
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
			logger.Warn("Dry-run mode: kubeconfig not found, skipping Kubernetes client creation",
				zap.String("kubeconfigPath", kubeconfigPath))
			logger.Info("To test with a real cluster, provide --kubeconfig or use --skip-k8s to suppress this warning")
			os.Exit(0)
		}

		clientset, err = createKubernetesClient(cfg.DryRun, cfg.KubeconfigPath)
		if err != nil {
			logger.Warn("Dry-run mode: failed to create Kubernetes client",
				zap.Error(err))
			logger.Info("Continuing with network checks only (DNS, connectivity)")
			logger.Info("Kubernetes API checks will be skipped")
			// clientset remains nil, checker will handle this
		}
	} else {
		// Production mode - client is required
		clientset, err = createKubernetesClient(cfg.DryRun, cfg.KubeconfigPath)
		if err != nil {
			logger.Fatal("Failed to create Kubernetes client", zap.Error(err))
		}
	}

	if clientset != nil {
		logger.Info("Kubernetes client created successfully")
	} else {
		logger.Info("Running without Kubernetes client (network checks only)")
	}

	// Create context with timeout and cancellation
	runCtx, cancel := context.WithTimeout(ctx, cfg.InitialCheckTimeout)
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		logger.Info("Received signal, shutting down", zap.String("signal", sig.String()))
		cancel()
	}()

	// Create checker
	chk := checker.NewChecker(logger, cfg, clientset)

	// Run verification checks with retry
	logger.Info("Starting verification checks")
	if err := chk.RunWithRetry(runCtx); err != nil {
		logger.Fatal("Verification failed", zap.Error(err))
	}

	logger.Info("All verification checks passed")

	// Remove taint and add verified label (only if not in dry-run and clientset is available)
	if cfg.DryRun {
		logger.Info("Dry-run mode: skipping node taint/label updates")
		logger.Info("In production mode, node would be marked as verified")
	} else if clientset == nil {
		logger.Warn("No Kubernetes client available: skipping node updates")
	} else {
		logger.Info("Updating node status")
		nodeManager := node.NewManager(logger, cfg, clientset)
		if err := nodeManager.RemoveTaintAndAddLabel(context.Background()); err != nil {
			logger.Fatal("Failed to update node", zap.Error(err))
		}
	}

	logger.Info("Node verification completed successfully",
		zap.String("node", cfg.NodeName),
		zap.String("label", cfg.VerifiedLabel),
	)

	// Give Kubernetes time to detect the label change before pod terminates
	time.Sleep(2 * time.Second)

	logger.Info("Exiting successfully")
	return nil
}

// createLogger creates a zap logger with the specified level and format
func createLogger(level, format string) (*zap.Logger, error) {
	// Parse log level
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	// Create config
	var loggerConfig zap.Config
	if format == "json" {
		loggerConfig = zap.NewProductionConfig()
	} else {
		loggerConfig = zap.NewDevelopmentConfig()
	}

	loggerConfig.Level = zap.NewAtomicLevelAt(zapLevel)
	loggerConfig.EncoderConfig.TimeKey = "timestamp"
	loggerConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	return loggerConfig.Build()
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

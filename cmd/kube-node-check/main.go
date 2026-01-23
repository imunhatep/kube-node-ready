package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/imunhatep/kube-node-ready/pkg/checker"
	"github.com/imunhatep/kube-node-ready/pkg/config"
	"github.com/imunhatep/kube-node-ready/pkg/node"
)

func main() {
	// Create logger
	logger, err := createLogger("info", "json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Info("Starting kube-node-ready")

	// Load configuration
	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
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
	)

	// Create Kubernetes client
	clientset, err := createKubernetesClient()
	if err != nil {
		logger.Fatal("Failed to create Kubernetes client", zap.Error(err))
	}

	logger.Info("Kubernetes client created successfully")

	// Create context with timeout and cancellation
	ctx, cancel := context.WithTimeout(context.Background(), cfg.InitialCheckTimeout)
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
	if err := chk.RunWithRetry(ctx); err != nil {
		logger.Fatal("Verification failed", zap.Error(err))
	}

	logger.Info("All verification checks passed")

	// Remove taint and add verified label
	logger.Info("Updating node status")
	nodeManager := node.NewManager(logger, cfg, clientset)
	if err := nodeManager.RemoveTaintAndAddLabel(context.Background()); err != nil {
		logger.Fatal("Failed to update node", zap.Error(err))
	}

	logger.Info("Node verification completed successfully",
		zap.String("node", cfg.NodeName),
		zap.String("label", cfg.VerifiedLabel),
	)

	// Give Kubernetes time to detect the label change before pod terminates
	time.Sleep(2 * time.Second)

	logger.Info("Exiting successfully")
	os.Exit(0)
}

// loadConfiguration loads configuration with dry-run mode support
func loadConfiguration(dryRun bool) (*config.Config, error) {
	cfg, err := config.LoadFromEnv()
	if err != nil && !dryRun {
		return nil, err
	}

	// In dry-run mode, provide sensible defaults if env vars are missing
	if dryRun && err != nil {
		cfg = &config.Config{
			NodeName:              getEnvOrDefault("NODE_NAME", "dry-run-node"),
			Namespace:             getEnvOrDefault("POD_NAMESPACE", "default"),
			InitialCheckTimeout:   300 * time.Second,
			MaxRetries:            5,
			RetryBackoff:          "exponential",
			DNSTestDomains:        []string{"kubernetes.default.svc.cluster.local", "google.com"},
			KubernetesServiceHost: getEnvOrDefault("KUBERNETES_SERVICE_HOST", "kubernetes.default.svc.cluster.local"),
			KubernetesServicePort: getEnvOrDefault("KUBERNETES_SERVICE_PORT", "443"),
			TaintKey:              "node-ready/unverified",
			TaintValue:            "true",
			TaintEffect:           "NoSchedule",
			VerifiedLabel:         "node-ready/verified",
			VerifiedLabelValue:    "true",
			MetricsEnabled:        true,
			MetricsPort:           8080,
			LogLevel:              "info",
			LogFormat:             "console",
		}
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
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
	var config *rest.Config
	var err error

	if dryRun && kubeconfigPath != "" {
		// Use kubeconfig file for dry-run mode
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create config from kubeconfig: %w", err)
		}
	} else if dryRun {
		// Dry-run without kubeconfig - use default kubeconfig location
		kubeconfigPath = os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			homeDir, _ := os.UserHomeDir()
			kubeconfigPath = homeDir + "/.kube/config"
		}
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create config from default kubeconfig (%s): %w", kubeconfigPath, err)
		}
	} else {
		// Use in-cluster config for production mode
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster config: %w", err)
		}
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}

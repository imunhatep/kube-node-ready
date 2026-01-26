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
	"k8s.io/klog/v2"

	"github.com/imunhatep/kube-node-ready/pkg/checker"
	"github.com/imunhatep/kube-node-ready/pkg/config"
	"github.com/imunhatep/kube-node-ready/pkg/k8sclient"
	"github.com/imunhatep/kube-node-ready/pkg/metrics"
	"github.com/imunhatep/kube-node-ready/pkg/node"
)

// Build-time variables that can be set using ldflags:
// go build -ldflags "-X main.version=1.2.3 -X main.commitHash=abc123 -X main.buildDate=2024-01-01T00:00:00Z"
var (
	version    = "dev"
	commitHash = "unknown"
	buildDate  = "unknown"
)

func main() {
	// Initialize klog flags (will be set from CLI later)
	klog.InitFlags(nil)

	app := &cli.Command{
		Name:    "kube-node-ready",
		Usage:   "Kubernetes node readiness verification tool",
		Version: version,
		Flags: []cli.Flag{
			// Node identification
			&cli.StringFlag{
				Name:    "node-name",
				Usage:   "Name of the node to verify (required in production mode)",
				Sources: cli.EnvVars("NODE_NAME"),
			},
			&cli.StringFlag{
				Name:    "namespace",
				Usage:   "Namespace for the pod",
				Value:   "kube-system",
				Sources: cli.EnvVars("POD_NAMESPACE"),
			},

			// Timeouts and retries
			&cli.DurationFlag{
				Name:    "initial-timeout",
				Usage:   "Maximum time for initial verification",
				Value:   300 * time.Second,
				Sources: cli.EnvVars("INITIAL_TIMEOUT"),
			},
			&cli.DurationFlag{
				Name:    "check-timeout",
				Usage:   "Timeout for individual checks (DNS, network, Kubernetes API)",
				Value:   3 * time.Second,
				Sources: cli.EnvVars("CHECK_TIMEOUT"),
			},
			&cli.IntFlag{
				Name:    "max-retries",
				Usage:   "Maximum number of retry attempts for verification checks",
				Value:   5,
				Sources: cli.EnvVars("MAX_RETRIES"),
			},
			&cli.StringFlag{
				Name:    "retry-backoff",
				Usage:   "Retry backoff strategy: exponential or linear",
				Value:   "exponential",
				Sources: cli.EnvVars("RETRY_BACKOFF"),
			},

			// DNS configuration
			&cli.StringSliceFlag{
				Name:    "dns-test-domains",
				Usage:   "DNS domains to test (comma-separated)",
				Value:   []string{"kubernetes.default.svc.cluster.local", "google.com"},
				Sources: cli.EnvVars("DNS_TEST_DOMAINS"),
			},
			&cli.StringFlag{
				Name:    "cluster-dns-ip",
				Usage:   "Cluster DNS IP address",
				Sources: cli.EnvVars("CLUSTER_DNS_IP"),
			},

			// Kubernetes service
			&cli.StringFlag{
				Name:    "k8s-service-host",
				Usage:   "Kubernetes API server host",
				Sources: cli.EnvVars("KUBERNETES_SERVICE_HOST"),
			},
			&cli.StringFlag{
				Name:    "k8s-service-port",
				Usage:   "Kubernetes API server port",
				Value:   "443",
				Sources: cli.EnvVars("KUBERNETES_SERVICE_PORT"),
			},

			// Taint and label configuration
			&cli.StringFlag{
				Name:    "taint-key",
				Usage:   "Taint key to remove on success",
				Value:   "node-ready/unverified",
				Sources: cli.EnvVars("TAINT_KEY"),
			},
			&cli.StringFlag{
				Name:    "taint-value",
				Usage:   "Taint value",
				Value:   "true",
				Sources: cli.EnvVars("TAINT_VALUE"),
			},
			&cli.StringFlag{
				Name:    "taint-effect",
				Usage:   "Taint effect",
				Value:   "NoSchedule",
				Sources: cli.EnvVars("TAINT_EFFECT"),
			},
			&cli.StringFlag{
				Name:    "verified-label",
				Usage:   "Label to add on success",
				Value:   "node-ready/verified",
				Sources: cli.EnvVars("VERIFIED_LABEL"),
			},
			&cli.StringFlag{
				Name:    "verified-label-value",
				Usage:   "Verified label value",
				Value:   "true",
				Sources: cli.EnvVars("VERIFIED_LABEL_VALUE"),
			},

			// Metrics
			&cli.BoolFlag{
				Name:    "enable-metrics",
				Usage:   "Enable Prometheus metrics endpoint",
				Value:   true,
				Sources: cli.EnvVars("ENABLE_METRICS"),
			},
			&cli.IntFlag{
				Name:    "metrics-port",
				Usage:   "Metrics port",
				Value:   8080,
				Sources: cli.EnvVars("METRICS_PORT"),
			},

			// Logging
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "Log level (debug/info/warn/error)",
				Value:   "info",
				Sources: cli.EnvVars("LOG_LEVEL"),
			},
			&cli.StringFlag{
				Name:    "log-format",
				Usage:   "Log format (json/console)",
				Value:   "json",
				Sources: cli.EnvVars("LOG_FORMAT"),
			},
			&cli.IntFlag{
				Name:    "klog-verbosity",
				Usage:   "Klog verbosity level (0-10, higher is more verbose)",
				Value:   0,
				Sources: cli.EnvVars("KLOG_VERBOSITY"),
			},

			// Mode flags
			&cli.BoolFlag{
				Name:    "dry-run",
				Usage:   "Run in dry-run mode (does not modify node taints/labels, performs all checks)",
				Value:   false,
				Sources: cli.EnvVars("DRY_RUN"),
			},
			&cli.StringFlag{
				Name:    "kubeconfig",
				Usage:   "Path to kubeconfig file (for out-of-cluster usage)",
				Sources: cli.EnvVars("KUBECONFIG"),
			},
		},
		Action: run,
	}

	defer klog.Flush()

	if err := app.Run(context.Background(), os.Args); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd *cli.Command) error {
	// Configure klog from CLI flags
	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("v", fmt.Sprintf("%d", cmd.Int("klog-verbosity")))

	klog.InfoS("Starting kube-node-ready",
		"version", version,
		"commit", commitHash,
		"buildDate", buildDate,
	)

	// Set build info for Prometheus metrics
	metrics.SetBuildInfo(version, commitHash, buildDate)

	// Build configuration directly from CLI flags (which already include env vars)
	cfg := &config.Config{
		NodeName:              cmd.String("node-name"),
		Namespace:             cmd.String("namespace"),
		InitialCheckTimeout:   cmd.Duration("initial-timeout"),
		CheckTimeout:          cmd.Duration("check-timeout"),
		MaxRetries:            cmd.Int("max-retries"),
		RetryBackoff:          cmd.String("retry-backoff"),
		DNSTestDomains:        cmd.StringSlice("dns-test-domains"),
		ClusterDNSIP:          cmd.String("cluster-dns-ip"),
		KubernetesServiceHost: cmd.String("k8s-service-host"),
		KubernetesServicePort: cmd.String("k8s-service-port"),
		TaintKey:              cmd.String("taint-key"),
		TaintValue:            cmd.String("taint-value"),
		TaintEffect:           cmd.String("taint-effect"),
		VerifiedLabel:         cmd.String("verified-label"),
		VerifiedLabelValue:    cmd.String("verified-label-value"),
		MetricsEnabled:        cmd.Bool("enable-metrics"),
		MetricsPort:           cmd.Int("metrics-port"),
		LogLevel:              cmd.String("log-level"),
		LogFormat:             cmd.String("log-format"),
		DryRun:                cmd.Bool("dry-run"),
		KubeconfigPath:        cmd.String("kubeconfig"),
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		klog.ErrorS(err, "Configuration validation failed")
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	klog.InfoS("Configuration loaded",
		"nodeName", cfg.NodeName,
		"namespace", cfg.Namespace,
		"initialTimeout", cfg.InitialCheckTimeout,
		"checkTimeout", cfg.CheckTimeout,
		"maxRetries", cfg.MaxRetries,
		"retryBackoff", cfg.RetryBackoff,
		"dryRun", cfg.DryRun,
		"metricsEnabled", cfg.MetricsEnabled,
		"metricsPort", cfg.MetricsPort,
	)

	// Start metrics server if enabled
	var metricsServer *metrics.Server
	if cfg.MetricsEnabled {
		metricsServer = metrics.NewServer(cfg.MetricsPort)
		if err := metricsServer.Start(); err != nil {
			klog.ErrorS(err, "Failed to start metrics server")
			return fmt.Errorf("failed to start metrics server: %w", err)
		}
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := metricsServer.Shutdown(shutdownCtx); err != nil {
				klog.ErrorS(err, "Error shutting down metrics server")
			}
		}()
	}

	// Create Kubernetes client
	clientset, err := k8sclient.CreateClient(cfg)
	if err != nil {
		return err
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
		nodeManager := node.NewController(cfg, clientset)
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

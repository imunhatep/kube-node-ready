package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/imunhatep/kube-node-ready/internal/checker"
	"github.com/imunhatep/kube-node-ready/internal/config"
	"github.com/imunhatep/kube-node-ready/internal/k8sclient"
)

// Build-time variables
var (
	version    = "dev"
	commitHash = "unknown"
	buildDate  = "unknown"
)

// Exit codes
const (
	ExitSuccess         = 0  // All checks passed
	ExitChecksFailed    = 1  // One or more checks failed
	ExitConfigError     = 2  // Configuration error
	ExitClientError     = 3  // Kubernetes client error
	ExitUnexpectedError = 10 // Unexpected error
)

func main() {
	// Initialize klog flags
	klog.InitFlags(nil)
	flag.Parse()

	exitCode := run()
	os.Exit(exitCode)
}

func run() int {
	klog.InfoS("Starting kube-node-ready worker",
		"version", version,
		"commitHash", commitHash,
		"buildDate", buildDate,
	)

	// Load configuration
	cfg, err := config.LoadWorkerConfigFromEnv()
	if err != nil {
		klog.ErrorS(err, "Failed to load configuration")
		return ExitConfigError
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		klog.ErrorS(err, "Invalid configuration")
		return ExitConfigError
	}

	klog.InfoS("Worker configuration loaded",
		"nodeName", cfg.NodeName,
		"namespace", cfg.Namespace,
		"checkTimeout", cfg.CheckTimeout,
		"dnsTestDomains", cfg.DNSTestDomains,
	)

	// Convert to legacy Config format for compatibility with existing checker
	legacyCfg := cfg.ToConfig()

	// Create Kubernetes client
	clientset, err := k8sclient.CreateClient(legacyCfg)
	if err != nil {
		klog.ErrorS(err, "Failed to create Kubernetes client")
		return ExitClientError
	}

	if clientset != nil {
		klog.Info("Kubernetes client created successfully")
	} else {
		klog.Warning("Running without Kubernetes client (network checks only)")
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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
	chk := checker.NewChecker(legacyCfg, clientset)

	// Run verification checks (single attempt - no retries in worker mode)
	klog.InfoS("Starting verification checks", "node", cfg.NodeName)
	startTime := time.Now()

	if err := chk.RunAllChecks(ctx); err != nil {
		duration := time.Since(startTime)
		klog.ErrorS(err, "Verification checks failed",
			"node", cfg.NodeName,
			"duration", duration,
		)
		return ExitChecksFailed
	}

	duration := time.Since(startTime)
	klog.InfoS("All verification checks passed",
		"node", cfg.NodeName,
		"duration", duration,
	)

	return ExitSuccess
}

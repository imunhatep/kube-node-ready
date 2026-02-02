package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/imunhatep/kube-node-ready/internal/config"
	"github.com/imunhatep/kube-node-ready/internal/controller"
	"github.com/imunhatep/kube-node-ready/internal/metrics"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

// Build-time variables
var (
	version    = "dev"
	commitHash = "unknown"
	buildDate  = "unknown"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool
	var leaderElectionNamespace string
	var leaderElectionID string
	var configPath string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", true,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&leaderElectionNamespace, "leader-election-namespace", "",
		"Namespace for leader election lease (defaults to controller namespace)")
	flag.StringVar(&leaderElectionID, "leader-election-id", "kube-node-ready-controller",
		"Leader election lease name")
	flag.StringVar(&configPath, "config", "/etc/kube-node-ready/config.yaml", "Path to configuration file")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)

	// Initialize klog flags
	klog.InitFlags(nil)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog.Info("Starting kube-node-ready controller",
		"version", version,
		"commitHash", commitHash,
		"buildDate", buildDate,
	)

	// Initialize metrics
	metrics.SetBuildInfo(version, commitHash, buildDate)
	metrics.SetControllerHealthy(true)
	metrics.SetControllerStartTime(float64(time.Now().Unix()))
	metrics.SetLeaderElectionLeaseDuration(15.0) // Will be updated from actual config

	// Load configuration from file (ConfigMap mount)
	setupLog.Info("Loading configuration from file", "path", configPath)
	cfg, err := config.LoadControllerConfigFromFile(configPath)
	if err != nil {
		setupLog.Error(err, "Failed to load configuration from file")
		metrics.SetControllerHealthy(false)
		os.Exit(1)
	}

	// Determine the namespace that will be used for worker pods
	workerNamespace := cfg.Worker.Namespace
	if workerNamespace == "" {
		workerNamespace = "<auto-detected>"
	}

	setupLog.Info("Configuration loaded",
		"workerImage", cfg.GetWorkerImage(),
		"workerNamespace", workerNamespace,
		"maxRetries", cfg.Reconciliation.MaxRetries,
		"taintCount", len(cfg.NodeManagement.Taints),
		"verifiedLabel", cfg.NodeManagement.VerifiedLabel.Key,
		"leaderElection", enableLeaderElection,
	)

	// Override metrics and health addresses if configured
	if cfg.Metrics.Enabled {
		metricsAddr = fmt.Sprintf(":%d", cfg.Metrics.Port)
	} else {
		metricsAddr = "0" // Disable metrics
	}
	probeAddr = fmt.Sprintf(":%d", cfg.Health.Port)

	// Create manager
	setupLog.Info("Creating k8s manager")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress:        probeAddr,
		LeaderElection:                enableLeaderElection,
		LeaderElectionID:              leaderElectionID,
		LeaderElectionNamespace:       leaderElectionNamespace,
		LeaderElectionResourceLock:    "leases",
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "Unable to create manager")
		os.Exit(1)
	}

	// Setup the NodeReconciler
	setupLog.Info("Starting kube-node-ready node reconciler")
	nodeReconciler := controller.NewNodeReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		cfg,
	)
	if err = nodeReconciler.SetupWithManager(mgr, ctrl.Log.WithName("reconciler")); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "Node")
		os.Exit(1)
	}

	// Start metrics updater
	stopCh := ctrl.SetupSignalHandler()
	go nodeReconciler.StartMetricsUpdater(stopCh.Done())

	// Add health check endpoints
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")
	if err := mgr.Start(stopCh); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}

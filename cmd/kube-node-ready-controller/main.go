package main

import (
	"flag"
	"os"
	"time"

	"github.com/imunhatep/kube-node-ready/internal/k8s"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

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
	var configPath string

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

	setupLog.Info("Configuration loaded",
		"workerImage", cfg.GetWorkerImage(),
		"workerNamespace", cfg.GetWorkerNamespace(),
		"maxRetries", cfg.Reconciliation.MaxRetries,
		"taintCount", len(cfg.NodeManagement.Taints),
		"verifiedLabel", cfg.NodeManagement.VerifiedLabel.Key,
		"leaderElection", cfg.LeaderElection.Enabled,
		"leaderElectionID", cfg.LeaderElection.ID,
		"leaderElectionNamespace", cfg.GetLeaderElectionNamespace(),
		"metricsPort", cfg.Metrics.Port,
		"healthPort", cfg.Health.Port,
	)

	// Create controller manager
	mgr, err := k8s.NewCtrlManager(scheme, cfg)
	if err != nil {
		setupLog.Error(err, "Failed to create manager")
		os.Exit(1)
	}

	// Create Kubernetes and dynamic clients for NodeManager
	setupLog.Info("Creating Kubernetes clients for node operations")
	k8sConfig := mgr.GetConfig()

	clientset, err := kubernetes.NewForConfig(k8sConfig)
	if err != nil {
		setupLog.Error(err, "Unable to create Kubernetes clientset")
		os.Exit(1)
	}

	dynamicClient, err := dynamic.NewForConfig(k8sConfig)
	if err != nil {
		setupLog.Error(err, "Unable to create dynamic client")
		os.Exit(1)
	}

	// Create NodeManager for handling node operations
	nodeManager := k8s.NewNodeManager(clientset, dynamicClient)

	// Setup the NodeReconciler
	setupLog.Info("Starting kube-node-ready node reconciler")
	nodeReconciler := controller.NewNodeReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		cfg,
		nodeManager,
	)
	if err = nodeReconciler.SetupWithManager(mgr, ctrl.Log.WithName("reconciler")); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "Node")
		os.Exit(1)
	}

	// Start metrics updater
	stopCh := ctrl.SetupSignalHandler()
	go nodeReconciler.StartMetricsUpdater(stopCh.Done())

	setupLog.Info("Starting manager")
	if err := mgr.Start(stopCh); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}

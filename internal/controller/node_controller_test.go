package controller

import (
	"context"
	"testing"
	"time"

	"github.com/imunhatep/kube-node-ready/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/imunhatep/kube-node-ready/internal/config"
)

func setupTestNodeReconciler(t *testing.T) (*NodeReconciler, client.Client, *config.ControllerConfig) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	cfg := &config.ControllerConfig{
		Worker: config.WorkerPodConfig{
			Image: config.ImageConfig{
				Repository: "test-repo",
				Tag:        "test-tag",
				PullPolicy: "IfNotPresent",
			},
			Namespace: "test-namespace",
			Job: config.JobConfig{
				ActiveDeadlineSeconds:   int32Ptr(300),
				BackoffLimit:            int32Ptr(3),
				Completions:             int32Ptr(1),
				TTLSecondsAfterFinished: int32Ptr(600),
			},
			ServiceAccountName: "test-sa",
			PriorityClassName:  "system-node-critical",
			Resources: config.ResourcesConfig{
				Requests: config.ResourceRequirements{
					CPU:    "50m",
					Memory: "64Mi",
				},
				Limits: config.ResourceRequirements{
					CPU:    "100m",
					Memory: "128Mi",
				},
			},
			ConfigMapName: "test-worker-config",
		},
		Reconciliation: config.ReconciliationConfig{
			IntervalSeconds: 30,
			MaxRetries:      5,
			RetryBackoff:    "exponential",
		},
		NodeManagement: config.NodeManagementConfig{
			DeleteFailedNodes: false,
			Taints: []config.TaintConfig{
				{
					Key:    "node-ready/unverified",
					Value:  "true",
					Effect: "NoSchedule",
				},
			},
			VerifiedLabel: config.LabelConfig{
				Key:   "node-ready/verified",
				Value: "true",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create a mock NodeManager - passing nil clients for testing
	// In real tests, you might want to use mock clients, but for basic functionality testing nil is ok
	mockNodeManager := k8s.NewNodeManager(nil, nil)

	reconciler := NewNodeReconciler(fakeClient, scheme, cfg, mockNodeManager)

	return reconciler, fakeClient, cfg
}

func createTestNode(name string, withTaint bool, withLabel bool) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{},
		},
		Spec: corev1.NodeSpec{
			Taints: []corev1.Taint{},
		},
	}

	if withTaint {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    "node-ready/unverified",
			Value:  "true",
			Effect: corev1.TaintEffectNoSchedule,
		})
	}

	if withLabel {
		node.Labels["node-ready/verified"] = "true"
	}

	return node
}

func TestNewNodeReconciler(t *testing.T) {
	reconciler, _, cfg := setupTestNodeReconciler(t)

	if reconciler == nil {
		t.Fatal("Expected NodeReconciler to be created")
	}

	if reconciler.Config != cfg {
		t.Error("Expected config to match")
	}

	if reconciler.StateCache == nil {
		t.Error("Expected NodeStateCache to be initialized")
	}

	if reconciler.WorkerManager == nil {
		t.Error("Expected WorkerManager to be initialized")
	}
}

func TestReconcileNodeDeleted(t *testing.T) {
	reconciler, _, _ := setupTestNodeReconciler(t)
	ctx := context.Background()

	// Add some state for a node
	reconciler.StateCache.Set("deleted-node", &k8s.NodeState{
		NodeName: "deleted-node",
		State:    k8s.NodeStateInProgress,
	})

	// Reconcile non-existent node
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name: "deleted-node",
		},
	}

	result, err := reconciler.Reconcile(ctx, req)
	if err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	if result.Requeue {
		t.Error("Expected no requeue for deleted node")
	}

	// Verify state is cleaned up
	state := reconciler.StateCache.Get("deleted-node")
	if state != nil {
		t.Error("Expected state to be deleted")
	}
}

func TestCalculateBackoffExponential(t *testing.T) {
	reconciler, _, cfg := setupTestNodeReconciler(t)
	cfg.Reconciliation.RetryBackoff = "exponential"

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, 30 * time.Second},
	}

	for _, tt := range tests {
		backoff := reconciler.calculateBackoff(tt.attempt)
		if backoff != tt.expected {
			t.Errorf("For attempt %d, expected backoff %v, got %v", tt.attempt, tt.expected, backoff)
		}
	}
}

func TestHasVerifiedLabel(t *testing.T) {
	tests := []struct {
		name     string
		node     *corev1.Node
		labelKey string
		expected bool
	}{
		{
			name:     "node with label",
			node:     createTestNode("test", false, true),
			labelKey: "node-ready/verified",
			expected: true,
		},
		{
			name:     "node without label",
			node:     createTestNode("test", false, false),
			labelKey: "node-ready/verified",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasVerifiedLabel(tt.node, tt.labelKey)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

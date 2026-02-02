package controller

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/imunhatep/kube-node-ready/internal/config"
)

func TestWorkerManagerJobCreation(t *testing.T) {
	// Create a fake client
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// Create test config with job settings
	cfg := &config.ControllerConfig{
		Worker: config.WorkerPodConfig{
			Image: config.ImageConfig{
				Repository: "test-repo",
				Tag:        "test-tag",
				PullPolicy: "IfNotPresent",
			},
			Namespace:         "test-namespace",
			ServiceAccount:    "test-sa",
			PriorityClassName: "test-priority",
			ConfigMapName:     "test-configmap",
			Job: config.JobConfig{
				ActiveDeadlineSeconds:   int32Ptr(300),
				BackoffLimit:            int32Ptr(2),
				Completions:             int32Ptr(1),
				TTLSecondsAfterFinished: int32Ptr(600),
			},
		},
		NodeManagement: config.NodeManagementConfig{
			Taints: []config.TaintConfig{},
		},
	}

	// Create worker manager
	wm := NewWorkerManager(fakeClient, cfg)

	// Test job creation
	ctx := context.Background()
	nodeName := "test-node"

	job, err := wm.CreateWorkerJob(ctx, nodeName)
	if err != nil {
		t.Fatalf("Failed to create worker job: %v", err)
	}

	// Verify job properties
	if job.Name == "" {
		t.Error("Job name should not be empty")
	}

	if job.Namespace != "test-namespace" {
		t.Errorf("Expected namespace 'test-namespace', got '%s'", job.Namespace)
	}

	// Verify job spec
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds != 300 {
		t.Errorf("Expected ActiveDeadlineSeconds to be 300, got %v", job.Spec.ActiveDeadlineSeconds)
	}

	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 2 {
		t.Errorf("Expected BackoffLimit to be 2, got %v", job.Spec.BackoffLimit)
	}

	if job.Spec.Completions == nil || *job.Spec.Completions != 1 {
		t.Errorf("Expected Completions to be 1, got %v", job.Spec.Completions)
	}

	if job.Spec.Parallelism == nil || *job.Spec.Parallelism != 1 {
		t.Errorf("Expected Parallelism to be 1, got %v", job.Spec.Parallelism)
	}

	if job.Spec.TTLSecondsAfterFinished == nil || *job.Spec.TTLSecondsAfterFinished != 600 {
		t.Errorf("Expected TTLSecondsAfterFinished to be 600, got %v", job.Spec.TTLSecondsAfterFinished)
	}

	// Verify labels
	if job.Labels["node"] != nodeName {
		t.Errorf("Expected node label to be '%s', got '%s'", nodeName, job.Labels["node"])
	}

	// Verify pod template
	podSpec := job.Spec.Template.Spec
	if podSpec.RestartPolicy != corev1.RestartPolicyNever {
		t.Errorf("Expected RestartPolicy to be Never, got %v", podSpec.RestartPolicy)
	}

	if podSpec.ServiceAccountName != "test-sa" {
		t.Errorf("Expected ServiceAccountName to be 'test-sa', got '%s'", podSpec.ServiceAccountName)
	}

	if len(podSpec.Containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(podSpec.Containers))
	}

	container := podSpec.Containers[0]
	if container.Image != "test-repo:test-tag" {
		t.Errorf("Expected image 'test-repo:test-tag', got '%s'", container.Image)
	}
}

func TestWorkerManagerJobStatus(t *testing.T) {
	// Create a fake client with a completed job
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	completedJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "test-namespace",
			UID:       types.UID("test-uid"),
		},
		Status: batchv1.JobStatus{
			Succeeded: 1,
			Conditions: []batchv1.JobCondition{
				{
					Type:               batchv1.JobComplete,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: metav1.Now(),
				},
			},
			StartTime: &metav1.Time{Time: time.Now().Add(-5 * time.Minute)},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(completedJob).Build()

	cfg := &config.ControllerConfig{
		Worker: config.WorkerPodConfig{
			Namespace: "test-namespace",
		},
	}

	wm := NewWorkerManager(fakeClient, cfg)

	// Test job status retrieval
	ctx := context.Background()
	status, err := wm.GetWorkerJobStatus(ctx, "test-job")
	if err != nil {
		t.Fatalf("Failed to get job status: %v", err)
	}

	if status.Succeeded != 1 {
		t.Errorf("Expected Succeeded to be 1, got %d", status.Succeeded)
	}

	if !status.Completed {
		t.Error("Expected job to be completed")
	}

	if status.StartTime == nil {
		t.Error("Expected StartTime to be set")
	}

	if status.FinishTime == nil {
		t.Error("Expected FinishTime to be set")
	}
}

func TestWorkerManagerJobDefaults(t *testing.T) {
	// Test that defaults are applied when job config is not provided
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	cfg := &config.ControllerConfig{
		Worker: config.WorkerPodConfig{
			Image: config.ImageConfig{
				Repository: "test-repo",
				Tag:        "test-tag",
				PullPolicy: "IfNotPresent",
			},
			Namespace: "test-namespace",
			Job: config.JobConfig{
				ActiveDeadlineSeconds: int32Ptr(180),
			},
			ConfigMapName: "test-configmap",
			// No Job config specified - should use defaults
		},
		NodeManagement: config.NodeManagementConfig{
			Taints: []config.TaintConfig{},
		},
	}

	wm := NewWorkerManager(fakeClient, cfg)

	ctx := context.Background()
	job, err := wm.CreateWorkerJob(ctx, "test-node")
	if err != nil {
		t.Fatalf("Failed to create worker job: %v", err)
	}

	// Verify defaults
	if job.Spec.ActiveDeadlineSeconds == nil || *job.Spec.ActiveDeadlineSeconds != 180 {
		t.Errorf("Expected ActiveDeadlineSeconds to be 180 (from timeoutSeconds), got %v", job.Spec.ActiveDeadlineSeconds)
	}

	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 2 {
		t.Errorf("Expected BackoffLimit default to be 2, got %v", job.Spec.BackoffLimit)
	}

	if job.Spec.Completions == nil || *job.Spec.Completions != 1 {
		t.Errorf("Expected Completions default to be 1, got %v", job.Spec.Completions)
	}

	if job.Spec.Parallelism == nil || *job.Spec.Parallelism != 1 {
		t.Errorf("Expected Parallelism default to be 1, got %v", job.Spec.Parallelism)
	}
}

// Helper function to create int32 pointer
func int32Ptr(i int32) *int32 {
	return &i
}

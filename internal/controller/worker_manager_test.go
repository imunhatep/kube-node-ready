package controller

import (
	"context"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/imunhatep/kube-node-ready/internal/config"
)

func createTestWorkerManager(t *testing.T) (*WorkerManager, client.Client) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	cfg := &config.ControllerConfig{
		Worker: config.WorkerPodConfig{
			Image: config.ImageConfig{
				Repository: "test-repo",
				Tag:        "test-tag",
				PullPolicy: "IfNotPresent",
			},
			Namespace:          "test-namespace",
			ServiceAccountName: "test-sa",
			PriorityClassName:  "system-node-critical",
			Resources: config.ResourcesConfig{
				Requests: config.ResourceRequirements{
					CPU:    "50m",
					Memory: "64Mi",
				},
				Limits: config.ResourceRequirements{
					Memory: "128Mi",
				},
			},
			ConfigMapName: "test-configmap",
			Job: config.JobConfig{
				ActiveDeadlineSeconds: int32Ptr(300),
				BackoffLimit:          int32Ptr(2),
				Completions:           int32Ptr(1),
			},
			Tolerations: []config.TolerationConfig{
				{
					Key:      "node-ready/unverified",
					Operator: "Exists",
					Effect:   "NoSchedule",
				},
			},
		},
		NodeManagement: config.NodeManagementConfig{
			Taints: []config.TaintConfig{
				{
					Key:    "node-ready/unverified",
					Value:  "true",
					Effect: "NoSchedule",
				},
			},
		},
	}

	// Set namespace in config (simulating what main.go does)
	cfg.Worker.Namespace = "test-namespace"

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	wm := NewWorkerManager(fakeClient, cfg)

	return wm, fakeClient
}

func int32Ptr(i int32) *int32 {
	return &i
}

func TestWorkerManager_CreateWorkerJob_Basic(t *testing.T) {
	wm, _ := createTestWorkerManager(t)
	ctx := context.Background()
	nodeName := "test-node"

	job, err := wm.CreateWorkerJob(ctx, nodeName)
	if err != nil {
		t.Fatalf("CreateWorkerJob failed: %v", err)
	}

	// Verify job metadata
	if job.Name == "" {
		t.Error("Expected job name to be set")
	}
	if job.Namespace != "test-namespace" {
		t.Errorf("Expected namespace 'test-namespace', got %s", job.Namespace)
	}
	if job.Labels["component"] != "worker" {
		t.Error("Expected job to have component=worker label")
	}
	if job.Labels["node"] != nodeName {
		t.Errorf("Expected job to have node=%s label", nodeName)
	}

	// Verify job spec
	if job.Spec.Parallelism == nil || *job.Spec.Parallelism != 1 {
		t.Error("Expected job parallelism to be 1")
	}
	if job.Spec.Completions == nil || *job.Spec.Completions != 1 {
		t.Error("Expected job completions to be 1")
	}
	if job.Spec.BackoffLimit == nil || *job.Spec.BackoffLimit != 2 {
		t.Error("Expected job backoff limit to be 2")
	}

	// Verify pod template
	podSpec := job.Spec.Template.Spec
	if podSpec.RestartPolicy != corev1.RestartPolicyNever {
		t.Error("Expected restart policy to be Never")
	}
	if podSpec.ServiceAccountName != "test-sa" {
		t.Errorf("Expected service account name 'test-sa', got %s", podSpec.ServiceAccountName)
	}
	if podSpec.PriorityClassName != "system-node-critical" {
		t.Errorf("Expected priority class 'system-node-critical', got %s", podSpec.PriorityClassName)
	}

	// Verify node selector
	if podSpec.NodeSelector["kubernetes.io/hostname"] != nodeName {
		t.Errorf("Expected node selector for hostname=%s", nodeName)
	}

	// Verify tolerations
	if len(podSpec.Tolerations) == 0 {
		t.Error("Expected tolerations to be set")
	}

	// Verify main container
	if len(podSpec.Containers) != 1 {
		t.Errorf("Expected 1 container, got %d", len(podSpec.Containers))
	}

	container := podSpec.Containers[0]
	if container.Name != "worker" {
		t.Errorf("Expected container name 'worker', got %s", container.Name)
	}
	if container.Image != "test-repo:test-tag" {
		t.Errorf("Expected image 'test-repo:test-tag', got %s", container.Image)
	}

	// Verify resources
	cpuRequest := container.Resources.Requests[corev1.ResourceCPU]
	if cpuRequest.String() != "50m" {
		t.Errorf("Expected CPU request of 50m, got %s", cpuRequest.String())
	}
	memoryRequest := container.Resources.Requests[corev1.ResourceMemory]
	if memoryRequest.String() != "64Mi" {
		t.Errorf("Expected memory request of 64Mi, got %s", memoryRequest.String())
	}
	memoryLimit := container.Resources.Limits[corev1.ResourceMemory]
	if memoryLimit.String() != "128Mi" {
		t.Errorf("Expected memory limit of 128Mi, got %s", memoryLimit.String())
	}
}

func TestWorkerManager_CreateWorkerJob_WithInitContainers(t *testing.T) {
	wm, _ := createTestWorkerManager(t)

	// Add init containers to configuration
	wm.config.Worker.InitContainers = []config.InitContainerConfig{
		{
			Name:    "network-check",
			Image:   "busybox:latest",
			Command: []string{"/bin/sh", "-c"},
			Args:    []string{"ping -c 3 google.com"},
			Env: []config.EnvVarConfig{
				{Name: "TEST_VAR", Value: "test-value"},
			},
			Resources: &config.ResourcesConfig{
				Requests: config.ResourceRequirements{
					CPU:    "10m",
					Memory: "16Mi",
				},
			},
			SecurityContext: &config.SecurityContextConfig{
				ReadOnlyRootFilesystem: boolPtr(true),
				RunAsNonRoot:           boolPtr(true),
			},
		},
		{
			Name:       "storage-check",
			Image:      "alpine:latest",
			Command:    []string{"test"},
			Args:       []string{"-d", "/host"},
			WorkingDir: "/tmp",
			VolumeMounts: []config.VolumeMountConfig{
				{
					Name:      "host-volume",
					MountPath: "/host",
					ReadOnly:  true,
				},
			},
		},
	}

	ctx := context.Background()
	nodeName := "test-node"

	job, err := wm.CreateWorkerJob(ctx, nodeName)
	if err != nil {
		t.Fatalf("CreateWorkerJob failed: %v", err)
	}

	podSpec := job.Spec.Template.Spec

	// Verify init containers are present
	if len(podSpec.InitContainers) != 2 {
		t.Errorf("Expected 2 init containers, got %d", len(podSpec.InitContainers))
	}

	// Verify first init container
	initContainer1 := podSpec.InitContainers[0]
	if initContainer1.Name != "network-check" {
		t.Errorf("Expected first init container name 'network-check', got %s", initContainer1.Name)
	}
	if initContainer1.Image != "busybox:latest" {
		t.Errorf("Expected first init container image 'busybox:latest', got %s", initContainer1.Image)
	}
	if len(initContainer1.Command) != 2 {
		t.Errorf("Expected 2 command elements, got %d", len(initContainer1.Command))
	}
	if len(initContainer1.Args) != 1 {
		t.Errorf("Expected 1 arg element, got %d", len(initContainer1.Args))
	}
	if len(initContainer1.Env) != 1 {
		t.Errorf("Expected 1 env var, got %d", len(initContainer1.Env))
	}
	if initContainer1.Env[0].Name != "TEST_VAR" || initContainer1.Env[0].Value != "test-value" {
		t.Error("Expected TEST_VAR=test-value")
	}
	cpuRequest := initContainer1.Resources.Requests[corev1.ResourceCPU]
	if cpuRequest.String() != "10m" {
		t.Errorf("Expected init container CPU request of 10m, got %s", cpuRequest.String())
	}
	if initContainer1.SecurityContext == nil {
		t.Error("Expected security context to be set")
	} else {
		if initContainer1.SecurityContext.ReadOnlyRootFilesystem == nil || !*initContainer1.SecurityContext.ReadOnlyRootFilesystem {
			t.Error("Expected ReadOnlyRootFilesystem to be true")
		}
		if initContainer1.SecurityContext.RunAsNonRoot == nil || !*initContainer1.SecurityContext.RunAsNonRoot {
			t.Error("Expected RunAsNonRoot to be true")
		}
	}

	// Verify second init container
	initContainer2 := podSpec.InitContainers[1]
	if initContainer2.Name != "storage-check" {
		t.Errorf("Expected second init container name 'storage-check', got %s", initContainer2.Name)
	}
	if initContainer2.Image != "alpine:latest" {
		t.Errorf("Expected second init container image 'alpine:latest', got %s", initContainer2.Image)
	}
	if initContainer2.WorkingDir != "/tmp" {
		t.Errorf("Expected working dir '/tmp', got %s", initContainer2.WorkingDir)
	}
	if len(initContainer2.VolumeMounts) != 1 {
		t.Errorf("Expected 1 volume mount, got %d", len(initContainer2.VolumeMounts))
	}
	if initContainer2.VolumeMounts[0].Name != "host-volume" {
		t.Error("Expected host-volume mount")
	}
	if initContainer2.VolumeMounts[0].ReadOnly != true {
		t.Error("Expected volume mount to be read-only")
	}
}

func TestWorkerManager_CreateWorkerJob_WithAdditionalTolerations(t *testing.T) {
	wm, _ := createTestWorkerManager(t)

	// Add additional tolerations
	wm.config.Worker.Tolerations = append(wm.config.Worker.Tolerations, config.TolerationConfig{
		Key:      "spot-instance",
		Operator: "Exists",
		Effect:   "",
	})
	wm.config.Worker.Tolerations = append(wm.config.Worker.Tolerations, config.TolerationConfig{
		Key:               "workload-type",
		Operator:          "Equal",
		Value:             "batch",
		Effect:            "NoExecute",
		TolerationSeconds: int64Ptr(300),
	})

	ctx := context.Background()
	nodeName := "test-node"

	job, err := wm.CreateWorkerJob(ctx, nodeName)
	if err != nil {
		t.Fatalf("CreateWorkerJob failed: %v", err)
	}

	tolerations := job.Spec.Template.Spec.Tolerations

	// Should have default tolerations plus configured ones
	// Default: verification taint, not-ready, unreachable
	// Configured: spot-instance, workload-type
	if len(tolerations) < 5 {
		t.Errorf("Expected at least 5 tolerations, got %d", len(tolerations))
	}

	// Check for spot-instance toleration
	foundSpotToleration := false
	for _, toleration := range tolerations {
		if toleration.Key == "spot-instance" {
			foundSpotToleration = true
			if toleration.Operator != corev1.TolerationOpExists {
				t.Error("Expected spot-instance toleration operator to be Exists")
			}
			break
		}
	}
	if !foundSpotToleration {
		t.Error("Expected to find spot-instance toleration")
	}

	// Check for workload-type toleration
	foundWorkloadToleration := false
	for _, toleration := range tolerations {
		if toleration.Key == "workload-type" {
			foundWorkloadToleration = true
			if toleration.Operator != corev1.TolerationOpEqual {
				t.Error("Expected workload-type toleration operator to be Equal")
			}
			if toleration.Value != "batch" {
				t.Error("Expected workload-type toleration value to be 'batch'")
			}
			if toleration.Effect != corev1.TaintEffectNoExecute {
				t.Error("Expected workload-type toleration effect to be NoExecute")
			}
			if toleration.TolerationSeconds == nil || *toleration.TolerationSeconds != 300 {
				t.Error("Expected workload-type toleration seconds to be 300")
			}
			break
		}
	}
	if !foundWorkloadToleration {
		t.Error("Expected to find workload-type toleration")
	}
}

func TestWorkerManager_GetWorkerJobStatus(t *testing.T) {
	wm, fakeClient := createTestWorkerManager(t)
	ctx := context.Background()

	// Create a test job
	jobName := "test-job"
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: "test-namespace",
		},
		Status: batchv1.JobStatus{
			Active:    1,
			Succeeded: 0,
			Failed:    0,
			StartTime: &metav1.Time{Time: time.Now()},
		},
	}

	err := fakeClient.Create(ctx, job)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Get job status
	status, err := wm.GetWorkerJobStatus(ctx, jobName)
	if err != nil {
		t.Fatalf("GetWorkerJobStatus failed: %v", err)
	}

	if status.Active != 1 {
		t.Errorf("Expected active=1, got %d", status.Active)
	}
	if status.Succeeded != 0 {
		t.Errorf("Expected succeeded=0, got %d", status.Succeeded)
	}
	if status.Failed != 0 {
		t.Errorf("Expected failed=0, got %d", status.Failed)
	}
	if status.Completed {
		t.Error("Expected job to not be completed")
	}
	if status.StartTime == nil {
		t.Error("Expected start time to be set")
	}
}

func TestWorkerManager_GetWorkerJobStatus_Completed(t *testing.T) {
	wm, fakeClient := createTestWorkerManager(t)
	ctx := context.Background()

	// Create a completed test job
	jobName := "test-job-completed"
	completionTime := metav1.Now()
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: "test-namespace",
		},
		Status: batchv1.JobStatus{
			Active:         0,
			Succeeded:      1,
			Failed:         0,
			CompletionTime: &completionTime,
			Conditions: []batchv1.JobCondition{
				{
					Type:               batchv1.JobComplete,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: completionTime,
				},
			},
		},
	}

	err := fakeClient.Create(ctx, job)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Get job status
	status, err := wm.GetWorkerJobStatus(ctx, jobName)
	if err != nil {
		t.Fatalf("GetWorkerJobStatus failed: %v", err)
	}

	if !status.Completed {
		t.Error("Expected job to be completed")
	}
	if status.Succeeded != 1 {
		t.Errorf("Expected succeeded=1, got %d", status.Succeeded)
	}
	if status.FinishTime == nil {
		t.Error("Expected finish time to be set")
	}
}

func TestWorkerManager_DeleteWorkerJob(t *testing.T) {
	wm, fakeClient := createTestWorkerManager(t)
	ctx := context.Background()

	// Create a test job
	jobName := "test-job-to-delete"
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: "test-namespace",
		},
	}

	err := fakeClient.Create(ctx, job)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Delete the job
	err = wm.DeleteWorkerJob(ctx, jobName)
	if err != nil {
		t.Fatalf("DeleteWorkerJob failed: %v", err)
	}

	// Verify job is deleted
	_, err = wm.GetWorkerJobStatus(ctx, jobName)
	if err == nil {
		t.Error("Expected error when getting deleted job status")
	}
}

func TestWorkerManager_FindWorkerJobForNode(t *testing.T) {
	wm, fakeClient := createTestWorkerManager(t)
	ctx := context.Background()
	nodeName := "test-node"

	// Create a test job for the node
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job-for-node",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"app":       "kube-node-ready",
				"component": "worker",
				"node":      nodeName,
			},
			CreationTimestamp: metav1.Now(),
		},
		Status: batchv1.JobStatus{
			Active: 1,
		},
	}

	err := fakeClient.Create(ctx, job)
	if err != nil {
		t.Fatalf("Failed to create test job: %v", err)
	}

	// Find job for node
	foundJob, err := wm.FindWorkerJobForNode(ctx, nodeName)
	if err != nil {
		t.Fatalf("FindWorkerJobForNode failed: %v", err)
	}

	if foundJob.Name != "test-job-for-node" {
		t.Errorf("Expected job name 'test-job-for-node', got %s", foundJob.Name)
	}
}

func TestWorkerManager_BuildTolerations(t *testing.T) {
	wm, _ := createTestWorkerManager(t)

	tolerations := wm.buildTolerations()

	// Should have at least the default tolerations
	if len(tolerations) < 3 {
		t.Errorf("Expected at least 3 tolerations, got %d", len(tolerations))
	}

	// Check for verification taint toleration
	found := false
	for _, toleration := range tolerations {
		if toleration.Key == "node-ready/unverified" {
			found = true
			if toleration.Operator != corev1.TolerationOpExists {
				t.Error("Expected verification taint toleration operator to be Exists")
			}
			if toleration.Effect != corev1.TaintEffectNoSchedule {
				t.Error("Expected verification taint toleration effect to be NoSchedule")
			}
			break
		}
	}
	if !found {
		t.Error("Expected to find verification taint toleration")
	}
}

func TestWorkerManager_BuildInitContainers_Empty(t *testing.T) {
	wm, _ := createTestWorkerManager(t)

	// No init containers configured
	wm.config.Worker.InitContainers = []config.InitContainerConfig{}

	initContainers := wm.buildInitContainers()

	if initContainers != nil {
		t.Errorf("Expected nil init containers, got %v", initContainers)
	}
}

func boolPtr(b bool) *bool {
	return &b
}

func int64Ptr(i int64) *int64 {
	return &i
}

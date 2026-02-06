package controller

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/imunhatep/kube-node-ready/internal/config"
)

// WorkerManager manages the lifecycle of worker jobs
type WorkerManager struct {
	client client.Client
	config *config.ControllerConfig
}

// NewWorkerManager creates a new worker manager
// Uses namespace from config (which should already be detected/set)
func NewWorkerManager(client client.Client, cfg *config.ControllerConfig) *WorkerManager {
	klog.InfoS("Worker manager initialized", "namespace", cfg.GetWorkerNamespace())

	return &WorkerManager{
		client: client,
		config: cfg,
	}
}

// buildWorkerEnvVars constructs environment variables for worker jobs
// Only includes node identity and k8s service info - worker runtime config comes from ConfigMap
func (w *WorkerManager) buildWorkerEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "NODE_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "spec.nodeName",
				},
			},
		},
		{
			Name: "POD_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
	}
}

// buildTolerations constructs tolerations for worker pods
func (w *WorkerManager) buildTolerations() []corev1.Toleration {
	tolerations := []corev1.Toleration{}

	// Add tolerations for verification taints (these will be removed on success)
	for _, taint := range w.config.NodeManagement.Taints {
		tolerations = append(tolerations, corev1.Toleration{
			Key:      taint.Key,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffect(taint.Effect),
		})
	}

	// Add default Kubernetes tolerations
	tolerations = append(tolerations,
		corev1.Toleration{
			Key:      "node.kubernetes.io/not-ready",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
		corev1.Toleration{
			Key:      "node.kubernetes.io/unreachable",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	)

	// Add configured additional tolerations
	for _, tolerationCfg := range w.config.Worker.Tolerations {
		toleration := corev1.Toleration{
			Key: tolerationCfg.Key,
		}

		// Set operator (default to Exists if not specified)
		if tolerationCfg.Operator != "" {
			toleration.Operator = corev1.TolerationOperator(tolerationCfg.Operator)
		} else {
			toleration.Operator = corev1.TolerationOpExists
		}

		// Set value (only used with Equal operator)
		if tolerationCfg.Value != "" {
			toleration.Value = tolerationCfg.Value
		}

		// Set effect (empty means all effects)
		if tolerationCfg.Effect != "" {
			toleration.Effect = corev1.TaintEffect(tolerationCfg.Effect)
		}

		// Set toleration seconds (for NoExecute effect)
		if tolerationCfg.TolerationSeconds != nil {
			toleration.TolerationSeconds = tolerationCfg.TolerationSeconds
		}

		tolerations = append(tolerations, toleration)
	}

	return tolerations
}

// buildInitContainers constructs init containers for worker pods from configuration
func (w *WorkerManager) buildInitContainers() []corev1.Container {
	if len(w.config.Worker.InitContainers) == 0 {
		return nil
	}

	initContainers := make([]corev1.Container, 0, len(w.config.Worker.InitContainers))

	for _, initCfg := range w.config.Worker.InitContainers {
		container := corev1.Container{
			Name:  initCfg.Name,
			Image: initCfg.Image,
		}

		// Set command and args
		if len(initCfg.Command) > 0 {
			container.Command = initCfg.Command
		}
		if len(initCfg.Args) > 0 {
			container.Args = initCfg.Args
		}

		// Set working directory
		if initCfg.WorkingDir != "" {
			container.WorkingDir = initCfg.WorkingDir
		}

		// Set environment variables
		if len(initCfg.Env) > 0 {
			container.Env = make([]corev1.EnvVar, 0, len(initCfg.Env))
			for _, envCfg := range initCfg.Env {
				container.Env = append(container.Env, corev1.EnvVar{
					Name:  envCfg.Name,
					Value: envCfg.Value,
				})
			}
		}

		// Set volume mounts
		if len(initCfg.VolumeMounts) > 0 {
			container.VolumeMounts = make([]corev1.VolumeMount, 0, len(initCfg.VolumeMounts))
			for _, vmCfg := range initCfg.VolumeMounts {
				volumeMount := corev1.VolumeMount{
					Name:      vmCfg.Name,
					MountPath: vmCfg.MountPath,
				}
				if vmCfg.ReadOnly {
					volumeMount.ReadOnly = vmCfg.ReadOnly
				}
				if vmCfg.SubPath != "" {
					volumeMount.SubPath = vmCfg.SubPath
				}
				container.VolumeMounts = append(container.VolumeMounts, volumeMount)
			}
		}

		// Set resources
		if initCfg.Resources != nil {
			container.Resources = corev1.ResourceRequirements{}

			// Set resource requests
			if initCfg.Resources.Requests.CPU != "" || initCfg.Resources.Requests.Memory != "" {
				container.Resources.Requests = make(corev1.ResourceList)
				if initCfg.Resources.Requests.CPU != "" {
					container.Resources.Requests[corev1.ResourceCPU] = resource.MustParse(initCfg.Resources.Requests.CPU)
				}
				if initCfg.Resources.Requests.Memory != "" {
					container.Resources.Requests[corev1.ResourceMemory] = resource.MustParse(initCfg.Resources.Requests.Memory)
				}
			}

			// Set resource limits
			if initCfg.Resources.Limits.CPU != "" || initCfg.Resources.Limits.Memory != "" {
				container.Resources.Limits = make(corev1.ResourceList)
				if initCfg.Resources.Limits.CPU != "" {
					container.Resources.Limits[corev1.ResourceCPU] = resource.MustParse(initCfg.Resources.Limits.CPU)
				}
				if initCfg.Resources.Limits.Memory != "" {
					container.Resources.Limits[corev1.ResourceMemory] = resource.MustParse(initCfg.Resources.Limits.Memory)
				}
			}
		}

		// Set security context
		if initCfg.SecurityContext != nil {
			container.SecurityContext = &corev1.SecurityContext{}
			if initCfg.SecurityContext.Privileged != nil {
				container.SecurityContext.Privileged = initCfg.SecurityContext.Privileged
			}
			if initCfg.SecurityContext.ReadOnlyRootFilesystem != nil {
				container.SecurityContext.ReadOnlyRootFilesystem = initCfg.SecurityContext.ReadOnlyRootFilesystem
			}
			if initCfg.SecurityContext.RunAsNonRoot != nil {
				container.SecurityContext.RunAsNonRoot = initCfg.SecurityContext.RunAsNonRoot
			}
			if initCfg.SecurityContext.RunAsUser != nil {
				container.SecurityContext.RunAsUser = initCfg.SecurityContext.RunAsUser
			}
			if initCfg.SecurityContext.RunAsGroup != nil {
				container.SecurityContext.RunAsGroup = initCfg.SecurityContext.RunAsGroup
			}
			if initCfg.SecurityContext.AllowPrivilegeEscalation != nil {
				container.SecurityContext.AllowPrivilegeEscalation = initCfg.SecurityContext.AllowPrivilegeEscalation
			}
		}

		initContainers = append(initContainers, container)
	}

	return initContainers
}

// WorkerJobStatus represents the status of a worker job
type WorkerJobStatus struct {
	Active     int32
	Succeeded  int32
	Failed     int32
	Completed  bool
	ExitCode   *int32
	Reason     string
	Message    string
	StartTime  *time.Time
	FinishTime *time.Time
}

// CreateWorkerJob creates a worker job for the given node
func (w *WorkerManager) CreateWorkerJob(ctx context.Context, nodeName string) (*batchv1.Job, error) {
	jobName := fmt.Sprintf("node-ready-worker-%s-%d", nodeName, time.Now().Unix())

	klog.InfoS("Creating worker job", "job", jobName, "node", nodeName)

	// Build tolerations using the buildTolerations method
	tolerations := w.buildTolerations()

	// Parse resources - make limits optional
	resourceReqs := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}

	// Add resource requests if specified
	if w.config.Worker.Resources.Requests.CPU != "" {
		resourceReqs.Requests[corev1.ResourceCPU] = resource.MustParse(w.config.Worker.Resources.Requests.CPU)
	}
	if w.config.Worker.Resources.Requests.Memory != "" {
		resourceReqs.Requests[corev1.ResourceMemory] = resource.MustParse(w.config.Worker.Resources.Requests.Memory)
	}

	// Add resource limits if specified (optional, especially for CPU)
	if w.config.Worker.Resources.Limits.CPU != "" {
		resourceReqs.Limits[corev1.ResourceCPU] = resource.MustParse(w.config.Worker.Resources.Limits.CPU)
	}

	if w.config.Worker.Resources.Limits.Memory != "" {
		resourceReqs.Limits[corev1.ResourceMemory] = resource.MustParse(w.config.Worker.Resources.Limits.Memory)
	}

	// Set job defaults if not specified
	var activeDeadlineSeconds *int64
	if w.config.Worker.Job.ActiveDeadlineSeconds != nil {
		deadline := int64(*w.config.Worker.Job.ActiveDeadlineSeconds)
		activeDeadlineSeconds = &deadline
	}

	var backoffLimit *int32
	if w.config.Worker.Job.BackoffLimit != nil {
		backoffLimit = w.config.Worker.Job.BackoffLimit
	} else {
		// Default to 2 retries
		defaultBackoffLimit := int32(2)
		backoffLimit = &defaultBackoffLimit
	}

	var completions *int32
	if w.config.Worker.Job.Completions != nil {
		completions = w.config.Worker.Job.Completions
	} else {
		// Default to 1 completion
		defaultCompletions := int32(1)
		completions = &defaultCompletions
	}

	parallelism := int32(1)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: w.config.GetWorkerNamespace(),
			Labels: map[string]string{
				"app":       "kube-node-ready",
				"component": "worker",
				"node":      nodeName,
			},
			Annotations: map[string]string{
				"kube-node-ready/target-node": nodeName,
				"kube-node-ready/created-at":  time.Now().Format(time.RFC3339),
			},
		},
		Spec: batchv1.JobSpec{
			ActiveDeadlineSeconds:   activeDeadlineSeconds,
			BackoffLimit:            backoffLimit,
			Completions:             completions,
			Parallelism:             &parallelism,
			TTLSecondsAfterFinished: w.config.Worker.Job.TTLSecondsAfterFinished,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "kube-node-ready",
						"component": "worker",
						"node":      nodeName,
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: w.config.Worker.ServiceAccountName,
					PriorityClassName:  w.config.Worker.PriorityClassName,
					NodeSelector: map[string]string{
						"kubernetes.io/hostname": nodeName,
					},
					Tolerations:    tolerations,
					InitContainers: w.buildInitContainers(),
					Containers: []corev1.Container{
						{
							Name:            "worker",
							Image:           w.config.GetWorkerImage(),
							ImagePullPolicy: corev1.PullPolicy(w.config.Worker.Image.PullPolicy),
							Command:         []string{"/kube-node-ready-worker"},
							Env:             w.buildWorkerEnvVars(),
							Resources:       resourceReqs,
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "worker-config",
									MountPath: "/etc/kube-node-ready",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "worker-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: w.config.Worker.ConfigMapName,
									},
									Items: []corev1.KeyToPath{
										{
											Key:  "worker-config.yaml",
											Path: "worker-config.yaml",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := w.client.Create(ctx, job); err != nil {
		if errors.IsAlreadyExists(err) {
			klog.InfoS("Worker job already exists", "job", jobName, "node", nodeName)
			// Get the existing job
			existingJob := &batchv1.Job{}
			jobQuery := client.ObjectKey{
				Namespace: w.config.GetWorkerNamespace(),
				Name:      jobName,
			}
			if err := w.client.Get(ctx, jobQuery, existingJob); err != nil {
				return nil, fmt.Errorf("failed to get existing job: %w", err)
			}
			return existingJob, nil
		}
		klog.ErrorS(err, "Failed to create worker job", "job", jobName, "node", nodeName)
		return nil, fmt.Errorf("failed to create worker job: %w", err)
	}

	klog.InfoS("Worker job created successfully", "job", jobName, "node", nodeName)
	return job, nil
}

// GetWorkerJobStatus gets the status of a worker job
func (w *WorkerManager) GetWorkerJobStatus(ctx context.Context, jobName string) (*WorkerJobStatus, error) {
	job := &batchv1.Job{}

	jobQuery := client.ObjectKey{
		Namespace: w.config.GetWorkerNamespace(),
		Name:      jobName,
	}

	err := w.client.Get(ctx, jobQuery, job)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("worker job not found: %s", jobName)
		}
		return nil, fmt.Errorf("failed to get worker job: %w", err)
	}

	status := &WorkerJobStatus{
		Active:    job.Status.Active,
		Succeeded: job.Status.Succeeded,
		Failed:    job.Status.Failed,
	}

	// Get start time
	if job.Status.StartTime != nil {
		startTime := job.Status.StartTime.Time
		status.StartTime = &startTime
	}

	// Check job conditions for completion status
	for _, condition := range job.Status.Conditions {
		switch condition.Type {
		case batchv1.JobComplete:
			if condition.Status == corev1.ConditionTrue {
				status.Completed = true
				finishTime := condition.LastTransitionTime.Time
				status.FinishTime = &finishTime
			}
		case batchv1.JobFailed:
			if condition.Status == corev1.ConditionTrue {
				status.Completed = true
				status.Reason = condition.Reason
				status.Message = condition.Message
				finishTime := condition.LastTransitionTime.Time
				status.FinishTime = &finishTime
			}
		}
	}

	// If job completed successfully, try to get exit code from pod
	if status.Completed && status.Succeeded > 0 {
		if exitCode, err := w.getJobPodExitCode(ctx, job); err == nil {
			status.ExitCode = exitCode
		}
	}

	return status, nil
}

// getJobPodExitCode attempts to get the exit code from a completed job's pod
func (w *WorkerManager) getJobPodExitCode(ctx context.Context, job *batchv1.Job) (*int32, error) {
	podList := &corev1.PodList{}
	err := w.client.List(ctx, podList,
		client.InNamespace(job.Namespace),
		client.MatchingLabels{
			"job-name": job.Name,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods for job: %w", err)
	}

	// Find a completed pod
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			if len(pod.Status.ContainerStatuses) > 0 {
				containerStatus := pod.Status.ContainerStatuses[0]
				if containerStatus.State.Terminated != nil {
					return &containerStatus.State.Terminated.ExitCode, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no terminated container found")
}

// DeleteWorkerJob deletes a worker job
func (w *WorkerManager) DeleteWorkerJob(ctx context.Context, jobName string) error {
	klog.InfoS("Deleting worker job", "job", jobName)

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: w.config.GetWorkerNamespace(),
		},
	}

	// Set propagation policy to delete associated pods
	propagationPolicy := metav1.DeletePropagationForeground
	deleteOptions := &client.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}

	if err := w.client.Delete(ctx, job, deleteOptions); err != nil {
		if errors.IsNotFound(err) {
			klog.InfoS("Worker job already deleted", "job", jobName)
			return nil
		}
		klog.ErrorS(err, "Failed to delete worker job", "job", jobName)
		return fmt.Errorf("failed to delete worker job: %w", err)
	}

	klog.InfoS("Worker job deleted successfully", "job", jobName)
	return nil
}

// FindWorkerJobForNode finds an existing worker job for a given node
func (w *WorkerManager) FindWorkerJobForNode(ctx context.Context, nodeName string) (*batchv1.Job, error) {
	jobList := &batchv1.JobList{}
	err := w.client.List(ctx, jobList,
		client.InNamespace(w.config.GetWorkerNamespace()),
		client.MatchingLabels{
			"app":       "kube-node-ready",
			"component": "worker",
			"node":      nodeName,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list worker jobs: %w", err)
	}

	// Find the most recent non-completed job
	var latestJob *batchv1.Job
	var latestTime time.Time

	for i := range jobList.Items {
		job := &jobList.Items[i]

		// Skip completed jobs (either succeeded or failed)
		completed := false
		for _, condition := range job.Status.Conditions {
			if (condition.Type == batchv1.JobComplete || condition.Type == batchv1.JobFailed) &&
				condition.Status == corev1.ConditionTrue {
				completed = true
				break
			}
		}
		if completed {
			continue
		}

		if job.CreationTimestamp.Time.After(latestTime) {
			latestTime = job.CreationTimestamp.Time
			latestJob = job
		}
	}

	if latestJob != nil {
		return latestJob, nil
	}

	return nil, fmt.Errorf("no active worker job found for node: %s", nodeName)
}

// GetJobUID gets the UID of a job for tracking purposes
func (w *WorkerManager) GetJobUID(ctx context.Context, jobName string) (types.UID, error) {
	job := &batchv1.Job{}
	err := w.client.Get(ctx, client.ObjectKey{
		Namespace: w.config.GetWorkerNamespace(),
		Name:      jobName,
	}, job)
	if err != nil {
		return "", err
	}
	return job.UID, nil
}

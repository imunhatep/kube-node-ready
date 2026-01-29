package controller

import (
	"context"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/imunhatep/kube-node-ready/internal/config"
)

const (
	// Default path to namespace file in Kubernetes service account
	namespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

// WorkerManager manages the lifecycle of worker pods
type WorkerManager struct {
	client    client.Client
	config    *config.ControllerConfig
	namespace string
}

// NewWorkerManager creates a new worker manager
// The namespace is auto-detected from the controller's environment
func NewWorkerManager(client client.Client, cfg *config.ControllerConfig) *WorkerManager {
	namespace := detectNamespace(cfg)
	klog.InfoS("Worker manager initialized", "namespace", namespace)

	return &WorkerManager{
		client:    client,
		config:    cfg,
		namespace: namespace,
	}
}

// detectNamespace determines the namespace where worker pods should be created
// Priority: 1. POD_NAMESPACE env var, 2. Service account namespace file, 3. Config, 4. Default
func detectNamespace(cfg *config.ControllerConfig) string {
	// 1. Try POD_NAMESPACE environment variable (injected by Kubernetes downward API)
	if ns := os.Getenv("POD_NAMESPACE"); ns != "" {
		klog.V(2).InfoS("Using namespace from POD_NAMESPACE env var", "namespace", ns)
		return ns
	}

	// 2. Try reading from service account namespace file (in-cluster)
	if data, err := os.ReadFile(namespaceFile); err == nil {
		ns := string(data)
		if ns != "" {
			klog.V(2).InfoS("Using namespace from service account file", "namespace", ns)
			return ns
		}
	}

	// 3. Try config if explicitly set (backwards compatibility)
	if cfg.Worker.Namespace != "" {
		klog.V(2).InfoS("Using namespace from config", "namespace", cfg.Worker.Namespace)
		return cfg.Worker.Namespace
	}

	// 4. Default to kube-system
	klog.V(2).InfoS("Using default namespace", "namespace", "kube-system")
	return "kube-system"
}

// buildWorkerEnvVars constructs environment variables for worker pods
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

// WorkerPodStatus represents the status of a worker pod
type WorkerPodStatus struct {
	Phase      corev1.PodPhase
	ExitCode   *int32
	Reason     string
	Message    string
	Completed  bool
	Succeeded  bool
	StartTime  *time.Time
	FinishTime *time.Time
}

// CreateWorkerPod creates a worker pod for the given node
func (w *WorkerManager) CreateWorkerPod(ctx context.Context, nodeName string) (*corev1.Pod, error) {
	podName := fmt.Sprintf("node-ready-worker-%s-%d", nodeName, time.Now().Unix())

	klog.InfoS("Creating worker pod", "pod", podName, "node", nodeName)

	// Build tolerations from config
	tolerations := []corev1.Toleration{}
	for _, taint := range w.config.NodeManagement.Taints {
		tolerations = append(tolerations, corev1.Toleration{
			Key:      taint.Key,
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffect(taint.Effect),
		})
	}
	// Add default tolerations
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
		resourceReqs.Limits[corev1.ResourceMemory] = resource.MustParse(w.config.Worker.Resources.Requests.Memory)
		resourceReqs.Requests[corev1.ResourceMemory] = resource.MustParse(w.config.Worker.Resources.Requests.Memory)
	}

	// Add resource limits if specified (optional, especially for CPU)
	if w.config.Worker.Resources.Limits.CPU != "" {
		resourceReqs.Limits[corev1.ResourceCPU] = resource.MustParse(w.config.Worker.Resources.Limits.CPU)
	}

	if w.config.Worker.Resources.Limits.Memory != "" {
		resourceReqs.Limits[corev1.ResourceMemory] = resource.MustParse(w.config.Worker.Resources.Limits.Memory)
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: w.namespace,
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
		Spec: corev1.PodSpec{
			RestartPolicy:      corev1.RestartPolicyNever,
			ServiceAccountName: w.config.Worker.ServiceAccount,
			PriorityClassName:  w.config.Worker.PriorityClassName,
			NodeSelector: map[string]string{
				"kubernetes.io/hostname": nodeName,
			},
			Tolerations: tolerations,
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
	}

	if err := w.client.Create(ctx, pod); err != nil {
		if errors.IsAlreadyExists(err) {
			klog.InfoS("Worker pod already exists", "pod", podName, "node", nodeName)
			// Get the existing pod
			existingPod := &corev1.Pod{}

			podQuery := client.ObjectKey{
				Namespace: w.namespace,
				Name:      podName,
			}
			if err := w.client.Get(ctx, podQuery, existingPod); err != nil {
				return nil, fmt.Errorf("failed to get existing pod: %w", err)
			}
			return existingPod, nil
		}
		klog.ErrorS(err, "Failed to create worker pod", "pod", podName, "node", nodeName)
		return nil, fmt.Errorf("failed to create worker pod: %w", err)
	}

	klog.InfoS("Worker pod created successfully", "pod", podName, "node", nodeName)
	return pod, nil
}

// GetWorkerPodStatus gets the status of a worker pod
func (w *WorkerManager) GetWorkerPodStatus(ctx context.Context, podName string) (*WorkerPodStatus, error) {
	pod := &corev1.Pod{}
	err := w.client.Get(ctx, client.ObjectKey{
		Namespace: w.namespace,
		Name:      podName,
	}, pod)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("worker pod not found: %s", podName)
		}
		return nil, fmt.Errorf("failed to get worker pod: %w", err)
	}

	status := &WorkerPodStatus{
		Phase: pod.Status.Phase,
	}

	// Get start time
	if pod.Status.StartTime != nil {
		startTime := pod.Status.StartTime.Time
		status.StartTime = &startTime
	}

	// Check if pod has completed
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		status.Completed = true
		status.Succeeded = pod.Status.Phase == corev1.PodSucceeded

		// Get container status for exit code
		if len(pod.Status.ContainerStatuses) > 0 {
			containerStatus := pod.Status.ContainerStatuses[0]
			if containerStatus.State.Terminated != nil {
				status.ExitCode = &containerStatus.State.Terminated.ExitCode
				status.Reason = containerStatus.State.Terminated.Reason
				status.Message = containerStatus.State.Terminated.Message
				finishTime := containerStatus.State.Terminated.FinishedAt.Time
				status.FinishTime = &finishTime
			}
		}
	}

	return status, nil
}

// DeleteWorkerPod deletes a worker pod
func (w *WorkerManager) DeleteWorkerPod(ctx context.Context, podName string) error {
	klog.InfoS("Deleting worker pod", "pod", podName)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: w.namespace,
		},
	}

	if err := w.client.Delete(ctx, pod); err != nil {
		if errors.IsNotFound(err) {
			klog.InfoS("Worker pod already deleted", "pod", podName)
			return nil
		}
		klog.ErrorS(err, "Failed to delete worker pod", "pod", podName)
		return fmt.Errorf("failed to delete worker pod: %w", err)
	}

	klog.InfoS("Worker pod deleted successfully", "pod", podName)
	return nil
}

// FindWorkerPodForNode finds an existing worker pod for a given node
func (w *WorkerManager) FindWorkerPodForNode(ctx context.Context, nodeName string) (*corev1.Pod, error) {
	podList := &corev1.PodList{}
	err := w.client.List(ctx, podList,
		client.InNamespace(w.namespace),
		client.MatchingLabels{
			"app":       "kube-node-ready",
			"component": "worker",
			"node":      nodeName,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list worker pods: %w", err)
	}

	// Find the most recent non-completed pod
	var latestPod *corev1.Pod
	var latestTime time.Time

	for i := range podList.Items {
		pod := &podList.Items[i]

		// Skip completed pods
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		if pod.CreationTimestamp.Time.After(latestTime) {
			latestTime = pod.CreationTimestamp.Time
			latestPod = pod
		}
	}

	if latestPod != nil {
		return latestPod, nil
	}

	return nil, fmt.Errorf("no active worker pod found for node: %s", nodeName)
}

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/imunhatep/kube-node-ready/internal/config"
	"github.com/imunhatep/kube-node-ready/internal/metrics"
)

// Controller handles node operations
type Controller struct {
	config    *config.Config
	clientset *kubernetes.Clientset
}

// NewController creates a new node manager
func NewController(cfg *config.Config, clientset *kubernetes.Clientset) *Controller {
	return &Controller{
		config:    cfg,
		clientset: clientset,
	}
}

// RemoveTaintAndAddLabel removes the verification taint and adds the verified label
func (m *Controller) RemoveTaintAndAddLabel(ctx context.Context) error {
	klog.InfoS("Updating node after successful verification",
		"node", m.config.NodeName,
		"taintKey", m.config.TaintKey,
		"label", m.config.VerifiedLabel,
	)

	// Get the current node
	node, err := m.clientset.CoreV1().Nodes().Get(ctx, m.config.NodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	// Create patch operations
	patches := []patchOperation{}

	// Find and remove the taint
	taintIndex := m.findTaintIndex(node.Spec.Taints)
	if taintIndex >= 0 {
		klog.InfoS("Found taint to remove", "index", taintIndex)
		patches = append(patches, patchOperation{
			Op:   "remove",
			Path: fmt.Sprintf("/spec/taints/%d", taintIndex),
		})
	} else {
		klog.InfoS("Taint not found on node", "taintKey", m.config.TaintKey)
	}

	// Add the verified label
	labelPath := "/metadata/labels/" + escapeLabelKey(m.config.VerifiedLabel)
	patches = append(patches, patchOperation{
		Op:    "add",
		Path:  labelPath,
		Value: m.config.VerifiedLabelValue,
	})

	// Apply the patch
	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	klog.InfoS("Applying patch to node", "patch", string(patchBytes))

	// Remove taint with metrics
	if taintIndex >= 0 {
		start := time.Now()
		_, err = m.clientset.CoreV1().Nodes().Patch(
			ctx,
			m.config.NodeName,
			types.JSONPatchType,
			patchBytes,
			metav1.PatchOptions{},
		)
		duration := time.Since(start)

		if err != nil {
			metrics.NodeTaintRemovalTotal.WithLabelValues(m.config.NodeName, "failure").Inc()
			metrics.NodeLabelAddTotal.WithLabelValues(m.config.NodeName, "failure").Inc()
			return fmt.Errorf("failed to patch node: %w", err)
		}

		metrics.NodeTaintRemovalTotal.WithLabelValues(m.config.NodeName, "success").Inc()
		metrics.NodeUpdateDuration.WithLabelValues(m.config.NodeName, "taint_removal").Observe(duration.Seconds())
	} else {
		// No taint to remove, just add label
		start := time.Now()
		_, err = m.clientset.CoreV1().Nodes().Patch(
			ctx,
			m.config.NodeName,
			types.JSONPatchType,
			patchBytes,
			metav1.PatchOptions{},
		)
		duration := time.Since(start)

		if err != nil {
			metrics.NodeLabelAddTotal.WithLabelValues(m.config.NodeName, "failure").Inc()
			return fmt.Errorf("failed to patch node: %w", err)
		}

		metrics.NodeUpdateDuration.WithLabelValues(m.config.NodeName, "label_add").Observe(duration.Seconds())
	}

	metrics.NodeLabelAddTotal.WithLabelValues(m.config.NodeName, "success").Inc()

	klog.InfoS("Successfully updated node",
		"node", m.config.NodeName,
		"label", m.config.VerifiedLabel,
	)

	return nil
}

// findTaintIndex finds the index of the verification taint
func (m *Controller) findTaintIndex(taints []corev1.Taint) int {
	for i, taint := range taints {
		if taint.Key == m.config.TaintKey {
			return i
		}
	}
	return -1
}

// patchOperation represents a JSON patch operation
type patchOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// escapeLabelKey escapes the label key for JSON patch path
// JSON Patch uses RFC 6901 which requires ~1 for / and ~0 for ~
func escapeLabelKey(key string) string {
	result := ""
	for _, char := range key {
		switch char {
		case '/':
			result += "~1"
		case '~':
			result += "~0"
		default:
			result += string(char)
		}
	}
	return result
}

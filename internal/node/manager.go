package node

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	nodeconfig "github.com/imunhatep/kube-node-ready/internal/config"
	"github.com/imunhatep/kube-node-ready/internal/metrics"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// Controller handles node operations
type Manager struct {
	config        *nodeconfig.Config
	clientset     *kubernetes.Clientset
	dynamicClient dynamic.Interface
}

// NewManager creates a new node manager
func NewManager(cfg *nodeconfig.Config, clientset *kubernetes.Clientset, dynamicClient dynamic.Interface) *Manager {
	return &Manager{
		config:        cfg,
		clientset:     clientset,
		dynamicClient: dynamicClient,
	}
}

// RemoveTaintAndAddLabel removes the verification taint and adds the verified label
func (m *Manager) RemoveTaintAndAddLabel(ctx context.Context) error {
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

	metrics.NodeUpdateDuration.WithLabelValues(m.config.NodeName, "label_add").Observe(duration.Seconds())
	metrics.NodeUpdateDuration.WithLabelValues(m.config.NodeName, "taint_removal").Observe(duration.Seconds())
	metrics.NodeLabelAddTotal.WithLabelValues(m.config.NodeName, "success").Inc()

	klog.InfoS("Successfully updated node",
		"node", m.config.NodeName,
		"label", m.config.VerifiedLabel,
	)

	return nil
}

// findTaintIndex finds the index of the verification taint
func (m *Manager) findTaintIndex(taints []corev1.Taint) int {
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

// getNodeClaimGVR returns the GroupVersionResource for Karpenter NodeClaim
func getNodeClaimGVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{
		Group:    "karpenter.sh",
		Version:  "v1beta1",
		Resource: "nodeclaims",
	}
}

// findNodeClaimForNode finds the NodeClaim that corresponds to the given node
func (m *Manager) findNodeClaimForNode(ctx context.Context) (string, error) {
	if m.dynamicClient == nil {
		return "", fmt.Errorf("dynamic client not available")
	}

	gvr := getNodeClaimGVR()

	// List all NodeClaims
	nodeClaims, err := m.dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		// If the resource doesn't exist (e.g., Karpenter not installed), that's not an error
		klog.InfoS("Could not list NodeClaims, Karpenter might not be installed", "error", err)
		return "", nil
	}

	// Find the NodeClaim that corresponds to our node
	for _, item := range nodeClaims.Items {
		// Check if the NodeClaim has a status.nodeName field that matches our node
		if status, found, err := getNestedString(item.Object, "status", "nodeName"); err == nil && found && status == m.config.NodeName {
			return item.GetName(), nil
		}

		// Also check metadata.labels for node reference (alternative pattern)
		if labels := item.GetLabels(); labels != nil {
			if nodeName, exists := labels["karpenter.sh/nodepool"]; exists {
				// Additional logic could be added here to match based on other criteria
				_ = nodeName // placeholder for potential future logic
			}
		}
	}

	return "", nil
}

// deleteNodeClaim deletes the specified NodeClaim
func (m *Manager) deleteNodeClaim(ctx context.Context, nodeClaimName string) error {
	if m.dynamicClient == nil {
		return fmt.Errorf("dynamic client not available")
	}

	gvr := getNodeClaimGVR()

	klog.InfoS("Deleting NodeClaim for failed node",
		"nodeClaim", nodeClaimName,
		"node", m.config.NodeName,
	)

	start := time.Now()
	err := m.dynamicClient.Resource(gvr).Delete(
		ctx,
		nodeClaimName,
		metav1.DeleteOptions{},
	)
	duration := time.Since(start)

	if err != nil {
		klog.ErrorS(err, "Failed to delete NodeClaim",
			"nodeClaim", nodeClaimName,
			"node", m.config.NodeName,
			"duration", duration,
		)
		return fmt.Errorf("failed to delete NodeClaim %s: %w", nodeClaimName, err)
	}

	klog.InfoS("Successfully deleted NodeClaim, node should be terminated by Karpenter",
		"nodeClaim", nodeClaimName,
		"node", m.config.NodeName,
		"duration", duration,
	)

	return nil
}

// getNestedString retrieves a nested string value from an unstructured object
func getNestedString(obj map[string]interface{}, fields ...string) (string, bool, error) {
	current := obj
	for i, field := range fields[:len(fields)-1] {
		val, found := current[field]
		if !found {
			return "", false, nil
		}
		switch v := val.(type) {
		case map[string]interface{}:
			current = v
		default:
			return "", false, fmt.Errorf("field %s at position %d is not a map", field, i)
		}
	}

	finalField := fields[len(fields)-1]
	val, found := current[finalField]
	if !found {
		return "", false, nil
	}

	strVal, ok := val.(string)
	if !ok {
		return "", false, fmt.Errorf("field %s is not a string", finalField)
	}

	return strVal, true, nil
}

// DeleteNode deletes the node from the cluster
// First attempts to delete associated Karpenter NodeClaim if it exists,
// falls back to direct node deletion if no NodeClaim is found
func (m *Manager) DeleteNode(ctx context.Context) error {
	// First, try to find and delete the associated NodeClaim
	nodeClaimName, err := m.findNodeClaimForNode(ctx)
	if err != nil {
		klog.InfoS("Error finding NodeClaim, falling back to direct node deletion",
			"node", m.config.NodeName,
			"error", err,
		)
	} else if nodeClaimName != "" {
		// Found a NodeClaim, delete it instead of the node directly
		err = m.deleteNodeClaim(ctx, nodeClaimName)
		if err != nil {
			klog.ErrorS(err, "Failed to delete NodeClaim, falling back to direct node deletion",
				"nodeClaim", nodeClaimName,
				"node", m.config.NodeName,
			)
			// Continue to direct node deletion as fallback
		} else {
			// Successfully deleted NodeClaim, Karpenter should handle the rest
			return nil
		}
	} else {
		klog.InfoS("No NodeClaim found for node, proceeding with direct node deletion",
			"node", m.config.NodeName,
		)
	}

	// Fallback: delete the node directly
	klog.InfoS("Deleting failed node directly",
		"node", m.config.NodeName,
	)

	start := time.Now()
	err = m.clientset.CoreV1().Nodes().Delete(
		ctx,
		m.config.NodeName,
		metav1.DeleteOptions{},
	)
	duration := time.Since(start)

	if err != nil {
		metrics.NodeDeletionTotal.WithLabelValues(m.config.NodeName, "failure").Inc()
		klog.ErrorS(err, "Failed to delete node",
			"node", m.config.NodeName,
			"duration", duration,
		)
		return fmt.Errorf("failed to delete node: %w", err)
	}

	metrics.NodeDeletionTotal.WithLabelValues(m.config.NodeName, "success").Inc()
	klog.InfoS("Successfully deleted node",
		"node", m.config.NodeName,
		"duration", duration,
	)

	return nil
}

package node

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/imunhatep/kube-node-ready/internal/metrics"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// Manager handles node operations
type Manager struct {
	clientset     *kubernetes.Clientset
	dynamicClient dynamic.Interface
}

// NewManager creates a new node manager
func NewManager(clientset *kubernetes.Clientset, dynamicClient dynamic.Interface) *Manager {
	return &Manager{
		clientset:     clientset,
		dynamicClient: dynamicClient,
	}
}

// UpdateNodeMetadata removes specified taints and adds specified labels to a node
func (m *Manager) UpdateNodeMetadata(ctx context.Context, node *corev1.Node, taintsToRemove []corev1.Taint, labelsToAdd map[string]string) error {
	if m.clientset == nil {
		return fmt.Errorf("kubernetes client not available")
	}

	nodeName := node.Name
	klog.InfoS("Updating node taints and labels",
		"node", nodeName,
		"taintsToRemove", len(taintsToRemove),
		"labelsToAdd", len(labelsToAdd),
	)

	// Create patch operations
	patches := []patchOperation{}

	// Remove specified taints
	for _, taintToRemove := range taintsToRemove {
		taintIndex := m.findTaintIndex(node.Spec.Taints, taintToRemove.Key)
		if taintIndex >= 0 {
			klog.InfoS("Found taint to remove", "taintKey", taintToRemove.Key, "index", taintIndex)
			patches = append(patches, patchOperation{
				Op:   "remove",
				Path: fmt.Sprintf("/spec/taints/%d", taintIndex),
			})
		} else {
			klog.InfoS("Taint not found on node", "taintKey", taintToRemove.Key, "node", nodeName)
		}
	}

	// Add specified labels
	for key, value := range labelsToAdd {
		labelPath := "/metadata/labels/" + escapeLabelKey(key)
		patches = append(patches, patchOperation{
			Op:    "add",
			Path:  labelPath,
			Value: value,
		})
	}

	if len(patches) == 0 {
		klog.InfoS("No patches to apply", "node", nodeName)
		return nil
	}

	// Apply the patch
	patchBytes, err := json.Marshal(patches)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	klog.InfoS("Applying patch to node", "node", nodeName, "patch", string(patchBytes))

	start := time.Now()
	_, err = m.clientset.CoreV1().Nodes().Patch(
		ctx,
		nodeName,
		types.JSONPatchType,
		patchBytes,
		metav1.PatchOptions{},
	)
	duration := time.Since(start)

	if err != nil {
		metrics.NodeTaintRemovalTotal.WithLabelValues(nodeName, "failure").Inc()
		metrics.NodeLabelAddTotal.WithLabelValues(nodeName, "failure").Inc()
		return fmt.Errorf("failed to patch node: %w", err)
	}

	metrics.NodeUpdateDuration.WithLabelValues(nodeName, "label_add").Observe(duration.Seconds())
	metrics.NodeUpdateDuration.WithLabelValues(nodeName, "taint_removal").Observe(duration.Seconds())
	metrics.NodeLabelAddTotal.WithLabelValues(nodeName, "success").Inc()

	klog.InfoS("Successfully updated node",
		"node", nodeName,
		"taintsRemoved", len(taintsToRemove),
		"labelsAdded", len(labelsToAdd),
	)

	return nil
}

// findTaintIndex finds the index of a taint with the specified key
func (m *Manager) findTaintIndex(taints []corev1.Taint, taintKey string) int {
	for i, taint := range taints {
		if taint.Key == taintKey {
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
func (m *Manager) findNodeClaimForNode(ctx context.Context, node *corev1.Node) (string, error) {
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
		if status, found, err := getNestedString(item.Object, "status", "nodeName"); err == nil && found && status == node.Name {
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
func (m *Manager) deleteNodeClaim(ctx context.Context, node *corev1.Node, nodeClaimName string) error {
	if m.dynamicClient == nil {
		return fmt.Errorf("dynamic client not available")
	}

	gvr := getNodeClaimGVR()

	klog.InfoS("Deleting NodeClaim for failed node",
		"nodeClaim", nodeClaimName,
		"node", node.Name,
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
			"node", node.Name,
			"duration", duration,
		)
		return fmt.Errorf("failed to delete NodeClaim %s: %w", nodeClaimName, err)
	}

	klog.InfoS("Successfully deleted NodeClaim, node should be terminated by Karpenter",
		"nodeClaim", nodeClaimName,
		"node", node.Name,
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
func (m *Manager) DeleteNode(ctx context.Context, node *corev1.Node) error {
	nodeName := node.Name

	// First, try to find and delete the associated NodeClaim
	nodeClaimName, err := m.findNodeClaimForNode(ctx, node)
	if err != nil {
		klog.InfoS("Error finding NodeClaim, falling back to direct node deletion",
			"node", nodeName,
			"error", err,
		)
	} else if nodeClaimName != "" {
		// Found a NodeClaim, delete it instead of the node directly
		err = m.deleteNodeClaim(ctx, node, nodeClaimName)
		if err != nil {
			klog.ErrorS(err, "Failed to delete NodeClaim, falling back to direct node deletion",
				"nodeClaim", nodeClaimName,
				"node", nodeName,
			)
			// Continue to direct node deletion as fallback
		} else {
			// Successfully deleted NodeClaim, Karpenter should handle the rest
			return nil
		}
	} else {
		klog.InfoS("No NodeClaim found for node, proceeding with direct node deletion",
			"node", nodeName,
		)
	}

	// Fallback: delete the node directly
	klog.InfoS("Deleting failed node directly",
		"node", nodeName,
	)

	start := time.Now()
	err = m.clientset.CoreV1().Nodes().Delete(
		ctx,
		nodeName,
		metav1.DeleteOptions{},
	)
	duration := time.Since(start)

	if err != nil {
		metrics.NodeDeletionTotal.WithLabelValues(nodeName, "failure").Inc()
		klog.ErrorS(err, "Failed to delete node",
			"node", nodeName,
			"duration", duration,
		)
		return fmt.Errorf("failed to delete node: %w", err)
	}

	metrics.NodeDeletionTotal.WithLabelValues(nodeName, "success").Inc()
	klog.InfoS("Successfully deleted node",
		"node", nodeName,
		"duration", duration,
	)

	return nil
}

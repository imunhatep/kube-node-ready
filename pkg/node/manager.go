package node

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"github.com/imunhatep/kube-node-ready/pkg/config"
)

// Manager handles node operations
type Manager struct {
	logger    *zap.Logger
	config    *config.Config
	clientset *kubernetes.Clientset
}

// NewManager creates a new node manager
func NewManager(logger *zap.Logger, cfg *config.Config, clientset *kubernetes.Clientset) *Manager {
	return &Manager{
		logger:    logger,
		config:    cfg,
		clientset: clientset,
	}
}

// RemoveTaintAndAddLabel removes the verification taint and adds the verified label
func (m *Manager) RemoveTaintAndAddLabel(ctx context.Context) error {
	m.logger.Info("Updating node after successful verification",
		zap.String("node", m.config.NodeName),
		zap.String("taintKey", m.config.TaintKey),
		zap.String("label", m.config.VerifiedLabel),
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
		m.logger.Info("Found taint to remove", zap.Int("index", taintIndex))
		patches = append(patches, patchOperation{
			Op:   "remove",
			Path: fmt.Sprintf("/spec/taints/%d", taintIndex),
		})
	} else {
		m.logger.Warn("Taint not found on node", zap.String("taintKey", m.config.TaintKey))
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

	m.logger.Info("Applying patch to node", zap.String("patch", string(patchBytes)))

	_, err = m.clientset.CoreV1().Nodes().Patch(
		ctx,
		m.config.NodeName,
		types.JSONPatchType,
		patchBytes,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch node: %w", err)
	}

	m.logger.Info("Successfully updated node",
		zap.String("node", m.config.NodeName),
		zap.String("label", m.config.VerifiedLabel),
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

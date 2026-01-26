package checker

import (
	"context"
	"fmt"
	"net"
	"time"

	"k8s.io/klog/v2"

	"github.com/imunhatep/kube-node-ready/pkg/metrics"
)

// NetworkChecker performs network connectivity checks
type NetworkChecker struct {
	timeout time.Duration
}

// NewNetworkChecker creates a new network checker
func NewNetworkChecker(timeout time.Duration) *NetworkChecker {
	return &NetworkChecker{
		timeout: timeout,
	}
}

// CheckTCP performs a TCP connection check to the given address
func (n *NetworkChecker) CheckTCP(ctx context.Context, address string) error {
	start := time.Now()
	klog.InfoS("Starting TCP connectivity check", "address", address)

	// Create a dialer with timeout
	dialer := &net.Dialer{
		Timeout: n.timeout,
	}

	// Attempt to connect
	conn, err := dialer.DialContext(ctx, "tcp", address)
	duration := time.Since(start)

	// Record metrics
	metrics.NetworkCheckDuration.WithLabelValues(address).Observe(duration.Seconds())

	if err != nil {
		metrics.NetworkCheckTotal.WithLabelValues(address, "failure").Inc()
		klog.ErrorS(err, "TCP connectivity check failed",
			"address", address,
			"duration", duration,
		)
		return fmt.Errorf("TCP connection to %s failed: %w", address, err)
	}
	defer conn.Close()

	metrics.NetworkCheckTotal.WithLabelValues(address, "success").Inc()
	klog.InfoS("TCP connectivity check passed",
		"address", address,
		"duration", duration,
	)

	return nil
}

// CheckKubernetesService checks connectivity to the Kubernetes API server
func (n *NetworkChecker) CheckKubernetesService(ctx context.Context, host, port string) error {
	address := net.JoinHostPort(host, port)
	return n.CheckTCP(ctx, address)
}

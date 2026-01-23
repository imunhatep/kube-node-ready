package checker

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
)

// NetworkChecker performs network connectivity checks
type NetworkChecker struct {
	logger  *zap.Logger
	timeout time.Duration
}

// NewNetworkChecker creates a new network checker
func NewNetworkChecker(logger *zap.Logger, timeout time.Duration) *NetworkChecker {
	return &NetworkChecker{
		logger:  logger,
		timeout: timeout,
	}
}

// CheckTCP performs a TCP connection check to the given address
func (n *NetworkChecker) CheckTCP(ctx context.Context, address string) error {
	start := time.Now()
	n.logger.Info("Starting TCP connectivity check", zap.String("address", address))

	// Create a dialer with timeout
	dialer := &net.Dialer{
		Timeout: n.timeout,
	}

	// Attempt to connect
	conn, err := dialer.DialContext(ctx, "tcp", address)
	duration := time.Since(start)

	if err != nil {
		n.logger.Error("TCP connectivity check failed",
			zap.String("address", address),
			zap.Duration("duration", duration),
			zap.Error(err),
		)
		return fmt.Errorf("TCP connection to %s failed: %w", address, err)
	}
	defer conn.Close()

	n.logger.Info("TCP connectivity check passed",
		zap.String("address", address),
		zap.Duration("duration", duration),
	)

	return nil
}

// CheckKubernetesService checks connectivity to the Kubernetes API server
func (n *NetworkChecker) CheckKubernetesService(ctx context.Context, host, port string) error {
	address := net.JoinHostPort(host, port)
	return n.CheckTCP(ctx, address)
}

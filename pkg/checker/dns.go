package checker

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
)

// DNSChecker performs DNS resolution checks
type DNSChecker struct {
	logger  *zap.Logger
	timeout time.Duration
}

// NewDNSChecker creates a new DNS checker
func NewDNSChecker(logger *zap.Logger, timeout time.Duration) *DNSChecker {
	return &DNSChecker{
		logger:  logger,
		timeout: timeout,
	}
}

// Check performs DNS resolution check for the given domain
func (d *DNSChecker) Check(ctx context.Context, domain string) error {
	start := time.Now()
	d.logger.Info("Starting DNS check", zap.String("domain", domain))

	// Create a context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	// Create a resolver
	resolver := &net.Resolver{}

	// Perform lookup
	addrs, err := resolver.LookupHost(checkCtx, domain)
	duration := time.Since(start)

	if err != nil {
		d.logger.Error("DNS check failed",
			zap.String("domain", domain),
			zap.Duration("duration", duration),
			zap.Error(err),
		)
		return fmt.Errorf("DNS resolution failed for %s: %w", domain, err)
	}

	if len(addrs) == 0 {
		d.logger.Error("DNS check returned no addresses",
			zap.String("domain", domain),
			zap.Duration("duration", duration),
		)
		return fmt.Errorf("DNS resolution returned no addresses for %s", domain)
	}

	d.logger.Info("DNS check passed",
		zap.String("domain", domain),
		zap.Strings("addresses", addrs),
		zap.Duration("duration", duration),
	)

	return nil
}

// CheckAll performs DNS checks for multiple domains
func (d *DNSChecker) CheckAll(ctx context.Context, domains []string) error {
	for _, domain := range domains {
		if err := d.Check(ctx, domain); err != nil {
			return err
		}
	}
	return nil
}

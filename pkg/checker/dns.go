package checker

import (
	"context"
	"fmt"
	"net"
	"time"

	"k8s.io/klog/v2"
)

// DNSChecker performs DNS resolution checks
type DNSChecker struct {
	timeout time.Duration
}

// NewDNSChecker creates a new DNS checker
func NewDNSChecker(timeout time.Duration) *DNSChecker {
	return &DNSChecker{
		timeout: timeout,
	}
}

// Check performs DNS resolution check for the given domain
func (d *DNSChecker) Check(ctx context.Context, domain string) error {
	start := time.Now()
	klog.InfoS("Starting DNS check", "domain", domain)

	// Create a context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	// Create a resolver
	resolver := &net.Resolver{}

	// Perform lookup
	addrs, err := resolver.LookupHost(checkCtx, domain)
	duration := time.Since(start)

	if err != nil {
		klog.ErrorS(err, "DNS check failed",
			"domain", domain,
			"duration", duration,
		)
		return fmt.Errorf("DNS resolution failed for %s: %w", domain, err)
	}

	if len(addrs) == 0 {
		klog.ErrorS(nil, "DNS check returned no addresses",
			"domain", domain,
			"duration", duration,
		)
		return fmt.Errorf("DNS resolution returned no addresses for %s", domain)
	}

	klog.InfoS("DNS check passed",
		"domain", domain,
		"addresses", addrs,
		"duration", duration,
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

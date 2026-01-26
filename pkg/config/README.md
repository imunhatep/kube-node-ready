# Config Package

This package handles configuration loading and validation for kube-node-ready.

## Features

- Command-line flag support with automatic environment variable binding
- Dry-run mode with sensible defaults
- Comprehensive validation
- Full test coverage

## Configuration Sources

Configuration is loaded via **urfave/cli/v3** which automatically handles both:
- **Command-line Flags** - Direct flag values
- **Environment Variables** - Automatically bound to flags via `Sources: cli.EnvVars()`

This means you can use either approach interchangeably - no need for manual environment variable parsing.

## Command-line Flags

### Core Flags
- `--node-name` - Name of the node to verify (required in production mode)
- `--namespace` - Namespace for the pod (default: `kube-system`)
- `--dry-run` - Run in dry-run mode (does not modify node taints/labels, performs all checks)
- `--kubeconfig` - Path to kubeconfig file (for out-of-cluster usage)

### Timeout & Retry Flags
- `--initial-timeout` - Maximum time for verification (default: `300s`)
- `--check-timeout` - Timeout for individual checks (default: `10s`)
- `--max-retries` - Maximum number of retry attempts (default: `5`)
- `--retry-backoff` - Retry backoff strategy: `exponential` or `linear` (default: `exponential`)

### DNS Flags
- `--dns-test-domains` - DNS domains to test (default: `kubernetes.default.svc.cluster.local,google.com`)
- `--cluster-dns-ip` - Cluster DNS IP address

### Kubernetes Service Flags
- `--k8s-service-host` - Kubernetes API server host
- `--k8s-service-port` - Kubernetes API server port (default: `443`)

### Taint & Label Flags
- `--taint-key` - Taint key to remove on success (default: `node-ready/unverified`)
- `--taint-value` - Taint value (default: `true`)
- `--taint-effect` - Taint effect (default: `NoSchedule`)
- `--verified-label` - Label to add on success (default: `node-ready/verified`)
- `--verified-label-value` - Verified label value (default: `true`)

### Metrics & Logging Flags
- `--enable-metrics` - Enable metrics endpoint (default: `true`)
- `--metrics-port` - Metrics port (default: `8080`)
- `--log-level` - Log level: `debug`, `info`, `warn`, `error` (default: `info`)
- `--log-format` - Log format: `json`, `console` (default: `json`)
- `--klog-verbosity` - Klog verbosity level 0-10, higher is more verbose (default: `0`)

## Environment Variables

### Required (Production Mode)

- `NODE_NAME` - Name of the node to verify
- `KUBERNETES_SERVICE_HOST` - Kubernetes API server host

### Optional

- `POD_NAMESPACE` - Namespace for the pod (default: `kube-system`)
- `INITIAL_TIMEOUT` - Initial check timeout (default: `300s`)
- `CHECK_TIMEOUT` - Timeout for individual checks (DNS, network, Kubernetes API) (default: `10s`)
- `MAX_RETRIES` - Maximum number of retries (default: `5`)
- `RETRY_BACKOFF` - Retry backoff strategy: `exponential` or `linear` (default: `exponential`)
- `DNS_TEST_DOMAINS` - Comma-separated list of domains to test (default: `kubernetes.default.svc.cluster.local,google.com`)
- `CLUSTER_DNS_IP` - Cluster DNS IP address
- `KUBERNETES_SERVICE_PORT` - Kubernetes API server port (default: `443`)
- `TAINT_KEY` - Taint key to apply (default: `node-ready/unverified`)
- `TAINT_VALUE` - Taint value (default: `true`)
- `TAINT_EFFECT` - Taint effect (default: `NoSchedule`)
- `VERIFIED_LABEL` - Label to apply after verification (default: `node-ready/verified`)
- `VERIFIED_LABEL_VALUE` - Verified label value (default: `true`)
- `ENABLE_METRICS` - Enable metrics endpoint (default: `true`)
- `METRICS_PORT` - Metrics port (default: `8080`)
- `LOG_LEVEL` - Log level (default: `info`)
- `LOG_FORMAT` - Log format (default: `json`)
- `KLOG_VERBOSITY` - Klog verbosity level (default: `0`)
- `DRY_RUN` - Enable dry-run mode (default: `false`)
- `KUBECONFIG` - Path to kubeconfig file

## Dry-Run Mode

In dry-run mode, the following defaults are automatically applied if not set:

- `NODE_NAME` → `dry-run-node`
- `KUBERNETES_SERVICE_HOST` → `kubernetes.default.svc.cluster.local`

This allows you to test the application locally without a full Kubernetes cluster configuration.

## Usage Examples

### Production Mode (In-Cluster)

```bash
# NODE_NAME and KUBERNETES_SERVICE_HOST are typically auto-injected by Kubernetes
export NODE_NAME=worker-node-1
export KUBERNETES_SERVICE_HOST=10.0.0.1
./kube-node-ready
```

### Dry-Run Mode (Local Testing)

```bash
# Test locally with your current kubeconfig
./kube-node-ready --dry-run --log-format=console

# Or using environment variables
export DRY_RUN=true
export LOG_FORMAT=console
./kube-node-ready
```

### With Custom Kubeconfig

```bash
# Via flag
./kube-node-ready --dry-run --kubeconfig=$HOME/.kube/config

# Via environment variable
export KUBECONFIG=$HOME/.kube/config
./kube-node-ready --dry-run
```

### Custom Timeout and Retry Settings

```bash
# Via flags
./kube-node-ready --check-timeout=15s --max-retries=10 --retry-backoff=linear

# Via environment variables
export CHECK_TIMEOUT=15s
export MAX_RETRIES=10
export RETRY_BACKOFF=linear
./kube-node-ready
```

## Testing

Run the tests:

```bash
go test ./pkg/config -v
```

Run tests with coverage:

```bash
go test ./pkg/config -v -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Test Coverage

The package includes comprehensive tests for:

- Configuration loading from environment variables
- Validation logic (both production and dry-run modes)
- Helper functions (getEnv, getIntEnv, getBoolEnv, etc.)
- Error conditions and edge cases

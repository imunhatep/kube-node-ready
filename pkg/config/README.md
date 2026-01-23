# Config Package

This package handles configuration loading and validation for kube-node-ready.

## Features

- Environment variable configuration
- Command-line flag support
- Dry-run mode with sensible defaults
- Comprehensive validation
- Full test coverage

## Configuration Sources

Configuration is loaded in the following order (later sources override earlier ones):

1. **Environment Variables** - Base configuration
2. **Command-line Flags** - Override environment variables

## Command-line Flags

- `--dry-run` - Run in dry-run mode (does not modify node taints/labels)
- `--skip-k8s` - Skip Kubernetes client creation (for testing configuration only, implies `--dry-run`)
- `--log-level` - Log level (debug, info, warn, error)
- `--log-format` - Log format (json, console)
- `--kubeconfig` - Path to kubeconfig file (for dry-run mode)

## Environment Variables

### Required (Production Mode)

- `NODE_NAME` - Name of the node to verify
- `KUBERNETES_SERVICE_HOST` - Kubernetes API server host

### Optional

- `POD_NAMESPACE` - Namespace for the pod (default: `kube-system`)
- `INITIAL_TIMEOUT` - Initial check timeout (default: `300s`)
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
- `DRY_RUN` - Enable dry-run mode (default: `false`)
- `KUBECONFIG` - Path to kubeconfig file

## Dry-Run Mode

In dry-run mode, the following defaults are automatically applied if not set:

- `NODE_NAME` → `dry-run-node`
- `KUBERNETES_SERVICE_HOST` → `kubernetes.default.svc.cluster.local`

This allows you to test the application locally without a full Kubernetes cluster configuration.

## Usage Examples

### Production Mode

```bash
export NODE_NAME=worker-node-1
export KUBERNETES_SERVICE_HOST=10.0.0.1
go run ./cmd/kube-node-ready
```

### Dry-Run Mode (Local Testing)

```bash
go run ./cmd/kube-node-ready --dry-run --log-format=console --log-level=debug
```

### Configuration Testing (No Kubernetes)

```bash
# Test configuration loading without connecting to Kubernetes
go run ./cmd/kube-node-ready --skip-k8s --log-format=console
```

### With Custom Kubeconfig

```bash
go run ./cmd/kube-node-ready --dry-run --kubeconfig=$HOME/.kube/config
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

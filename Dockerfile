# Build stage
FROM golang:1.25-alpine AS builder

# Version arguments (can be passed at build time)
ARG VERSION=dev
ARG COMMIT_HASH=unknown
ARG BUILD_DATE=unknown

# Architecture arguments (automatically set by Docker buildx)
ARG TARGETOS=linux
ARG TARGETARCH

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build all binaries with version information for target architecture
# DaemonSet binary (legacy mode)
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH} -X main.buildDate=${BUILD_DATE}" \
    -o kube-node-ready \
    ./cmd/kube-node-ready

# Controller binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH} -X main.buildDate=${BUILD_DATE}" \
    -o kube-node-ready-controller \
    ./cmd/kube-node-ready-controller

# Worker binary
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH} -X main.buildDate=${BUILD_DATE}" \
    -o kube-node-ready-worker \
    ./cmd/kube-node-ready-worker

# Runtime stage
FROM alpine:3.21

# Install ca-certificates for HTTPS connections
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN addgroup -g 1000 nodecheck && \
    adduser -D -u 1000 -G nodecheck nodecheck

# Copy all binaries from builder
COPY --from=builder /build/kube-node-ready /usr/bin/kube-node-ready
COPY --from=builder /build/kube-node-ready-controller /usr/bin/kube-node-ready-controller
COPY --from=builder /build/kube-node-ready-worker /usr/bin/kube-node-ready-worker

# Set ownership
RUN chown nodecheck:nodecheck /usr/bin/kube-node-ready && \
    chown nodecheck:nodecheck /usr/bin/kube-node-ready-controller && \
    chown nodecheck:nodecheck /usr/bin/kube-node-ready-worker

# Use non-root user
USER nodecheck

# Set entrypoint
ENTRYPOINT ["/usr/bin/kube-node-ready"]

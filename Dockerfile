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

# Build the binary with version information for target architecture
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commitHash=${COMMIT_HASH} -X main.buildDate=${BUILD_DATE}" \
    -o kube-node-ready \
    ./cmd/kube-node-ready

# Runtime stage
FROM alpine:3.21

# Install ca-certificates for HTTPS connections
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN addgroup -g 1000 nodecheck && \
    adduser -D -u 1000 -G nodecheck nodecheck

# Copy binary from builder
COPY --from=builder /build/kube-node-ready /usr/local/bin/kube-node-ready

# Set ownership
RUN chown nodecheck:nodecheck /usr/local/bin/kube-node-ready

# Use non-root user
USER nodecheck

# Set entrypoint
ENTRYPOINT ["/usr/local/bin/kube-node-ready"]

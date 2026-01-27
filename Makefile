# Makefile for kube-node-ready

# Variables
BINARY_NAME=kube-node-ready
BINARY_NAME_CONTROLLER=kube-node-ready-controller
BINARY_NAME_WORKER=kube-node-ready-worker
IMAGE_NAME=kube-node-ready
IMAGE_TAG?=latest
REGISTRY?=docker.io
GO_VERSION=1.25
HELM_CHART=./deploy/helm/kube-node-ready

# Version information
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT_HASH?=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# Linker flags to set version information
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commitHash=$(COMMIT_HASH) -X main.buildDate=$(BUILD_DATE)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

# Main targets
.PHONY: all build clean test docker-build docker-push helm-lint help

all: test build

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME) version $(VERSION)..."
	mkdir -p bin/
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME) -v ./cmd/kube-node-ready

## build-worker: Build the worker binary
build-worker:
	@echo "Building $(BINARY_NAME_WORKER) version $(VERSION)..."
	mkdir -p bin/
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME_WORKER) -v ./cmd/kube-node-ready-worker

## build-controller: Build the controller binary
build-controller:
	@echo "Building $(BINARY_NAME_CONTROLLER) version $(VERSION)..."
	mkdir -p bin/
	$(GOBUILD) $(LDFLAGS) -o bin/$(BINARY_NAME_CONTROLLER) -v ./cmd/kube-node-ready-controller

## build-all: Build all binaries (legacy, worker, and controller)
build-all: build build-worker build-controller

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf bin/
	rm -rf dist/

## test: Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.txt -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.txt -o coverage.html

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GOCMD) fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

## lint: Run linters (requires golangci-lint)
lint:
	@echo "Running linters..."
	golangci-lint run

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image $(IMAGE_NAME):$(IMAGE_TAG) version $(VERSION)..."
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT_HASH=$(COMMIT_HASH) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) \
		.

## docker-build-multiarch: Build multi-architecture Docker image (amd64, arm64)
docker-build-multiarch:
	@echo "Building multi-arch Docker image $(IMAGE_NAME):$(IMAGE_TAG) version $(VERSION)..."
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT_HASH=$(COMMIT_HASH) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) \
		.

## docker-push: Push Docker image
docker-push:
	@echo "Pushing Docker image..."
	docker push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

## docker-push-multiarch: Build and push multi-architecture Docker image
docker-push-multiarch:
	@echo "Building and pushing multi-arch Docker image $(IMAGE_NAME):$(IMAGE_TAG) version $(VERSION)..."
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT_HASH=$(COMMIT_HASH) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) \
		--push \
		.

## docker-run: Run Docker container locally
docker-run:
	@echo "Running Docker container..."
	docker run --rm $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

## helm-lint: Lint Helm chart
helm-lint:
	@echo "Linting Helm chart..."
	helm lint $(HELM_CHART)

## helm-template: Render Helm chart templates
helm-template:
	@echo "Rendering Helm chart templates..."
	helm template kube-node-ready $(HELM_CHART) --debug

## helm-install: Install Helm chart
helm-install:
	@echo "Installing Helm chart..."
	helm install kube-node-ready $(HELM_CHART) \
		--namespace kube-system \
		--create-namespace

## helm-upgrade: Upgrade Helm chart
helm-upgrade:
	@echo "Upgrading Helm chart..."
	helm upgrade kube-node-ready $(HELM_CHART) \
		--namespace kube-system

## helm-uninstall: Uninstall Helm chart
helm-uninstall:
	@echo "Uninstalling Helm chart..."
	helm uninstall kube-node-ready --namespace kube-system

## helm-package: Package Helm chart
helm-package:
	@echo "Packaging Helm chart..."
	helm package $(HELM_CHART) -d dist/

## run: Run locally (requires kubeconfig)
run: build
	@echo "Running $(BINARY_NAME) locally..."
	./bin/$(BINARY_NAME)

## dry-run: Run in dry-run mode (no node modifications)
dry-run: build
	@echo "Running $(BINARY_NAME) in dry-run mode..."
	./bin/$(BINARY_NAME) --dry-run --log-format=console

## dry-run-debug: Run in dry-run mode with debug logging
dry-run-debug: build
	@echo "Running $(BINARY_NAME) in dry-run mode with debug logging..."
	./bin/$(BINARY_NAME) --dry-run --log-format=console --log-level=debug

## install: Install binary to GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GOCMD) install ./cmd/kube-node-ready

## version: Show version information
version:
	@echo "Version:     $(VERSION)"
	@echo "Commit Hash: $(COMMIT_HASH)"
	@echo "Build Date:  $(BUILD_DATE)"

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

.DEFAULT_GOAL := help

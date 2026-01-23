# Makefile for kube-node-ready

# Variables
BINARY_NAME=kube-node-ready
IMAGE_NAME=kube-node-ready
IMAGE_TAG?=latest
REGISTRY?=docker.io
GO_VERSION=1.25
HELM_CHART=./deploy/helm/kube-node-ready

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
	@echo "Building $(BINARY_NAME)..."
	$(GOBUILD) -o bin/$(BINARY_NAME) -v ./cmd/kube-node-ready

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
	@echo "Building Docker image..."
	docker build -t $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG) .

## docker-push: Push Docker image
docker-push:
	@echo "Pushing Docker image..."
	docker push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

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

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

.DEFAULT_GOAL := help

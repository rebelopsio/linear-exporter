# =============================================================================
# Housekeeping

# Set variables
GO 										:= golang:1.25
ALPINE 								:= alpine:3.23
VERSION 							:= "0.0.1-$(shell git rev-parse --short HEAD)"
GOLANGCI_LINT_VERSION	:= 2.7.2

# Docker/Build variables
EXPORTER_IMAGE 			?= linear-exporter:$(VERSION)
REGISTRY 						?=
FULL_IMAGE 					= $(if $(REGISTRY),$(REGISTRY)/$(EXPORTER_IMAGE),$(EXPORTER_IMAGE))
BUILD_DATE 					:= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# ==============================================================================
# Setup

# Configure git to use project hooks so pre-commit runs for all developers.
setup:
	git config core.hooksPath .githooks

print-golangci-lint-version:
	@echo $(GOLANGCI_LINT_VERSION)

# ==============================================================================
# Detect operating system and set the appropriate open command

UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
	OPEN_CMD := open
else
	OPEN_CMD := xdg-open
endif

# ==============================================================================
# Building containers

build: exporter

exporter:
	@echo "Building exporter image: $(FULL_IMAGE)"
	docker build \
		-f Dockerfile \
		-t $(EXPORTER_IMAGE) \
		--build-arg BUILD_TAG=$(VERSION) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		--label "version=$(VERSION)" \
		--label "build.date=$(BUILD_DATE)" \
		--label "vcs.ref=$(shell git rev-parse HEAD 2>/dev/null || echo 'unknown')" \
		.
	@echo "✓ Image built: $(EXPORTER_IMAGE)"
	@docker images $(shell echo $(EXPORTER_IMAGE) | cut -d: -f1) --format "{{.Repository}}:{{.Tag}}\t{{.Size}}"

# Push Docker image to registry
push: exporter
	@if [ -z "$(REGISTRY)" ]; then \
		echo "Error: REGISTRY not set"; \
		echo "Usage: make push REGISTRY=gcr.io/my-project"; \
		exit 1; \
	fi
	@echo "Pushing Docker image: $(FULL_IMAGE)"
	docker tag $(EXPORTER_IMAGE) $(FULL_IMAGE)
	docker push $(FULL_IMAGE)
	@echo "✓ Image pushed to registry: $(FULL_IMAGE)"

# Scan image for vulnerabilities (requires trivy)
scan: exporter
	@echo "Scanning image for vulnerabilities: $(EXPORTER_IMAGE)"
	@if command -v trivy &> /dev/null; then \
		trivy image $(EXPORTER_IMAGE); \
	else \
		echo "trivy not found. Install from: https://github.com/aquasecurity/trivy"; \
		exit 1; \
	fi

# Show image details
image-info: exporter
	@echo "Image information:"
	@docker images $(shell echo $(EXPORTER_IMAGE) | cut -d: -f1) --format "Name: {{.Repository}}:{{.Tag}}\nSize: {{.Size}}\nCreated: {{.CreatedAt}}"
	@echo ""
	@echo "Image layers (largest first):"
	@docker history $(EXPORTER_IMAGE) --human

# ==============================================================================
# Docker Compose

compose-up:
	docker compose -f ./docker-compose.yml -p compose up -d

compose-down:
	docker compose -f ./docker-compose.yml -p compose down

compose-logs:
	docker compose -f ./docker-compose.yml -p compose logs -f

compose-rebuild:
	docker compose -f ./docker-compose.yml -p compose build --no-cache
	docker compose -f ./docker-compose.yml -p compose up -d

# ==============================================================================
# Modules support

deps-reset:
	git checkout -- go.mod 
	go mod tidy
	go mod vendor

tidy:
	go mod tidy
	go mod vendor

deps-list:
	go list -m -u -mod=readonly all

deps-upgrade:
	go get -u -v ./... 
	go mod tidy
	go mod vendor

deps-cleancache:
	go clean -modcache

list:
	go list -mod=mod all

# ==============================================================================
# Local testing

run-local: exporter
	@if [ -z "$(LINEAR_API_KEY)" ]; then \
		echo "Error: LINEAR_API_KEY environment variable not set"; \
		exit 1; \
	fi
	@echo "Running exporter locally on port 8080"
	@echo "Press Ctrl+C to stop"
	@docker run --rm \
		--name linear-exporter-dev \
		-p 8080:8080 \
		-e LINEAR_API_KEY=$(LINEAR_API_KEY) \
		$(EXPORTER_IMAGE)

# ==============================================================================
# Help

.PHONY: help setup print-golangci-lint-version build exporter push scan image-info compose-up compose-down compose-logs compose-rebuild deps-reset tidy deps-list deps-upgrade deps-cleancache list run-local help

help:
	@echo "Linear Exporter Build System"
	@echo "============================="
	@echo ""
	@echo "Setup & Config:"
	@echo "  make setup                    - Configure git hooks"
	@echo "  make print-golangci-lint-version - Show linter version"
	@echo ""
	@echo "Building:"
	@echo "  make build                    - Build exporter image (default)"
	@echo "  make exporter                 - Build exporter image"
	@echo "  make push                     - Push to registry (requires REGISTRY var)"
	@echo "  make scan                     - Scan image for vulnerabilities (requires trivy)"
	@echo "  make image-info               - Show image size and layers"
	@echo ""
	@echo "Running:"
	@echo "  make run-local                - Run exporter locally (requires LINEAR_API_KEY)"
	@echo ""
	@echo "Docker Compose:"
	@echo "  make compose-up               - Start services"
	@echo "  make compose-down             - Stop services"
	@echo "  make compose-logs             - View service logs"
	@echo "  make compose-rebuild          - Rebuild and restart services"
	@echo ""
	@echo "Dependencies:"
	@echo "  make deps-reset               - Reset go.mod and vendor"
	@echo "  make deps-list                - List available dependency updates"
	@echo "  make deps-upgrade             - Upgrade all dependencies"
	@echo "  make deps-cleancache          - Clean Go module cache"
	@echo "  make tidy                     - Tidy and vendor modules"
	@echo "  make list                     - List all modules"
	@echo ""
	@echo "Variables:"
	@echo "  REGISTRY=URL                  - Docker registry for push target"
	@echo "  LINEAR_API_KEY=KEY            - Linear API key for local run"
	@echo ""
	@echo "Examples:"
	@echo "  make push REGISTRY=gcr.io/my-project"
	@echo "  make run-local LINEAR_API_KEY=lin_xxx"
	@echo "  make image-info"

PLUGIN_BINARY:=nomad-driver-virt
SHELL = bash

THIS_OS := $(shell uname | cut -d- -f1)
THIS_ARCH := $(shell uname -m)
GO_MODULE := github.com/hashicorp/nomad-driver-virt

# CGO is required due to libvirt.
CGO_ENABLED := 1

# Go modules are used to compile the binary.
GO111MODULE := on

# Attempt to use gotestsum for running tests, otherwise fallback to go test.
GO_TEST_CMD := $(if $(shell command -v gotestsum 2>/dev/null),gotestsum --,go test)

GOLANG_VERSION?=$(shell head -n 1 .go-version)

default: check-go-mod lint test build

ifeq (Linux,$(THIS_OS))
ALL_TARGETS = linux_amd64 \
	linux_arm64
endif

# Allow overriding ALL_TARGETS via $TARGETS
ifdef TARGETS
ALL_TARGETS = $(TARGETS)
endif

SUPPORTED_OSES := Linux

.PHONY: clean
clean: ## Remove build artifacts
	@echo "==> Removing build artifact..."
	@rm -rf build/${PLUGIN_BINARY}
	@echo "==> Done"

.PHONY: -docker-prep-linux
-docker-prep-linux:
	docker buildx build \
		--build-arg GO_VERSION=$(GOLANG_VERSION) \
		--build-arg USER_ID=$(shell id -u) \
		--build-arg GROUP_ID=$(shell id -g) \
		-f Dockerfile-build \
		-t nomad-driver-virt-build .

.PHONY: docker-build-linux
docker-build-linux: -docker-prep-linux ## Compile the current driver codebase in a container.
	docker run --rm -it \
		-v "$(shell go env GOMODCACHE):/home/build/go/pkg/mod" \
		-v "$$(pwd):/data" \
		nomad-driver-virt-build bash \
		-c 'cd /data && make build'

.PHONY: docker-test-linux
docker-test-linux: -docker-prep-linux ## Test the current driver codebase in a container.
	docker run --rm -it \
		-v "$(shell go env GOMODCACHE):/home/build/go/pkg/mod" \
		-v "$$(pwd):/data" \
		nomad-driver-virt-build bash \
		-c 'cd /data && go test ./...'

.PHONY: copywrite-headers
copywrite-headers: ## Ensure files have the copywrite header
	@echo "==> Checking copywrite headers..."
	@copywrite headers --plan
	@echo "==> Done"

.PHONY: lint
lint: ## Lint and vet the codebase
	@echo "==> Linting source code..."
	@golangci-lint run --timeout=5m .
	@echo "==> Done"

	@echo "==> Linting hclog statements..."
	@hclogvet .
	@echo "==> Done"

.PHONY: lint-tools
lint-tools: ## Install the tools used to run lint and vet
	@echo "==> Installing lint and vet tools..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.2
	go install github.com/hashicorp/go-hclog/hclogvet@feaf6d2ec20fd895e711195c99e3fde93a68afc5
	@echo "==> Done"

.PHONY: test-tools
test-tools: ## Install the tools used to run tests
	@echo "==> Installing test tools..."
	go install gotest.tools/gotestsum@v1.13.0
	@echo "==> Done"

.PHONY: test
test: ## Test the source code
	@echo "==> Testing source code..."
	@$(GO_TEST_CMD) -v -race -cover ./...
	@echo "==> Done"

.PHONY: check-go-mod
check-go-mod: ## Checks the Go mod is tidy
	@echo "==> Checking Go mod and Go sum..."
	@go mod tidy
	@if (git status --porcelain | grep -Eq "go\.(mod|sum)"); then \
		echo go.mod or go.sum needs updating; \
		git --no-pager diff go.mod; \
		git --no-pager diff go.sum; \
		exit 1; fi
	@echo "==> Done"

HELP_FORMAT="    \033[36m%-25s\033[0m %s\n"
.PHONY: help
help: ## Display this usage information
	@echo "Valid targets:"
	@grep -E '^[^ ]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; \
			{printf $(HELP_FORMAT), $$1, $$2}'
	@echo ""

.PHONY: deps
deps: ## Install build dependencies
	@echo "==> Installing build dependencies ..."
	go install gotest.tools/gotestsum@v1.10.0
	go install github.com/hashicorp/hcl/v2/cmd/hclfmt@d0c4fa8b0bbc2e4eeccd1ed2a32c2089ed8c5cf1

.PHONY: build
build: ## Compile the current driver codebase
	@echo "==> Compiling binary..."
	@go build -race -trimpath -o build/${PLUGIN_BINARY} .
	@echo "==> Done"

.PHONY: dev
dev: clean build ## Build the nomad-driver-virt plugin

# CRT version generation
.PHONY: version
version:
	@$(CURDIR)/version/generate.sh version/version.go version/version.go

# CRT release compilation
dist/%/nomad-driver-virt: GO_OUT ?= $@
dist/%/nomad-driver-virt: CC ?= $(shell go env CC)
dist/%/nomad-driver-virt: ## Build Nomad for GOOS_GOARCH, e.g. pkg/linux_amd64/nomad
ifeq (,$(findstring $(THIS_OS),$(SUPPORTED_OSES)))
	$(warning WARNING: Building Nomad Driver Virt is only supported on $(SUPPORTED_OSES); not $(THIS_OS))
endif
	@echo "==> Building $@ with tags $(GO_TAGS)..."
	@CGO_ENABLED=$(CGO_ENABLED) \
		GOOS=$(firstword $(subst _, ,$*)) \
		GOARCH=$(lastword $(subst _, ,$*)) \
		CC=$(CC) \
		go build -trimpath -ldflags "$(GO_LDFLAGS)" -tags "$(GO_TAGS)" -o $(GO_OUT)

ifneq (aarch64,$(THIS_ARCH))
dist/linux_arm64/nomad-driver-virt: CC = aarch64-linux-gnu-gcc
endif

# CRT release packaging (zip only)
.PRECIOUS: dist/%/nomad-driver-virt
dist/%.zip: dist/%/nomad-driver-virt
	@echo "==> RELEASE PACKAGING of $@ ..."
	zip -j $@ $(dir $<)*

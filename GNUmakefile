PLUGIN_BINARY=nomad-driver-virt

# CGO is required due to libvirt.
CGO_ENABLED = 1

# Go modules are used to compile the binary.
GO111MODULE = on

# Attempt to use gotestsum for running tests, otherwise fallback to go test.
GO_TEST_CMD = $(if $(shell command -v gotestsum 2>/dev/null),gotestsum --,go test)

default: check-go-mod lint test build

.PHONY: clean
clean: ## Remove build artifacts
	@echo "==> Removing build artifact..."
	@rm -rf ${PLUGIN_BINARY}
	@echo "==> Done"

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
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.1
	go install github.com/hashicorp/go-hclog/hclogvet@v0.2.0
	@echo "==> Done"

.PHONY: test-tools
test-tools: ## Install the tools used to run tests
	@echo "==> Installing test tools..."
	go install gotest.tools/gotestsum@v1.12.0
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

.PHONY: clean
clean: ## Cleanup previous build
	@echo "==> Cleanup previous build"
	rm -f ./build/nomad-driver-virt

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
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

.PHONY: build
build: ## Compile the current driver codebase
	@echo "==> Compiling binary..."
	@go build -race -trimpath -o ${PLUGIN_BINARY} .
	@echo "==> Done"

.PHONY: lint
lint: ## Lint and vet the codebase
	@echo "==> Linting source code..."
	@golangci-lint run .
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

PLUGIN_BINARY=nomad-driver-virt

# CGO is required due to libvirt.
CGO_ENABLED = 1

# Go modules are used to compile the binary.
GO111MODULE = on

# Attempt to use gotestsum for running tests, otherwise fallback to go test.
GO_TEST_CMD = $(if $(shell command -v gotestsum 2>/dev/null),gotestsum --,go test)

default: test build

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

HELP_FORMAT="    \033[36m%-25s\033[0m %s\n"
.PHONY: help
help: ## Display this usage information
	@echo "Valid targets:"
	@grep -E '^[^ ]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		sort | \
		awk 'BEGIN {FS = ":.*?## "}; \
			{printf $(HELP_FORMAT), $$1, $$2}'
	@echo ""

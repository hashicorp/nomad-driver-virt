PLUGIN_BINARY=nomad-driver-virt

# CGO is required due to libvirt.
CGO_ENABLED = 1

# Go modules are used to compile the binary.
GO111MODULE = on

default: build

.PHONY: clean
clean: ## Remove build artifacts
	@echo "==> Removing build artifact..."
	@rm -rf ${PLUGIN_BINARY}

build:
	@echo "==> Compiling binary..."
	@go build -race -trimpath -o ${PLUGIN_BINARY} .

.PHONY: build test lint clean install

BINARY_NAME := gosigrefactor
BINARY_PATH := bin/$(BINARY_NAME)
GO_FILES := $(shell find . -name '*.go' -type f)

build: $(BINARY_PATH)

$(BINARY_PATH): $(GO_FILES)
	go build -o $(BINARY_PATH) ./cmd/gosigrefactor

test:
	go test ./... -v

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed, running go vet"; \
		go vet ./...; \
	fi

clean:
	rm -f $(BINARY_PATH)
	rm -f coverage.out coverage.html

install: build
	@echo "Binary built at $(BINARY_PATH)"
	@echo "Add to your Neovim config:"
	@echo ""
	@echo "  require('go-sigrefactor').setup({"
	@echo "    binary = '$(PWD)/$(BINARY_PATH)',"
	@echo "  })"

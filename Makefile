BINARY  := docker-gateway
IMAGE   := docker-gateway
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags="-s -w -X main.version=$(VERSION)"

.PHONY: build test lint clean docker-build help

build: ## Compile the binary
	CGO_ENABLED=0 go build -mod=vendor $(LDFLAGS) -o $(BINARY) .

test: ## Run all tests with race detector
	go test -mod=vendor -race -count=1 -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -1

lint: ## Run golangci-lint
	golangci-lint run --timeout=5m ./...

clean: ## Remove build artifacts
	rm -f $(BINARY) coverage.out

docker-build: ## Build Docker image locally
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  %-15s %s\n", $$1, $$2}'

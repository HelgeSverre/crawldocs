# Binary configuration
BINARY_NAME=crawldocs
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION) -X main.buildDate=$(BUILD_DATE)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Directories
OUTPUT_DIR=bin
COVERAGE_DIR=coverage

# Platforms for cross-compilation
PLATFORMS=darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

.PHONY: all build clean test coverage deps fmt vet install run dev help release cross-compile check

# Default target
all: clean deps fmt test build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(OUTPUT_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME) .
	@echo "Build complete: $(OUTPUT_DIR)/$(BINARY_NAME)"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(OUTPUT_DIR) $(COVERAGE_DIR)
	@rm -f $(BINARY_NAME)
	@echo "Clean complete"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race ./...

# Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	@mkdir -p $(COVERAGE_DIR)
	$(GOTEST) -v -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	@echo "Coverage report: $(COVERAGE_DIR)/coverage.html"

# Manage dependencies
deps:
	@echo "Managing dependencies..."
	$(GOMOD) tidy
	$(GOMOD) download
	$(GOMOD) verify

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

# Install the binary to $GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME)..."
	@cp $(OUTPUT_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "Installed to $(GOPATH)/bin/$(BINARY_NAME)"

# Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(OUTPUT_DIR)/$(BINARY_NAME) $(ARGS)

# Development mode - show help
dev:
	@$(GOCMD) run . -help

# Quick check - format, vet, and test
check: fmt vet test
	@echo "All checks passed!"

# Build for multiple platforms
cross-compile:
	@echo "Cross-compiling for multiple platforms..."
	@mkdir -p $(OUTPUT_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*} GOARCH=$${platform#*/} \
		$(GOBUILD) $(LDFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME)-$${platform%/*}-$${platform#*/} . ; \
		echo "Built: $(OUTPUT_DIR)/$(BINARY_NAME)-$${platform%/*}-$${platform#*/}" ; \
	done

# Create release artifacts
release: clean test cross-compile
	@echo "Creating release artifacts..."
	@mkdir -p $(OUTPUT_DIR)/release
	@for platform in $(PLATFORMS); do \
		if [ "$${platform%/*}" = "windows" ]; then \
			zip -j $(OUTPUT_DIR)/release/$(BINARY_NAME)-$(VERSION)-$${platform%/*}-$${platform#*/}.zip \
				$(OUTPUT_DIR)/$(BINARY_NAME)-$${platform%/*}-$${platform#*/} README.md LICENSE ; \
		else \
			tar -czf $(OUTPUT_DIR)/release/$(BINARY_NAME)-$(VERSION)-$${platform%/*}-$${platform#*/}.tar.gz \
				-C $(OUTPUT_DIR) $(BINARY_NAME)-$${platform%/*}-$${platform#*/} -C .. README.md LICENSE ; \
		fi ; \
		echo "Created: $(OUTPUT_DIR)/release/$(BINARY_NAME)-$(VERSION)-$${platform%/*}-$${platform#*/}.*" ; \
	done

# Benchmark
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Profile CPU
profile-cpu:
	@echo "CPU profiling..."
	$(GOTEST) -cpuprofile=$(COVERAGE_DIR)/cpu.prof -bench=.
	$(GOCMD) tool pprof $(COVERAGE_DIR)/cpu.prof

# Profile Memory
profile-mem:
	@echo "Memory profiling..."
	$(GOTEST) -memprofile=$(COVERAGE_DIR)/mem.prof -bench=.
	$(GOCMD) tool pprof $(COVERAGE_DIR)/mem.prof

# Quick build and test
quick: fmt build test
	@echo "Quick build and test complete!"

# Full CI pipeline
ci: clean deps fmt vet test coverage build
	@echo "CI pipeline complete!"

# Watch for changes and rebuild (requires entr)
watch:
	@which entr > /dev/null || (echo "entr not found. Install it from http://eradman.com/entrproject/" && exit 1)
	@find . -name '*.go' | entr -c make build

# Help
help:
	@echo "CrawlDocs Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make                - Run clean, deps, fmt, test, and build"
	@echo "  make build          - Build the binary"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run tests"
	@echo "  make coverage       - Run tests with coverage report"
	@echo "  make deps           - Download and verify dependencies"
	@echo "  make fmt            - Format code"
	@echo ""
	@echo "  make vet            - Run go vet"
	@echo "  make install        - Install binary to GOPATH/bin"
	@echo "  make run ARGS=...   - Run the application with arguments"
	@echo "  make dev            - Show application help"
	@echo "  make check          - Run format, vet, and test"
	@echo "  make quick          - Quick format, build, and test"
	@echo "  make ci             - Run full CI pipeline"
	@echo "  make cross-compile  - Build for multiple platforms"
	@echo "  make release        - Create release artifacts"
	@echo "  make bench          - Run benchmarks"
	@echo "  make watch          - Watch for changes and rebuild"
	@echo "  make help           - Show this help message"

.DEFAULT_GOAL := all
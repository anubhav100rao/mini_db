# Variables
BINARY_NAME=minidb
GO=go
DATA_DIR=data

# Default target
all: build

# Build the project
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build -o $(BINARY_NAME) ./cmd/minidb

# Run in REPL mode
run: build
	@echo "Starting $(BINARY_NAME) in REPL mode..."
	./$(BINARY_NAME)

# Run in Server mode
server: build
	@echo "Starting $(BINARY_NAME) in Server mode on port 3000..."
	./$(BINARY_NAME) -server -port 3000

# Run all tests
test:
	@echo "Running tests..."
	$(GO) test ./...

# Run tests with verbose output
test-verbose:
	@echo "Running tests (verbose)..."
	$(GO) test -v ./...

# Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

# Clean build artifacts and data
clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME)
	rm -rf $(DATA_DIR)

# Reset data only (keep binary)
reset:
	@echo "Resetting database data..."
	rm -rf $(DATA_DIR)

.PHONY: all build run server test test-verbose fmt clean reset

# HydraStream Development & Build Automation Makefile

.PHONY: dev build test stress-test chaos-test run clean help

# Default target
all: dev

# Run in Live Development Mode (Zero-build HTML/CSS/JS live reload + instant Go execution)
dev:
	@echo "🚀 Starting HydraStream in LIVE DEV MODE..."
	@echo "💡 Web UI changes in web/ will reflect instantly on browser refresh (F5)!"
	go run ./cmd/hydrastream

# Build final production binary
build:
	@echo "📦 Building production binary in bin/hydrastream..."
	mkdir -p bin
	go build -o bin/hydrastream ./cmd/hydrastream

# Run tests
test:
	@echo "🧪 Running unit & concurrency tests..."
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/

# HydraStream Development & Build Automation Makefile

.PHONY: dev dev-all build test cargo-test benchmark mediamtx stream-sample clean help

# Default target
all: dev

# Run HydraStream in Live Development Mode
dev:
	@echo "🚀 Starting HydraStream in LIVE DEV MODE..."
	@echo "💡 Web UI changes in web/ will reflect instantly on browser refresh (F5)!"
	go run ./cmd/hydrastream

# Run MediaMTX RTSP Server binary
mediamtx:
	@echo "📡 Starting MediaMTX RTSP Server (v1.20.1) on ports 8554 (RTSP), 1935 (RTMP), 8888 (HLS), 8889 (WebRTC)..."
	./bin/mediamtx

# Publish a sample RTSP test feed to MediaMTX
stream-sample:
	@echo "🎥 Publishing test camera video pattern to rtsp://localhost:8554/tenant_company_alpha/cam_entrance_01..."
	ffmpeg -re -f lavfi -i testsrc=size=1920x1080:rate=30 -c:v libx264 -preset ultrafast -tune zerolatency -pix_fmt yuv420p -f rtsp rtsp://localhost:8554/tenant_company_alpha/cam_entrance_01

# Build final production binary and Rust data plane
build:
	@echo "📦 Building Rust Data Plane Engine..."
	cargo build --release --manifest-path crates/hydra-engine/Cargo.toml
	@echo "📦 Building Go Control Plane binary in bin/hydrastream..."
	mkdir -p bin
	go build -o bin/hydrastream ./cmd/hydrastream

# Run all Go and Rust tests
test:
	@echo "🧪 Running Go unit & concurrency tests..."
	go test -v ./...
	@echo "🦀 Running Rust unit tests..."
	cargo test --manifest-path crates/hydra-engine/Cargo.toml

# Run Rust unit tests specifically
cargo-test:
	@echo "🦀 Running Rust unit tests..."
	cargo test --manifest-path crates/hydra-engine/Cargo.toml

# Run Rust high-throughput SHM benchmark
benchmark:
	@echo "⚡ Running HydraStream Rust Zero-Copy SHM Benchmark..."
	cargo run --release --manifest-path crates/hydra-engine/Cargo.toml

# Clean build artifacts
clean:
	rm -rf bin/hydrastream
	cargo clean --manifest-path crates/hydra-engine/Cargo.toml

# HydraStream Development & Build Automation Makefile

.PHONY: dev dev-all build build-engine test cargo-test benchmark wsp-server stream-sample clean help

# Default target
all: dev

# Build native Rust Data Plane Engine
build-engine:
	@echo "📦 Building Rust Data Plane Engine (hydra-engine)..."
	@cargo build --release --manifest-path crates/hydra-engine/Cargo.toml
	@mkdir -p bin
	@cp target/release/hydra-engine bin/ 2>/dev/null || cp target/release/hydra-engine.exe bin/ 2>/dev/null || true
	@echo "✅ Native Rust Engine built in bin/hydra-engine"

# Run HydraStream in Live Development Mode
dev: build-engine
	@echo "🚀 Starting HydraStream Data Plane Engine..."
	@echo "📡 REST API active on port 8080 | Rust WSP Server on port 8085"
	go run ./cmd/hydrastream

# Run Dedicated Rust WSP Server standalone
wsp-server: build-engine
	@echo "📡 Starting Dedicated Rust WSP Server on port 8085..."
	./bin/hydra-engine server --port 8085

# Publish a sample RTSP test feed
stream-sample:
	@echo "🎥 Publishing test camera video pattern to rtsp://localhost:8554/tenant_company_alpha/cam_entrance_01..."
	ffmpeg -re -f lavfi -i testsrc=size=1920x1080:rate=30 -c:v libx264 -preset ultrafast -tune zerolatency -pix_fmt yuv420p -f rtsp rtsp://localhost:8554/tenant_company_alpha/cam_entrance_01

# Build final production binary and Rust data plane
build: build-engine
	@echo "📦 Building Go Control Plane binary in bin/hydrastream..."
	@mkdir -p bin
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

# Run Side-by-Side Comparison: Traditional vs HydraStream CPU vs HydraStream GPU
benchmark-compare:
	@echo "⚡ Running Real-Hardware Benchmark: Traditional vs HydraStream CPU vs HydraStream GPU..."
	python3 benchmarks/run_comparison_benchmark.py

# Run Direct MediaMTX vs HydraStream Native Architecture Benchmark
benchmark-mtx:
	@echo "⚡ Running MediaMTX vs HydraStream Native Zero-Copy Benchmark..."
	python3 benchmarks/mediamtx_vs_hydrastream_benchmark.py

# Build Debian / Ubuntu .deb package
package-deb:
	@echo "📦 Building HydraStream .deb Debian/Ubuntu package..."
	bash scripts/build_deb.sh

# Build Windows .exe & .zip distribution
package-windows:
	@echo "📦 Building HydraStream Windows .exe & .zip package..."
	bash scripts/build_windows.sh

# Build all distribution packages (Linux .deb + Windows .zip)
package-all: package-deb package-windows
	@echo "🎉 All distribution packages generated in dist/"

# Clean build artifacts
clean:
	rm -rf bin/hydrastream bin/hydrastream.exe dist
	cargo clean --manifest-path crates/hydra-engine/Cargo.toml



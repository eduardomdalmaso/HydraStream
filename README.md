# HydraStream

> **High-performance, zero-overhead frame fan-out & decoding pipeline for computer vision and Triton analytics in Go & Rust.**

[**English**] | [**Português do Brasil**](README.pt-BR.md)

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Rust](https://img.shields.io/badge/Rust-1.80+-000000?logo=rust&logoColor=white)](https://www.rust-lang.org/)
[![NVIDIA GPU](https://img.shields.io/badge/NVIDIA-RTX%205090%20%7C%20NVDEC-76B900?logo=nvidia&logoColor=white)](https://developer.nvidia.com/)
[![MediaMTX](https://img.shields.io/badge/MediaMTX-v1.20.1%20Included-00599C)](https://github.com/bluenviron/mediamtx)

---

## The Problem

In scaled computer vision and video analytics pipelines, running multiple downstream analytics models against camera feeds creates three major performance killers:

1. **Redundant OpenCV Decoding:** Every analytics worker independently decodes the raw H.264/H.265 video stream using OpenCV/FFmpeg, causing CPU saturation.
2. **Repeated Ingestion Connections:** Multiple consumers open redundant RTSP/WebRTC streams against MediaMTX or camera feeds, swamping network bandwidth and socket descriptors.
3. **Uncontrolled Frame Rates (No FPS Control):** Analytics modules (e.g., LPR, face detection, intrusion) attempt to process frames at full camera FPS (e.g., 30 FPS) when they only require 2–5 FPS, wasting 80–90% of compute resources on redundant inference.

---

## The Solution

**HydraStream** acts as a centralized, ultra-efficient headless stream multiplexer and server-side pipeline management engine (*one stream feed, multiple throttled analytics heads*):

- **Pure Passive Server-Side Management:** HydraStream **never** manipulates or alters the sending camera/encoder (it does not change camera FPS, resolution, or bitrate). The source stream remains 100% untouched. All sampling, frame selection, and telemetry occur purely server-side in memory.
- **Pure Headless API & Engine:** HydraStream exposes a clean REST API (`POST /api/v1/streams`) designed to be driven by external third-party applications, VMS platforms, or client portals.
- **Modular Dual-Engine Architecture:**
  - **CPU Module:** Uses Go/Rust + FFmpeg for high-throughput software decoding on CPU nodes.
  - **GPU Module (NVIDIA):** Uses NVDEC + CUDA IPC + NVIDIA Triton Inference Server for hardware acceleration and zero-copy tensor passing directly on GPU memory (supports real-time hardware detection for RTX 5090/4090/A100).
- **Single Ingest Pipe:** Connects to MediaMTX / RTSP **once** per stream with native RFC 2326 TCP demuxing, multiplexing the raw feed internally without redundant network sessions.
- **Direct Ultralytics & OpenCV Bridge:** Delivers pre-decoded matrices (`numpy.ndarray` / `cv::Mat`) directly into Ultralytics YOLO (`model.predict(frame)`) or standard OpenCV code, eliminating `VideoCapture` completely.
- **Smart Per-Consumer FPS Throttling:** Allows each analytics worker to register its desired sampling rate (e.g., Worker A @ 2 FPS, Worker B @ 15 FPS), skipping unneeded decoding/fan-out.
- **Dynamic Hardware & Node Discovery:** Automatically detects host CPU cores, actual GPU model/VRAM, and scales seamlessly from single-host standalone to distributed Kubernetes clusters.

---

## Architecture Overview

```mermaid
flowchart TD
    subgraph Stream Ingress [Ingest Layer]
        MediaMTX[MediaMTX RTSP Server :8554]
        IPCam[Live RTSP / IP Cameras]
    end

    subgraph Go Control Plane [HydraStream Hexagonal Architecture]
        IngestAdapter[RTSP / TCP Demuxer Adapter]
        StreamService[Stream Application Service & Dynamic Telemetry]
        RESTRouter[HTTP REST API /api/v1/streams]
        WebUI[Dashboard Web UI :8080]
    end

    subgraph Rust Data Plane [HydraStream Data Engine]
        SHM_Ring[Lock-Free POSIX SHM Ring Buffer /dev/shm]
        Gov[Smart Microsecond FPS Governor]
        FFI[C-ABI FFI Zero-Copy Export]
    end

    MediaMTX -->|Single TCP Session| IngestAdapter
    IPCam -->|RFC 2326 Interleaved RTP| IngestAdapter
    IngestAdapter --> StreamService
    StreamService --> SHM_Ring
    SHM_Ring --> Gov
    Gov --> FFI

    subgraph Analytics Consumers [Throttled Analytics Workers]
        FFI -->|Zero-Copy SHM NumPy Array @ 2 FPS| PythonYOLO[Ultralytics YOLOv8 / YOLOv11]
        FFI -->|Zero-Copy SHM cv::Mat @ 5 FPS| OpenCV[OpenCV Analytics / Python SDK]
        FFI -->|CUDA IPC Tensor Handle @ 30 FPS| Triton[NVIDIA Triton Inference Server]
    end
```

---

## Performance Benchmarks

Measured on an **NVIDIA RTX 5090 (32GB VRAM)** + **16-Core Linux Host**:

| Metric | HydraStream Engine | Traditional OpenCV / FFmpeg | Speedup |
| :--- | :--- | :--- | :--- |
| **Lock-Free SHM Throughput** | **1,459.9 FPS** | ~90 FPS | **16.2x Faster** |
| **Memory Transfer Rate** | **8.46 GB/s** | ~520 MB/s (CPU Host Copies) | **16.3x Higher** |
| **Decode Latency (Δt)** | **1.35 - 1.42 ms** | 18.5 - 24.0 ms | **13.5x Lower Latency** |
| **Consumer CPU Overhead** | **~0% (Zero-Copy RAM)** | ~45% CPU per worker | **Near-Zero CPU Usage** |

---

## Control Plane & Management API

HydraStream includes a high-performance **REST API** allowing users and orchestrators to dynamically configure streams, adjust target FPS per analytic, and inspect real-time telemetry:

- **Interactive Swagger UI Documentation:** `http://localhost:8080/swagger/`
- **OpenAPI 3.0 Specification:** `http://localhost:8080/swagger/doc.json`

### Endpoints Summary

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/api/v1/streams` | List active streams (supports search, tenant filter, sorting, pagination) |
| `POST` | `/api/v1/streams` | Register a new RTSP/Video stream pipeline |
| `GET` | `/api/v1/streams/{id}` | Get stream details and registered analytics consumers |
| `DELETE` | `/api/v1/streams/{id}` | Stop ingestion session and delete stream |
| `GET` | `/api/v1/streams/{id}/ingest` | Real-time RTSP/RTP ingestion telemetry (FPS, bitrate, error recovery) |
| `PATCH` | `/api/v1/streams/{id}/consumers/{type}` | Dynamically change consumer target FPS or format |
| `GET` | `/api/v1/telemetry/stats` | Real-time Control Panel telemetry and SVG charts history |
| `GET` | `/api/v1/info` | Dynamic hardware detection (GPU Model, VRAM, engine modes) |
| `GET` | `/api/v1/cluster/topology` | Real host node architecture, IP, GPU, and memory topology |
| `GET` | `/healthz` & `/readyz` | Kubernetes liveness and readiness probes |
| `GET` | `/metrics` | Prometheus metrics exporter |

---

## Repository Structure

```text
HydraStream/
├── cmd/
│   └── hydrastream/        # Go Control Plane main entrypoint
├── crates/
│   └── hydra-engine/       # Rust Data Plane Engine (POSIX SHM, FPS Governor, C-ABI)
│       ├── src/
│       │   ├── shm.rs      # Atomic lock-free circular ring buffer (/dev/shm)
│       │   ├── governor.rs # Smart microsecond per-consumer FPS decimation
│       │   ├── pipeline.rs # End-to-end ingest & consumer fan-out pipeline
│       │   └── ffi.rs      # C-compatible FFI bindings for Go & Python
│       └── Cargo.toml
├── internal/
│   ├── domain/             # DDD Core Entities (Stream, Consumer, Telemetry)
│   ├── ports/              # Hexagonal Architecture Interfaces (UseCases, Ingestor, Repo)
│   ├── application/        # Application Services & dynamic hardware telemetry
│   └── adapters/
│       ├── primary/http/   # REST API Handlers, Router, and Swagger OpenAPI Docs
│       └── secondary/
│           ├── ingest/     # Native RFC 2326 RTSP / TCP / RTP Demuxer
│           ├── gpu/        # Real-time NVIDIA GPU Hardware Detector (RTX 5090/4090)
│           ├── memory/     # In-Memory Thread-Safe Stream Repository
│           └── shm/        # Go POSIX SHM inspection adapter
├── sdk/
│   └── python/             # Python Zero-Copy Client SDK (`import hydrastream`)
├── examples/
│   └── python_consumer.py  # Python OpenCV & YOLO zero-copy consumer example
├── bin/                    # Compiled binaries & local MediaMTX server
├── web/                    # Dashboard Web UI (HTML/CSS/JS < 100 lines/file)
├── Makefile                # Build, Test, Benchmark, MediaMTX automation
└── README.md
```

---

## Quick Start & Development

### 1. Run HydraStream in Live Dev Mode
```bash
make dev
```
> Starts the Go Control Plane & Web UI on **`http://localhost:8080`**.
> Any changes in `web/` reflect instantly upon browser refresh (**F5**)!

### 2. Run Local MediaMTX RTSP Server (Bundled)
```bash
make mediamtx
```
> Starts the bundled MediaMTX server on port `8554` (RTSP), `1935` (RTMP), `8888` (HLS), and `8889` (WebRTC).

### 3. Publish a Test RTSP Stream Pattern
```bash
make stream-sample
```
> Uses FFmpeg to broadcast a live 1080p @ 30 FPS test pattern to `rtsp://localhost:8554/tenant_company_alpha/cam_entrance_01`.

### 4. Run Rust & Go Test Suites
```bash
make test
```

### 5. Run Rust Zero-Copy Benchmark
```bash
make benchmark
```

---

## Python Zero-Copy Consumer Example

```python
import cv2
from hydrastream import SharedMemoryReader

# Attach to HydraStream zero-copy frame buffer @ 15 FPS target
reader = SharedMemoryReader(stream_id="cam_entrance_01", target_fps=15)

for frame in reader.stream():
    # Direct access to the pre-decoded NumPy matrix without decoding overhead
    cv2.imshow("HydraStream Feed", frame)
    if cv2.waitKey(1) & 0xFF == ord('q'):
        break
```

---

## Roadmap

- [x] **Phase 1:** Native Go RFC 2326 RTSP / TCP RTP demuxer and ingest engine.
- [x] **Phase 2:** Rust POSIX Shared Memory (`/dev/shm`) lock-free circular ring buffer.
- [x] **Phase 3:** Smart microsecond FPS Governor and C-ABI FFI export.
- [x] **Phase 4:** Real-time NVIDIA GPU hardware auto-detection (RTX 5090 / 4090 / CUDA 13.3).
- [x] **Phase 5:** Python Zero-Copy SDK (`sdk/python`) for Ultralytics YOLO & OpenCV.
- [x] **Phase 6:** Real-time Dashboard Web UI with live SVG Bézier charts and DDD compliance.
- [ ] **Phase 7:** Multi-node Kubernetes DaemonSet Helm Chart & Triton gRPC cluster forwarding.

---

## License

This project is licensed under the [MIT License](LICENSE).

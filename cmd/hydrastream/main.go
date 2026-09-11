package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	httpAdapter "hydrastream/internal/adapters/primary/http"
	"hydrastream/internal/adapters/secondary/gpu"
	"hydrastream/internal/adapters/secondary/ingest"
	"hydrastream/internal/adapters/secondary/logger"
	"hydrastream/internal/adapters/secondary/memory"
	natsAdapter "hydrastream/internal/adapters/secondary/nats"
	"hydrastream/internal/adapters/secondary/onvif"
	"hydrastream/internal/application"
)

func isPortInUse(port string) bool {
	ln, err := net.Listen("tcp", port)
	if err != nil {
		return true
	}
	_ = ln.Close()
	return false
}

func startEmbeddedMediaMTX() *exec.Cmd {
	binPath := "./bin/mediamtx"
	if _, err := os.Stat(binPath); err != nil {
		log.Printf("⚠️ [HydraStream] MediaMTX binary not found at %s. Skipping embedded start.\n", binPath)
		return nil
	}

	if isPortInUse(":8554") {
		log.Printf("ℹ️ [HydraStream] MediaMTX port :8554 already active. Attaching to existing RTSP instance.\n")
		return nil
	}

	cmd := exec.Command(binPath, "mediamtx.yml")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Printf("❌ [HydraStream] Failed to start embedded MediaMTX: %v\n", err)
		return nil
	}

	log.Printf("📡 [HydraStream] Embedded MediaMTX RTSP Server started (PID %d) on ports 8554 (RTSP), 8889 (WebRTC)\n", cmd.Process.Pid)
	return cmd
}

func main() {
	// Initialize in-memory ring-buffer logger and pipe stdout
	ringLogger := logger.NewRingLogger(1000)
	log.SetOutput(io.MultiWriter(os.Stdout, ringLogger))

	log.Println("[HydraStream] Initializing Data Plane Engine (Hexagonal Architecture + DDD)...")

	// Detect underlying GPU Hardware
	hw := gpu.DetectHardware()

	// 1. Driven Adapters (Secondary - Storage, RTSP Ingestor, ONVIF Scanner & Ring Logger)
	streamRepo := memory.NewStreamRepository()
	rtspIngestor := ingest.NewRTSPIngestor()
	onvifAdapter := onvif.NewONVIFAdapter()

	// 2. Application Layer (Service / Use Case)
	streamService := application.NewStreamService(streamRepo, rtspIngestor, onvifAdapter, ringLogger)

	// 3. Driving Adapter (Primary - HTTP REST API & Telemetry)
	apiHandler := httpAdapter.NewHandler(streamService)

	// 4. Create ServeMux and register all routes
	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux)

	// 5. Start Embedded MediaMTX RTSP server if not already running
	mtxCmd := startEmbeddedMediaMTX()

	// 6. Connect to NATS Event Mesh if available
	natsPub, err := natsAdapter.NewStreamNATSPublisher("nats://localhost:4222")
	if err != nil {
		log.Printf("⚠️ [HydraStream] NATS Event Mesh not reachable: %v (continuing standalone mode)\n", err)
	} else {
		log.Println("✅ [HydraStream] NATS Event Mesh connected and active!")
		rtspIngestor.SetPublisher(natsPub)
	}

	port := ":8080"
	server := &http.Server{
		Addr:         port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	go func() {
		log.Printf("[HydraStream] Data Plane API & Telemetry listening on http://localhost%s\n", port)
		log.Printf("[HydraStream] Status: ONLINE | Active Hardware: %s [%s]\n", hw.Model, hw.EngineName)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[HydraStream] Server failed: %v", err)
		}
	}()

	// Graceful Shutdown & MediaMTX Process Termination
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 [HydraStream] Shutting down Data Plane gracefully...")
	if natsPub != nil {
		natsPub.Close()
	}
	if mtxCmd != nil && mtxCmd.Process != nil {
		log.Println("🛑 [HydraStream] Terminating embedded MediaMTX server...")
		_ = mtxCmd.Process.Signal(syscall.SIGINT)
		time.Sleep(500 * time.Millisecond)
		_ = mtxCmd.Process.Kill()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
	log.Println("✅ [HydraStream] Stopped.")
}

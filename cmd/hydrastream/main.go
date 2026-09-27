package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func autoProvisionMediaMTX() string {
	targetDir := "./bin"
	_ = os.MkdirAll(targetDir, 0755)

	targetBin := filepath.Join(targetDir, "mediamtx")
	if runtime.GOOS == "windows" {
		targetBin += ".exe"
	}

	if fileExists(targetBin) {
		return targetBin
	}

	log.Printf("📥 [HydraStream] MediaMTX not found locally. Auto-downloading v1.21.1 for %s/%s...\n", runtime.GOOS, runtime.GOARCH)

	goos := runtime.GOOS
	goarch := runtime.GOARCH
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}

	downloadURL := fmt.Sprintf("https://github.com/bluenviron/mediamtx/releases/download/v1.21.1/mediamtx_v1.21.1_%s_%s.%s", goos, goarch, ext)

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		log.Printf("⚠️ [HydraStream] Could not auto-download MediaMTX: %v (status: %d)\n", err, status)
		return ""
	}
	defer resp.Body.Close()

	if ext == "tar.gz" {
		gzr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return ""
		}
		defer gzr.Close()
		tr := tar.NewReader(gzr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			if hdr.Name == "mediamtx" || hdr.Name == "mediamtx.exe" {
				outFile, err := os.OpenFile(targetBin, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0755)
				if err == nil {
					_, _ = io.Copy(outFile, tr)
					_ = outFile.Close()
				}
			} else if hdr.Name == "mediamtx.yml" && !fileExists("mediamtx.yml") {
				ymlFile, err := os.OpenFile("mediamtx.yml", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
				if err == nil {
					_, _ = io.Copy(ymlFile, tr)
					_ = ymlFile.Close()
				}
			}
		}
	}

	if fileExists(targetBin) {
		log.Printf("✅ [HydraStream] MediaMTX v1.21.1 auto-provisioned successfully in %s\n", targetBin)
		return targetBin
	}
	return ""
}

func startEmbeddedMediaMTX() *exec.Cmd {
	candidates := []string{
		"./bin/mediamtx.exe",
		"./bin/mediamtx",
		"./mediamtx.exe",
		"mediamtx.exe",
		"mediamtx",
	}
	binPath := ""
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			binPath = p
			break
		}
		if _, err := os.Stat(c); err == nil {
			binPath = c
			break
		}
	}
	if binPath == "" {
		binPath = autoProvisionMediaMTX()
	}
	if binPath == "" {
		log.Printf("⚠️ [HydraStream] MediaMTX binary not found and auto-download failed. Skipping embedded start.\n")
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

	// 6. Connect to NATS Event Mesh if configured or available
	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}
	natsPub, err := natsAdapter.NewStreamNATSPublisher(natsURL)
	if err != nil {
		log.Printf("⚠️ [HydraStream] NATS Event Mesh (%s) not reachable: %v (continuing standalone mode)\n", natsURL, err)
	} else {
		log.Printf("✅ [HydraStream] NATS Event Mesh connected to %s!\n", natsURL)
		rtspIngestor.SetPublisher(natsPub)
		streamService.SetRecordingPublisher(natsPub)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = ":8080"
	} else if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}
	server := &http.Server{
		Addr:         port,
		Handler:      httpAdapter.WithCORS(mux),
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

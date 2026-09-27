package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
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

func main() {
	ringLogger := logger.NewRingLogger(1000)
	log.SetOutput(io.MultiWriter(os.Stdout, ringLogger))

	log.Println("[HydraStream] Initializing Data Plane Engine (Hexagonal Architecture + DDD)...")

	hw := gpu.DetectHardware()

	streamRepo := memory.NewStreamRepository()
	rtspIngestor := ingest.NewRTSPIngestor()
	onvifAdapter := onvif.NewONVIFAdapter()

	streamService := application.NewStreamService(streamRepo, rtspIngestor, onvifAdapter, ringLogger)

	apiHandler := httpAdapter.NewHandler(streamService)

	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux)

	mtxCmd := startEmbeddedMediaMTX()

	go func() {
		time.Sleep(1 * time.Second)
		syncCamerasFromDB(streamService)
	}()

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

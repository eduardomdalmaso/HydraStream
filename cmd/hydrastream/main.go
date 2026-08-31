package main

import (
	"log"
	"net/http"

	httpAdapter "hydrastream/internal/adapters/primary/http"
	"hydrastream/internal/adapters/secondary/gpu"
	"hydrastream/internal/adapters/secondary/ingest"
	"hydrastream/internal/adapters/secondary/memory"
	"hydrastream/internal/application"
)

func main() {
	log.Println("[HydraStream] Initializing Control Plane Engine (Hexagonal Architecture + DDD)...")

	// Detect underlying GPU Hardware
	hw := gpu.DetectHardware()

	// 1. Driven Adapters (Secondary - Storage & RTSP Ingestor)
	streamRepo := memory.NewStreamRepository()
	rtspIngestor := ingest.NewRTSPIngestor()

	// 2. Application Layer (Service / Use Case)
	streamService := application.NewStreamService(streamRepo, rtspIngestor)

	// 3. Driving Adapter (Primary - HTTP REST API)
	apiHandler := httpAdapter.NewHandler(streamService)

	// 4. Create ServeMux and register routes
	mux := http.NewServeMux()
	apiHandler.RegisterRoutes(mux)

	// Serve Cyberpunk 2077 HUD Web UI files
	fileServer := http.FileServer(http.Dir("web"))
	mux.Handle("/", fileServer)

	port := ":8080"
	log.Printf("[HydraStream] Control Plane & Web UI listening on http://localhost%s\n", port)
	log.Printf("[HydraStream] Status: ONLINE | Active Hardware: %s [%s]\n", hw.Model, hw.EngineName)

	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("[HydraStream] Server failed: %v", err)
	}
}

package main

import (
	"log"
	"net/http"

	httpAdapter "hydrastream/internal/adapters/primary/http"
	"hydrastream/internal/adapters/secondary/memory"
	"hydrastream/internal/application"
)

func main() {
	log.Println("[HydraStream] Initializing Control Plane Engine (Hexagonal Architecture + DDD)...")

	// 1. Driven Adapter (Secondary - Storage)
	streamRepo := memory.NewStreamRepository()

	// 2. Application Layer (Service / Use Case)
	streamService := application.NewStreamService(streamRepo)

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
	log.Printf("[HydraStream] Status: ONLINE | Active GPU: NVIDIA RTX 4090 (NVDEC Enabled)\n")

	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("[HydraStream] Server failed: %v", err)
	}
}

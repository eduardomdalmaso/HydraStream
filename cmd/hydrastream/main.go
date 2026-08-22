package main

import (
	"log"
	"net/http"

	"hydrastream/internal/api"
	"hydrastream/internal/store"
)

func main() {
	log.Println("[HydraStream] Initializing Control Plane Engine...")

	// Initialize thread-safe in-memory stream store
	streamStore := store.NewStreamStore()

	// Initialize REST API handlers
	apiHandler := api.NewHandler(streamStore)

	// Create ServeMux
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

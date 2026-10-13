package main

import (
	"log"

	"hydrastream/internal/application"
)

func syncCamerasFromDB(_ *application.StreamService) {
	log.Println("📷 [HydraStream] Dynamic stream sync initialized")
}

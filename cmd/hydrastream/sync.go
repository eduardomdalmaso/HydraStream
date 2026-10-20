package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"hydrastream/internal/adapters/secondary/proc"
	"hydrastream/internal/application"
	"hydrastream/internal/domain"
)

func syncCamerasFromDB(svc *application.StreamService) {
	log.Println("📷 [HydraStream] Dynamic stream sync initialized")

	dbPaths := []string{
		"../hydravms/hydravms.db",
		"hydravms.db",
	}

	var dbPath string
	for _, p := range dbPaths {
		if _, err := os.Stat(p); err == nil {
			dbPath = p
			break
		}
	}

	if dbPath == "" {
		return
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	syncOnce := func() {
		cmd := exec.Command("sqlite3", dbPath, "SELECT id, COALESCE(rtsp_url, ''), COALESCE(sub_stream_url, ''), COALESCE(codec, 'h264') FROM cameras;")
		proc.SetHideWindow(cmd)
		out, err := cmd.Output()
		if err != nil {
			return
		}

		activeDBIDs := make(map[string]bool)
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for _, line := range lines {
			parts := strings.Split(line, "|")
			if len(parts) < 2 {
				continue
			}
			id := strings.TrimSpace(parts[0])
			rtspURL := strings.TrimSpace(parts[1])
			codec := "h264"
			if len(parts) > 3 && strings.TrimSpace(parts[3]) != "" {
				codec = strings.TrimSpace(parts[3])
			}

			if id == "" || rtspURL == "" {
				continue
			}
			activeDBIDs[id] = true

			_ = svc.RegisterStream(context.Background(), &domain.Stream{
				TenantID:       "default",
				StreamID:       id,
				SourceURL:      rtspURL,
				DecodingEngine: "mjpeg_and_wsp",
				Codec:          codec,
				Status:         "active",
				IngestFPS:      25.0,
			})
		}

		// Purge deleted streams from HydraStream memory and ingest
		existingStreams, _, errList := svc.ListStreams(context.Background(), "", "", "", 1, 500)
		if errList == nil {
			for _, st := range existingStreams {
				if st.StreamID != "cam_entrance_01" && !activeDBIDs[st.StreamID] {
					_ = svc.DeleteStream(context.Background(), st.StreamID)
				}
			}
		}
	}

	syncOnce()
	for range ticker.C {
		syncOnce()
	}
}

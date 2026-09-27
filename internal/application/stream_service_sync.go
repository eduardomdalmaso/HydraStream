package application

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

func syncMediaMTXPath(streamID, sourceURL, codec string) {
	if streamID == "" || sourceURL == "" {
		return
	}
	client := &http.Client{Timeout: 2 * time.Second}
	isH265 := strings.Contains(strings.ToUpper(codec), "265") || strings.Contains(strings.ToUpper(codec), "HEVC")

	subURL := sourceURL
	if strings.Contains(sourceURL, "/stream1") {
		subURL = strings.Replace(sourceURL, "/stream1", "/stream2", 1)
	} else if strings.Contains(sourceURL, "/live/ch0") {
		subURL = strings.Replace(sourceURL, "/live/ch0", "/live/ch1", 1)
	} else if strings.Contains(sourceURL, "/Streaming/Channels/101") {
		subURL = strings.Replace(sourceURL, "/Streaming/Channels/101", "/Streaming/Channels/102", 1)
	}

	for _, name := range []string{streamID, streamID + "_sub"} {
		curURL := sourceURL
		if strings.HasSuffix(name, "_sub") {
			curURL = subURL
		}
		var payload map[string]interface{}
		if isH265 {
			ffmpegBin := "ffmpeg"
			if runtime.GOOS == "windows" {
				ffmpegBin = "./bin/silent_ffmpeg.exe"
			}
			payload = map[string]interface{}{
				"runOnDemand":        fmt.Sprintf("%s -rtsp_transport tcp -i \"%s\" -c:v libx264 -preset ultrafast -tune zerolatency -b:v 1500k -f rtsp rtsp://127.0.0.1:8554/%s", ffmpegBin, curURL, name),
				"runOnDemandRestart": true,
			}
		} else {
			payload = map[string]interface{}{
				"source":         curURL,
				"sourceOnDemand": true,
				"rtspTransport":  "tcp",
			}
		}
		body, _ := json.Marshal(payload)

		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:9997/v3/config/paths/add/%s", name), bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			resp, errDo := client.Do(req)
			if errDo == nil && resp != nil {
				if resp.StatusCode == http.StatusBadRequest {
					_ = resp.Body.Close()
					reqReplace, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:9997/v3/config/paths/replace/%s", name), bytes.NewReader(body))
					if reqReplace != nil {
						reqReplace.Header.Set("Content-Type", "application/json")
						respReplace, errReplace := client.Do(reqReplace)
						if errReplace == nil && respReplace != nil {
							_ = respReplace.Body.Close()
						}
					}
				} else {
					_ = resp.Body.Close()
				}
			}
		}
	}
}

func deleteMediaMTXPath(streamID string) {
	if streamID == "" {
		return
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, name := range []string{streamID, streamID + "_sub"} {
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("http://127.0.0.1:9997/v3/config/paths/delete/%s", name), nil)
		if req != nil {
			resp, err := client.Do(req)
			if err == nil && resp != nil {
				_ = resp.Body.Close()
			}
		}
	}
}

package api

import (
	"net/http"
)

// OpenAPI3Spec contains the complete OpenAPI 3.0 specification for HydraStream REST API.
const OpenAPI3Spec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "HydraStream Control Plane API",
    "description": "High-performance, zero-overhead frame fan-out & decoding pipeline API for computer vision and Triton analytics.",
    "version": "1.0.0"
  },
  "paths": {
    "/api/v1/streams": {
      "get": {
        "summary": "List all registered camera streams",
        "responses": {
          "200": { "description": "List of active streams" }
        }
      },
      "post": {
        "summary": "Register a new RTSP camera stream and configure analytics pipeline",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "example": {
                "tenant_id": "tenant_company_alpha",
                "stream_id": "cam_entrance_01",
                "source_url": "rtsp://mediamtx:8554/tenant_company_alpha/cam_entrance_01",
                "decoding_engine": "nvidia_nvdec",
                "consumers": [
                  {
                    "analytic_type": "lpr_ocr",
                    "target_fps": 2.0,
                    "output_format": "shm_numpy"
                  }
                ]
              }
            }
          }
        },
        "responses": {
          "201": { "description": "Stream successfully registered" }
        }
      }
    },
    "/api/v1/streams/{stream_id}": {
      "get": {
        "summary": "Get stream details by Stream ID",
        "parameters": [
          { "name": "stream_id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": {
          "200": { "description": "Stream details" }
        }
      },
      "delete": {
        "summary": "Delete a registered stream",
        "parameters": [
          { "name": "stream_id", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": {
          "204": { "description": "Stream deleted" }
        }
      }
    },
    "/api/v1/streams/{stream_id}/consumers/{analytic_type}": {
      "patch": {
        "summary": "Dynamically update target FPS or output format for an analytic consumer",
        "parameters": [
          { "name": "stream_id", "in": "path", "required": true, "schema": { "type": "string" } },
          { "name": "analytic_type", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "responses": {
          "200": { "description": "Consumer target FPS updated" }
        }
      }
    },
    "/api/v1/streams/{stream_id}/snapshot.jpg": {
      "get": {
        "summary": "Get single JPEG frame snapshot on demand from SHM ring buffer",
        "responses": {
          "200": { "description": "JPEG image binary" }
        }
      }
    },
    "/api/v1/streams/{stream_id}/mjpeg": {
      "get": {
        "summary": "Stream continuous MJPEG live preview",
        "responses": {
          "200": { "description": "Multipart MJPEG video stream" }
        }
      }
    },
    "/api/v1/cluster/topology": {
      "get": {
        "summary": "Get cluster hardware and node topology mapping",
        "responses": {
          "200": { "description": "Cluster topology JSON" }
        }
      }
    },
    "/api/v1/info": {
      "get": {
        "summary": "Get system and GPU engine status",
        "responses": {
          "200": { "description": "System status JSON" }
        }
      }
    }
  }
}`

// ServeSwaggerUI returns a lightweight standalone Swagger UI HTML page.
func ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>HydraStream API // Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    body { margin: 0; padding: 0; background-color: #0b0c10; }
    .swagger-ui .topbar { display: none; }
    .swagger-ui { filter: invert(0.88) hue-rotate(180deg); }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function() {
      SwaggerUIBundle({
        url: "/swagger/doc.json",
        dom_id: '#swagger-ui'
      });
    };
  </script>
</body>
</html>`
	w.Write([]byte(html))
}

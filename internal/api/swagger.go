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

// ServeSwaggerUI returns a lightweight standalone Swagger UI HTML page with Cyberpunk 2077 HUD styling.
func ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>HydraStream API // Cyberpunk 2077 Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Barlow:wght@600;700&family=Orbitron:wght@800&family=Share+Tech+Mono&display=swap">
  <style>
    body {
      margin: 0;
      padding: 0;
      background-color: #07080c !important;
      font-family: 'Barlow', sans-serif !important;
      color: #f0f2f5 !important;
      background-image: 
        linear-gradient(rgba(0, 240, 255, 0.04) 1px, transparent 1px),
        linear-gradient(90deg, rgba(0, 240, 255, 0.04) 1px, transparent 1px) !important;
      background-size: 30px 30px !important;
    }
    .swagger-ui .topbar { display: none !important; }
    .swagger-ui {
      max-width: 1200px;
      margin: 0 auto;
      padding: 2rem;
    }
    .swagger-ui .info { margin: 2rem 0; border-bottom: 2px solid #fcee0a; padding-bottom: 1rem; }
    .swagger-ui .info .title {
      font-family: 'Orbitron', sans-serif !important;
      color: #fcee0a !important;
      text-transform: uppercase;
      font-size: 2.2rem !important;
      letter-spacing: 0.08em;
    }
    .swagger-ui .info p, .swagger-ui .info li { color: #8b94a7 !important; font-size: 1rem; }
    .swagger-ui .opblock-tag {
      font-family: 'Orbitron', sans-serif !important;
      color: #00f0ff !important;
      border-bottom: 1px solid #232736 !important;
      text-transform: uppercase;
    }
    .swagger-ui .opblock {
      background: #0e1017 !important;
      border: 1px solid #232736 !important;
      border-left: 4px solid #00f0ff !important;
      box-shadow: 0 0 10px rgba(0, 240, 255, 0.1) !important;
      margin-bottom: 1rem !important;
    }
    .swagger-ui .opblock .opblock-summary-method {
      font-family: 'Orbitron', sans-serif !important;
      font-weight: 900 !important;
      border-radius: 0 !important;
      text-transform: uppercase !important;
    }
    .swagger-ui .opblock-get { border-left-color: #00f0ff !important; }
    .swagger-ui .opblock-get .opblock-summary-method { background-color: #00f0ff !important; color: #07080c !important; }
    .swagger-ui .opblock-post { border-left-color: #fcee0a !important; }
    .swagger-ui .opblock-post .opblock-summary-method { background-color: #fcee0a !important; color: #07080c !important; }
    .swagger-ui .opblock-patch { border-left-color: #ffaa00 !important; }
    .swagger-ui .opblock-patch .opblock-summary-method { background-color: #ffaa00 !important; color: #07080c !important; }
    .swagger-ui .opblock-delete { border-left-color: #ff0055 !important; }
    .swagger-ui .opblock-delete .opblock-summary-method { background-color: #ff0055 !important; color: #ffffff !important; }
    .swagger-ui .opblock-summary-path, .swagger-ui .opblock-summary-description {
      color: #f0f2f5 !important;
      font-family: 'Share Tech Mono', monospace !important;
    }
    .swagger-ui .btn {
      font-family: 'Orbitron', sans-serif !important;
      background: #fcee0a !important;
      color: #07080c !important;
      border: none !important;
      text-transform: uppercase;
      font-weight: 800;
    }
    .swagger-ui table thead tr th, .swagger-ui .response-col_status, .swagger-ui .parameter__name {
      color: #00f0ff !important;
      font-family: 'Share Tech Mono', monospace !important;
    }
    .swagger-ui textarea, .swagger-ui input[type=text] {
      background: #07080c !important;
      color: #00f0ff !important;
      border: 1px solid #232736 !important;
      font-family: 'Share Tech Mono', monospace !important;
    }
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

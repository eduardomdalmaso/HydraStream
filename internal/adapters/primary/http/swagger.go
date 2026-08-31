package http

import (
	"fmt"
	"net/http"
)

// OpenAPI3Spec contains the complete OpenAPI 3.0 specification for HydraStream REST API.
const OpenAPI3Spec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "HydraStream Control Plane API",
    "description": "High-Performance Video Analytics Stream Management REST API",
    "version": "1.0.0"
  },
  "paths": {
    "/api/v1/streams": {
      "get": {
        "summary": "List active video streams",
        "parameters": [
          { "name": "search", "in": "query", "schema": { "type": "string" } },
          { "name": "tenant", "in": "query", "schema": { "type": "string" } },
          { "name": "sort_by", "in": "query", "schema": { "type": "string" } },
          { "name": "page", "in": "query", "schema": { "type": "integer", "default": 1 } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 10 } }
        ],
        "responses": { "200": { "description": "List of active streams" } }
      },
      "post": {
        "summary": "Register a new RTSP/Video stream",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["stream_id", "source_url"],
                "properties": {
                  "tenant_id": { "type": "string" },
                  "stream_id": { "type": "string" },
                  "source_url": { "type": "string" },
                  "decoding_engine": { "type": "string" },
                  "ingest_fps": { "type": "number" }
                }
              }
            }
          }
        },
        "responses": { "201": { "description": "Stream created and ingestion started" } }
      }
    },
    "/api/v1/streams/{id}": {
      "get": {
        "summary": "Get stream details by ID",
        "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }],
        "responses": { "200": { "description": "Stream details" }, "404": { "description": "Not found" } }
      },
      "delete": {
        "summary": "Stop ingestion and delete stream",
        "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }],
        "responses": { "200": { "description": "Stream deleted" }, "404": { "description": "Not found" } }
      }
    },
    "/api/v1/streams/{id}/ingest": {
      "get": {
        "summary": "Get live RTSP/RTP ingestion telemetry stats",
        "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }],
        "responses": { "200": { "description": "Live ingestion stats" } }
      }
    },
    "/api/v1/streams/{id}/consumers/{analytic_type}": {
      "patch": {
        "summary": "Update consumer sampling FPS or output format",
        "parameters": [
          { "name": "id", "in": "path", "required": true, "schema": { "type": "string" } },
          { "name": "analytic_type", "in": "path", "required": true, "schema": { "type": "string" } }
        ],
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "properties": {
                  "target_fps": { "type": "number" },
                  "output_format": { "type": "string" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Consumer updated" } }
      }
    },
    "/api/v1/telemetry/stats": {
      "get": {
        "summary": "Get real-time dashboard telemetry and charts history",
        "responses": { "200": { "description": "Control panel telemetry" } }
      }
    },
    "/api/v1/info": {
      "get": {
        "summary": "System and GPU Hardware Readout",
        "responses": { "200": { "description": "Hardware and engine info" } }
      }
    },
    "/api/v1/cluster/topology": {
      "get": {
        "summary": "Active Cluster Nodes and Transport Routing Topology",
        "responses": { "200": { "description": "Cluster topology" } }
      }
    },
    "/api/v1/chaos/inject": {
      "post": {
        "summary": "Inject real fault/chaos experiment into stream pipeline",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["experiment_type"],
                "properties": {
                  "experiment_type": { "type": "string", "enum": ["packet_drop", "disconnect", "gpu_stall", "shm_overflow"] },
                  "intensity": { "type": "number", "default": 25 },
                  "stream_id": { "type": "string", "default": "cam_entrance_01" }
                }
              }
            }
          }
        },
        "responses": { "200": { "description": "Chaos experiment outcome and recovery metrics" } }
      }
    },
    "/api/v1/chaos/reset": {
      "post": {
        "summary": "Disarm all chaos injection circuits",
        "responses": { "200": { "description": "Circuits reset" } }
      }
    },
    "/healthz": {
      "get": { "summary": "Liveness probe", "responses": { "200": { "description": "OK" } } }
    },
    "/metrics": {
      "get": { "summary": "Prometheus Metrics", "responses": { "200": { "description": "Prometheus text format" } } }
    }
  }
}`

// ServeSwaggerUI serves the embedded Swagger UI interface.
func ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>HydraStream API Docs - Swagger UI</title>
  <link rel="stylesheet" type="text/css" href="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/4.18.3/swagger-ui.css" />
  <style>
    html { box-sizing: border-box; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #07080c; color: #fff; }
    .swagger-ui .topbar { display: none; }
    .swagger-ui { filter: invert(88%%) hue-rotate(180deg); }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/swagger-ui/4.18.3/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function() {
      SwaggerUIBundle({
        url: "/swagger/doc.json",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIBundle.SwaggerUIStandalonePreset
        ]
      });
    };
  </script>
</body>
</html>`)
	w.Write([]byte(html))
}

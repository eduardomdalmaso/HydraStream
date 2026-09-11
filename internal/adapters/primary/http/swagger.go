package http

import (
	"fmt"
	"net/http"
)

// OpenAPI3Spec contains the complete OpenAPI 3.0 specification for HydraStream REST API.
const OpenAPI3Spec = `{
  "openapi": "3.0.3",
  "info": {
    "title": "HydraStream Data Plane & Telemetry API",
    "description": "High-Performance Video Ingest, RFC 2326 TCP Demuxing, Zero-Copy SHM and Real-Time Telemetry API",
    "version": "1.0.0"
  },
  "paths": {
    "/api/v1/health": {
      "get": {
        "summary": "Detailed System Health & Service Readiness",
        "description": "Returns overall health status, subsystem readiness (RTSP, MediaMTX, SHM, NATS, GPU), active stream counters and anomaly counts.",
        "responses": {
          "200": { "description": "System is healthy or degraded" },
          "503": { "description": "System is unhealthy" }
        }
      }
    },
    "/api/v1/telemetry": {
      "get": {
        "summary": "Unified Observability & Telemetry",
        "description": "Returns combined health indicators, hardware metrics, and error summaries in a single response.",
        "responses": { "200": { "description": "Unified telemetry payload" } }
      }
    },
    "/api/v1/telemetry/hardware": {
      "get": {
        "summary": "Detailed Hardware Consumption Telemetry",
        "description": "Returns host CPU cores/goroutines, Go memory & host RAM, NVIDIA RTX GPU VRAM/utilization/temp/power, and /dev/shm ring buffer occupancy.",
        "responses": { "200": { "description": "Hardware telemetry payload" } }
      }
    },
    "/api/v1/telemetry/logs": {
      "get": {
        "summary": "Query In-Memory Ring Buffer Telemetry Logs",
        "parameters": [
          { "name": "level", "in": "query", "schema": { "type": "string", "enum": ["DEBUG", "INFO", "WARN", "ERROR", "CRITICAL"] } },
          { "name": "component", "in": "query", "schema": { "type": "string" } },
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 50 } },
          { "name": "since", "in": "query", "schema": { "type": "string", "format": "date-time" } }
        ],
        "responses": { "200": { "description": "Filtered log entries" } }
      },
      "post": {
        "summary": "Ingest Custom Diagnostic Log Entry",
        "requestBody": {
          "required": true,
          "content": {
            "application/json": {
              "schema": {
                "type": "object",
                "required": ["message"],
                "properties": {
                  "level": { "type": "string", "default": "INFO" },
                  "component": { "type": "string", "default": "custom" },
                  "message": { "type": "string" },
                  "details": { "type": "object" }
                }
              }
            }
          }
        },
        "responses": { "201": { "description": "Log entry recorded" } }
      }
    },
    "/api/v1/telemetry/errors": {
      "get": {
        "summary": "Aggregated Error & Anomaly Telemetry",
        "description": "Returns error breakdown by component, recent error traces, and frequency statistics.",
        "responses": { "200": { "description": "Error summary payload" } }
      }
    },
    "/api/v1/telemetry/stats": {
      "get": {
        "summary": "Control Panel Live Stream Telemetry",
        "responses": { "200": { "description": "Control panel telemetry" } }
      }
    },
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
    "/healthz": {
      "get": { "summary": "Liveness probe", "responses": { "200": { "description": "OK" } } }
    },
    "/readyz": {
      "get": { "summary": "Readiness probe", "responses": { "200": { "description": "READY" } } }
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

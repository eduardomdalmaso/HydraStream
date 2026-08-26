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
        "summary": "List active streams",
        "responses": {
          "200": { "description": "OK" }
        }
      },
      "post": {
        "summary": "Register a new RTSP stream",
        "responses": {
          "201": { "description": "Stream created" }
        }
      }
    },
    "/api/v1/info": {
      "get": {
        "summary": "System Info & Hardware Readout",
        "responses": {
          "200": { "description": "OK" }
        }
      }
    },
    "/api/v1/cluster/topology": {
      "get": {
        "summary": "Cluster Zero-Copy Transport Topology",
        "responses": {
          "200": { "description": "OK" }
        }
      }
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
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #0b0e14; color: #fff; }
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

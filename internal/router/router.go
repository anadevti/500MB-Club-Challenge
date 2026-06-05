package router

import (
	"net/http"

	"github.com/eleven/500mb-challenge/internal/handler"
	"github.com/eleven/500mb-challenge/internal/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// New creates and configures an [http.ServeMux] with all application routes,
// wrapped with the metrics middleware.
func New(h *handler.Handler) http.Handler {
	mux := http.NewServeMux()

	// Health & observability
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("GET /readyz", h.Readyz)
	mux.Handle("GET /metrics", instanceHeader(h, promhttp.Handler()))

	// Telemetry ingestion
	mux.HandleFunc("POST /devices/{id}/telemetry/batch", h.IngestBatch)
	mux.HandleFunc("POST /devices/{id}/telemetry", h.IngestSingle)

	// Telemetry query
	mux.HandleFunc("GET /devices/{id}/telemetry", h.QueryTelemetry)

	// Anomaly detection
	mux.HandleFunc("GET /devices/{id}/anomaly", h.Anomaly)

	return middleware.Metrics(mux)
}

// instanceHeader wraps an [http.Handler] to set the X-Instance-Id header.
func instanceHeader(h *handler.Handler, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.SetInstanceHeader(w)
		next.ServeHTTP(w, r)
	}
}

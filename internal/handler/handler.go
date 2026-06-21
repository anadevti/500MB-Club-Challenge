package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/eleven/500mb-challenge/internal/model"
	"github.com/prometheus/client_golang/prometheus"
)

const maxBodySize = 1 << 20

var telemetryPointsIngested = prometheus.NewCounter(
	prometheus.CounterOpts{
		Name: "telemetry_points_ingested_total",
		Help: "Total telemetry points ingested",
	},
)

func init() {
	prometheus.MustRegister(telemetryPointsIngested)
}

type Store interface {
	Ping(ctx context.Context) error
	IngestPoint(ctx context.Context, deviceID string, p *model.TelemetryPoint) error
	IngestBatch(ctx context.Context, deviceID string, points []model.TelemetryPoint) (int, error)
	QueryPoints(ctx context.Context, deviceID string, from, to int64, limit int, cursor string) ([]model.TelemetryPoint, string, error)
	ComputeAnomaly(ctx context.Context, deviceID string) (*model.AnomalyResponse, error)
}

type Handler struct {
	store      Store
	instanceID string
}

func New(store Store, instanceID string) *Handler {
	return &Handler{store: store, instanceID: instanceID}
}

func (h *Handler) SetInstanceHeader(w http.ResponseWriter) {
	w.Header().Set("X-Instance-Id", h.instanceID)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	h.SetInstanceHeader(w)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (h *Handler) writeEmpty(w http.ResponseWriter, status int) {
	h.SetInstanceHeader(w)
	w.WriteHeader(status)
}

func (h *Handler) writeError(w http.ResponseWriter, status int) {
	h.SetInstanceHeader(w)
	w.WriteHeader(status)
}

func validateDeviceID(id string) bool {
	return model.DeviceIDRegex.MatchString(id)
}

func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(r.Context()); err != nil {
		h.writeError(w, http.StatusServiceUnavailable)
		return
	}
	h.writeEmpty(w, http.StatusOK)
}

func (h *Handler) IngestSingle(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateDeviceID(id) {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxBodySize)
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			h.writeError(w, http.StatusRequestEntityTooLarge)
			return
		}
		h.writeError(w, http.StatusBadRequest)
		return
	}

	var point model.TelemetryPoint
	if err := json.Unmarshal(data, &point); err != nil {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	if !point.Validate() {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	if err := h.store.IngestPoint(r.Context(), id, &point); err != nil {
		h.writeError(w, http.StatusInternalServerError)
		return
	}

	telemetryPointsIngested.Inc()
	h.writeEmpty(w, http.StatusAccepted)
}

func (h *Handler) IngestBatch(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateDeviceID(id) {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxBodySize)
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			h.writeError(w, http.StatusRequestEntityTooLarge)
			return
		}
		h.writeError(w, http.StatusBadRequest)
		return
	}

	var batch model.BatchRequest
	if err := json.Unmarshal(data, &batch); err != nil {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	if len(batch.Points) == 0 {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	if len(batch.Points) > 100 {
		h.writeError(w, http.StatusRequestEntityTooLarge)
		return
	}

	for i := range batch.Points {
		if !batch.Points[i].Validate() {
			h.writeError(w, http.StatusBadRequest)
			return
		}
	}

	accepted, err := h.store.IngestBatch(r.Context(), id, batch.Points)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError)
		return
	}

	telemetryPointsIngested.Add(float64(accepted))
	h.writeJSON(w, http.StatusAccepted, model.BatchResponse{Accepted: accepted})
}

func (h *Handler) QueryTelemetry(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateDeviceID(id) {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	q := r.URL.Query()

	fromStr := q.Get("from")
	toStr := q.Get("to")
	if fromStr == "" || toStr == "" {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	from, err := strconv.ParseInt(fromStr, 10, 64)
	if err != nil {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	to, err := strconv.ParseInt(toStr, 10, 64)
	if err != nil {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	if from > to {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	limit := 100
	if limitStr := q.Get("limit"); limitStr != "" {
		l, err := strconv.Atoi(limitStr)
		if err != nil || l < 1 || l > 500 {
			h.writeError(w, http.StatusBadRequest)
			return
		}
		limit = l
	}

	cursor := q.Get("cursor")

	points, nextCursor, err := h.store.QueryPoints(r.Context(), id, from, to, limit, cursor)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError)
		return
	}

	if points == nil {
		points = []model.TelemetryPoint{}
	}

	resp := model.QueryResponse{
		Points: points,
	}
	if nextCursor != "" {
		resp.NextCursor = &nextCursor
	}

	h.writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) Anomaly(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !validateDeviceID(id) {
		h.writeError(w, http.StatusBadRequest)
		return
	}

	result, err := h.store.ComputeAnomaly(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient data") {
			h.writeError(w, http.StatusNotFound)
			return
		}
		h.writeError(w, http.StatusInternalServerError)
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

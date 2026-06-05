package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eleven/500mb-challenge/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock store ---

type mockStore struct {
	pingErr        error
	ingestPointErr error
	ingestBatchN   int
	ingestBatchErr error
	queryPoints    []model.TelemetryPoint
	queryCursor    string
	queryErr       error
	anomalyResult  *model.AnomalyResponse
	anomalyErr     error
}

func (m *mockStore) Ping(_ context.Context) error { return m.pingErr }

func (m *mockStore) IngestPoint(_ context.Context, _ string, _ *model.TelemetryPoint) error {
	return m.ingestPointErr
}

func (m *mockStore) IngestBatch(_ context.Context, _ string, points []model.TelemetryPoint) (int, error) {
	if m.ingestBatchErr != nil {
		return 0, m.ingestBatchErr
	}
	if m.ingestBatchN > 0 {
		return m.ingestBatchN, nil
	}
	return len(points), nil
}

func (m *mockStore) QueryPoints(_ context.Context, _ string, _, _ int64, _ int, _ string) ([]model.TelemetryPoint, string, error) {
	return m.queryPoints, m.queryCursor, m.queryErr
}

func (m *mockStore) ComputeAnomaly(_ context.Context, _ string) (*model.AnomalyResponse, error) {
	return m.anomalyResult, m.anomalyErr
}

// --- helpers ---

func newHandler(store *mockStore) *Handler {
	return New(store, "test-instance")
}

func validPointJSON() []byte {
	b, _ := json.Marshal(model.TelemetryPoint{
		TS:  1700000000,
		Lat: 40.7128,
		Lon: -74.0060,
		AX:  0.1,
		AY:  0.2,
		AZ:  9.8,
	})
	return b
}

// newRequest creates a request with the given path value set via the mux.
// Go 1.22+ ServeMux sets PathValue automatically when routing,
// but for direct handler tests we need the mux to do it.
func serveWithMux(
	t *testing.T,
	method, pattern, target string,
	body []byte,
	handlerFunc http.HandlerFunc,
) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(method+" "+pattern, handlerFunc)

	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// --- Healthz ---

func TestHandler_Healthz(t *testing.T) {
	t.Parallel()

	h := newHandler(&mockStore{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.Healthz(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test-instance", rec.Header().Get("X-Instance-Id"))
}

// --- Readyz ---

func TestHandler_Readyz(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
	}{
		{
			name:       "healthy",
			pingErr:    nil,
			wantStatus: http.StatusOK,
		},
		{
			name:       "unhealthy",
			pingErr:    fmt.Errorf("connection refused"),
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHandler(&mockStore{pingErr: tt.pingErr})
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			rec := httptest.NewRecorder()

			h.Readyz(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "test-instance", rec.Header().Get("X-Instance-Id"))
		})
	}
}

// --- IngestSingle ---

func TestHandler_IngestSingle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		deviceID   string
		body       []byte
		storeErr   error
		wantStatus int
	}{
		{
			name:       "valid point",
			deviceID:   "device-01",
			body:       validPointJSON(),
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "invalid device id",
			deviceID:   "invalid.device",
			body:       validPointJSON(),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid json",
			deviceID:   "device-01",
			body:       []byte(`{invalid`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid telemetry - zero ts",
			deviceID:   "device-01",
			body:       []byte(`{"ts":0,"lat":0,"lon":0,"ax":0,"ay":0,"az":0}`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "store error",
			deviceID:   "device-01",
			body:       validPointJSON(),
			storeErr:   fmt.Errorf("redis unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHandler(&mockStore{ingestPointErr: tt.storeErr})
			rec := serveWithMux(
				t,
				http.MethodPost, "/devices/{id}/telemetry",
				"/devices/"+tt.deviceID+"/telemetry",
				tt.body,
				h.IngestSingle,
			)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// --- IngestBatch ---

func TestHandler_IngestBatch(t *testing.T) {
	t.Parallel()

	validBatch := model.BatchRequest{
		Points: []model.TelemetryPoint{
			{TS: 1700000000, Lat: 40, Lon: -74, AX: 0.1, AY: 0.2, AZ: 9.8},
			{TS: 1700000001, Lat: 41, Lon: -73, AX: 0.2, AY: 0.3, AZ: 9.7},
		},
	}
	validBatchJSON, _ := json.Marshal(validBatch)

	tests := []struct {
		name       string
		deviceID   string
		body       []byte
		storeErr   error
		wantStatus int
	}{
		{
			name:       "valid batch",
			deviceID:   "device-01",
			body:       validBatchJSON,
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "invalid device id",
			deviceID:   "bad.device",
			body:       validBatchJSON,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty points",
			deviceID:   "device-01",
			body:       []byte(`{"points":[]}`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid json",
			deviceID:   "device-01",
			body:       []byte(`not json`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid point in batch",
			deviceID:   "device-01",
			body:       []byte(`{"points":[{"ts":0,"lat":0,"lon":0,"ax":0,"ay":0,"az":0}]}`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "store error",
			deviceID:   "device-01",
			body:       validBatchJSON,
			storeErr:   fmt.Errorf("redis unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHandler(&mockStore{ingestBatchErr: tt.storeErr})
			rec := serveWithMux(
				t,
				http.MethodPost, "/devices/{id}/telemetry/batch",
				"/devices/"+tt.deviceID+"/telemetry/batch",
				tt.body,
				h.IngestBatch,
			)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_IngestBatch_TooManyPoints(t *testing.T) {
	t.Parallel()

	points := make([]model.TelemetryPoint, 101)
	for i := range points {
		points[i] = model.TelemetryPoint{
			TS: int64(1700000000 + i), Lat: 40, Lon: -74,
			AX: 0.1, AY: 0.2, AZ: 9.8,
		}
	}
	body, _ := json.Marshal(model.BatchRequest{Points: points})

	h := newHandler(&mockStore{})
	rec := serveWithMux(
		t,
		http.MethodPost, "/devices/{id}/telemetry/batch",
		"/devices/device-01/telemetry/batch",
		body,
		h.IngestBatch,
	)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
}

func TestHandler_IngestBatch_ResponseBody(t *testing.T) {
	t.Parallel()

	batch := model.BatchRequest{
		Points: []model.TelemetryPoint{
			{TS: 1700000000, Lat: 40, Lon: -74, AX: 0.1, AY: 0.2, AZ: 9.8},
			{TS: 1700000001, Lat: 41, Lon: -73, AX: 0.2, AY: 0.3, AZ: 9.7},
		},
	}
	body, _ := json.Marshal(batch)

	h := newHandler(&mockStore{})
	rec := serveWithMux(
		t,
		http.MethodPost, "/devices/{id}/telemetry/batch",
		"/devices/device-01/telemetry/batch",
		body,
		h.IngestBatch,
	)

	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp model.BatchResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 2, resp.Accepted)
}

// --- QueryTelemetry ---

func TestHandler_QueryTelemetry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		deviceID   string
		query      string
		storeErr   error
		wantStatus int
	}{
		{
			name:       "valid query",
			deviceID:   "device-01",
			query:      "?from=1000&to=2000",
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid device id",
			deviceID:   "bad.device",
			query:      "?from=1000&to=2000",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing from",
			deviceID:   "device-01",
			query:      "?to=2000",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing to",
			deviceID:   "device-01",
			query:      "?from=1000",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "from greater than to",
			deviceID:   "device-01",
			query:      "?from=2000&to=1000",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid from",
			deviceID:   "device-01",
			query:      "?from=abc&to=2000",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid to",
			deviceID:   "device-01",
			query:      "?from=1000&to=abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid limit - zero",
			deviceID:   "device-01",
			query:      "?from=1000&to=2000&limit=0",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid limit - above 500",
			deviceID:   "device-01",
			query:      "?from=1000&to=2000&limit=501",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid limit - not a number",
			deviceID:   "device-01",
			query:      "?from=1000&to=2000&limit=abc",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "valid limit",
			deviceID:   "device-01",
			query:      "?from=1000&to=2000&limit=50",
			wantStatus: http.StatusOK,
		},
		{
			name:       "store error",
			deviceID:   "device-01",
			query:      "?from=1000&to=2000",
			storeErr:   fmt.Errorf("redis unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHandler(&mockStore{queryErr: tt.storeErr})
			rec := serveWithMux(
				t,
				http.MethodGet, "/devices/{id}/telemetry",
				"/devices/"+tt.deviceID+"/telemetry"+tt.query,
				nil,
				h.QueryTelemetry,
			)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_QueryTelemetry_WithCursor(t *testing.T) {
	t.Parallel()

	points := []model.TelemetryPoint{
		{TS: 1700000000, Lat: 40, Lon: -74, AX: 0.1, AY: 0.2, AZ: 9.8},
	}
	cursor := "1700000000"

	h := newHandler(&mockStore{queryPoints: points, queryCursor: cursor})
	rec := serveWithMux(
		t,
		http.MethodGet, "/devices/{id}/telemetry",
		"/devices/device-01/telemetry?from=1000&to=2000",
		nil,
		h.QueryTelemetry,
	)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp model.QueryResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Len(t, resp.Points, 1)
	require.NotNil(t, resp.NextCursor)
	assert.Equal(t, "1700000000", *resp.NextCursor)
}

func TestHandler_QueryTelemetry_NoCursor(t *testing.T) {
	t.Parallel()

	h := newHandler(&mockStore{queryPoints: []model.TelemetryPoint{}})
	rec := serveWithMux(
		t,
		http.MethodGet, "/devices/{id}/telemetry",
		"/devices/device-01/telemetry?from=1000&to=2000",
		nil,
		h.QueryTelemetry,
	)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp model.QueryResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Nil(t, resp.NextCursor)
}

// --- Anomaly ---

func TestHandler_Anomaly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		deviceID   string
		result     *model.AnomalyResponse
		storeErr   error
		wantStatus int
	}{
		{
			name:     "valid anomaly",
			deviceID: "device-01",
			result: &model.AnomalyResponse{
				ZScore:    1.5,
				Samples:   100,
				Anomalous: false,
				Mean:      9.8,
				StdDev:    0.3,
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "invalid device id",
			deviceID:   "bad.device",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "insufficient data",
			deviceID:   "device-01",
			storeErr:   fmt.Errorf("insufficient data: 3 points"),
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "store error",
			deviceID:   "device-01",
			storeErr:   fmt.Errorf("redis unavailable"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHandler(&mockStore{
				anomalyResult: tt.result,
				anomalyErr:    tt.storeErr,
			})
			rec := serveWithMux(
				t,
				http.MethodGet, "/devices/{id}/anomaly",
				"/devices/"+tt.deviceID+"/anomaly",
				nil,
				h.Anomaly,
			)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestHandler_Anomaly_ResponseBody(t *testing.T) {
	t.Parallel()

	expected := &model.AnomalyResponse{
		ZScore:    3.5,
		Samples:   256,
		Anomalous: true,
		Mean:      9.8,
		StdDev:    0.2,
	}

	h := newHandler(&mockStore{anomalyResult: expected})
	rec := serveWithMux(
		t,
		http.MethodGet, "/devices/{id}/anomaly",
		"/devices/device-01/anomaly",
		nil,
		h.Anomaly,
	)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp model.AnomalyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 3.5, resp.ZScore)
	assert.True(t, resp.Anomalous)
	assert.Equal(t, 256, resp.Samples)
}

// --- validateDeviceID ---

func TestValidateDeviceID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"valid alphanumeric", "device01", true},
		{"valid with hyphens", "my-device", true},
		{"valid with underscores", "my_device", true},
		{"empty", "", false},
		{"with spaces", "bad device", false},
		{"with special chars", "bad@device!", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, validateDeviceID(tt.id))
		})
	}
}

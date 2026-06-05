package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetrics_SetsStatusAndCallsNext(t *testing.T) {
	t.Parallel()

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	})

	handler := Metrics(next)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.True(t, called, "next handler should be called")
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestMetrics_DefaultsTo200(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	handler := Metrics(next)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestStatusRecorder_CapturesWriteHeader(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	sr := &statusRecorder{ResponseWriter: rec, status: http.StatusOK}

	sr.WriteHeader(http.StatusNotFound)

	assert.Equal(t, http.StatusNotFound, sr.status)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestNormalizePath_UsesPattern(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	var captured string
	mux.HandleFunc("GET /devices/{id}/telemetry", func(w http.ResponseWriter, r *http.Request) {
		captured = normalizePath(r)
	})

	req := httptest.NewRequest(http.MethodGet, "/devices/abc-123/telemetry", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, "GET /devices/{id}/telemetry", captured)
}

func TestNormalizePath_FallsBackToPath(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/some/path", nil)
	result := normalizePath(req)

	assert.Equal(t, "/some/path", result)
}

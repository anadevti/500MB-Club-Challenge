package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eleven/500mb-challenge/internal/handler"
	"github.com/stretchr/testify/assert"
)

func TestNew_RoutesRegistered(t *testing.T) {
	t.Parallel()

	// We can't use nullStore since handler.Store is an interface with typed methods.
	// Instead, we create the handler and just check the mux routes return non-404.
	// The handler needs a real Store — use a minimal mock.
	h := handler.New(&routerTestStore{}, "test-instance")
	mux := New(h)

	tests := []struct {
		name   string
		method string
		path   string
		want   int // expected status (anything except 404/405 means route exists)
	}{
		{"healthz", http.MethodGet, "/healthz", http.StatusOK},
		{"readyz", http.MethodGet, "/readyz", http.StatusOK},
		{"metrics", http.MethodGet, "/metrics", http.StatusOK},
		{"ingest single", http.MethodPost, "/devices/dev-01/telemetry", http.StatusBadRequest},      // no body → 400
		{"ingest batch", http.MethodPost, "/devices/dev-01/telemetry/batch", http.StatusBadRequest}, // no body → 400
		{"query telemetry", http.MethodGet, "/devices/dev-01/telemetry", http.StatusBadRequest},     // no params → 400
		{"anomaly", http.MethodGet, "/devices/dev-01/anomaly", http.StatusNotFound},                 // insufficient data
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			// Route is registered when the handler sets X-Instance-Id.
			// A mux-level 404 would NOT have this header.
			assert.NotEmpty(t, rec.Header().Get("X-Instance-Id"), "route %s %s should be registered (missing X-Instance-Id)", tt.method, tt.path)
		})
	}
}

func TestNew_UnknownRoute_Returns404(t *testing.T) {
	t.Parallel()

	h := handler.New(&routerTestStore{}, "test-instance")
	mux := New(h)

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestNew_WrongMethod_Returns405(t *testing.T) {
	t.Parallel()

	h := handler.New(&routerTestStore{}, "test-instance")
	mux := New(h)

	req := httptest.NewRequest(http.MethodDelete, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestNew_MetricsHasInstanceHeader(t *testing.T) {
	t.Parallel()

	h := handler.New(&routerTestStore{}, "my-instance")
	mux := New(h)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, "my-instance", rec.Header().Get("X-Instance-Id"))
}

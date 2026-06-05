package router

import (
	"context"
	"fmt"

	"github.com/eleven/500mb-challenge/internal/model"
)

// routerTestStore is a minimal [handler.Store] implementation for router tests.
type routerTestStore struct{}

func (routerTestStore) Ping(_ context.Context) error { return nil }

func (routerTestStore) IngestPoint(_ context.Context, _ string, _ *model.TelemetryPoint) error {
	return nil
}

func (routerTestStore) IngestBatch(_ context.Context, _ string, points []model.TelemetryPoint) (int, error) {
	return len(points), nil
}

func (routerTestStore) QueryPoints(_ context.Context, _ string, _, _ int64, _ int, _ string) ([]model.TelemetryPoint, string, error) {
	return []model.TelemetryPoint{}, "", nil
}

func (routerTestStore) ComputeAnomaly(_ context.Context, _ string) (*model.AnomalyResponse, error) {
	return nil, fmt.Errorf("insufficient data: 0 points")
}

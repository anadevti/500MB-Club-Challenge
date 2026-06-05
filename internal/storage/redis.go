package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/eleven/500mb-challenge/internal/model"
	"github.com/redis/go-redis/v9"
)

type RedisStore struct {
	client *redis.Client
}

func NewRedisStore(addr, password string, db int) *RedisStore {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     64,
		MinIdleConns: 8,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
		DialTimeout:  5 * time.Second,
	})
	return &RedisStore{client: client}
}

func (s *RedisStore) Ping(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *RedisStore) Close() error {
	return s.client.Close()
}

func telemetryKey(deviceID string) string {
	return "t:" + deviceID
}

func (s *RedisStore) IngestPoint(ctx context.Context, deviceID string, p *model.TelemetryPoint) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}

	return s.client.ZAdd(ctx, telemetryKey(deviceID), redis.Z{
		Score:  float64(p.TS),
		Member: data,
	}).Err()
}

func (s *RedisStore) IngestBatch(ctx context.Context, deviceID string, points []model.TelemetryPoint) (int, error) {
	key := telemetryKey(deviceID)
	members := make([]redis.Z, 0, len(points))

	for i := range points {
		data, err := json.Marshal(&points[i])
		if err != nil {
			return 0, err
		}
		members = append(members, redis.Z{
			Score:  float64(points[i].TS),
			Member: data,
		})
	}

	pipe := s.client.Pipeline()
	pipe.ZAdd(ctx, key, members...)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return len(points), nil
}

func (s *RedisStore) QueryPoints(ctx context.Context, deviceID string, from, to int64, limit int, cursor string) ([]model.TelemetryPoint, string, error) {
	key := telemetryKey(deviceID)

	minScore := strconv.FormatInt(from, 10)
	if cursor != "" {
		minScore = "(" + cursor
	}
	maxScore := strconv.FormatInt(to, 10)

	results, err := s.client.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min:   minScore,
		Max:   maxScore,
		Count: int64(limit + 1),
	}).Result()
	if err != nil {
		return nil, "", err
	}

	hasMore := len(results) > limit
	if hasMore {
		results = results[:limit]
	}

	points := make([]model.TelemetryPoint, 0, len(results))
	var lastTS int64

	for _, raw := range results {
		var p model.TelemetryPoint
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		points = append(points, p)
		lastTS = p.TS
	}

	var nextCursor string
	if hasMore {
		nextCursor = strconv.FormatInt(lastTS, 10)
	}

	return points, nextCursor, nil
}

func (s *RedisStore) GetLastNPoints(ctx context.Context, deviceID string, n int) ([]model.TelemetryPoint, error) {
	key := telemetryKey(deviceID)

	results, err := s.client.ZRevRange(ctx, key, 0, int64(n-1)).Result()
	if err != nil {
		return nil, err
	}

	points := make([]model.TelemetryPoint, 0, len(results))

	for i := len(results) - 1; i >= 0; i-- {
		var p model.TelemetryPoint
		if err := json.Unmarshal([]byte(results[i]), &p); err != nil {
			continue
		}
		points = append(points, p)
	}
	return points, nil
}

func (s *RedisStore) ComputeAnomaly(ctx context.Context, deviceID string) (*model.AnomalyResponse, error) {
	points, err := s.GetLastNPoints(ctx, deviceID, 256)
	if err != nil {
		return nil, err
	}

	if len(points) < 8 {
		return nil, fmt.Errorf("insufficient data: %d points", len(points))
	}

	magnitudes := make([]float64, len(points))
	for i, p := range points {
		magnitudes[i] = math.Sqrt(p.AX*p.AX + p.AY*p.AY + p.AZ*p.AZ)
	}

	var sum float64
	for _, m := range magnitudes {
		sum += m
	}
	mean := sum / float64(len(magnitudes))

	var varianceSum float64
	for _, m := range magnitudes {
		diff := m - mean
		varianceSum += diff * diff
	}
	stddev := math.Sqrt(varianceSum / float64(len(magnitudes)))

	latest := magnitudes[len(magnitudes)-1]
	var zScore float64
	if stddev == 0 {
		zScore = 0
	} else {
		zScore = (latest - mean) / stddev
	}

	return &model.AnomalyResponse{
		ZScore:    zScore,
		Samples:   len(points),
		Anomalous: zScore > 3,
		Mean:      mean,
		StdDev:    stddev,
	}, nil
}

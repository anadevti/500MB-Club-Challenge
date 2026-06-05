package model

import (
	"math"
	"regexp"
)

var DeviceIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

type TelemetryPoint struct {
	TS      int64    `json:"ts"`
	Lat     float64  `json:"lat"`
	Lon     float64  `json:"lon"`
	Battery *float64 `json:"battery,omitempty"`
	AX      float64  `json:"ax"`
	AY      float64  `json:"ay"`
	AZ      float64  `json:"az"`
}

type BatchRequest struct {
	Points []TelemetryPoint `json:"points"`
}

type BatchResponse struct {
	Accepted int `json:"accepted"`
}

type QueryResponse struct {
	Points     []TelemetryPoint `json:"points"`
	NextCursor *string          `json:"next_cursor"`
}

type AnomalyResponse struct {
	ZScore    float64 `json:"z_score"`
	Samples   int     `json:"samples"`
	Anomalous bool    `json:"anomalous"`
	Mean      float64 `json:"mean"`
	StdDev    float64 `json:"stddev"`
}

func (p *TelemetryPoint) Validate() bool {
	if p.TS <= 0 {
		return false
	}
	if p.Lat < -90 || p.Lat > 90 {
		return false
	}
	if p.Lon < -180 || p.Lon > 180 {
		return false
	}
	if p.Battery != nil && (*p.Battery < 0 || *p.Battery > 1) {
		return false
	}
	if math.IsInf(p.AX, 0) || math.IsNaN(p.AX) {
		return false
	}
	if math.IsInf(p.AY, 0) || math.IsNaN(p.AY) {
		return false
	}
	if math.IsInf(p.AZ, 0) || math.IsNaN(p.AZ) {
		return false
	}
	return true
}

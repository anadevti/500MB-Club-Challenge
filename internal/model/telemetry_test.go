package model

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func ptr(v float64) *float64 { return &v }

func validPoint() TelemetryPoint {
	return TelemetryPoint{
		TS:  1700000000,
		Lat: 40.7128,
		Lon: -74.0060,
		AX:  0.1,
		AY:  0.2,
		AZ:  9.8,
	}
}

func TestTelemetryPoint_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		point TelemetryPoint
		want  bool
	}{
		{
			name:  "valid point without battery",
			point: validPoint(),
			want:  true,
		},
		{
			name: "valid point with battery",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Battery = ptr(0.85)
				return p
			}(),
			want: true,
		},
		{
			name: "battery at lower bound",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Battery = ptr(0)
				return p
			}(),
			want: true,
		},
		{
			name: "battery at upper bound",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Battery = ptr(1)
				return p
			}(),
			want: true,
		},
		{
			name: "zero timestamp",
			point: func() TelemetryPoint {
				p := validPoint()
				p.TS = 0
				return p
			}(),
			want: false,
		},
		{
			name: "negative timestamp",
			point: func() TelemetryPoint {
				p := validPoint()
				p.TS = -1
				return p
			}(),
			want: false,
		},
		{
			name: "lat below -90",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Lat = -90.1
				return p
			}(),
			want: false,
		},
		{
			name: "lat above 90",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Lat = 90.1
				return p
			}(),
			want: false,
		},
		{
			name: "lon below -180",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Lon = -180.1
				return p
			}(),
			want: false,
		},
		{
			name: "lon above 180",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Lon = 180.1
				return p
			}(),
			want: false,
		},
		{
			name: "battery below 0",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Battery = ptr(-0.1)
				return p
			}(),
			want: false,
		},
		{
			name: "battery above 1",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Battery = ptr(1.1)
				return p
			}(),
			want: false,
		},
		{
			name: "AX is NaN",
			point: func() TelemetryPoint {
				p := validPoint()
				p.AX = math.NaN()
				return p
			}(),
			want: false,
		},
		{
			name: "AY is +Inf",
			point: func() TelemetryPoint {
				p := validPoint()
				p.AY = math.Inf(1)
				return p
			}(),
			want: false,
		},
		{
			name: "AZ is -Inf",
			point: func() TelemetryPoint {
				p := validPoint()
				p.AZ = math.Inf(-1)
				return p
			}(),
			want: false,
		},
		{
			name: "boundary lat -90",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Lat = -90
				return p
			}(),
			want: true,
		},
		{
			name: "boundary lat 90",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Lat = 90
				return p
			}(),
			want: true,
		},
		{
			name: "boundary lon -180",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Lon = -180
				return p
			}(),
			want: true,
		},
		{
			name: "boundary lon 180",
			point: func() TelemetryPoint {
				p := validPoint()
				p.Lon = 180
				return p
			}(),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.point.Validate())
		})
	}
}

func TestDeviceIDRegex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"simple alphanumeric", "device01", true},
		{"with hyphens", "my-device-1", true},
		{"with underscores", "my_device_1", true},
		{"mixed", "Device_01-ABC", true},
		{"single char", "x", true},
		{"max 64 chars", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"empty", "", false},
		{"65 chars", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
		{"contains space", "device 01", false},
		{"contains dot", "device.01", false},
		{"contains slash", "device/01", false},
		{"special chars", "device@01!", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, DeviceIDRegex.MatchString(tt.id))
		})
	}
}

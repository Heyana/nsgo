package gameservices

import (
	"math"
	"testing"
)

func TestClampPercent(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{-100, 0},
		{-0.01, 0},
		{0, 0},
		{42.5, 42.5},
		{100, 100},
		{100.1, 100},
		{999, 100},
		{math.NaN(), 0},
		{math.Inf(-1), 0},
		{math.Inf(1), 100},
	}
	for _, tc := range cases {
		if got := clampPercent(tc.in); got != tc.want {
			t.Errorf("clampPercent(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeLimit(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{-10, defaultScoreLimit},
		{0, defaultScoreLimit},
		{1, 1},
		{25, 25},
		{100, 100},
		{500, maxScoreLimit},
	}
	for _, tc := range cases {
		if got := normalizeLimit(tc.in); got != tc.want {
			t.Errorf("normalizeLimit(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

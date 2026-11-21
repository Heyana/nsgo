package driver

import (
	"math"
	"testing"
)

func TestAudioType(t *testing.T) {
	for _, tc := range []struct {
		data string
		want string
	}{
		{"RIFF\x24\x00\x00\x00WAVEfmt ", "audio/wav"},
		{"FORM\x00\x00\x00\x00AIFC", "audio/aiff"},
		{"caff\x00\x01", "audio/x-caf"},
		{"fLaC\x00", "audio/flac"},
		{"OggS\x00\x02", "audio/ogg"},
		{"\x00\x00\x00\x20ftypM4A ", "audio/mp4"},
		{"ID3\x04\x00", "audio/mpeg"},
		{"\xFF\xFB\x90\x64", "audio/mpeg"},
		{"\xFF\xF1\x50\x80", "audio/aac"},
		{"\xFF\x00", "application/octet-stream"},
		{"RIFF", "application/octet-stream"},
		{"", "application/octet-stream"},
	} {
		if got := AudioType([]byte(tc.data)); got != tc.want {
			t.Errorf("AudioType(%q) = %s, want %s", tc.data, got, tc.want)
		}
	}
}

func TestAudioPanGains(t *testing.T) {
	half := float32(math.Sqrt2 / 2)
	for _, tc := range []struct {
		pan         float32
		left, right float32
	}{
		{0, 1, 1},
		{float32(math.Copysign(0, -1)), 1, 1},
		{float32(math.NaN()), 1, 1},
		{-1, 1, 0},
		{-2, 1, 0},
		{float32(math.Inf(-1)), 1, 0},
		{1, 0, 1},
		{2, 0, 1},
		{float32(math.Inf(1)), 0, 1},
		{-0.5, 1, half},
		{0.5, half, 1},
	} {
		l, r := AudioPanGains(tc.pan)
		if math.Abs(float64(l-tc.left)) > 1e-6 || math.Abs(float64(r-tc.right)) > 1e-6 {
			t.Errorf("AudioPanGains(%v) = (%v, %v), want (%v, %v)", tc.pan, l, r, tc.left, tc.right)
		}
	}
}

package microphone

import (
	"encoding/binary"
	"math"
	"testing"
)

func buildWaveFormatEx(tag, channels uint16, rate uint32, bits uint16) []byte {
	b := make([]byte, 18)
	blockAlign := channels * (bits / 8)
	binary.LittleEndian.PutUint16(b[0:2], tag)
	binary.LittleEndian.PutUint16(b[2:4], channels)
	binary.LittleEndian.PutUint32(b[4:8], rate)
	binary.LittleEndian.PutUint32(b[8:12], rate*uint32(blockAlign))
	binary.LittleEndian.PutUint16(b[12:14], blockAlign)
	binary.LittleEndian.PutUint16(b[14:16], bits)
	binary.LittleEndian.PutUint16(b[16:18], 0)
	return b
}

func buildWaveFormatExtensible(channels uint16, rate uint32, bits uint16, sub [16]byte) []byte {
	b := make([]byte, 40)
	copy(b[:18], buildWaveFormatEx(waveFormatExtensible, channels, rate, bits))
	binary.LittleEndian.PutUint16(b[16:18], 22)
	binary.LittleEndian.PutUint16(b[18:20], bits)
	binary.LittleEndian.PutUint32(b[20:24], 3) // stereo speaker mask
	copy(b[24:40], sub[:])
	return b
}

func TestParseWaveFormat(t *testing.T) {
	f, err := parseWaveFormat(buildWaveFormatEx(waveFormatPCM, 1, 48000, 16))
	if err != nil || f.encoding != pcmInt16 || f.channels != 1 || f.sampleRate != 48000 {
		t.Fatalf("PCM 16: got %+v, err=%v", f, err)
	}
	f, err = parseWaveFormat(buildWaveFormatEx(waveFormatIEEEFloat, 2, 44100, 32))
	if err != nil || f.encoding != pcmFloat32 || f.channels != 2 || f.sampleRate != 44100 {
		t.Fatalf("Float 32: got %+v, err=%v", f, err)
	}
	f, err = parseWaveFormat(buildWaveFormatExtensible(2, 48000, 32, ksSubtypeIEEEFloat))
	if err != nil || f.encoding != pcmFloat32 || f.channels != 2 || f.sampleRate != 48000 {
		t.Fatalf("Extensible float32: got %+v, err=%v", f, err)
	}
	f, err = parseWaveFormat(buildWaveFormatExtensible(2, 48000, 24, ksSubtypePCM))
	if err != nil || f.encoding != pcmInt24 || f.channels != 2 {
		t.Fatalf("Extensible PCM24: got %+v, err=%v", f, err)
	}
	if _, err := parseWaveFormat(make([]byte, 10)); err == nil {
		t.Error("expected error for truncated WAVEFORMATEX")
	}
	if _, err := parseWaveFormat(buildWaveFormatExtensible(2, 48000, 32, [16]byte{0xff})); err == nil {
		t.Error("expected error for unknown extensible subtype")
	}
}

func TestPCMConverterEncodingsAndChannels(t *testing.T) {
	// Float32 stereo -> mono downmix + clamping.
	src := waveFormat{encoding: pcmFloat32, channels: 2, sampleRate: 48000, bitsPerSample: 32, blockAlign: 8}
	conv := newPCMConverter(src, 48000, 1)
	floats := []float32{
		0.5, 0.5, // avg 0.5 -> ~16384
		-1.5, -1.5, // clamped -1.0 -> -32768
		1.5, 1.5, // clamped 1.0 -> 32767
		1.0, -1.0, // avg 0.0 -> 0
	}
	raw := make([]byte, len(floats)*4)
	for i, v := range floats {
		binary.LittleEndian.PutUint32(raw[4*i:], math.Float32bits(v))
	}
	got := conv.convert(raw, 4, false)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[0] < 16383 || got[0] > 16384 || got[1] != -32768 || got[2] != 32767 || got[3] != 0 {
		t.Errorf("float32 stereo->mono = %v", got)
	}

	// 24-bit signed PCM mono -> stereo upmix.
	src24 := waveFormat{encoding: pcmInt24, channels: 1, sampleRate: 48000, bitsPerSample: 24, blockAlign: 3}
	conv24 := newPCMConverter(src24, 48000, 2)
	// +0x400000 (+0.5) and -0x800000 (-1.0)
	raw24 := []byte{0x00, 0x00, 0x40, 0x00, 0x00, 0x80}
	got24 := conv24.convert(raw24, 2, false)
	if len(got24) != 4 || got24[0] != got24[1] || got24[0] < 16383 || got24[0] > 16384 || got24[2] != -32768 || got24[3] != -32768 {
		t.Errorf("int24 mono->stereo = %v", got24)
	}

	// Silent flag produces zero frames without reading raw.
	silent := conv.convert(nil, 10, true)
	if len(silent) != 10 {
		t.Fatalf("silent len = %d, want 10", len(silent))
	}
	for i, s := range silent {
		if s != 0 {
			t.Fatalf("silent[%d] = %d, want 0", i, s)
		}
	}
}

func TestPCMConverterResamplingAcrossPackets(t *testing.T) {
	// 48 kHz mono float32 -> 16 kHz mono int16 across 10 packets of 480 frames (100ms total = 1600 output frames).
	src := waveFormat{encoding: pcmFloat32, channels: 1, sampleRate: 48000, bitsPerSample: 32, blockAlign: 4}
	conv := newPCMConverter(src, 16000, 1)

	var all []int16
	const freq = 440.0
	frameIdx := 0
	for range 10 {
		raw := make([]byte, 480*4)
		for f := range 480 {
			v := float32(0.5 * math.Sin(2*math.Pi*freq*float64(frameIdx)/48000.0))
			binary.LittleEndian.PutUint32(raw[4*f:], math.Float32bits(v))
			frameIdx++
		}
		all = append(all, conv.convert(raw, 480, false)...)
	}
	if len(all) < 1598 || len(all) > 1602 {
		t.Fatalf("resampled frames = %d, want ~1600", len(all))
	}
	// Verify every output sample matches the 440 Hz sine wave closely (no packet-edge discontinuities).
	for i, got := range all {
		want := 0.5 * math.Sin(2*math.Pi*freq*float64(i)/16000.0) * 32767.0
		if diff := math.Abs(float64(got) - want); diff > 400 {
			t.Fatalf("sample %d: got %d, want %.0f (diff %.0f)", i, got, want, diff)
		}
	}
}

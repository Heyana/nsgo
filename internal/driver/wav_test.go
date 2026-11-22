package driver

import (
	"encoding/binary"
	"testing"
)

func TestWAVHeaderRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		rate, channels int
		data           uint32
	}{
		{48000, 1, 0},
		{44100, 2, 176400},
		{16000, 1, MaxWAVDataBytes},
	} {
		h := WAVHeader(tc.rate, tc.channels, tc.data)
		if got := binary.LittleEndian.Uint32(h[4:8]); got != 36+tc.data {
			t.Errorf("%+v: RIFF size = %d, want %d", tc, got, 36+tc.data)
		}
		if got := binary.LittleEndian.Uint32(h[28:32]); got != uint32(tc.rate*tc.channels*2) {
			t.Errorf("%+v: byte rate = %d, want %d", tc, got, tc.rate*tc.channels*2)
		}
		if got := binary.LittleEndian.Uint16(h[32:34]); got != uint16(tc.channels*2) {
			t.Errorf("%+v: block align = %d, want %d", tc, got, tc.channels*2)
		}
		rate, ch, data, ok := ParseWAVHeader(h[:])
		if !ok || rate != tc.rate || ch != tc.channels || data != tc.data {
			t.Errorf("ParseWAVHeader(%+v) = (%d, %d, %d, %v)", tc, rate, ch, data, ok)
		}
	}
}

func TestParseWAVHeaderRejectsInvalid(t *testing.T) {
	valid := WAVHeader(48000, 1, 96000)
	if _, _, _, ok := ParseWAVHeader(valid[:10]); ok {
		t.Error("accepted truncated header")
	}
	badMagic := valid
	badMagic[0] = 'X'
	if _, _, _, ok := ParseWAVHeader(badMagic[:]); ok {
		t.Error("accepted bad RIFF magic")
	}
	badFmt := valid
	binary.LittleEndian.PutUint16(badFmt[20:22], 3) // IEEE float, not PCM
	if _, _, _, ok := ParseWAVHeader(badFmt[:]); ok {
		t.Error("accepted non-PCM format")
	}
	badChannels := valid
	binary.LittleEndian.PutUint16(badChannels[22:24], 0)
	if _, _, _, ok := ParseWAVHeader(badChannels[:]); ok {
		t.Error("accepted 0 channels")
	}
	badRate := valid
	binary.LittleEndian.PutUint32(badRate[24:28], 0)
	if _, _, _, ok := ParseWAVHeader(badRate[:]); ok {
		t.Error("accepted 0 sample rate")
	}
}

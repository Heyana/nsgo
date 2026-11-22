package driver

import "encoding/binary"

const (
	// WAVHeaderSize is the size in bytes of a standard 16-bit PCM RIFF/WAVE header.
	WAVHeaderSize = 44
	// MaxWAVDataBytes is the most PCM payload a 32-bit RIFF/WAVE file can hold
	// (0xffffffff minus the 36 header bytes counted in the RIFF chunk size).
	MaxWAVDataBytes = uint32(0xffffffff - 36)
)

// WAVHeader returns the 44-byte RIFF/WAVE header for dataBytes of 16-bit
// little-endian signed linear PCM at rate hertz and channels (1 or 2).
func WAVHeader(rate, channels int, dataBytes uint32) [WAVHeaderSize]byte {
	var h [WAVHeaderSize]byte
	copy(h[0:4], "RIFF")
	binary.LittleEndian.PutUint32(h[4:8], 36+dataBytes)
	copy(h[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:20], 16) // PCM fmt chunk size
	binary.LittleEndian.PutUint16(h[20:22], 1)  // WAVE_FORMAT_PCM
	binary.LittleEndian.PutUint16(h[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(h[24:28], uint32(rate))
	binary.LittleEndian.PutUint32(h[28:32], uint32(rate)*uint32(channels)*2)
	binary.LittleEndian.PutUint16(h[32:34], uint16(channels*2))
	binary.LittleEndian.PutUint16(h[34:36], 16)
	copy(h[36:40], "data")
	binary.LittleEndian.PutUint32(h[40:44], dataBytes)
	return h
}

// ParseWAVHeader validates a 44-byte 16-bit PCM RIFF/WAVE header written by
// WAVHeader and returns its sample rate, channel count and PCM data size.
func ParseWAVHeader(h []byte) (rate, channels int, dataBytes uint32, ok bool) {
	if len(h) < WAVHeaderSize {
		return 0, 0, 0, false
	}
	if string(h[0:4]) != "RIFF" || string(h[8:16]) != "WAVEfmt " || string(h[36:40]) != "data" {
		return 0, 0, 0, false
	}
	if binary.LittleEndian.Uint32(h[16:20]) != 16 || binary.LittleEndian.Uint16(h[20:22]) != 1 || binary.LittleEndian.Uint16(h[34:36]) != 16 {
		return 0, 0, 0, false
	}
	channels = int(binary.LittleEndian.Uint16(h[22:24]))
	rate = int(binary.LittleEndian.Uint32(h[24:28]))
	if channels < 1 || channels > 2 || rate <= 0 {
		return 0, 0, 0, false
	}
	dataBytes = binary.LittleEndian.Uint32(h[40:44])
	return rate, channels, dataBytes, true
}

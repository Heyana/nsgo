package microphone

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

type pcmEncoding int

const (
	pcmFloat32 pcmEncoding = iota + 1
	pcmInt16
	pcmInt24
	pcmInt32
	pcmUint8
)

const (
	waveFormatPCM        = 0x0001
	waveFormatIEEEFloat  = 0x0003
	waveFormatExtensible = 0xFFFE
)

// Standard Windows KSDATAFORMAT_SUBTYPE GUIDs in little-endian wire layout.
var (
	ksSubtypePCM       = [16]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}
	ksSubtypeIEEEFloat = [16]byte{0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10, 0x00, 0x80, 0x00, 0x00, 0xaa, 0x00, 0x38, 0x9b, 0x71}
)

// waveFormat describes an interleaved PCM stream (such as a WASAPI mix format).
type waveFormat struct {
	encoding      pcmEncoding
	channels      int
	sampleRate    int
	bitsPerSample int
	blockAlign    int
}

// parseWaveFormat decodes a Windows WAVEFORMATEX (18 bytes) or
// WAVEFORMATEXTENSIBLE (40 bytes) byte slice into a waveFormat.
func parseWaveFormat(raw []byte) (waveFormat, error) {
	if len(raw) < 18 {
		return waveFormat{}, errors.New("microphone: truncated WAVEFORMATEX")
	}
	tag := binary.LittleEndian.Uint16(raw[0:2])
	channels := int(binary.LittleEndian.Uint16(raw[2:4]))
	rate := int(binary.LittleEndian.Uint32(raw[4:8]))
	blockAlign := int(binary.LittleEndian.Uint16(raw[12:14]))
	bits := int(binary.LittleEndian.Uint16(raw[14:16]))
	cbSize := int(binary.LittleEndian.Uint16(raw[16:18]))

	if channels <= 0 || rate <= 0 || bits <= 0 || blockAlign < channels*(bits/8) {
		return waveFormat{}, fmt.Errorf("microphone: invalid capture format (%d ch, %d Hz, %d-bit, align %d)", channels, rate, bits, blockAlign)
	}

	isFloat := false
	switch tag {
	case waveFormatPCM:
	case waveFormatIEEEFloat:
		isFloat = true
	case waveFormatExtensible:
		if cbSize < 22 || len(raw) < 40 {
			return waveFormat{}, errors.New("microphone: truncated WAVEFORMATEXTENSIBLE")
		}
		var sub [16]byte
		copy(sub[:], raw[24:40])
		switch sub {
		case ksSubtypePCM:
		case ksSubtypeIEEEFloat:
			isFloat = true
		default:
			return waveFormat{}, fmt.Errorf("microphone: unsupported extensible audio subtype %x", sub)
		}
	default:
		return waveFormat{}, fmt.Errorf("microphone: unsupported audio format tag 0x%x", tag)
	}

	var enc pcmEncoding
	if isFloat {
		if bits != 32 {
			return waveFormat{}, fmt.Errorf("microphone: unsupported IEEE float bit depth %d", bits)
		}
		enc = pcmFloat32
	} else {
		switch bits {
		case 8:
			enc = pcmUint8
		case 16:
			enc = pcmInt16
		case 24:
			enc = pcmInt24
		case 32:
			enc = pcmInt32
		default:
			return waveFormat{}, fmt.Errorf("microphone: unsupported PCM bit depth %d", bits)
		}
	}

	return waveFormat{
		encoding:      enc,
		channels:      channels,
		sampleRate:    rate,
		bitsPerSample: bits,
		blockAlign:    blockAlign,
	}, nil
}

// pcmConverter converts interleaved PCM packets in src into interleaved 16-bit
// signed PCM at dstRate and dstChannels (1 or 2), maintaining streaming
// linear-resampler state across packet boundaries.
type pcmConverter struct {
	src         waveFormat
	dstRate     int
	dstChannels int

	// Resampler state when src.sampleRate != dstRate.
	// pos is the fractional source frame index of the next output frame,
	// relative to the start of the current packet. When pos < 0, it
	// interpolates between prev (at index -1) and packet frame 0.
	hasPrev bool
	prev    [2]float64
	pos     float64
	step    float64
}

func newPCMConverter(src waveFormat, dstRate, dstChannels int) *pcmConverter {
	if dstRate <= 0 {
		dstRate = src.sampleRate
	}
	if dstChannels < 1 {
		dstChannels = 1
	} else if dstChannels > 2 {
		dstChannels = 2
	}
	return &pcmConverter{
		src:         src,
		dstRate:     dstRate,
		dstChannels: dstChannels,
		step:        float64(src.sampleRate) / float64(dstRate),
	}
}

// convert decodes frames from raw (or zero frames when silent is true) and
// returns a newly allocated interleaved int16 slice.
func (c *pcmConverter) convert(raw []byte, frames int, silent bool) []int16 {
	if frames <= 0 {
		return nil
	}
	if !silent && len(raw) < frames*c.src.blockAlign {
		frames = len(raw) / c.src.blockAlign
		if frames <= 0 {
			return nil
		}
	}

	// Fast path: identical sample rate, channel count, and 16-bit signed PCM.
	if c.src.sampleRate == c.dstRate && c.src.channels == c.dstChannels && c.src.encoding == pcmInt16 && c.src.blockAlign == c.dstChannels*2 {
		out := make([]int16, frames*c.dstChannels)
		if silent {
			return out
		}
		for i := range out {
			out[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
		}
		return out
	}

	// Fast path: same sample rate, no resampling needed.
	if c.src.sampleRate == c.dstRate {
		out := make([]int16, frames*c.dstChannels)
		if silent {
			return out
		}
		for f := range frames {
			s := c.decodeFrame(raw[f*c.src.blockAlign:])
			if c.dstChannels == 1 {
				out[f] = floatToInt16(s[0])
			} else {
				out[2*f] = floatToInt16(s[0])
				out[2*f+1] = floatToInt16(s[1])
			}
		}
		return out
	}

	// General path: channel-mapped streaming linear resampling.
	// Decode into a compact buffer of mapped frames ([2]float64).
	mapped := make([][2]float64, frames)
	if !silent {
		for f := range frames {
			mapped[f] = c.decodeFrame(raw[f*c.src.blockAlign:])
		}
	}

	est := int(math.Ceil(float64(frames)/c.step)) + 2
	out := make([]int16, 0, est*c.dstChannels)

	// We can emit output frames while pos <= float64(frames-1) (or < float64(frames)
	// when interpolating into the next packet on the following call).
	limit := float64(frames - 1)
	for c.pos <= limit {
		var s0, s1 [2]float64
		var frac float64
		if c.pos < 0 {
			if c.hasPrev {
				s0 = c.prev
			} else {
				s0 = mapped[0]
			}
			s1 = mapped[0]
			frac = c.pos + 1.0
		} else {
			idx := int(c.pos)
			frac = c.pos - float64(idx)
			s0 = mapped[idx]
			if idx+1 < frames {
				s1 = mapped[idx+1]
			} else {
				s1 = s0
			}
		}
		if c.dstChannels == 1 {
			out = append(out, floatToInt16(s0[0]+(s1[0]-s0[0])*frac))
		} else {
			out = append(out,
				floatToInt16(s0[0]+(s1[0]-s0[0])*frac),
				floatToInt16(s0[1]+(s1[1]-s0[1])*frac),
			)
		}
		c.pos += c.step
	}

	c.prev = mapped[frames-1]
	c.hasPrev = true
	c.pos -= float64(frames)
	return out
}

// decodeFrame reads one source frame at raw and maps it to dstChannels in [-1, 1].
func (c *pcmConverter) decodeFrame(raw []byte) [2]float64 {
	bps := c.src.bitsPerSample / 8
	readSample := func(ch int) float64 {
		off := ch * bps
		switch c.src.encoding {
		case pcmFloat32:
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[off:])))
		case pcmInt16:
			return float64(int16(binary.LittleEndian.Uint16(raw[off:]))) / 32768.0
		case pcmInt24:
			v := int32(raw[off]) | int32(raw[off+1])<<8 | int32(raw[off+2])<<16
			if v&0x800000 != 0 {
				v |= ^0xffffff
			}
			return float64(v) / 8388608.0
		case pcmInt32:
			return float64(int32(binary.LittleEndian.Uint32(raw[off:]))) / 2147483648.0
		case pcmUint8:
			return (float64(raw[off]) - 128.0) / 128.0
		default:
			return 0
		}
	}

	if c.dstChannels == 1 {
		if c.src.channels == 1 {
			return [2]float64{readSample(0), 0}
		}
		var sum float64
		for ch := range c.src.channels {
			sum += readSample(ch)
		}
		return [2]float64{sum / float64(c.src.channels), 0}
	}

	// dstChannels == 2
	if c.src.channels == 1 {
		s := readSample(0)
		return [2]float64{s, s}
	}
	return [2]float64{readSample(0), readSample(1)}
}

func floatToInt16(v float64) int16 {
	if math.IsNaN(v) {
		return 0
	}
	if v >= 1.0 {
		return 32767
	}
	if v <= -1.0 {
		return -32768
	}
	scaled := math.Round(v * 32767.0)
	if scaled > 32767 {
		return 32767
	}
	if scaled < -32768 {
		return -32768
	}
	return int16(scaled)
}

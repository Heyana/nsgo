package android

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestTextCacheKeepsSurfaceDensityAndLegacyLayouts(t *testing.T) {
	var input writer
	for i, text := range []string{"main", "甲😀", "legacy"} {
		a := driver.AttributedText{
			Text: text,
			Style: driver.TextStyle{Font: driver.Font{
				Family: "serif", Language: "ja-JP", Size: 16,
			}},
		}
		var request writer
		request.u64(uint64(i + 1))
		request.byte(textPrepare)
		switch i {
		case 0:
			request.u32(1)
			writeTextPrepare(&request, &a, 0)
		case 1:
			a.MaxLines = 2
			a.Shadow = driver.TextShadow{Color: 0x112233ff, Offset: driver.Point{X: 2}}
			// A hidden span's flag precedes the surface ID, which the cache must still find.
			a.Spans = []driver.TextSpan{{Start: 0, End: 3, Style: a.Style, Hidden: true}}
			request.u32(1)
			writeTextPrepare(&request, &a, 7)
		case 2:
			writeLegacyPreparedText(&request, 2, &a)
		}
		input.u32(uint32(len(request.buf)))
		input.buf = append(input.buf, request.buf...)
	}
	runJavaAlgorithm(t, "pipe", "TextCacheCheck", input.buf, "Protocol", "NsgoEngine", "TextCache")
}

// writeLegacyPreparedText removes the base style's surface flag and suffix from the supported packet. The JVM fixture feeds it to the current reader.
func writeLegacyPreparedText(w *writer, id uint32, text *driver.AttributedText) {
	start := len(w.buf)
	w.u32(id)
	writeTextPrepare(w, text, 0)
	r := reader{w.buf[start:]}
	r.u32()
	r.str()               // text
	r.str()               // family
	r.str()               // language
	const styleFlags = 19 // size, weight, color, line height, letter spacing and alignment
	w.buf[len(w.buf)-len(r.buf)+styleFlags] &^= 1 << 4
	w.buf = w.buf[:len(w.buf)-4]
}

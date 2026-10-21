//go:build android

// Package android is NSGo's Android backend. The app is a plain Go
// executable, built with CGO_ENABLED=0, that the prebuilt Java runtime in
// java/ starts as a child process. The two talk over the process's pipes:
// one message per commit, per event and per step of text. Java keeps the
// text the engine prepares, measures it and lays it out with StaticLayout,
// and mounts one Android view per node; measuring is the only round trip.
//
// Go builds Android shared libraries only with cgo and the NDK; a second
// process keeps every build down to Go alone.
package android

import (
	"bufio"
	"encoding/binary"
	"io"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/engine"
)

// Message types, kept in sync with Protocol.java.
const (
	// Go to Java.
	msgBatch   = 1 // a MountBatch, applied on the UI thread
	msgPrepare = 2 // keeps text and its style under a new id
	msgMeasure = 3 // asks a prepared text's size at a width
	msgLayout  = 4 // lays a prepared text out under a new layout id
	msgRelease = 5 // frees a prepared text

	// Java to Go.
	msgStart    = 1
	msgResize   = 2
	msgEvent    = 3
	msgMeasured = 4
)

type surface struct {
	mu       sync.Mutex // guards out and buf
	out      *os.File
	buf      []byte
	ids      atomic.Uint32 // the last prepared text or layout id
	measured chan core.Size
	engine   *engine.Engine
}

// Run serves app to the Java runtime over standard input and output until
// the runtime closes them. Call it from main.
func Run(app engine.App) {
	// Keep the pipe to the runtime to ourselves and send the app's own
	// prints to standard error, which the runtime logs.
	fd, err := syscall.Dup(1)
	if err != nil {
		panic(err)
	}
	if err := syscall.Dup3(2, 1, 0); err != nil {
		panic(err)
	}
	s := &surface{out: os.NewFile(uintptr(fd), "nsgo-runtime"), measured: make(chan core.Size, 1)}
	in := bufio.NewReader(os.Stdin)
	for {
		kind, payload, err := readMessage(in)
		if err != nil {
			return // the runtime is gone
		}
		r := reader{payload}
		switch kind {
		case msgStart:
			size := core.Size{Width: r.points(), Height: r.points()}
			s.engine = engine.Start(s, app, engine.Options{Size: size})
		case msgResize:
			s.engine.Resize(core.Size{Width: r.points(), Height: r.points()})
		case msgEvent:
			s.engine.Dispatch(core.Event{
				Kind:  core.EventKind(r.byte()),
				ID:    core.NodeID(r.u32()),
				Count: r.u32(),
				Point: core.Point{X: r.points(), Y: r.points()},
				Text:  r.str(),
			})
		case msgMeasured:
			s.measured <- core.Size{Width: r.points(), Height: r.points()}
		}
	}
}

// Prepare implements core.TextEngine: Java keeps the text and its paint
// under a new id, so measuring and laying it out send only the id.
func (s *surface) Prepare(t *core.AttributedText) core.PreparedText {
	p := preparedText{s, s.ids.Add(1)}
	s.send(msgPrepare, func(w *writer) {
		w.u32(p.id)
		w.str(t.Text)
		writeStyle(w, &t.Style)
		w.u32(uint32(t.MaxLines))
	})
	return p
}

// preparedText is text Java keeps under id until Release.
type preparedText struct {
	s  *surface
	id uint32
}

// Measure asks the runtime's StaticLayout and waits for the answer. Only
// the engine goroutine measures, so one request is in flight at a time.
func (p preparedText) Measure(maxWidth core.Points) core.Size {
	p.s.send(msgMeasure, func(w *writer) {
		w.u32(p.id)
		w.points(maxWidth) // Auto, a NaN, does not wrap
	})
	return <-p.s.measured
}

// Layout has Java build the final StaticLayout under a new id for an
// ItemText to name. It needs no answer: Java reads it before the batch.
func (p preparedText) Layout(size core.Size) core.TextLayout {
	l := textLayout(p.s.ids.Add(1))
	p.s.send(msgLayout, func(w *writer) {
		w.u32(p.id)
		w.u32(uint32(l))
		w.points(size.Width) // the height is the one measured
	})
	return l
}

func (p preparedText) Release() {
	p.s.send(msgRelease, func(w *writer) { w.u32(p.id) })
}

// textLayout is the id of a StaticLayout Java built. The runtime drops
// it itself when a later ItemText replaces it or its view is deleted.
type textLayout uint32

func (textLayout) Release() {}

// Mount implements engine.Platform: one message per commit.
func (s *surface) Mount(b *core.MountBatch) {
	s.send(msgBatch, func(w *writer) { encode(w, b) })
}

// send writes one message whose payload body writes.
func (s *surface) send(kind byte, body func(w *writer)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w := writer{append(s.buf[:0], kind, 0, 0, 0, 0)}
	body(&w)
	binary.BigEndian.PutUint32(w.buf[1:], uint32(len(w.buf)-5))
	s.out.Write(w.buf)
	s.buf = w.buf
}

func readMessage(r *bufio.Reader) (byte, []byte, error) {
	var head [5]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}
	payload := make([]byte, binary.BigEndian.Uint32(head[1:]))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return head[0], payload, nil
}

// encode writes a batch in the format NsgoView.mount reads: each item is
// its kind, its ID and the fields of that kind.
func encode(w *writer, b *core.MountBatch) {
	w.u32(uint32(b.Root))
	w.u32(uint32(len(b.Items)))
	for i := range b.Items {
		it := &b.Items[i]
		w.byte(byte(it.Kind))
		w.u32(uint32(it.ID))
		switch it.Kind {
		case core.ItemCreate:
			w.byte(byte(it.View))
		case core.ItemInsert:
			w.u32(uint32(it.Parent))
			w.u32(uint32(it.Before))
		case core.ItemFrame:
			w.bounds(it.Frame)
		case core.ItemViewStyle:
			v := it.ViewStyle
			w.u32(uint32(v.Background))
			w.points(v.CornerRadius)
			w.points(v.BorderWidth)
			w.u32(uint32(v.BorderColor))
			w.float(v.Opacity)
			// Android draws shadows from elevation, which has no offset.
			w.u32(uint32(v.ShadowColor))
			w.points(v.ShadowRadius)
			w.flags(v.Clip, v.Hidden)
		case core.ItemText:
			t := it.Text
			layout, _ := t.Layout.(textLayout)
			w.str(t.Text)
			w.u32(uint32(layout))
			w.points(t.Content.Origin.X)
			w.points(t.Content.Origin.Y)
		case core.ItemInput:
			in := it.Input
			writeStyle(w, &in.Style)
			w.str(in.Placeholder)
			w.byte(byte(in.KeyboardType))
			w.flags(in.Masked, in.Readonly, in.Disabled)
			w.bounds(in.Content)
		case core.ItemInputValue:
			w.str(it.Value)
			w.u32(it.Count)
		case core.ItemEvents:
			w.u64(uint64(it.Events))
		case core.ItemViewProps:
			p := it.ViewProps
			w.byte(byte(p.Role))
			w.flags(p.Disabled, p.Selected, p.Checked)
			w.str(p.Label)
			w.str(p.TestID)
		case core.ItemScrollConfig:
			w.flags(it.Scroll.Horizontal, it.Scroll.HideIndicator)
		case core.ItemScrollContent:
			w.points(it.Size.Width)
			w.points(it.Size.Height)
		}
	}
}

// writeStyle writes a text style in the format Text.Style reads.
func writeStyle(w *writer, t *core.TextStyle) {
	w.str(t.Font.Family)
	w.points(t.Font.Size)
	w.u16(uint16(t.Font.Weight))
	w.u32(uint32(t.Color))
	w.points(t.LineHeight)
	w.points(t.LetterSpacing)
	w.byte(byte(t.Align))
	w.flags(t.Font.Italic,
		t.Decoration&core.TextDecorationUnderline != 0,
		t.Decoration&core.TextDecorationLineThrough != 0)
}

// writer and reader are the protocol's encoding: big-endian, lengths
// as uint32, points as float32, strings as a length and UTF-8 bytes and
// booleans as the bits of one byte, the first lowest.
type writer struct{ buf []byte }

func (w *writer) byte(b byte)          { w.buf = append(w.buf, b) }
func (w *writer) u16(v uint16)         { w.buf = binary.BigEndian.AppendUint16(w.buf, v) }
func (w *writer) u32(v uint32)         { w.buf = binary.BigEndian.AppendUint32(w.buf, v) }
func (w *writer) u64(v uint64)         { w.buf = binary.BigEndian.AppendUint64(w.buf, v) }
func (w *writer) float(f float32)      { w.u32(math.Float32bits(f)) }
func (w *writer) points(p core.Points) { w.float(float32(p)) }
func (w *writer) str(s string)         { w.u32(uint32(len(s))); w.buf = append(w.buf, s...) }

func (w *writer) bounds(b core.Bounds) {
	w.points(b.Origin.X)
	w.points(b.Origin.Y)
	w.points(b.Size.Width)
	w.points(b.Size.Height)
}

func (w *writer) flags(bits ...bool) {
	var b byte
	for i, on := range bits {
		if on {
			b |= 1 << i
		}
	}
	w.byte(b)
}

type reader struct{ buf []byte }

func (r *reader) byte() byte {
	if len(r.buf) < 1 {
		return 0
	}
	b := r.buf[0]
	r.buf = r.buf[1:]
	return b
}

func (r *reader) u32() uint32 {
	if len(r.buf) < 4 {
		return 0
	}
	v := binary.BigEndian.Uint32(r.buf)
	r.buf = r.buf[4:]
	return v
}

func (r *reader) points() core.Points { return core.Points(math.Float32frombits(r.u32())) }

func (r *reader) str() string {
	n := int(r.u32())
	if n > len(r.buf) {
		n = len(r.buf)
	}
	s := string(r.buf[:n])
	r.buf = r.buf[n:]
	return s
}

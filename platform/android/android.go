//go:build android

// Package android is NSGo's Android backend. The app is a plain Go
// executable, built with CGO_ENABLED=0, that the prebuilt Java runtime in
// java/ starts as a child process. The two talk over the process's pipes:
// one message per commit, per event and per text measurement. Java mounts
// one Android view per node and measures and draws text with StaticLayout.
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
	"syscall"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/engine"
)

// Message types, kept in sync with Protocol.java.
const (
	// Go to Java.
	msgBatch   = 1
	msgMeasure = 2

	// Java to Go.
	msgStart    = 1
	msgResize   = 2
	msgPress    = 3
	msgChange   = 4
	msgSubmit   = 5
	msgMeasured = 6
)

type surface struct {
	mu       sync.Mutex // guards out
	out      *bufio.Writer
	measured chan core.Size
	buf      []byte
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
	protocol := os.NewFile(uintptr(fd), "nsgo-runtime")
	s := &surface{out: bufio.NewWriter(protocol), measured: make(chan core.Size, 1)}
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
			s.engine = engine.Start(s, app, size)
		case msgResize:
			s.engine.Resize(core.Size{Width: r.points(), Height: r.points()})
		case msgPress:
			s.engine.Press(core.NodeID(r.u32()))
		case msgChange:
			id := core.NodeID(r.u32())
			s.engine.Change(id, r.str())
		case msgSubmit:
			id := core.NodeID(r.u32())
			s.engine.Submit(id, r.str())
		case msgMeasured:
			s.measured <- core.Size{Width: r.points(), Height: r.points()}
		}
	}
}

// Measure implements core.TextEngine: it asks the runtime's StaticLayout
// and waits for the answer. Only the engine goroutine measures, so one
// request is in flight at a time.
func (s *surface) Measure(text string, font core.Font, maxWidth core.Points) core.Size {
	w := writer{}
	w.points(font.Size)
	w.u32(uint32(font.Weight))
	if maxWidth.IsAuto() {
		w.points(-1)
	} else {
		w.points(maxWidth)
	}
	w.str(text)
	s.send(msgMeasure, w.buf)
	return <-s.measured
}

// Mount implements engine.Platform: one message per commit.
func (s *surface) Mount(b *core.MountBatch) {
	w := writer{buf: s.buf[:0]}
	encode(&w, b)
	s.buf = w.buf
	s.send(msgBatch, w.buf)
}

func (s *surface) send(kind byte, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var head [5]byte
	head[0] = kind
	binary.BigEndian.PutUint32(head[1:], uint32(len(payload)))
	s.out.Write(head[:])
	s.out.Write(payload)
	s.out.Flush()
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

// encode writes a batch in the format Mounter.java reads.
func encode(w *writer, b *core.MountBatch) {
	w.u32(uint32(b.Root))
	w.u32(uint32(len(b.Items)))
	for i := range b.Items {
		it := &b.Items[i]
		w.byte(byte(it.Kind))
		w.u32(uint32(it.ID))
		switch it.Kind {
		case core.ItemCreate:
			w.byte(byte(it.Tag))
		case core.ItemProps:
			p := &it.Props
			w.u32(uint32(p.Background))
			w.points(p.CornerRadius)
			w.flag(p.Pressable)
			w.byte(byte(p.Role))
			w.str(p.Text)
			w.points(p.Font.Size)
			w.u32(uint32(p.Font.Weight))
			w.u32(uint32(p.Color))
			w.points(p.Padding.Top)
			w.points(p.Padding.Right)
			w.points(p.Padding.Bottom)
			w.points(p.Padding.Left)
			w.str(p.Placeholder)
		case core.ItemInsert:
			w.u32(uint32(it.Parent))
			w.u32(uint32(it.Index))
		case core.ItemFrame:
			w.points(it.Frame.Origin.X)
			w.points(it.Frame.Origin.Y)
			w.points(it.Frame.Size.Width)
			w.points(it.Frame.Size.Height)
		}
	}
}

// writer and reader are the protocol's encoding: big-endian, lengths
// as uint32, points as float32, strings as a length and UTF-8 bytes.
type writer struct{ buf []byte }

func (w *writer) byte(b byte)          { w.buf = append(w.buf, b) }
func (w *writer) u32(v uint32)         { w.buf = binary.BigEndian.AppendUint32(w.buf, v) }
func (w *writer) points(p core.Points) { w.u32(math.Float32bits(float32(p))) }
func (w *writer) str(s string)         { w.u32(uint32(len(s))); w.buf = append(w.buf, s...) }
func (w *writer) flag(v bool) {
	if v {
		w.byte(1)
	} else {
		w.byte(0)
	}
}

type reader struct{ buf []byte }

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

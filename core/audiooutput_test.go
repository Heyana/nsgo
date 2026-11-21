package core

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeAudioOutputText struct {
	ApproxTextEngine
	opened []*fakeAudioOutputBackend
}

func (f *fakeAudioOutputText) OpenAudioOutput(conf driver.AudioOutputConfig) (driver.AudioOutput, error) {
	b := &fakeAudioOutputBackend{conf: conf}
	f.opened = append(f.opened, b)
	return b, nil
}

type fakeAudioOutputBackend struct {
	conf    driver.AudioOutputConfig
	started atomic.Bool
	closed  atomic.Bool
}

func (b *fakeAudioOutputBackend) Start() error                 { b.started.Store(true); return nil }
func (b *fakeAudioOutputBackend) Pause()                       { b.started.Store(false) }
func (b *fakeAudioOutputBackend) Close()                       { b.closed.Store(true); b.started.Store(false) }
func (b *fakeAudioOutputBackend) Flush() int                   { return 12 }
func (b *fakeAudioOutputBackend) Position() (int64, time.Time) { return 48, time.Unix(10, 0) }
func (b *fakeAudioOutputBackend) Latency() time.Duration       { return 10 * time.Millisecond }
func (b *fakeAudioOutputBackend) Underruns() int               { return 2 }

func TestNewAudioOutputLifecycleAndInterrupt(t *testing.T) {
	h := &host{t: t, clock: NewManualClock(time.Unix(0, 0))}
	h.root = testRoot(RootOptions{
		Post: func(fn func()) {
			h.mu.Lock()
			h.posts = append(h.posts, fn)
			h.mu.Unlock()
		},
		Clock:  h.clock,
		Report: func(err error) { h.errs = append(h.errs, err) },
	})
	factory := &fakeAudioOutputText{}
	h.tree = NewTree(h.root, factory)
	h.tree.Resize(Size{Width: 100, Height: 100}, Geometry{})

	var out *AudioOutput
	var openErr error
	var gotInterrupts []error
	// Note: Env.CanPlayAudio is intentionally left false to verify pull PCM depends on AudioOutputFactory rather than Sound/Player availability.
	h.tree.Mount(func(cx *Scope) Node {
		out, openErr = cx.NewAudioOutput(AudioOutputConfig{
			SampleRate: 48000,
			Channels:   2,
			Render:     func([]float32) int { return 0 },
			OnInterrupt: func(err error) {
				gotInterrupts = append(gotInterrupts, err)
			},
		})
		return View{}
	})
	if openErr != nil || out == nil || len(factory.opened) != 1 {
		t.Fatalf("open: out=%v err=%v opened=%d", out, openErr, len(factory.opened))
	}
	b := factory.opened[0]

	// Interrupt is posted onto the engine goroutine.
	wantErr := errors.New("route changed")
	b.conf.Interrupt(wantErr)
	if len(gotInterrupts) != 0 {
		t.Fatal("OnInterrupt ran synchronously instead of posting to engine")
	}
	h.drain()
	if len(gotInterrupts) != 1 || !errors.Is(gotInterrupts[0], wantErr) {
		t.Fatalf("gotInterrupts = %v, want [%v]", gotInterrupts, wantErr)
	}

	// Disposing the owning scope automatically closes the output and suppresses late interrupts.
	h.root.Dispose()
	if !b.closed.Load() {
		t.Fatal("disposing owning scope did not close AudioOutput")
	}
	b.conf.Interrupt(errors.New("late"))
	h.drain()
	if len(gotInterrupts) != 1 {
		t.Fatalf("late interrupt delivered after Close: %v", gotInterrupts)
	}
	if err := out.Start(); err == nil {
		t.Fatal("Start succeeded after scope disposal closed the output")
	}
}

type fakeAudioOutputEngine struct {
	ApproxTextEngine
	opened *fakeAudioOutput
	err    error
}

func (f *fakeAudioOutputEngine) OpenAudioOutput(c driver.AudioOutputConfig) (driver.AudioOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.opened = &fakeAudioOutput{conf: c}
	return f.opened, nil
}

type fakeAudioOutput struct {
	conf      driver.AudioOutputConfig
	started   int
	paused    int
	closed    int
	flushed   int
	frames    int64
	at        time.Time
	latency   time.Duration
	underruns int
}

func (o *fakeAudioOutput) Start() error                 { o.started++; return nil }
func (o *fakeAudioOutput) Pause()                       { o.paused++ }
func (o *fakeAudioOutput) Close()                       { o.closed++ }
func (o *fakeAudioOutput) Flush() int                   { o.flushed++; return 128 }
func (o *fakeAudioOutput) Position() (int64, time.Time) { return o.frames, o.at }
func (o *fakeAudioOutput) Latency() time.Duration       { return o.latency }
func (o *fakeAudioOutput) Underruns() int               { return o.underruns }

func TestAudioOutputFactoryOpensWithoutCanPlayAudio(t *testing.T) {
	eng := &fakeAudioOutputEngine{}
	var posts []func()
	root := testRoot(RootOptions{
		Post: func(fn func()) { posts = append(posts, fn) },
	})
	tree := NewTree(root, eng)
	tree.Resize(Size{Width: 100, Height: 100}, Geometry{})
	// Leave Environment.Audio false (like Web before ItemAudio lands) and verify
	// driver.AudioOutputFactory still opens.
	var out *AudioOutput
	var openErr error
	var interrupted []error
	tree.Mount(func(cx *Scope) Node {
		out, openErr = cx.NewAudioOutput(AudioOutputConfig{
			SampleRate: 48000,
			Channels:   2,
			Latency:    10 * time.Millisecond,
			Render:     func(buf []float32) int { return len(buf) / 2 },
			OnInterrupt: func(err error) {
				interrupted = append(interrupted, err)
			},
		})
		return View{}
	})
	if openErr != nil || out == nil || eng.opened == nil {
		t.Fatalf("NewAudioOutput = (%v, %v), opened=%v", out, openErr, eng.opened)
	}
	if err := out.Start(); err != nil || eng.opened.started != 1 {
		t.Fatalf("Start = %v, started=%d", err, eng.opened.started)
	}
	eng.opened.frames = 1024
	eng.opened.at = time.Unix(100, 0)
	eng.opened.latency = 15 * time.Millisecond
	eng.opened.underruns = 2
	if f, at := out.Position(); f != 1024 || !at.Equal(time.Unix(100, 0)) {
		t.Fatalf("Position = (%d, %v)", f, at)
	}
	if out.Latency() != 15*time.Millisecond || out.Underruns() != 2 || out.Flush() != 128 {
		t.Fatalf("Latency=%v Underruns=%d Flush", out.Latency(), out.Underruns())
	}
	// Interrupt runs via owner.Post and is delivered before Close.
	intrErr := errors.New("interrupted")
	eng.opened.conf.Interrupt(intrErr)
	for _, fn := range posts {
		fn()
	}
	posts = nil
	if len(interrupted) != 1 || !errors.Is(interrupted[0], intrErr) {
		t.Fatalf("interrupted = %v, want [%v]", interrupted, intrErr)
	}

	// Scope disposal closes the output, and interrupts after Close are suppressed.
	out.Pause()
	if eng.opened.paused != 1 {
		t.Fatalf("paused = %d, want 1", eng.opened.paused)
	}
	root.Dispose()
	if eng.opened.closed != 1 {
		t.Fatalf("dispose closed = %d, want 1", eng.opened.closed)
	}
	if err := out.Start(); err == nil {
		t.Fatal("Start after Close succeeded")
	}
	eng.opened.conf.Interrupt(errors.New("late"))
	for _, fn := range posts {
		fn()
	}
	if len(interrupted) != 1 {
		t.Fatalf("interrupt delivered after Close: %v", interrupted)
	}
}

func TestAudioOutputValidation(t *testing.T) {
	eng := &fakeAudioOutputEngine{}
	root := testRoot(RootOptions{})
	tree := NewTree(root, eng)
	tree.Resize(Size{Width: 100, Height: 100}, Geometry{})
	noop := func(buf []float32) int { return 0 }
	cases := []AudioOutputConfig{
		{SampleRate: 0, Channels: 2, Render: noop},
		{SampleRate: 48000, Channels: 0, Render: noop},
		{SampleRate: 48000, Channels: 3, Render: noop},
		{SampleRate: 48000, Channels: 2, Render: nil},
		{SampleRate: 48000, Channels: 2, Latency: -time.Millisecond, Render: noop},
	}
	tree.Mount(func(cx *Scope) Node {
		for i, c := range cases {
			if _, err := cx.NewAudioOutput(c); !errors.Is(err, AudioNotSupported) {
				t.Errorf("case %d: got %v, want AudioNotSupported", i, err)
			}
		}
		return View{}
	})
}

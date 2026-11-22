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

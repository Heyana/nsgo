//go:build android && nsgotest

package android

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

type imeInput struct {
	surface                        *testSurface
	bindings, automatic, submits   atomic.Int32
	blurs, keyPresses, textChanges atomic.Int32
}

// openIMEInput waits for the native field's focus before sending input through the same InputConnection an IME uses; Activity.dispatchKeyEvent bypasses that path.
func openIMEInput(t *testing.T, bind bool, submit core.SubmitBehavior, multiline bool) *imeInput {
	t.Helper()
	i := &imeInput{}
	focused := make(chan struct{}, 1)
	s, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		if bind {
			cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, func() { i.bindings.Add(1) })
		}
		// A newer automatic binding must leave the field its Return, so the older KeyAlways binding can take it.
		cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyNotEditing}, func() { i.automatic.Add(1) })
		cx.OnKey("enter", func() { i.automatic.Add(1) })
		return core.Input{AutoFocus: true, Multiline: multiline, SubmitBehavior: submit,
			Style: core.Style{Width: core.Pt(240), Height: core.Pt(100)},
			OnFocus: func() {
				select {
				case focused <- struct{}{}:
				default:
				}
			},
			OnBlur:     func() { i.blurs.Add(1) },
			OnSubmit:   func(string) { i.submits.Add(1) },
			OnKeyPress: func(string) { i.keyPresses.Add(1) },
			OnChange:   func(string) { i.textChanges.Add(1) },
		}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	i.surface = s.(*testSurface)
	select {
	case <-focused:
	case <-time.After(5 * time.Second):
		t.Fatal("the native input did not take the focus")
	}
	i.surface.Sync()
	return i
}

func TestIMEActionRunsEnterBinding(t *testing.T) {
	i := openIMEInput(t, true, core.SubmitBehaviorDefault, false)
	for n := range 2 {
		if err := i.surface.do(map[string]any{"op": "editor-action"}); err != nil {
			t.Fatal(err)
		}
		i.surface.Sync()
		if i.bindings.Load() != int32(n+1) || i.automatic.Load() != 0 || i.submits.Load() != 0 || i.blurs.Load() != 0 || i.keyPresses.Load() != 0 {
			t.Fatalf("IME action: bindings=%d automatic=%d submits=%d blurs=%d key presses=%d", i.bindings.Load(), i.automatic.Load(), i.submits.Load(), i.blurs.Load(), i.keyPresses.Load())
		}
	}
}

func TestIMEActionWithoutBindingSubmits(t *testing.T) {
	i := openIMEInput(t, false, core.SubmitBehaviorDefault, false)
	if err := i.surface.do(map[string]any{"op": "editor-action"}); err != nil {
		t.Fatal(err)
	}
	i.surface.Sync()
	if i.automatic.Load() != 0 || i.submits.Load() != 1 || i.blurs.Load() != 1 || i.keyPresses.Load() != 1 {
		t.Fatalf("IME action: automatic=%d submits=%d blurs=%d key presses=%d", i.automatic.Load(), i.submits.Load(), i.blurs.Load(), i.keyPresses.Load())
	}
}

func TestIMEActionSubmitKeepsFocus(t *testing.T) {
	i := openIMEInput(t, false, core.SubmitBehaviorSubmit, false)
	for range 2 {
		if err := i.surface.do(map[string]any{"op": "editor-action"}); err != nil {
			t.Fatal(err)
		}
		i.surface.Sync()
	}
	if i.automatic.Load() != 0 || i.submits.Load() != 2 || i.blurs.Load() != 0 || i.keyPresses.Load() != 2 {
		t.Fatalf("IME action: automatic=%d submits=%d blurs=%d key presses=%d", i.automatic.Load(), i.submits.Load(), i.blurs.Load(), i.keyPresses.Load())
	}
}

func TestIMEReturnCommitRunsBinding(t *testing.T) {
	i := openIMEInput(t, true, core.SubmitBehaviorDefault, true)
	for range 2 {
		if err := i.surface.Commit("\n"); err != nil {
			t.Fatal(err)
		}
		i.surface.Sync()
	}
	if i.bindings.Load() != 2 || i.automatic.Load() != 0 || i.submits.Load() != 0 || i.blurs.Load() != 0 || i.keyPresses.Load() != 0 || i.textChanges.Load() != 0 {
		t.Fatalf("IME Return: bindings=%d automatic=%d submits=%d blurs=%d key presses=%d text changes=%d", i.bindings.Load(), i.automatic.Load(), i.submits.Load(), i.blurs.Load(), i.keyPresses.Load(), i.textChanges.Load())
	}
}

func TestIMEReturnCommitWithoutBindingInsertsNewline(t *testing.T) {
	i := openIMEInput(t, false, core.SubmitBehaviorDefault, true)
	if err := i.surface.Commit("\n"); err != nil {
		t.Fatal(err)
	}
	i.surface.Sync()
	if i.automatic.Load() != 0 || i.submits.Load() != 0 || i.blurs.Load() != 0 || i.keyPresses.Load() != 1 || i.textChanges.Load() != 1 {
		t.Fatalf("IME Return: automatic=%d submits=%d blurs=%d key presses=%d text changes=%d", i.automatic.Load(), i.submits.Load(), i.blurs.Load(), i.keyPresses.Load(), i.textChanges.Load())
	}
}

func TestIMEReturnCommitWithoutBindingSubmits(t *testing.T) {
	i := openIMEInput(t, false, core.SubmitBehaviorSubmit, true)
	if err := i.surface.Commit("\n"); err != nil {
		t.Fatal(err)
	}
	i.surface.Sync()
	if i.automatic.Load() != 0 || i.submits.Load() != 1 || i.blurs.Load() != 0 || i.keyPresses.Load() != 1 || i.textChanges.Load() != 0 {
		t.Fatalf("IME Return: automatic=%d submits=%d blurs=%d key presses=%d text changes=%d", i.automatic.Load(), i.submits.Load(), i.blurs.Load(), i.keyPresses.Load(), i.textChanges.Load())
	}
}

func TestIMEHardwareEnterRunsBinding(t *testing.T) {
	i := openIMEInput(t, true, core.SubmitBehaviorDefault, true)
	for range 2 {
		if err := i.surface.do(map[string]any{"op": "editor-action", "hardware": true}); err != nil {
			t.Fatal(err)
		}
		i.surface.Sync()
	}
	if i.bindings.Load() != 2 || i.automatic.Load() != 0 || i.submits.Load() != 0 || i.blurs.Load() != 0 || i.keyPresses.Load() != 0 || i.textChanges.Load() != 0 {
		t.Fatalf("IME hardware Enter: bindings=%d automatic=%d submits=%d blurs=%d key presses=%d text changes=%d", i.bindings.Load(), i.automatic.Load(), i.submits.Load(), i.blurs.Load(), i.keyPresses.Load(), i.textChanges.Load())
	}
}

func TestIMEHardwareShiftEnterInsertsNewline(t *testing.T) {
	i := openIMEInput(t, true, core.SubmitBehaviorSubmit, true)
	if err := i.surface.do(map[string]any{"op": "editor-action", "hardware": true, "shift": true}); err != nil {
		t.Fatal(err)
	}
	i.surface.Sync()
	if i.bindings.Load() != 0 || i.automatic.Load() != 0 || i.submits.Load() != 0 || i.blurs.Load() != 0 || i.textChanges.Load() != 1 {
		t.Fatalf("IME Shift+Enter: bindings=%d automatic=%d submits=%d blurs=%d text changes=%d", i.bindings.Load(), i.automatic.Load(), i.submits.Load(), i.blurs.Load(), i.textChanges.Load())
	}
}

func TestHardwareEnterRunsOneBinding(t *testing.T) {
	i := openIMEInput(t, true, core.SubmitBehaviorDefault, false)
	for range 2 {
		if err := i.surface.Key("enter"); err != nil {
			t.Fatal(err)
		}
		i.surface.Sync()
	}
	if i.bindings.Load() != 2 || i.automatic.Load() != 0 || i.submits.Load() != 0 || i.blurs.Load() != 0 || i.keyPresses.Load() != 0 {
		t.Fatalf("hardware Enter: bindings=%d automatic=%d submits=%d blurs=%d key presses=%d", i.bindings.Load(), i.automatic.Load(), i.submits.Load(), i.blurs.Load(), i.keyPresses.Load())
	}
}

func TestHardwareEnterWithoutBindingSubmits(t *testing.T) {
	i := openIMEInput(t, false, core.SubmitBehaviorSubmit, false)
	for range 2 {
		if err := i.surface.Key("enter"); err != nil {
			t.Fatal(err)
		}
		i.surface.Sync()
	}
	if i.automatic.Load() != 0 || i.submits.Load() != 2 || i.blurs.Load() != 0 || i.keyPresses.Load() != 2 {
		t.Fatalf("hardware Enter: automatic=%d submits=%d blurs=%d key presses=%d", i.automatic.Load(), i.submits.Load(), i.blurs.Load(), i.keyPresses.Load())
	}
}

// TestCompositionEndsBeforeBlur checks that blurring a composing InputField
// reports OnCompositionChange{Composing:false} before OnBlur and keeps the
// composed text (#1563).
func TestCompositionEndsBeforeBlur(t *testing.T) {
	var mu sync.Mutex
	var events []string
	var text string
	focused := make(chan struct{}, 1)
	blurred := make(chan struct{}, 1)
	var root *core.Scope
	ref := &core.Ref{}
	record := func(ev string) {
		mu.Lock()
		events = append(events, ev)
		mu.Unlock()
	}
	s, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		root = cx
		return core.Input{
			Ref:       ref,
			AutoFocus: true,
			Style:     core.Style{Width: core.Pt(240), Height: core.Pt(60)},
			OnFocus: func() {
				record("focus")
				select {
				case focused <- struct{}{}:
				default:
				}
			},
			OnChange: func(v string) {
				mu.Lock()
				text = v
				events = append(events, "change:"+v)
				mu.Unlock()
			},
			OnCompositionChange: func(c core.Composition) {
				if c.Composing {
					record("comp:" + c.Text)
				} else {
					record("end")
				}
			},
			OnBlur: func() {
				record("blur")
				select {
				case blurred <- struct{}{}:
				default:
				}
			},
		}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ts := s.(*testSurface)
	select {
	case <-focused:
	case <-time.After(5 * time.Second):
		t.Fatal("the native input did not take the focus")
	}
	ts.Sync()
	if err := ts.Compose("ni"); err != nil {
		t.Fatal(err)
	}
	ts.Sync()
	root.Post(ref.Blur)
	select {
	case <-blurred:
	case <-time.After(5 * time.Second):
		t.Fatal("the native input did not blur")
	}
	ts.Sync()
	mu.Lock()
	gotEvents := slices.Clone(events)
	gotText := text
	mu.Unlock()
	endIdx := slices.Index(gotEvents, "end")
	blurIdx := slices.Index(gotEvents, "blur")
	if gotText != "ni" || endIdx < 0 || blurIdx < 0 || endIdx > blurIdx {
		t.Fatalf("text=%q events=%v, want text \"ni\" and \"end\" before \"blur\"", gotText, gotEvents)
	}
}

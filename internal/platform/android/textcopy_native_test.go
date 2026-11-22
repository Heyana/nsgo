//go:build android && nsgotest

package android

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/formula"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestFormulaSelectionCopiesOnAndroid(t *testing.T) {
	selected := make(chan string, 32)
	ready := make(chan *core.Scope, 1)
	var ref core.Ref
	var copyText *core.Signal[string]
	window, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		ready <- cx
		copyText = core.NewSignal(cx, "$x_i^2$")
		attachment := &formula.View{Source: "x_i^2"}
		return core.View{TestID: "root", Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Padding: core.All(40), FontSize: 32}, Children: []core.Node{
			core.View{Selectable: true, OnTextSelection: func(s core.TextSelection) { selected <- s.Text }, Children: []core.Node{
				core.Text{TestID: "formula", Ref: &ref, SpansFunc: func() []core.Span {
					return []core.Span{{Content: copyText.Get(), Attachment: attachment}}
				}},
			}},
		}}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := window.(*testSurface)
	defer s.s.engine.Close()
	cx := <-ready
	s.Sync()
	deadline := time.Now().Add(5 * time.Second)
	for {
		metrics, err := s.ask(map[string]any{"op": "view-metrics", "testID": "root"})
		var views []struct{ Width, Height float64 }
		if err == nil && json.Unmarshal([]byte(metrics), &views) == nil && len(views) == 1 && views[0].Width > 0 && views[0].Height > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("native root did not lay out: %s %v", metrics, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	var at driver.Point
	position := make(chan driver.Point, 1)
	cx.Post(func() {
		b, _ := ref.Bounds()
		position <- driver.Point{X: b.Origin.X + 8, Y: b.Origin.Y + b.Size.Height/2}
	})
	at = <-position
	if err := s.Press(at, 800*time.Millisecond, driver.Modifiers{}); err != nil {
		t.Fatal(err)
	}
	waitSelection := func(want string) {
		t.Helper()
		timeout := time.NewTimer(5 * time.Second)
		defer timeout.Stop()
		var reports []string
		for {
			select {
			case got := <-selected:
				reports = append(reports, got)
				if got == want {
					return
				}
			case <-timeout.C:
				metrics, _ := s.ask(map[string]any{"op": "view-metrics", "testID": "formula"})
				t.Fatalf("native selection did not report %q; reports=%q metrics=%s", want, reports, metrics)
			}
		}
	}
	checkCopy := func(want string) {
		t.Helper()
		if err := s.do(map[string]any{"op": "copy-selection"}); err != nil {
			t.Fatal(err)
		}
		clipboard := make(chan string, 1)
		cx.Post(func() { cx.ReadClipboard(func(text string) { clipboard <- text }) })
		select {
		case got := <-clipboard:
			if got != want {
				t.Fatalf("Android clipboard=%q want=%q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("clipboard read did not answer")
		}
	}
	waitSelection("$x_i^2$")
	checkCopy("$x_i^2$")
	// Changing only the attachment's copy text must not retain old metadata.
	cx.Post(func() { copyText.Set("$y^3$") })
	s.Sync()
	waitSelection("$y^3$")
	checkCopy("$y^3$")
}

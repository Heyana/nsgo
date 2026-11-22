//go:build android && nsgotest

package android

import (
	"encoding/json"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// Native font and placement checks exercise Toolbar/TextView and real input; the core and JVM tests cover the protocol and collapse math separately.
func TestNativeAndroidHeaderTitleStyles(t *testing.T) {
	var cx *core.Scope
	var mode *core.Signal[int]
	var scroll core.Ref
	var presses atomic.Int32
	live, err := testhost.Current().Open(func(scope *core.Scope) core.Node {
		cx = scope
		mode = core.NewSignal(cx, 0)
		return core.Stack{Style: core.Style{FlexGrow: 1}, Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
			m := mode.Get()
			page := core.Screen{Title: "Inbox", TestID: "page", HeaderLargeTitle: true,
				HeaderTitleStyle:      core.Style{FontSize: 20, FontWeight: core.FontWeightMedium, FontFamily: "serif"},
				HeaderLargeTitleStyle: core.Style{FontSize: 32, FontWeight: core.FontWeightSemibold, FontFamily: "monospace"},
				Android:               core.AndroidScreenProps{HeaderRightWithLargeTitle: true},
				HeaderRight: core.View{TestID: "action", Style: core.Style{Width: core.Pt(44), Height: core.Pt(44)},
					OnPress: func(core.PressEvent) { presses.Add(1) }},
				Children: []core.Node{core.Scroll{Ref: &scroll, Style: core.Style{FlexGrow: 1}, Children: []core.Node{
					core.View{Style: core.Style{Height: core.Pt(1500)}},
				}}}}
			if m >= 1 {
				page.HeaderTitle = core.View{TestID: "compact", Style: core.Style{Width: core.Pt(120), Height: core.Pt(36)},
					Children: []core.Node{core.Text{Content: "Search", Style: core.Style{FontSize: 18}}}}
			}
			if m == 4 {
				page.HeaderLargeTitle = false
				page.HeaderTitle = nil
			}
			if m == 2 {
				page.HeaderTitleStyle = core.Style{}
				page.HeaderLargeTitleStyle = core.Style{FontSize: 80, FontWeight: core.FontWeightBold}
			}
			return page
		})}}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	live.Sync()
	target := live.(*testSurface)
	type metric struct {
		X, Y, Width, Height, Alpha, PaddingTop, PaddingBottom, Size, VisibleHeight float64
		Direction, RootDirection                                                   int
		RTLSupported                                                               bool
		Weight                                                                     int
		Visible, FamilyMatches                                                     bool
	}
	metrics := func(query map[string]any) []metric {
		t.Helper()
		query["op"] = "view-metrics"
		data, err := target.ask(query)
		if err != nil {
			t.Fatal(err)
		}
		var out []metric
		if err := json.Unmarshal([]byte(data), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	wait := func() { live.Sync(); time.Sleep(150 * time.Millisecond) }
	center := func(v metric) float64 { return v.Y + v.PaddingTop + (v.Height-v.PaddingTop-v.PaddingBottom)/2 }
	action := func() metric {
		t.Helper()
		m := metrics(map[string]any{"testID": "action"})
		if len(m) != 1 {
			t.Fatalf("action metrics: %+v", m)
		}
		return m[0]
	}
	for m := range 4 {
		cx.Post(func() {
			if m == 3 {
				cx.SetLocale("ar")
			} else {
				cx.SetLocale("en")
			}
			mode.Set(m)
		})
		wait()
		var compact, large *metric
		for _, v := range metrics(map[string]any{"text": "Inbox"}) {
			v := v
			if v.Size > 25 {
				large = &v
			} else {
				compact = &v
			}
		}
		if large == nil || !large.Visible || large.Alpha != 1 || large.Weight != []int{600, 600, 700, 600}[m] {
			t.Fatalf("mode %d expanded title: %+v", m, large)
		}
		if m == 0 {
			if compact == nil || compact.Size != 20 || compact.Weight != 500 {
				t.Fatalf("compact font: %+v", compact)
			}
			fonts := metrics(map[string]any{"text": "Inbox", "family": "serif"})
			if len(fonts) != 2 || !fonts[0].FamilyMatches {
				t.Fatalf("compact family: %+v", fonts)
			}
		} else {
			custom := metrics(map[string]any{"testID": "compact"})
			if len(custom) != 1 || custom[0].Visible || custom[0].Alpha != 0 {
				t.Fatalf("custom compact title showed when expanded: %+v", custom)
			}
		}
		if m != 2 {
			matched := false
			for _, v := range metrics(map[string]any{"text": "Inbox", "family": "monospace"}) {
				matched = matched || v.Size > 25 && v.FamilyMatches
			}
			if !matched {
				t.Fatal("large title did not use its requested family")
			}
		}
		if large.VisibleHeight < large.Height-1 {
			t.Fatalf("mode %d large font was clipped: %+v", m, large)
		}
		a := action()
		page := metrics(map[string]any{"testID": "page"})
		if len(page) != 1 || (m == 3) != (a.X < page[0].Width/2) {
			t.Fatalf("mode %d trailing placement: action %+v, page %+v", m, a, page)
		}
		if math.Abs(center(a)-center(*large)) > 1 {
			t.Fatalf("mode %d action center %.2f, large title %.2f", m, center(a), center(*large))
		}
		if err := target.do(map[string]any{"op": "press", "x": a.X + a.Width/2, "y": center(a)}); err != nil {
			t.Fatal(err)
		}
		wait()
		if presses.Load() != int32(2*m+1) {
			t.Fatal("expanded action did not take its press")
		}
		cx.Post(func() { scroll.ScrollTo(core.Point{Y: 500}, false) })
		wait()
		a = action()
		if a.Y > large.Y || !a.Visible {
			t.Fatalf("collapsed action did not enter the toolbar: %+v", a)
		}
		if m > 0 {
			custom := metrics(map[string]any{"testID": "compact"})
			if len(custom) != 1 || !custom[0].Visible || custom[0].Alpha != 1 {
				t.Fatalf("custom compact title did not return: %+v", custom)
			}
		}
		if err := target.do(map[string]any{"op": "press", "x": a.X + a.Width/2, "y": center(a)}); err != nil {
			t.Fatal(err)
		}
		wait()
		if presses.Load() != int32(2*m+2) {
			t.Fatal("collapsed action did not take its press")
		}
		cx.Post(func() { scroll.ScrollTo(core.Point{Y: -10000}, false) })
		wait()
		a = action()
		if math.Abs(center(a)-center(*large)) > 1 {
			t.Fatalf("action did not return beside the expanded title: action %+v, initial title %+v", a, large)
		}
	}
	// The placement option leaves a regular header's action in its toolbar.
	cx.Post(func() { cx.SetLocale("en"); mode.Set(4) })
	wait()
	var title *metric
	for _, v := range metrics(map[string]any{"text": "Inbox"}) {
		if v.Visible {
			title = &v
		}
	}
	a := action()
	if title == nil || title.Size != 20 || math.Abs(center(a)-center(*title)) > 3 {
		t.Fatalf("regular title/action placement: title %+v, action %+v", title, a)
	}
}

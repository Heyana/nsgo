package miniprogram

import (
	"encoding/json"
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// fakeHost records what the mounter hands the mini program.
type fakeHost struct {
	batches     [][]any
	events      []driver.Event
	services    []driver.ItemKind
	ticks       int
	pixelImages bool
	pixelFrames []*image.RGBA
	pixelTints  []driver.Color
	clipboard   []string
	maskURL     string
	paints      []fakePaint
}

func (h *fakeHost) submit(ops []byte) {
	var batch []any
	if err := json.Unmarshal(ops, &batch); err != nil {
		panic(string(ops) + ": " + err.Error())
	}
	h.batches = append(h.batches, batch)
}
func (h *fakeHost) dispatch(ev driver.Event)     { h.events = append(h.events, ev) }
func (h *fakeHost) service(it *driver.MountItem) { h.services = append(h.services, it.Kind) }
func (h *fakeHost) imageURL(src driver.ImageSource) string {
	if p, ok := strings.CutPrefix(src.URI, "asset:"); ok {
		return "/resources/" + strings.TrimPrefix(p, "/")
	}
	return "img"
}
func (h *fakeHost) imageMaskURL(src driver.ImageSource) string {
	if h.maskURL != "" {
		return h.maskURL
	}
	return h.imageURL(src)
}
func (h *fakeHost) supportsPixelImages() bool { return h.pixelImages }
func (h *fakeHost) imagePixels(_ driver.NodeID, pixels *image.RGBA, tint driver.Color) {
	h.pixelFrames = append(h.pixelFrames, pixels)
	h.pixelTints = append(h.pixelTints, tint)
}
func (h *fakeHost) setClipboard(text string) { h.clipboard = append(h.clipboard, text) }

func (h *fakeHost) paint(_ driver.NodeID, _ *driver.Drawing, _ []driver.DrawOp, _ driver.Size, scale float64, smooth bool) string {
	h.paints = append(h.paints, fakePaint{scale, smooth})
	return "data:image/png;base64,"
}

// fakePaint is the resolution a canvas was painted at.
type fakePaint struct {
	scale  float64
	smooth bool
}

func (h *fakeHost) tick() { h.ticks++ }

// last returns the operations of the last batch with code, as JSON.
func (h *fakeHost) ops(code string) []string {
	var out []string
	for _, b := range h.batches {
		for _, op := range b {
			if o := op.([]any); o[0] == code {
				data, _ := json.Marshal(o[1:])
				out = append(out, string(data))
			}
		}
	}
	return out
}

func frame(id driver.NodeID, x, y, w, h driver.Points) driver.MountItem {
	return driver.MountItem{Kind: driver.ItemFrame, ID: id, Frame: driver.Bounds{Origin: driver.Point{X: x, Y: y}, Size: driver.Size{Width: w, Height: h}}}
}

// mounted is a root view 1 holding view 2 and text 3.
func mounted() (*mounter, *fakeHost) {
	h := &fakeHost{}
	m := newMounter(h)
	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 3, View: driver.KindText},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemInsert, ID: 3, Parent: 2},
		frame(1, 0, 0, 375, 600),
		frame(2, 10, 20, 200, 100),
		frame(3, 5, 6, 100, 20),
	}})
	return m, h
}

func TestBatchesBecomeOperations(t *testing.T) {
	_, h := mounted()
	if len(h.batches) != 1 {
		t.Fatalf("batches = %d", len(h.batches))
	}
	got := strings.Join(h.ops("c"), " ") + " " + strings.Join(h.ops("root"), " ") + " " + strings.Join(h.ops("i"), " ")
	if want := `[1,"view"] [2,"view"] [3,"text"] [1] [1,2,null] [2,3,null]`; got != want {
		t.Fatalf("structure = %s, want %s", got, want)
	}
	if f := h.ops("f"); f[len(f)-1] != "[3,5,6,100,20]" {
		t.Fatalf("frames = %v", f)
	}
}

func TestChildrenArePlacedInsideTheBorder(t *testing.T) {
	m, h := mounted()
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 2, ViewStyle: &driver.ViewStyle{Opacity: 1, BorderWidth: 2, BorderColor: driver.RGB(0),
			CornerRadius: 4, Background: driver.RGB(0xffffff)}},
	}})
	f := h.ops("f")
	if f[len(f)-1] != "[3,3,4,100,20]" {
		t.Fatalf("child frame = %v", f)
	}
	s := h.ops("s")
	if !strings.Contains(s[len(s)-1], "border-width:2px") || !strings.Contains(s[len(s)-1], "border-radius:4px") {
		t.Fatalf("style = %v", s)
	}
}

func TestPressesReachTheViewsThatAskForThem(t *testing.T) {
	m, h := mounted()
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemEvents, ID: 2, Events: 1<<driver.EventPress | 1<<driver.EventPressDown | 1<<driver.EventPressUp},
	}})
	if e := h.ops("e"); e[len(e)-1] != "[2,3]" {
		t.Fatalf("events = %v", e)
	}
	m.press(2, driver.EventPress, driver.Point{X: 30, Y: 50})
	m.press(3, driver.EventPress, driver.Point{X: 30, Y: 50})
	if len(h.events) != 1 || h.events[0].ID != 2 || h.events[0].Point != (driver.Point{X: 20, Y: 30}) {
		t.Fatalf("events = %+v", h.events)
	}
}

func TestTextRunsAndLinks(t *testing.T) {
	m, h := mounted()
	e := NewTextEngine(&approx{})
	at := plain("go to docs")
	at.Spans = []driver.TextSpan{{Start: 6, End: 10, Style: at.Style, Link: true}}
	l := e.Prepare(at).Layout(driver.Size{Width: 100, Height: 12})
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemText, ID: 3, Text: &driver.TextContent{Text: at.Text, Layout: l, Spans: at.Spans}},
		{Kind: driver.ItemEvents, ID: 3, Events: 1 << driver.EventPress},
	}})
	if tx := h.ops("t"); len(tx) != 1 || !strings.Contains(tx[0], `"go to docs"`) {
		t.Fatalf("text = %v", tx)
	}
	// The text is at 15, 26 on the page; "docs" starts 30 points in.
	m.press(3, driver.EventPress, driver.Point{X: 15 + 5, Y: 30})
	m.press(3, driver.EventPress, driver.Point{X: 15 + 37, Y: 30})
	if len(h.events) != 1 || h.events[0].Index < 6 {
		t.Fatalf("link presses = %+v", h.events)
	}
}

func TestStaleValuesAreNotShown(t *testing.T) {
	h := &fakeHost{}
	m := newMounter(h)
	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindInput},
		{Kind: driver.ItemEvents, ID: 1, Events: 1 << driver.EventChangeText},
	}})
	m.edited(1, "ab")
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemInputValue, ID: 1, Value: "a", Count: 0},
		{Kind: driver.ItemInputValue, ID: 1, Value: "ab", Count: 1},
		{Kind: driver.ItemInputValue, ID: 1, Value: "AB", Count: 1},
	}})
	if v := h.ops("iv"); len(v) != 1 || v[0] != `[1,"AB"]` {
		t.Fatalf("values = %v", v)
	}
	if len(h.events) != 1 || h.events[0].Count != 1 || h.events[0].Text != "ab" {
		t.Fatalf("edits = %+v", h.events)
	}
}

func TestScrollsCountWhatTheyApplied(t *testing.T) {
	h := &fakeHost{}
	m := newMounter(h)
	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindScroll},
		{Kind: driver.ItemCreate, ID: 2, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 2, Parent: 1},
		{Kind: driver.ItemEvents, ID: 1, Events: 1 << driver.EventScroll},
		{Kind: driver.ItemScrollTo, ID: 1, Point: driver.Point{Y: 100}},
		{Kind: driver.ItemStickyCells, ID: 1, Sticky: []driver.StickyCell{{Cell: 2, Start: 0, End: 1000}}},
	}})
	m.scrolled(1, driver.Point{Y: 40})
	if len(h.events) != 1 || h.events[0].Count != 1 || h.events[0].Point.Y != 40 {
		t.Fatalf("scroll = %+v", h.events)
	}
	if a := h.ops("a"); len(a) == 0 || !strings.Contains(a[len(a)-1], "translate(0px, 40px)") {
		t.Fatalf("sticky = %v", a)
	}
}

func TestDragsPanOnceTheyActivate(t *testing.T) {
	m, h := mounted()
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemEvents, ID: 2, Events: 1 << driver.EventPan},
	}})
	if e := h.ops("e"); e[len(e)-1] != "[2,24]" {
		t.Fatalf("events = %v", e)
	}
	f := func(x, y driver.Points) []finger { return []finger{{id: 7, page: driver.Point{X: x, Y: y}}} }
	m.touch(3, 1, driver.GestureBegan, f(50, 50), f(50, 50), 0)
	m.touch(2, 1, driver.GestureBegan, f(50, 50), f(50, 50), 0) // bubbled: dropped
	m.touch(3, 2, driver.GestureChanged, f(55, 50), f(55, 50), 10*time.Millisecond)
	if len(h.events) != 0 {
		t.Fatalf("panned before the slop: %+v", h.events)
	}
	m.touch(3, 3, driver.GestureChanged, f(70, 50), f(70, 50), 20*time.Millisecond)
	m.touch(3, 4, driver.GestureEnded, f(80, 50), nil, 30*time.Millisecond)
	if len(h.events) != 2 || h.events[0].Phase != driver.GestureBegan || h.events[1].Phase != driver.GestureEnded ||
		h.events[1].Point.X != 30 || h.events[1].Velocity.X <= 0 {
		t.Fatalf("pan = %+v", h.events)
	}
}

func TestModalsAreAnimatedInAndBackGoesThroughTheApp(t *testing.T) {
	m, h := mounted()
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindModal},
		{Kind: driver.ItemModal, ID: 4, Modal: &driver.ModalConfig{Animation: driver.ModalFade}},
		{Kind: driver.ItemInsert, ID: 4, Parent: 2},
		{Kind: driver.ItemHistory, Count: 1},
		{Kind: driver.ItemAlert, Alert: &driver.AlertConfig{Title: "Hi"}},
	}})
	if ka := h.ops("ka"); len(ka) != 1 || !strings.Contains(ka[0], `"opacity":0`) {
		t.Fatalf("presented = %v", ka)
	}
	if bk := h.ops("bk"); len(bk) != 1 || bk[0] != "[0,true]" {
		t.Fatalf("back = %v", bk)
	}
	if len(h.services) != 1 || h.services[0] != driver.ItemAlert {
		t.Fatalf("services = %v", h.services)
	}
}

func TestStringsAreJSON(t *testing.T) {
	for _, s := range []string{"a\"b\\c\n\t", "\x01", "你好  ", string([]byte{0xff})} {
		var back string
		if err := json.Unmarshal(appendString(nil, s), &back); err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		if s != string([]byte{0xff}) && back != s {
			t.Fatalf("%q came back %q", s, back)
		}
	}
}

func TestPixelImagesSendFramesOutsideTheTree(t *testing.T) {
	h := &fakeHost{pixelImages: true}
	m := newMounter(h)
	first := image.NewRGBA(image.Rect(0, 0, 2, 1))
	second := image.NewRGBA(first.Rect)
	config := driver.ImageConfig{Source: driver.ImageSource{Pixels: first}, ResizeMode: driver.Center}
	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindImage},
		frame(1, 0, 0, 10, 5),
		{Kind: driver.ItemImage, ID: 1, Image: &config},
	}})
	update := config
	update.Source = driver.ImageSource{Pixels: second, Version: 1}
	apply := func(c *driver.ImageConfig) {
		m.mount(&driver.MountBatch{Items: []driver.MountItem{
			{Kind: driver.ItemImage, ID: 1, Image: c},
		}})
	}
	apply(&update)
	apply(&update)
	if got, want := h.pixelFrames, []*image.RGBA{first, second}; !slices.Equal(got, want) {
		t.Fatalf("uploaded %v, want each distinct copy once", got)
	}
	if got := h.ops("px"); len(got) != 1 || !strings.Contains(got[0], "left:4px;top:2px;width:2px;height:1px") {
		t.Fatalf("pixel canvas placements = %v", got)
	}
	apply(&driver.ImageConfig{Source: driver.ImageSource{URI: "img.png"}})
	if got := h.ops("px"); len(got) != 2 || got[1] != `[1,""]` || len(h.pixelFrames) != 3 || h.pixelFrames[2] != nil {
		t.Fatalf("replaced pixel canvas not released: placements %v, frames %v", got, h.pixelFrames)
	}
}

// TestImageSamplingReachesTheImage checks that a nearest-sampled image view scales its pixels as blocks: an encoded image's <image> and a pixel image's canvas take image-rendering: pixelated, and smooth ones take nothing.
func TestImageSamplingReachesTheImage(t *testing.T) {
	h := &fakeHost{pixelImages: true}
	m := newMounter(h)
	apply := func(c *driver.ImageConfig) {
		m.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemImage, ID: 1, Image: c}}})
	}
	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindImage},
		frame(1, 0, 0, 16, 16),
	}})
	apply(&driver.ImageConfig{Source: driver.ImageSource{URI: "a.png"}, Sampling: driver.ImageSamplingNearest})
	apply(&driver.ImageConfig{Source: driver.ImageSource{URI: "a.png"}})
	if got, want := h.ops("im"), []string{`[1,"img","aspectFill","image-rendering:pixelated"]`, `[1,"img","aspectFill",""]`}; !slices.Equal(got, want) {
		t.Fatalf("image operations = %v, want %v", got, want)
	}
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 2))
	apply(&driver.ImageConfig{Source: driver.ImageSource{Pixels: pixels}, ResizeMode: driver.Stretch})
	apply(&driver.ImageConfig{Source: driver.ImageSource{Pixels: pixels}, ResizeMode: driver.Stretch, Sampling: driver.ImageSamplingNearest})
	got := h.ops("px")
	if len(got) != 2 || strings.Contains(got[0], "image-rendering") || !strings.Contains(got[1], ";image-rendering:pixelated") {
		t.Fatalf("pixel canvas placements = %v, want the second alone pixelated", got)
	}
}

func TestDevToolsOverlayPointerEventsAndApplyTiming(t *testing.T) {
	m, h := mounted()
	var applied *driver.MountBatch
	prev := driver.Applied
	driver.Applied = func(b *driver.MountBatch, _ time.Duration) { applied = b }
	t.Cleanup(func() { driver.Applied = prev })

	// Mount a DevTools-style Topmost overlay (PointerEventsBoxNone) containing a layout-audit canvas (PointerEventsNone) and an interactive readout button.
	batch := &driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 10, View: driver.KindView},
		{Kind: driver.ItemCreate, ID: 11, View: driver.KindCanvas},
		{Kind: driver.ItemCreate, ID: 12, View: driver.KindView},
		{Kind: driver.ItemInsert, ID: 10, Parent: 1},
		{Kind: driver.ItemInsert, ID: 11, Parent: 10},
		{Kind: driver.ItemInsert, ID: 12, Parent: 10},
		frame(10, 0, 0, 375, 600),
		frame(11, 0, 0, 375, 600),
		frame(12, 8, 560, 120, 32),
		{Kind: driver.ItemViewProps, ID: 10, ViewProps: &driver.ViewProps{PointerEvents: driver.PointerEventsBoxNone}},
		{Kind: driver.ItemViewProps, ID: 11, ViewProps: &driver.ViewProps{PointerEvents: driver.PointerEventsNone}},
		{Kind: driver.ItemCanvas, ID: 11, Drawing: &driver.Drawing{Ops: []driver.DrawOp{{Kind: driver.DrawStroke}}}},
		{Kind: driver.ItemEvents, ID: 12, Events: 1 << driver.EventPress},
	}}
	driver.Apply(batch, m.mount)

	if applied != batch {
		t.Fatalf("driver.Applied did not record the mounted batch")
	}
	styles := map[int]string{}
	for _, raw := range h.ops("s") {
		var pair []any
		if err := json.Unmarshal([]byte(raw), &pair); err != nil {
			t.Fatal(err)
		}
		styles[int(pair[0].(float64))] = pair[1].(string)
	}
	if !strings.Contains(styles[10], "pointer-events:none;") {
		t.Errorf("overlay style = %q, want pointer-events:none;", styles[10])
	}
	if !strings.Contains(styles[11], "pointer-events:none;") {
		t.Errorf("audit canvas style = %q, want pointer-events:none;", styles[11])
	}
	if !strings.Contains(styles[12], "pointer-events:auto;") {
		t.Errorf("readout child style = %q, want pointer-events:auto;", styles[12])
	}
	if cv := h.ops("cv"); len(cv) == 0 || !strings.Contains(cv[len(cv)-1], "[11,") {
		t.Errorf("audit canvas did not paint: %v", cv)
	}
	m.press(12, driver.EventPress, driver.Point{X: 20, Y: 570})
	if len(h.events) != 1 || h.events[0].ID != 12 || h.events[0].Kind != driver.EventPress {
		t.Fatalf("readout press events = %+v", h.events)
	}
}

func TestTextAndInputsSendLanguageAndKeepAll(t *testing.T) {
	m, h := mounted()
	e := NewTextEngine(&approx{})
	at := plain("直 안녕")
	at.Style.Font.Language = "ja"
	ko := at.Style
	ko.Font.Language = "ko-KR"
	at.Spans = []driver.TextSpan{{Start: len("直 "), End: len(at.Text), Style: ko}}
	l := e.Prepare(at).Layout(driver.Size{Width: 200, Height: 20})
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemText, ID: 3, Text: &driver.TextContent{Text: at.Text, Layout: l, Spans: at.Spans}},
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindInput},
		{Kind: driver.ItemInput, ID: 4, Input: &driver.InputConfig{
			Multiline:   true,
			Placeholder: "안녕하세요",
			Style:       driver.TextStyle{Font: driver.Font{Size: 14, Weight: 400, Language: "ko-KR"}, Color: driver.RGB(0)},
		}},
		{Kind: driver.ItemCreate, ID: 5, View: driver.KindInput},
		{Kind: driver.ItemInput, ID: 5, Input: &driver.InputConfig{
			Placeholder: "直",
			Style:       driver.TextStyle{Font: driver.Font{Size: 14, Weight: 400, Language: "ja"}, Color: driver.RGB(0)},
		}},
	}})
	tx := h.ops("t")
	if len(tx) != 1 || !strings.Contains(tx[0], `"l":["ja","ko-KR"]`) {
		t.Fatalf("text op = %v, want l:[ja,ko-KR]", tx)
	}
	fds := h.ops("fd")
	if len(fds) != 2 {
		t.Fatalf("fd ops = %v, want 2", fds)
	}
	if !strings.Contains(fds[0], `"lang":"ko-KR"`) || !strings.Contains(fds[0], "word-break:keep-all;") || !strings.Contains(fds[0], "Apple SD Gothic Neo") {
		t.Errorf("multiline Korean input fd = %s", fds[0])
	}
	if !strings.Contains(fds[1], `"lang":"ja"`) || strings.Contains(fds[1], "word-break:keep-all;") || !strings.Contains(fds[1], "Hiragino Sans") {
		t.Errorf("single-line Japanese input fd = %s", fds[1])
	}

	readonly := &driver.InputConfig{Readonly: true, Placeholder: "copy me"}
	m.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemInput, ID: 5, Input: readonly}}})
	fds = h.ops("fd")
	if len(fds) < 3 || !strings.Contains(fds[len(fds)-1], `"ro":true`) || strings.Contains(fds[len(fds)-1], `"off":true`) {
		t.Fatalf("readonly input fd = %v, want ro=true and off=false", fds)
	}
}

func TestModalsReportShownAndDismissed(t *testing.T) {
	m, h := mounted()
	// ModalNone reports EventModalShown immediately upon insert.
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 4, View: driver.KindModal},
		{Kind: driver.ItemModal, ID: 4, Modal: &driver.ModalConfig{Animation: driver.ModalNone}},
		{Kind: driver.ItemInsert, ID: 4, Parent: 2},
	}})
	if len(h.events) != 1 || h.events[0].Kind != driver.EventModalShown || h.events[0].ID != 4 {
		t.Fatalf("ModalNone events = %+v, want EventModalShown for 4", h.events)
	}
	// Deleting reports EventModalDismissed.
	h.events = nil
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemDelete, ID: 4},
	}})
	if len(h.events) != 1 || h.events[0].Kind != driver.EventModalDismissed || h.events[0].ID != 4 {
		t.Fatalf("delete events = %+v, want EventModalDismissed for 4", h.events)
	}

	// ModalFade waits for its 200ms animation on tick before reporting EventModalShown.
	h.events = nil
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 5, View: driver.KindModal},
		{Kind: driver.ItemModal, ID: 5, Modal: &driver.ModalConfig{Animation: driver.ModalFade}},
		{Kind: driver.ItemInsert, ID: 5, Parent: 2},
	}})
	if len(h.events) != 0 {
		t.Fatalf("ModalFade fired early: %+v", h.events)
	}
	m.tick(0)
	m.tick(100 * time.Millisecond)
	if len(h.events) != 0 {
		t.Fatalf("ModalFade fired at 100ms: %+v", h.events)
	}
	m.tick(200 * time.Millisecond)
	if len(h.events) != 1 || h.events[0].Kind != driver.EventModalShown || h.events[0].ID != 5 {
		t.Fatalf("ModalFade at 200ms: %+v, want EventModalShown for 5", h.events)
	}

	// Deleting mid-entrance cancels EventModalShown and emits EventModalDismissed.
	h.events = nil
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 6, View: driver.KindModal},
		{Kind: driver.ItemModal, ID: 6, Modal: &driver.ModalConfig{Animation: driver.ModalFade}},
		{Kind: driver.ItemInsert, ID: 6, Parent: 2},
	}})
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemDelete, ID: 6},
	}})
	m.tick(500 * time.Millisecond)
	if len(h.events) != 1 || h.events[0].Kind != driver.EventModalDismissed || h.events[0].ID != 6 {
		t.Fatalf("deleted mid-entrance: %+v, want only EventModalDismissed for 6", h.events)
	}
}

func TestCanvasPaintsAtItsResolutionScale(t *testing.T) {
	m, h := mounted()
	ops := []driver.DrawOp{{Kind: driver.DrawStroke}}
	driver.Apply(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 10, View: driver.KindCanvas},
		{Kind: driver.ItemInsert, ID: 10, Parent: 1},
		frame(10, 0, 0, 100, 50),
		{Kind: driver.ItemCanvas, ID: 10, Drawing: &driver.Drawing{Ops: ops, Resolution: 0.5, Upscale: driver.ImageSamplingNearest}},
	}}, m.mount)
	// A new scale comes as the same ops in a new drawing, which paints again.
	for _, r := range []float32{0, 1.5} {
		driver.Apply(&driver.MountBatch{Items: []driver.MountItem{
			{Kind: driver.ItemCanvas, ID: 10, Drawing: &driver.Drawing{Ops: ops, Resolution: r}},
		}}, m.mount)
	}
	want := []fakePaint{{0.5, false}, {1, true}, {1, true}}
	if !slices.Equal(h.paints, want) {
		t.Fatalf("paints = %v, want %v", h.paints, want)
	}
}

func TestImageTintColorEmitsMaskAndRefreshesOnLoadAndResize(t *testing.T) {
	h := &fakeHost{pixelImages: true, maskURL: "data:image/png;base64,AAAA"}
	m := newMounter(h)
	m.mount(&driver.MountBatch{Root: 1, Items: []driver.MountItem{
		{Kind: driver.ItemCreate, ID: 1, View: driver.KindImage},
		frame(1, 0, 0, 100, 50),
		{Kind: driver.ItemImage, ID: 1, Image: &driver.ImageConfig{
			Source:     driver.ImageSource{URI: "https://example.com/icon.png"},
			ResizeMode: driver.Contain,
			Sampling:   driver.ImageSamplingNearest,
		}},
	}})
	// Untinted image keeps the 4-argument op without mask CSS.
	if got := h.ops("im"); len(got) != 1 || got[0] != `[1,"img","aspectFit","image-rendering:pixelated"]` {
		t.Fatalf("untinted im = %v", got)
	}

	// Setting TintColor switches to a 5-argument op with mask CSS.
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &driver.ViewStyle{Opacity: 1, TintColor: driver.RGB(0xff3300)}},
	}})
	ims := h.ops("im")
	last := ims[len(ims)-1]
	for _, want := range []string{
		`background-color:rgba(255,51,0,1);`,
		`mask-image:url(\"data:image/png;base64,AAAA\");`,
		`-webkit-mask-image:url(\"data:image/png;base64,AAAA\");`,
		`mask-size:contain;`,
		`mask-position:50% 50%;`,
		`image-rendering:pixelated;`,
	} {
		if !strings.Contains(last, want) {
			t.Fatalf("tinted im = %s, missing %q", last, want)
		}
	}

	// Reporting natural dimensions fits the mask exactly inside the 100x50 box (20x20 Contain -> 50x50 centered at x=25, y=0).
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemEvents, ID: 1, Events: 1<<driver.EventLoad | 1<<driver.EventLoadEnd},
	}})
	m.imageLoaded(1, 20, 20)
	ims = h.ops("im")
	last = ims[len(ims)-1]
	if !strings.Contains(last, `mask-size:50px 50px;`) || !strings.Contains(last, `mask-position:25px 0px;`) {
		t.Fatalf("fitted tinted im = %s", last)
	}
	if len(h.events) != 2 || h.events[0].Kind != driver.EventLoad || h.events[1].Kind != driver.EventLoadEnd {
		t.Fatalf("imgload events = %+v", h.events)
	}

	// Resizing updates the fitted mask geometry.
	m.mount(&driver.MountBatch{Items: []driver.MountItem{frame(1, 0, 0, 40, 20)}})
	ims = h.ops("im")
	last = ims[len(ims)-1]
	if !strings.Contains(last, `mask-size:20px 20px;`) || !strings.Contains(last, `mask-position:10px 0px;`) {
		t.Fatalf("resized tinted im = %s", last)
	}

	// Clearing TintColor drops the mask CSS.
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &driver.ViewStyle{Opacity: 1}},
	}})
	ims = h.ops("im")
	if got := ims[len(ims)-1]; got != `[1,"img","aspectFit","image-rendering:pixelated"]` {
		t.Fatalf("cleared tint im = %s", got)
	}

	// Pixel-buffer images also re-upload when TintColor changes.
	px := image.NewRGBA(image.Rect(0, 0, 4, 4))
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemImage, ID: 1, Image: &driver.ImageConfig{Source: driver.ImageSource{Pixels: px}}},
	}})
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemViewStyle, ID: 1, ViewStyle: &driver.ViewStyle{Opacity: 1, TintColor: driver.RGBA(0x00ff00, 0.5)}},
	}})
	if len(h.pixelTints) != 2 || h.pixelTints[0] != 0 || h.pixelTints[1] != driver.RGBA(0x00ff00, 0.5) {
		t.Fatalf("pixel tints = %v", h.pixelTints)
	}
}

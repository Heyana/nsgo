package capture_test

import (
	"image"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/videoalpha/clips"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestAppleNativeClipKeepsOuterDecoration(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios", "ios_simulator":
	default:
		t.Skip("checks Apple's hosted native content")
	}
	movie := filepath.Join(t.TempDir(), "blue.mov")
	if err := os.WriteFile(movie, clips.HEVC, 0o600); err != nil {
		t.Fatal(err)
	}
	type sample struct {
		kind, corners string
		plain         bool
	}
	kinds := []string{"web", "view", "input", "multiline", "video", "video-controls", "native"}
	var selected *core.Signal[sample]
	var message *core.Signal[string]
	input, web := new(core.Ref), new(core.Ref)
	var ratio float32
	var focus, blur int
	var changes []bool
	var videoLoads atomic.Int32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ratio = cx.Env().PixelRatio()
		selected = core.NewSignal(cx, sample{kind: "input", corners: "round", plain: true})
		message = core.NewSignal(cx, "loading")
		children := []core.Node{core.Text{ContentFunc: message.Get}}
		for _, kind := range kinds {
			style := func() core.Style {
				current := selected.Get()
				style := core.Style{Position: core.Absolute, Left: core.Pt(80), Top: core.Pt(80),
					Width: core.Pt(120), Height: core.Pt(80), Overflow: core.Hidden,
					Background: core.RGB(0x0000ff), CornerRadius: 24,
					Shadows:      []core.BoxShadow{{Spread: 12, Color: core.RGB(0xff0000)}},
					OutlineWidth: 4, OutlineOffset: 2, OutlineColor: core.RGB(0x00ff00)}
				if current.kind != kind {
					style.Display = core.DisplayNone
				}
				if current.plain {
					style.Shadows, style.OutlineWidth = nil, 0
					if kind == "native" {
						style.Width, style.Height = core.Pt(51), core.Pt(31)
					}
				}
				switch current.corners {
				case "shaped":
					style.CornerRadii = core.Corners{TopLeft: 24, TopRight: 12, BottomRight: 32, BottomLeft: 8}
				case "continuous":
					style.BorderCurve = core.BorderCurveContinuous
				}
				if kind == "input" || kind == "multiline" {
					style.FontSize, style.Color = 120, core.RGB(0xffff00)
				}
				if kind == "video" || kind == "video-controls" {
					style.Background = core.RGB(0xffff00)
				}
				return style
			}
			id := "clip-" + kind
			switch kind {
			case "input":
				children = append(children, core.Input{TestID: id, Ref: input, StyleFunc: style, DefaultValue: "kept",
					OnFocus: func() { focus++ }, OnBlur: func() { blur++ }})
			case "multiline":
				children = append(children, core.Input{TestID: id, StyleFunc: style, Multiline: true, DefaultValue: "MMMM"})
			case "web":
				children = append(children, core.WebView{TestID: id, Ref: web, StyleFunc: style, OnMessage: message.Set,
					Source: core.WebHTML(`<body style="margin:0;width:100vw;height:100vh;background:#ffff00"><div style="width:100%;height:100%;background:#ffff00"></div><script>
window.counter=41; window.nsgo.postMessage('ready:'+counter);
window.addEventListener('message',()=>window.nsgo.postMessage('step:'+(++counter)));
</script></body>`, "")})
			case "video", "video-controls":
				children = append(children, core.Video{TestID: id, StyleFunc: style, Source: movie,
					ResizeMode: core.Stretch, Repeat: true, Muted: true, Controls: kind == "video-controls",
					OnLoad: func(core.VideoInfo) { videoLoads.Add(1) }})
			case "native":
				children = append(children, core.Native{TestID: id, StyleFunc: style, Component: "nsgo/switch", Props: map[string]any{"checked": true},
					OnEvent: func(name string, data any) {
						if on, ok := data.(bool); name == "change" && ok {
							changes = append(changes, on)
						}
					}})
			default:
				children = append(children, core.View{TestID: id, StyleFunc: style, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(180), Height: core.Pt(140), Background: core.RGB(0xffff00)}},
				}})
			}
		}
		return core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0xffffff)}, Children: children}
	}, nsgotest.Size(300, 260))
	app.WaitFor("text:ready:41", 10*time.Second)
	app.WaitUntil(10*time.Second, func() bool { return videoLoads.Load() == 2 })
	app.Post(func() { input.Focus(); input.SetSelection(1, 3) })
	app.Settle()
	app.Post(func() { selected.Set(sample{kind: "input", corners: "round"}) })
	app.Settle()
	if field := app.Find("id:clip-input"); !field.Focused || field.Text != "kept" || focus != 1 || blur != 0 {
		t.Errorf("adding a focused editor's clip holder: focused=%v, text=%q, focus=%d, blur=%d", field.Focused, field.Text, focus, blur)
	}
	if nsgotest.Target() != "macos" {
		app.Type("id:clip-input", "x")
		if got := app.Find("id:clip-input").Text; got != "kxt" {
			t.Errorf("typing after adding the holder: %q, want selection replaced in kept", got)
		}
	}
	app.Post(input.Blur)
	app.Post(func() { web.PostMessage("step") })
	app.WaitFor("text:step:42", 5*time.Second)
	near := func(got, want uint32) bool { return got+3 >= want && got <= want+3 }
	for _, kind := range kinds {
		for _, corners := range []string{"round", "shaped", "continuous"} {
			t.Run(kind+"/"+corners, func(t *testing.T) {
				app.Post(func() { selected.Set(sample{kind: kind, corners: corners}) })
				app.Settle()
				origin := app.Find("id:clip-" + kind).Bounds().Origin
				at := func(img image.Image, x, y core.Points) (uint32, uint32, uint32) {
					r, g, b, _ := img.At(img.Bounds().Min.X+int(float32(origin.X+x)*ratio), img.Bounds().Min.Y+int(float32(origin.Y+y)*ratio)).RGBA()
					return r >> 8, g >> 8, b >> 8
				}
				img := app.Capture()
				if kind == "web" || kind == "video" {
					deadline := time.Now().Add(7 * time.Second)
					for time.Now().Before(deadline) {
						r, g, b := at(img, 60, 40)
						if kind == "web" && r > 200 && g > 200 && b < 40 || kind == "video" && r < 64 && g < 64 && b > 200 {
							break
						}
						time.Sleep(20 * time.Millisecond)
						img = app.Capture()
					}
				}

				check := func(label string, x, y core.Points, want uint32) {
					t.Helper()
					r, g, b := at(img, x, y)
					if !near(r, want>>16) || !near(g, want>>8&255) || !near(b, want&255) {
						t.Errorf("%s/%s %s at (%g,%g): #%02x%02x%02x, want #%06x", kind, corners, label, x, y, r, g, b, want)
					}
				}
				check("outer shadow", -9, 40, 0xff0000)
				check("outline", -4, 40, 0x00ff00)
				check("clipped overflow", 129, 40, 0xff0000)
				check("past shadow", 137, 40, 0xffffff)
				check("rounded content", 1, 1, 0xff0000)
				if kind == "view" || kind == "web" {
					check("content", 60, 40, 0xffff00)
				}
				if kind == "video" {
					// The encoded HEVC fixture goes through the platform's video color conversion.
					r, g, b := at(img, 60, 40)
					if r >= 64 || g >= 64 || b <= 200 {
						t.Errorf("video frame: #%02x%02x%02x, want blue over the yellow host", r, g, b)
					}
				}
			})
		}
	}
	app.Post(func() { selected.Set(sample{kind: "native", corners: "round", plain: true}) })
	app.Settle()
	for range 2 {
		app.Press("id:clip-native")
	}
	if got := videoLoads.Load(); got != 2 {
		t.Errorf("videos loaded %d times, want one load per retained player", got)
	}
	if len(changes) != 2 || changes[0] || !changes[1] {
		t.Errorf("native component's changes after reparenting: %v, want [false true]", changes)
	}
}

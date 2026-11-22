package diagram

import (
	"encoding/base64"
	"math"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func mediaSVG(t *testing.T, fill string) *core.SVG {
	t.Helper()
	svg, err := core.ParseSVG([]byte(`<svg viewBox="0 0 24 24"><path fill="` + fill + `" d="M0 0 H24 V24 H0 Z"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	return svg
}

// Layout caches distinguish icon availability, while each view draws its own documents.
func TestDiagramIconsUseTheAppsSet(t *testing.T) {
	source := `flowchart LR
 A[fa:fa-car Drive] --> B@{ icon: "fa:fa-car", form: "square", label: "Car" }`
	red, blue := core.RGB(0xe13723), core.RGB(0x2347e1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			View{TestID: "missing", Source: source},
			View{TestID: "red", Source: source, Icons: map[string]*core.SVG{"fa:fa-car": mediaSVG(t, "#e13723")}},
			View{TestID: "blue", Source: source, Icons: map[string]*core.SVG{"fa:fa-car": mediaSVG(t, "#2347e1"), "fa:fa-missing": nil}},
			View{TestID: "current", Source: source, Icons: map[string]*core.SVG{"fa:fa-car": mediaSVG(t, "currentColor")}, Theme: &Theme{Text: red}},
		}}
	}, nsgotest.Size(800, 900))
	for _, id := range []string{"red", "blue", "current"} {
		v := app.Find("id:" + id)
		if v.Kind != nsgotest.KindCanvas {
			t.Fatalf("%s: diagram did not parse", id)
		}
	}
	count := func(id string, c core.Color) int {
		n := 0
		for _, f := range fills(app.Find("id:" + id)) {
			if f == c {
				n++
			}
		}
		return n
	}
	if count("missing", red) != 0 || count("red", red) != 2 || count("blue", blue) != 2 || count("blue", red) != 0 || count("current", red) != 2 {
		t.Fatalf("inline/node icons missing or documents leaked between cached layouts: missing=%v red=%v blue=%v current=%v", fills(app.Find("id:missing")), fills(app.Find("id:red")), fills(app.Find("id:blue")), fills(app.Find("id:current")))
	}
	if app.Find("id:red").Frame.Size.Width <= app.Find("id:missing").Frame.Size.Width {
		t.Error("an available inline icon reserves no room")
	}
	app.Screenshot("diagram-icons")
}

// Image pictures use the canvas image loader, with the node's fit or stretch geometry.
func TestDiagramImagesLoadAndFit(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAFAAAAAoCAIAAADmAupWAAAASElEQVR4nO3PwRAAAAgAsFyCCCF/hEyyqM/uJrCYyhfZ8yKEhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYWFhYVvLal8xmLAbnkGAAAAAElFTkSuQmCC")
	if err != nil {
		t.Fatal(err)
	}
	uri := pictureFixture(t, data)
	var source *core.Signal[string]
	code := func(extra string) string {
		return `flowchart LR
 A@{ img: "` + uri + `", w: 80, h: 80, constraint: "on", label: "Fitted" } --> B@{ img: "` + uri + `", w: 80, h: 80, label: "Stretched" }` + extra
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		source = core.NewSignal(cx, code(""))
		return View{TestID: "images", SourceFunc: source.Get}
	}, nsgotest.Size(800, 600), func(a *nsgotest.App) { a.ImageSizes = map[string]core.Size{uri: {Width: 80, Height: 40}} })
	images := func() []driver.DrawOp {
		var out []driver.DrawOp
		for _, op := range app.Find("id:images").Ops {
			if op.Kind == driver.DrawImage {
				out = append(out, op)
			}
		}
		return out
	}
	app.WaitUntil(5*time.Second, func() bool { return len(images()) == 2 })
	ops := images()
	if ops[0].Image.URI != uri || ops[1].Image.URI != uri || math.Abs(float64(ops[0].W-80)) > 0.01 || math.Abs(float64(ops[0].H-40)) > 0.01 || math.Abs(float64(ops[1].W-80)) > 0.01 || math.Abs(float64(ops[1].H-80)) > 0.01 {
		t.Fatalf("fit/stretch image operations: %+v", ops)
	}
	app.Screenshot("diagram-images")
	app.Post(func() { source.Set("flowchart LR\n A[No image]") })
	app.Settle()
	if len(images()) != 0 {
		t.Error("replacing source retained the old pictures")
	}
	app.Post(func() { source.Set(code("")) })
	app.Settle()
	app.WaitUntil(5*time.Second, func() bool { return len(images()) == 2 })
}

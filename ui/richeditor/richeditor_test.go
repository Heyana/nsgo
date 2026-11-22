package richeditor_test

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/base/richeditor"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
	uirich "github.com/nsgo-dev/nsgo/ui/richeditor"
)

// TestViewTakesTyping lays a document out in ui's look and types into it.
func TestViewTakesTyping(t *testing.T) {
	s, err := richeditor.NewSession(richeditor.SessionConfig{
		Extensions: append(richeditor.StarterKit(richeditor.StarterOptions{}), richeditor.TableKit(richeditor.TableOptions{})),
		Markdown:   "# Title\n\nSome **bold** text\n\n| a | b |\n| --- | --- |\n| c | d |\n"})
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(core.DefaultTheme())
		return uirich.View{Editor: s, Label: "Doc", Style: core.Style{Width: core.Pt(360), Height: core.Pt(400)}}
	})
	app.Find("text:Title")
	if h := app.Find("text:Title").Frame.Size.Height; h < 30 {
		t.Fatalf("a heading's line is %v tall", h)
	}
	if a, d := app.Find(`text:"a"`).Parent, app.Find(`text:"d"`).Parent; a.Frame.Origin.X == d.Frame.Origin.X {
		t.Fatalf("the table's cells stack: %+v %+v", a.Frame, d.Frame)
	}
	app.Press("text:Some bold text")
	app.Settle()
	if _, ok := s.State().Selection.(*richeditor.TextSelection); !ok || s.State().Selection.FromPos().Parent().TextContent() != "Some bold text" {
		t.Fatalf("a press landed in %q", s.State().Selection.FromPos().Parent().TextContent())
	}
	app.Type("role:textbox", "!")
	if !strings.Contains(s.Text(), "!") {
		t.Fatalf("typing left %q", s.Text())
	}
}

// TestViewShowsAnInlineImage lays a sentence holding an image out in ui's
// look: the paragraph keeps the picture in the line, a press on it selects
// the node, and the Markdown reads as it came.
func TestViewShowsAnInlineImage(t *testing.T) {
	exts := append(richeditor.StarterKit(richeditor.StarterOptions{}), richeditor.Extension{Name: "inlineImage",
		Nodes: []richeditor.NodeSpec{{Name: "inlineImage", Inline: true, Group: "inline", Atom: true,
			Attrs: []richeditor.AttrSpec{{Name: "src"}, {Name: "alt", HasDefault: true}, {Name: "title", HasDefault: true}}}}})
	s, err := richeditor.NewSession(richeditor.SessionConfig{Extensions: exts, Markdown: "see ![pic](a.png) here\n\n![solo](b.png)\n"})
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(core.DefaultTheme())
		return uirich.View{Editor: s, Label: "Doc", Style: core.Style{Width: core.Pt(360), Height: core.Pt(400)}}
	})
	// Two pictures, both views: the inline one's in the first paragraph's
	// line, the standalone one the second block.
	images := app.QueryMounted("role:image")
	if len(images) != 2 {
		t.Fatalf("%d image views", len(images))
	}
	text := app.Find("text:see ￼ here")
	pageY := func(v *nsgotest.View) driver.Points {
		y := v.Frame.Origin.Y
		for p := v.Parent; p != nil; p = p.Parent {
			y += p.Frame.Origin.Y - p.Offset.Y
		}
		return y
	}
	if pageY(images[0]) < pageY(text) || pageY(images[0]) > pageY(text)+text.Frame.Size.Height {
		t.Fatalf("the inline image is not in the text's line: %v vs %v", images[0].Frame, text.Frame)
	}
	if pageY(images[1]) <= pageY(text) {
		t.Fatalf("the block image did not follow the line: %v", images[1].Frame)
	}
	app.PressAt("role:image", driver.Point{X: images[0].Frame.Size.Width / 2, Y: images[0].Frame.Size.Height / 2})
	sel, ok := s.State().Selection.(*richeditor.NodeSelection)
	if !ok || sel.Node.Type.Name != "inlineImage" {
		t.Fatalf("selection %+v", s.State().Selection)
	}
	if got := s.Markdown(); !strings.Contains(got, "![pic](a.png)") || !strings.Contains(got, "![solo](b.png)") {
		t.Fatalf("markdown %q", got)
	}
}

// TestViewTouchSelectionHandlesUseThemeAccent checks that ui/richeditor styles touch selection handles with the theme accent and lets apps override them.
func TestViewTouchSelectionHandlesUseThemeAccent(t *testing.T) {
	s, err := richeditor.NewSession(richeditor.SessionConfig{
		Extensions: richeditor.StarterKit(richeditor.StarterOptions{}),
		Markdown:   "touch selection test\n"})
	if err != nil {
		t.Fatal(err)
	}
	override := core.RGB(0xff2200)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(core.DefaultTheme())
		return uirich.View{Editor: s, Label: "Doc",
			Styles: richeditor.Styles{SelectionHandle: core.Style{Background: override}},
			Style:  core.Style{Width: core.Pt(360), Height: core.Pt(200)}}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}))
	app.Settle()
	r, ok := s.PageCoordsAtPos(3)
	if !ok {
		t.Fatal("no page coords")
	}
	app.LongPressAt("id:rich-editor-content", driver.Point{X: driver.Points(r.X), Y: driver.Points(r.Y + r.Height/2)})
	start := app.Find("id:rich-editor-selection-handle-start")
	if len(start.Children) == 0 || len(start.Children[0].Children) == 0 {
		t.Fatalf("handle missing knob view: %+v", start)
	}
	knob := start.Children[0].Children[0]
	if knob.Style.Background != override {
		t.Fatalf("knob background = %v, want %v", knob.Style.Background, override)
	}
	if app.EditMenu == nil {
		t.Fatal("expected system edit menu on touch long press")
	}
}

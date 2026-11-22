//go:build ios && nsgotest

package ios_test

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base/richeditor"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/nsgotest"
	styled "github.com/nsgo-dev/nsgo/ui/richeditor"
)

func TestRichEditorNativeTouchEditMenu(t *testing.T) {
	const wait = 10 * time.Second
	s, err := richeditor.NewSession(richeditor.SessionConfig{
		Extensions: richeditor.StarterKit(richeditor.StarterOptions{}), Markdown: "one two three\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(60)}, Children: []core.Node{
			styled.View{Editor: s, Label: "Native touch editor", Style: core.Style{Height: core.Pt(220), FontSize: 20}},
		}}
	})
	t.Cleanup(func() {
		t.Logf("UIKit visible menu labels %q; test clipboard %q", ios.NativeEditMenuLabelsForTest(), app.Clipboard)
	})
	var word richeditor.Rect2
	app.WaitUntil(wait, func() bool {
		var ok bool
		word, ok = s.PageCoordsAtPos(6)
		return ok
	})
	content := app.Find("id:rich-editor-content").Bounds()
	app.LongPressAt("id:rich-editor-content", core.Point{
		X: core.Points(word.X) - content.Origin.X,
		Y: core.Points(word.Y+word.Height/2) - content.Origin.Y,
	})
	app.WaitUntil(wait, func() bool {
		sel := s.State().Selection
		return s.State().Doc.TextBetween(sel.From(), sel.To(), "", "") == "two"
	})
	app.WaitFor("id:rich-editor-selection-handle-start", wait)
	app.WaitFor("id:rich-editor-selection-handle-end", wait)
	app.WaitUntil(wait, func() bool {
		labels := ios.NativeEditMenuLabelsForTest()
		return slices.Contains(labels, "Copy") && slices.Contains(labels, "Cut")
	})
	t.Logf("UIKit visible menu after native long press: %q", ios.NativeEditMenuLabelsForTest())
	end, endOK := s.PageCoordsAtPos(8)
	target, targetOK := s.PageCoordsAtPos(14)
	if !endOK || !targetOK {
		t.Fatal("native selection endpoints have no geometry")
	}
	handle := app.Find("id:rich-editor-selection-handle-end").Bounds()
	app.DragAt("id:rich-editor-selection-handle-end",
		core.Point{X: handle.Size.Width / 2, Y: handle.Size.Height - 8},
		core.Point{X: core.Points(target.X - end.X), Y: core.Points(target.Y - end.Y)})
	app.WaitUntil(wait, func() bool {
		return s.State().Selection.From() == 5 && s.State().Selection.To() == 14
	})

	app.Post(func() { s.SetEditable(false) })
	app.WaitUntil(wait, func() bool {
		labels := ios.NativeEditMenuLabelsForTest()
		return slices.Contains(labels, "Copy") && !slices.Contains(labels, "Cut") && !slices.Contains(labels, "Paste")
	})
	if !ios.ChooseNativeEditActionForTest(core.EditCopy) {
		t.Fatal("native input did not retain the offered Copy action")
	}
	app.WaitUntil(wait, func() bool { return app.Clipboard == "two three" })
	if s.Text() != "one two three" {
		t.Fatalf("Copy changed the document: %q", s.Text())
	}
}

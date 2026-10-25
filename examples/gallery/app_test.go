package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/nsgotest"
)

var desktopEnv = core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en-US"}

// startDesktop runs the gallery in a wide desktop window.
func startDesktop(t *testing.T, opts ...nsgotest.Option) *nsgotest.App {
	app := nsgotest.Start(t, Root, append([]nsgotest.Option{nsgotest.Size(1100, 760)}, opts...)...)
	app.SetEnvironment(desktopEnv)
	return app
}

// TestSectionList keeps the list grouped by what the pages show: base's
// components, then core's elements, media and system services.
func TestSectionList(t *testing.T) {
	app := startDesktop(t)
	last := core.Points(-1)
	for _, group := range []struct{ heading, first string }{
		{"Components", "nav-Button"},
		{"Elements", "nav-Text"},
		{"Media", "nav-Web View"},
		{"System", "nav-Window"},
	} {
		root := app.Find("id:nav-group-" + group.heading)
		app.Find("id:nav-group-" + group.heading + " >> role:header " + group.heading)
		app.Find("id:nav-group-" + group.heading + " >> id:" + group.first)
		if root.Frame.Origin.Y <= last {
			t.Fatalf("the %s group is above the group before it", group.heading)
		}
		last = root.Frame.Origin.Y
	}
}

// TestListKeepsItsWidth keeps the section list as wide on every page, so
// a page's long lines cannot squeeze it.
func TestListKeepsItsWidth(t *testing.T) {
	app := startDesktop(t)
	for _, s := range sections {
		app.Press("id:nav-" + s.name)
		if w := app.Find("id:nav-sections").Frame.Size.Width; w != listWidth {
			t.Fatalf("the list is %v wide on %s, want %v", w, s.name, listWidth)
		}
	}
}

func TestListSwitchesSections(t *testing.T) {
	app := startDesktop(t)
	if got := app.Find("id:title").Text; got != "Button" {
		t.Fatalf("first section = %q, want Button", got)
	}
	app.Press("role:button Primary")
	app.Find("text:1 presses")

	app.Press("id:nav-Images")
	app.Find("id:section-Images")
	if !app.Find("id:nav-Images").Props.Selected || app.Find("id:nav-Button").Props.Selected {
		t.Fatal("the list does not highlight the selected section")
	}
	if app.Title != "NSGo Base — Images" {
		t.Fatalf("title = %q", app.Title)
	}
	app.Load("id:logo", core.Size{Width: 192, Height: 192})
	if got := app.Find("id:logo").Frame.Size; got != (core.Size{Width: 96, Height: 96}) {
		t.Fatalf("logo = %v, want its declared size", got)
	}
	if got := app.Find("label:Cover, left").Image.Position.X; got != core.PositionStart {
		t.Fatalf("position = %+v, want the start", got)
	}
	if got := app.Find("id:blurred").Image.BlurRadius; got != 6 {
		t.Fatalf("blur radius = %v", got)
	}
	app.ImageSizes = map[string]core.Size{riverURL: {Width: 600, Height: 300}}
	app.Press("label:Get size")
	if got := app.Find("id:image-size").Text; got != "600×300 px" {
		t.Fatalf("size = %q", got)
	}
}

// TestContentScrollsWithinTheWindow keeps the section list and the
// section's scroll view inside the window, so long ones scroll rather
// than overflow.
func TestContentScrollsWithinTheWindow(t *testing.T) {
	app := startDesktop(t, nsgotest.Size(1100, 500))
	app.Press("id:nav-Layout")
	for _, id := range []string{"content", "nav-sections"} {
		v := app.Find("id:" + id)
		if v.Frame.Size.Height > 500 || v.Content.Height <= v.Frame.Size.Height {
			t.Fatalf("%s frame %v holds %v, want it within the window and scrolling", id, v.Frame, v.Content)
		}
	}
}

func TestPhoneStack(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("id:index")
	app.Press("id:nav-Popups")
	app.Find("id:screen-Popups")
	app.Press("role:button Menu")
	// On a phone the menu is a sheet, which comes in and goes.
	app.Advance(300 * time.Millisecond)
	app.Press("label:Paste")
	app.Find("text:Picked Paste")
	app.Advance(300 * time.Millisecond)
	app.Press("role:button Dialog")
	app.Find("text:A dialog")
	app.Back()
	if app.Exists("text:A dialog") {
		t.Fatal("back did not close the dialog")
	}
	app.Back()
	app.Find("id:index")

	// Widening the window keeps the section last open.
	app.Press("id:nav-Lists")
	app.Resize(1000, 700)
	app.Find("id:section-Lists")
}

func TestPhoneSystemHeader(t *testing.T) {
	ios := core.Environment{OS: core.OSIOS, Locale: "en-US", SafeArea: core.Edges{Top: 47, Bottom: 34},
		HeaderHeight: 44, LargeHeaderHeight: 96}
	app := nsgotest.Start(t, Root, nsgotest.Environment(ios))
	index := app.Find("id:index").Screen
	if index.Title != "Base" || index.CanGoBack {
		t.Fatalf("index header = %+v", index)
	}
	app.Press("id:nav-Popups")
	popups := app.Find("id:screen-Popups").Screen
	if !popups.CanGoBack || popups.BackTitle != "Base" || popups.Background == 0 {
		t.Fatalf("section header = %+v", popups)
	}

	// A page sheet lightens the status bar while it is open, and a swipe
	// closes it.
	app.Press("role:button Page sheet")
	app.Find("text:A page sheet")
	if m := app.Find("id:presented").Parent.Modal; m.Presentation != core.ModalPageSheet || m.Orientations != core.OrientationPortrait {
		t.Fatalf("modal = %+v", m)
	}
	if app.StatusBar.Style != core.StatusBarLight {
		t.Errorf("status bar = %+v under the page sheet", app.StatusBar)
	}
	app.SwipeClose("id:presented")
	app.Find("text:Last closed by swipe")
	if app.Exists("text:A page sheet") || app.StatusBar.Style != core.StatusBarAuto {
		t.Errorf("the swiped sheet stayed, status bar %+v", app.StatusBar)
	}

	app.HeaderBack("id:screen-Popups")
	app.Find("id:index")
	if app.Exists("id:screen-Popups") {
		t.Error("the header's back button did not pop the section")
	}
}

func TestApplicationMenu(t *testing.T) {
	app := startDesktop(t)
	app.ChooseMenu("View", "Text View")
	app.Find("id:section-Text View")
	view := app.Menu[slices.IndexFunc(app.Menu, func(e core.MenuEntry) bool { return e.Label == "View" })]
	for _, e := range view.Items {
		if e.Label == "Text View" && (!e.Checked || e.Shortcut != "mod+8") {
			t.Fatalf("View › Text View = %+v, want checked with mod+8", e)
		}
	}
	app.ChooseMenu("View", "Next Section")
	app.Find("id:section-Text")

	app.Find("id:about")
	app.ChooseMenu("View", "Show Descriptions")
	if app.Exists("id:about") {
		t.Fatal("Show Descriptions did not hide the description")
	}
	app.ChooseMenu("View", "App Menu")
	app.Find("text:Last chosen: App Menu")
	app.Press("role:switch Show descriptions")
	app.Find("id:about")
	if app.Menu[0].Items[0].Role != core.MenuAbout || app.Menu[0].Items[0].Label != "About" {
		t.Fatalf("app menu starts with %+v", app.Menu[0].Items[0])
	}
}

func TestWindowCommands(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Window")
	app.Press("role:button Hide title bar")
	app.Press("role:button Minimize")
	app.Press("role:button Zoom")
	app.Press("role:button Full screen")
	app.Press("role:button Close")
	want := []core.WindowCommand{
		{Action: core.WindowSetTitleBar, TitleBar: core.TitleBarHidden},
		{Action: core.WindowMinimize}, {Action: core.WindowToggleMaximize}, {Action: core.WindowToggleFullScreen}, {Action: core.WindowClose},
	}
	if !slices.Equal(app.Window, want) {
		t.Fatalf("window commands = %+v, want %+v", app.Window, want)
	}

	// With the title bar hidden, the layout keeps clear of the controls.
	env := desktopEnv
	env.TitleBar = core.Edges{Top: 28, Left: 78}
	app.SetEnvironment(env)
	y := core.Points(0)
	for v := app.Find("id:nav-sections"); v != nil; v = v.Parent {
		y += v.Frame.Origin.Y
	}
	if y != 28 {
		t.Fatalf("the section list starts at %v, want below the controls' 28", y)
	}

	app.SetEnvironment(core.Environment{Idiom: core.IdiomTablet})
	app.Find("text:Desktop only")
}

func TestEverySectionBuilds(t *testing.T) {
	wide, phone := startDesktop(t), nsgotest.Start(t, Root)
	for _, s := range sections {
		wide.Press("id:nav-" + s.name)
		wide.Find("id:section-" + s.name)
		phone.Press("id:nav-" + s.name)
		phone.Find("id:section-" + s.name)
		phone.Back()
	}
	// Changing the appearance rebuilds the gallery around the same section.
	env := desktopEnv
	env.Dark = true
	wide.SetEnvironment(env)
	wide.Find("id:section-" + sections[len(sections)-1].name)
	if got := wide.Find("id:content").Parent.Style.Background; got != newTheme(true).background {
		t.Fatalf("background = %x, want the dark one", got)
	}
}

func TestGestures(t *testing.T) {
	// Tall enough to show the demos without scrolling, for the touches.
	app := startDesktop(t, nsgotest.Size(1100, 2400))
	app.Press("id:nav-Gestures")

	pad := app.Find("id:pointer-pad").Frame.Size
	app.Press("id:pointer-pad")
	want := fmt.Sprintf("press %.0f, %.0f · clicks 1 · keys none", pad.Width/2, pad.Height/2)
	if got := app.Find("id:press-status").Text; got != want {
		t.Fatalf("press status = %q, want %q", got, want)
	}
	app.RightClick("id:pointer-pad")
	if got := app.Find("id:menu-status").Text; !strings.HasPrefix(got, fmt.Sprintf("context menu %.0f,", pad.Width/2)) {
		t.Fatalf("menu status = %q", got)
	}

	app.Drag("id:responder-knob", core.Point{X: 20}, core.Point{X: 60, Y: 5})
	if got := app.Find("id:responder-status").Text; got != "the knob let go · 1 fingers" {
		t.Fatalf("responder status = %q", got)
	}
	if got := app.Find("id:responder-knob").Frame.Origin; got != (core.Point{X: 84, Y: 63}) {
		t.Fatalf("knob at %v after the drag", got)
	}
	app.Drag("id:responder-knob", core.Point{Y: 10}, core.Point{Y: 40})
	if got := app.Find("id:responder-status").Text; got != "the pad let go · 1 fingers" {
		t.Fatalf("responder status = %q after a vertical drag", got)
	}

	app.Pan("id:drag-card", core.Point{X: 20}, core.Point{X: 80, Y: 30})
	if got := app.Find("id:pan-status").Text; !strings.HasPrefix(got, "ended · translation 80, 30") {
		t.Fatalf("pan status = %q", got)
	}
	if got := app.Find("id:drag-card").Style.Transform; got != (core.Transform{}) {
		t.Fatalf("the card stayed at %+v after the pan ended", got)
	}

	before := app.Find("id:slider-value").Text
	app.Pan("id:slider", core.Point{X: 12}, core.Point{X: 2000})
	if got := app.Find("id:slider-value").Text; got != "Volume 100%" {
		t.Fatalf("slider = %q after dragging right, was %q", got, before)
	}
	if a := app.Find("id:slider").Props.PanActivation; a != (core.PanActivation{ActiveOffsetX: 10, FailOffsetY: 10}) {
		t.Fatalf("slider activation = %+v", a)
	}

	app.Pinch("id:pinch-target", 1.5, 2)
	app.Rotate("id:pinch-target", 0.5, 1.5707964)
	status := app.Find("id:pinch-status").Text
	if !strings.Contains(status, "zoom 2.00") || !strings.Contains(status, "turned 90°") {
		t.Fatalf("pinch status = %q", status)
	}
	if s := app.Find("id:pinch-target").Style.Transform.Scale; s != 2 {
		t.Fatalf("target scale = %v, want 2", s)
	}
}

func TestAccessibilitySection(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Accessibility")
	q := app.Find("role:adjustable Quantity")
	if q.Props.Hint == "" || q.Props.Value != (core.AccessibilityValue{Max: 10, Now: 2, Text: "2 items"}) {
		t.Fatalf("quantity props = %+v", q.Props)
	}
	app.AccessibilityAction("id:a11y-quantity", core.ActionIncrement)
	if v := app.Find("id:a11y-quantity").Props.Value; v.Now != 3 || v.Text != "3 items" {
		t.Errorf("after an increment the value is %+v", v)
	}

	if d := app.Find("id:a11y-disclosure"); !d.Props.Expandable || d.Props.Expanded {
		t.Fatalf("disclosure props = %+v, want collapsed", d.Props)
	}
	app.Press("id:a11y-disclosure")
	if !app.Find("id:a11y-disclosure").Props.Expanded {
		t.Error("the disclosure does not report it expanded")
	}

	app.AccessibilityAction("id:a11y-message", "archive")
	app.Find("text:Last action: archive")
	if m := app.Find("id:a11y-message"); m.Props.Label != "Ana Lunch on Friday?" || m.Props.Accessible != core.AccessibleYes {
		t.Errorf("message props = %+v, want one element read from its text", m.Props)
	}
	app.Press("role:button Change the price")
	if got := app.Find("id:a11y-card").Props.Label; got != "Apples $1.35 a kilo" {
		t.Errorf("card label = %q after the price changed", got)
	}

	if !app.Find("id:a11y-hidden").Props.ElementsHidden {
		t.Error("the decoration is not hidden")
	}
	app.Press("role:switch Hide the decoration")
	if app.Find("id:a11y-hidden").Props.ElementsHidden {
		t.Error("the switch did not show the decoration")
	}
	if app.Find("id:a11y-live").Props.LiveRegion != core.LiveRegionPolite {
		t.Error("the basket count is no live region")
	}
	app.Press("id:a11y-search")
	if f := app.AccessibilityFocus; f == nil || f.Props.TestID != "a11y-result" {
		t.Errorf("accessibility focus = %v, want the results", f)
	}
}

func TestStorageSection(t *testing.T) {
	app := startDesktop(t, nsgotest.Storage(map[string]string{"gallery.draft": "kept", "gallery.opened": "2"}))
	app.Press("id:nav-Storage")
	app.Find(`text:Kept: "kept"`)
	app.Find("text:opened 3 times")
	app.Type("id:draft", "new")
	app.Find(`text:Kept: "new"`)
	// A change from another window shows here too.
	app.Storage().SetItem("gallery.draft", "elsewhere")
	app.Settle()
	app.Find(`text:Kept: "elsewhere"`)
	app.Press("role:button Clear storage")
	if keys := app.Storage().Keys(); len(keys) != 0 {
		t.Fatalf("keys after clearing: %v", keys)
	}
}

func TestEditingFields(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Input")
	app.Type("id:code", "12345678")
	if v := app.Find("id:code"); v.Text != "123456" {
		t.Fatalf("code %q, want it cut to 6", v.Text)
	}
	app.Find("text:6 of 6 characters")
	// Return sends a message and keeps the field; a multiline note grows.
	app.Type("id:message", "hi")
	app.Submit("id:message")
	app.Find(`text:Sent 1: "hi"`)
	if v := app.Find("id:message"); v.Text != "" {
		t.Fatalf("message %q after sending", v.Text)
	}
	short := app.Find("id:note").Frame.Size.Height
	app.Type("id:note", "one")
	app.Submit("id:note")
	app.KeyPress("id:note", "t")
	if v := app.Find("id:note"); v.Text != "one\nt" || v.Frame.Size.Height <= short {
		t.Fatalf("note %q, %v tall (was %v)", v.Text, v.Frame.Size.Height, short)
	}
	app.Press("role:button Select all")
	if r := app.Find("id:words").Selection; r != (core.TextRange{Start: 0, End: 26}) {
		t.Fatalf("selection %v", r)
	}
	app.Select("id:words", 2, 4)
	app.Find(`text:Selection 2 to 4, last key ""`)
	app.KeyPress("id:words", "x")
	app.Find(`text:Selection 27 to 27, last key "x"`)
	// A value the app sets while an input method composes waits for its end.
	app.Compose("id:upper", "ni")
	app.Type("id:upper", "ni")
	if v := app.Find("id:upper"); v.Text != "ni" {
		t.Fatalf("upper %q during the composition", v.Text)
	}
	app.Compose("id:upper", "")
	if v := app.Find("id:upper"); v.Text != "NI" {
		t.Fatalf("upper %q", v.Text)
	}
}

func TestTextTransformsFitsAndReportsLines(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Text")
	app.Find("text:UPPERCASE")
	app.Find("text:Capitalize Each Word")
	if got := app.Find("text:1 line, ").Text; !strings.HasPrefix(got, "1 line, ") {
		t.Fatalf("reported lines = %q, want the shrunk text on one line", got)
	}
}

func TestWebViewMessages(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Web View")
	app.PostMessage("id:bridge", "clicked 1")
	app.Find("text:The page said: clicked 1")
	app.Press("role:button Post to the page")
	cmds := app.Find("id:bridge").WebCommands
	if len(cmds) != 1 || cmds[0] != (nsgotest.WebCommand{Command: nsgotest.WebPostMessage, Value: "hello #1"}) {
		t.Fatalf("commands = %v", cmds)
	}
	app.Navigate("id:site", core.WebNavigation{URL: "https://example.com/", Title: "Example", CanGoBack: true})
	app.Find("text:Example — https://example.com/ (loaded, back true, forward false)")
}

func TestVideoPlayback(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Video")
	if v := app.Find("id:player").Video; !v.Paused || !v.Muted || !v.Repeat || v.ResizeMode != core.Contain {
		t.Fatalf("config = %+v", v)
	}
	app.Play("id:player", core.VideoInfo{Duration: 5 * time.Second, Size: core.Size{Width: 640, Height: 360}})
	app.Find("text:loaded: 5s, 640×360")
	app.Press("role:button Play / pause")
	app.Press("role:button Cover")
	if v := app.Find("id:player").Video; v.Paused || v.ResizeMode != core.Cover {
		t.Fatalf("config = %+v", v)
	}
	app.Progress("id:player", 2*time.Second, 5*time.Second)
	app.Find("text:2s of 5s")
	app.Press("role:button Start over")
	if s := app.Find("id:player").Seeks; len(s) != 1 || s[0] != 0 {
		t.Fatalf("seeks = %v", s)
	}
}

func TestNativeViewAndFileDrop(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Native View")
	slider := app.Find("id:native-slider")
	if slider.Kind != nsgotest.KindNative || slider.Component != sliderComponent || slider.NativeProps["value"] != 0.4 {
		t.Fatalf("native view %+v", slider)
	}
	app.Press("text:Set 0.8")
	if v := app.Find("id:native-slider").NativeProps["value"]; v != 0.8 {
		t.Fatalf("value prop %v after Set 0.8", v)
	}
	app.Press("text:Reset command")
	if cmds := app.Find("id:native-slider").NativeCommands; len(cmds) != 1 || cmds[0].Name != "reset" {
		t.Fatalf("commands %+v", cmds)
	}
	app.Emit("id:native-slider", "change", 0.25)
	if got := app.Find("id:native-value").Text; !strings.HasPrefix(got, "value 0.25") {
		t.Fatalf("after a change event: %q", got)
	}

	app.Press("id:nav-File Drop")
	app.DropFiles("id:drop-zone", "file:///tmp/a%20b.txt", "blob:http://localhost/1234#notes.md")
	if got := app.Find("id:dropped").Text; got != "2 dropped: a b.txt, notes.md" {
		t.Fatalf("dropped = %q", got)
	}
}

func TestPlatformAnimations(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Animation")
	app.Press("text:Timing")
	app.Advance(300 * time.Millisecond)
	mid := app.Find("id:ball").Animated
	if !mid.Has(nsgotest.PropTranslateX) || mid.TranslateX <= 0 || mid.TranslateX >= 220 {
		t.Fatalf("ball halfway at %+v", mid)
	}
	app.Advance(time.Second)
	if x := app.Find("id:ball").Animated.TranslateX; x != 220 {
		t.Fatalf("ball ended at %v", x)
	}
	if got := app.Find("id:animated-state").Text; got != "Timing finished" {
		t.Fatalf("state = %q", got)
	}

	app.Pan("id:platform-card", core.Point{X: 30}, core.Point{X: 60, Y: 20})
	app.Advance(3 * time.Second)
	if v := app.Find("id:platform-card").Animated; v.TranslateX != 0 || v.TranslateY != 0 {
		t.Fatalf("the card did not spring home: %+v", v)
	}

	// The second pinch scales on from where the first left the tile.
	app.Pinch("id:platform-pinch", 1.5, 2)
	app.Pinch("id:platform-pinch", 1.5)
	app.Rotate("id:platform-pinch", 0.5, 1.5707964)
	if v := app.Find("id:platform-pinch").Animated; v.Scale != 3 || v.Rotate < 89.9 || v.Rotate > 90.1 {
		t.Fatalf("the tile is at %+v, want scale 3 and 90 degrees", v)
	}
}

func TestKeyboardAccessory(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Input")
	if app.Exists("id:accessory-bar") {
		t.Fatal("the bar shows before its field has focus")
	}
	app.Focus("id:accessory-field")
	app.Type("id:accessory-field", "hello")
	if got := app.Find("id:accessory-bar >> text:characters").Text; got != "5 characters" {
		t.Fatalf("bar says %q", got)
	}
	app.Press("id:accessory-bar >> text:Clear")
	if got := app.Find("id:accessory-field").Text; got != "" {
		t.Fatalf("field holds %q after Clear", got)
	}
}

func TestScrollingSection(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Scrolling")
	pages := app.Find("id:pages")
	if !pages.Scroll.Behavior.Paging || pages.Offset.X != 280 {
		t.Fatalf("pages: config %+v at %v, want paging from the second page", pages.Scroll, pages.Offset)
	}
	// A fling forward pages on, one page at a time.
	app.DragScroll("id:pages", core.Point{X: 330}, 900)
	if got := app.Find("id:pages").Offset.X; got != 560 {
		t.Fatalf("paged to %v, want 560", got)
	}
	app.Find("text:Page 3 of 5")

	app.Press("role:button One card per fling")
	app.Find("text:Each fling moves one card")
	if c := app.Find("id:cards").Scroll.Behavior; !c.DisableIntervalMomentum || c.SnapToInterval != 152 {
		t.Fatalf("cards: %+v", c)
	}

	app.DragScroll("id:feed", core.Point{Y: 100}, 0)
	feed := app.Find("id:feed")
	app.Find(fmt.Sprintf("text:Drag end at 100 · content %.0f × %.0f in %.0f × 220",
		feed.Content.Width, feed.Content.Height, feed.Frame.Size.Width))
	app.PullToRefresh("id:feed")
	if !app.Find("id:feed").Scroll.Refreshing {
		t.Fatal("the pull did not refresh")
	}
	app.Find("text:Refreshing…")
	app.Advance(1500 * time.Millisecond)
	app.Find("text:Refreshed 1 times")
	if app.Find("id:feed").Scroll.Refreshing {
		t.Fatal("still refreshing")
	}
	app.Press("role:button Scrolling on or off")
	app.DragScroll("id:feed", core.Point{Y: 300}, 0)
	if got := app.Find("id:feed").Offset.Y; got != 100 {
		t.Fatalf("a view that does not scroll moved to %v", got)
	}

	if z := app.Find("id:zoom").Scroll; !z.Zooms() || z.MinZoomScale != 0.5 || z.MaxZoomScale != 4 || !z.CenterContent {
		t.Fatalf("zoom: %+v", z)
	}
	app.PinchZoom("id:zoom", 2)
	app.Find("text:Zoom 2.0×")
	app.Press("role:button Zoom in on C")
	app.Find("text:Zoom 3.0×")
}

func TestListProps(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Lists")

	// Rows inserted above the viewport keep the visible ones in place.
	sections := app.Find("id:sections")
	app.Scroll("id:sections", core.Point{Y: 600})
	row := func(text string) *nsgotest.View { return app.Find("text:" + text).Parent.Parent }
	first := app.Query("text:Row #")[0]
	name := first.Text
	at := row(name).Frame.Origin.Y - sections.Offset.Y
	app.Press("text:Insert 5 above")
	if got := row(name).Frame.Origin.Y - sections.Offset.Y; got != at {
		t.Errorf("%s moved from %v to %v on screen after inserting above it", name, at, got)
	}
	// Section headers stick: one is pinned at the top.
	if len(sections.Sticky) == 0 {
		t.Error("no sticky cells")
	}
	if !strings.HasPrefix(app.Find("text:Viewing").Text, "Viewing") {
		t.Error("no viewable rows reported")
	}

	// Three columns of tiles.
	tile := func(n string) *nsgotest.View {
		for _, v := range app.Query("id:grid >> text:" + n) {
			if v.Text == n {
				return v.Parent.Parent
			}
		}
		t.Fatalf("no tile %s", n)
		return nil
	}
	one, two := tile("1"), tile("2")
	if one.Frame.Origin.Y != two.Frame.Origin.Y || two.Frame.Origin.X <= one.Frame.Origin.X {
		t.Errorf("tiles 1 and 2 at %+v and %+v, want side by side", one.Frame, two.Frame)
	}
	app.PullToRefresh("id:grid")
	if !app.Find("id:grid").Scroll.Refreshing {
		t.Error("pulling the grid did not refresh it")
	}

	// The chat starts at its end, the newest message at the bottom.
	chat := app.Find("id:chat")
	app.Press("text:Send")
	newest := app.Find("text:Message 3").Parent.Parent
	if bottom := newest.Frame.Origin.Y + newest.Frame.Size.Height - chat.Offset.Y; bottom < chat.Frame.Size.Height-20 {
		t.Errorf("the newest message ends at %v in a chat %v tall", bottom, chat.Frame.Size.Height)
	}

	// The page list is laid out at its full length in the page.
	if l := app.Find("id:page-list"); l.Kind != nsgotest.KindView || l.Frame.Size.Height < 200*40 {
		t.Errorf("the page list is a %v %v tall", l.Kind, l.Frame.Size.Height)
	}
	if app.Exists("text:Page row 150") {
		t.Error("a row far below the page's viewport is mounted")
	}
}

func TestCaretAndPaste(t *testing.T) {
	app := startDesktop(t, nsgotest.Size(1100, 3000))
	app.Press("id:nav-Input")
	app.Focus("id:caret-field")
	app.Type("id:caret-field", "hi @a")
	if !app.Exists("id:mentions") {
		t.Fatal("no suggestions after @")
	}
	app.Press("id:mention-grace")
	if got := app.Find("id:caret-field").Text; got != "hi @grace " {
		t.Fatalf("field = %q", got)
	}
	app.Paste("id:caret-field", core.PasteEvent{Text: "x", Files: []core.PastedFile{{MIME: "image/png", Data: []byte{1, 2}}}})
	if got := app.Find("id:paste-status").Text; got != "pasted 1 characters, a image/png of 2 bytes" {
		t.Fatalf("paste status = %q", got)
	}
}

// TestCanvasNodesAndAnimatedArguments presses a bar the canvas names as a
// node, which its hit test selects, and checks that bound drawing
// arguments move with the clock.
func TestCanvasNodesAndAnimatedArguments(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Canvas")
	bar := app.Find("label:Bar 2, 80%")
	if bar.Props.Role != core.RoleButton || bar.Props.Selected {
		t.Fatalf("bar 2 = %+v", bar.Props)
	}
	app.Press("id:bar-2")
	if !app.Exists("text:Bar 2 selected") || !app.Find("id:bar-2").Props.Selected {
		t.Fatalf("pressing bar 2 did not select it\n%s", app.Dump())
	}
	radius := func() float32 {
		for _, op := range app.Find("id:animated-drawing").Ops {
			if op.Kind == core.DrawArc {
				return op.R
			}
		}
		return -1
	}
	app.Advance(16 * time.Millisecond)
	start := radius()
	app.Advance(500 * time.Millisecond)
	if r := radius(); r <= start || r > 60 {
		t.Fatalf("radius %v after %v: want it growing toward 60", r, start)
	}
}

// TestMotionsKeyframesAndSnapping plays a motion preset and keyframes,
// resets them, and checks that a snapping header settles when its list
// rests and that the sticky titles follow it.
func TestMotionsKeyframesAndSnapping(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Animation")
	app.Press("text:shake")
	app.Advance(50 * time.Millisecond)
	if x := app.Find("id:motion-card").Animated.TranslateX; x >= 0 || x < -8 {
		t.Fatalf("shake at %v after 50 ms, want moving left", x)
	}
	app.Advance(time.Second)
	if x := app.Find("id:motion-card").Animated.TranslateX; x != 0 || !app.Exists("text:shake done") {
		t.Fatalf("shake ended at %v", x)
	}

	app.Press("text:Play keyframes")
	app.Advance(400 * time.Millisecond)
	if x := app.Find("id:keyframe-dot").Animated.TranslateX; x <= 0 || x >= 140 {
		t.Fatalf("keyframes at %v after 400 ms", x)
	}
	app.Press("text:Reset keyframes")
	app.Advance(16 * time.Millisecond)
	if x := app.Find("id:keyframe-dot").Animated.TranslateX; x != 0 {
		t.Fatalf("reset left the dot at %v", x)
	}

	if v := app.Find("id:glow-text").Animated; !v.Has(nsgotest.PropTextColor) {
		t.Fatal("the text color is not bound")
	}

	header := func() core.Points { return -app.Find("id:snap-header").Animated.TranslateY }
	app.DragScroll("id:snap-list", core.Point{Y: 30}, 0)
	if h := header(); h != 30 {
		t.Fatalf("the header hid %v, want 30", h)
	}
	app.Advance(300 * time.Millisecond)
	if h := header(); h != 0 {
		t.Fatalf("near the top the header settled at %v, want shown", h)
	}
	app.DragScroll("id:snap-list", core.Point{Y: 200}, 0)
	app.DragScroll("id:snap-list", core.Point{Y: 180}, 0)
	app.Advance(300 * time.Millisecond)
	if h := header(); h != 44 {
		t.Fatalf("further down the header settled at %v, want hidden", h)
	}
	list := app.Find("id:snap-list")
	// Its frame is 44 down the content: it moves to the offset itself.
	if s := list.StickyShift(app.Find("id:section-0").ID); s != 180-44 {
		t.Fatalf("the first title shifts %v, want to the top with the header gone", s)
	}
}

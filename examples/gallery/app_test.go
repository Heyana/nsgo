package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/nsgotest"
	"github.com/oneo-me/nsgo/ui"
)

var desktopEnv = core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en-US"}

// startDesktop runs the gallery in a wide desktop window.
func startDesktop(t *testing.T, opts ...nsgotest.Option) *nsgotest.App {
	app := nsgotest.Start(t, Root, append([]nsgotest.Option{nsgotest.Size(1100, 760)}, opts...)...)
	app.SetEnvironment(desktopEnv)
	return app
}

func TestSidebarSwitchesSections(t *testing.T) {
	app := startDesktop(t)
	if got := app.Find("id:title").Text; got != "Welcome" {
		t.Fatalf("first section = %q, want Welcome", got)
	}
	app.Press("id:nav-Controls")
	app.Find("id:section-Controls")
	if !app.Find("id:nav-Controls").Props.Selected || app.Find("id:nav-Welcome").Props.Selected {
		t.Fatal("the sidebar does not highlight the selected section")
	}
	app.Find("id:header >> text:Components /")
	if app.Title != "NSGo Gallery — Controls" {
		t.Fatalf("title = %q", app.Title)
	}
	app.Press("label:Agree")
	if !app.Find("role:checkbox Agree").Props.Checked {
		t.Fatal("the checkbox did not check")
	}
	app.Press("label:L")
	if !app.Find("role:radio L").Props.Checked {
		t.Fatal("the radio did not select")
	}

	app.Press("id:nav-Images")
	app.Load("id:logo", core.Size{Width: 192, Height: 192})
	if got := app.Find("id:logo").Frame.Size; got != (core.Size{Width: 96, Height: 96}) {
		t.Fatalf("logo = %v, want its declared size", got)
	}
}

func TestDocumentationSwitchesLanguages(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Documentation")
	app.Find("id:section-Documentation")
	app.Find("text:Getting started")
	app.Press("id:docs-zh")
	app.Find("text:开始")
	app.Press("id:docs-en")
	app.Find("text:Getting started")
}

// TestContentScrollsWithinTheWindow keeps the section's scroll view
// inside the window, so a long section scrolls rather than overflows.
func TestContentScrollsWithinTheWindow(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Layout")
	content := app.Find("id:content")
	if content.Frame.Size.Height > 760 || content.Content.Height <= content.Frame.Size.Height {
		t.Fatalf("content frame %v holds %v, want it within the window and scrolling", content.Frame, content.Content)
	}
}

func TestPhoneStack(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("id:index")
	app.Press("id:nav-Popups")
	app.Find("id:screen-Popups")
	app.Press("role:button Menu")
	app.Press("label:Paste")
	app.Find("text:Picked Paste")
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

func TestGestures(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Gestures")

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

func TestApplicationMenu(t *testing.T) {
	app := startDesktop(t)
	app.ChooseMenu("View", "Images")
	app.Find("id:section-Images")
	view := app.Menu[slices.IndexFunc(app.Menu, func(e core.MenuEntry) bool { return e.Label == "View" })]
	for _, e := range view.Items {
		if e.Label == "Images" && (!e.Checked || e.Shortcut != "mod+6") {
			t.Fatalf("View › Images = %+v, want checked with mod+6", e)
		}
	}
	app.ChooseMenu("View", "Next Section")
	app.Find("id:section-Icons")

	app.Find("id:about")
	app.ChooseMenu("View", "Show Descriptions")
	if app.Exists("id:about") {
		t.Fatal("Show Descriptions did not hide the description")
	}
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
	hidden := core.WindowCommand{Action: core.WindowSetTitleBar, TitleBar: core.TitleBarHidden, Height: headerHeight}
	want := []core.WindowCommand{
		// The gallery hides the title bar as it starts.
		hidden, hidden,
		{Action: core.WindowMinimize}, {Action: core.WindowToggleMaximize}, {Action: core.WindowToggleFullScreen}, {Action: core.WindowClose},
	}
	if !slices.Equal(app.Window, want) {
		t.Fatalf("window commands = %+v, want %+v", app.Window, want)
	}

	// The platform reports the hidden title bar: the sidebar's top row
	// clears the window controls and the header lines up with the content.
	env := desktopEnv
	env.TitleBar = core.Edges{Top: headerHeight, Left: 78}
	app.SetEnvironment(env)
	for _, id := range []string{"id:sidebar-header", "id:header"} {
		if bar := app.Find(id); bar.Props.WindowDrag != core.WindowDragOn || bar.Frame.Size.Height != headerHeight {
			t.Fatalf("%s = %+v", id, bar)
		}
	}
	x := core.Points(0)
	for v := app.Find("id:hide-sidebar"); v != nil; v = v.Parent {
		x += v.Frame.Origin.X
	}
	if x < 78 {
		t.Fatalf("the sidebar's toggle is at %v, under the window controls", x)
	}

	app.SetEnvironment(core.Environment{Idiom: core.IdiomTablet})
	app.Find("text:Desktop only")
}

func TestEverySectionBuilds(t *testing.T) {
	wide, phone := startDesktop(t), nsgotest.Start(t, Root)
	for _, s := range sections {
		if s.kind != kindNone {
			wide.Press("id:tab-" + string(s.kind))
		}
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
}

func TestOverlaysSection(t *testing.T) {
	app := startDesktop(t)
	app.ChooseMenu("View", "Overlays")
	app.Press("role:button Dialog")
	app.Find("text:Discard changes?")
	app.Press("text:Cancel")
	if app.Exists("text:Discard changes?") {
		t.Fatal("Cancel left the dialog open")
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

func TestTextViewStreams(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:tab-ui")
	app.Press("id:nav-Text View")
	app.Find("id:markdown >> role:header Text View")
	app.Find("id:html >> role:header An HTML page")
	if app.Exists("id:html >> text:Menu") {
		t.Error("a hidden element shows")
	}
	app.Press("id:stream")
	for range 200 {
		app.Advance(50 * time.Millisecond)
	}
	app.Find("id:streamed >> text:Done.")
	app.Find("id:streamed >> text:Never")
}

func TestWebViewMessages(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Web View")
	app.PostMessage("id:bridge", "clicked 1")
	app.Find("text:The page said: clicked 1")
	app.Press("role:button Post to the page")
	cmds := app.Find("id:bridge").WebCommands
	if len(cmds) != 1 || cmds[0] != (nsgotest.WebCommand{Command: core.WebPostMessage, Value: "hello #1"}) {
		t.Fatalf("commands = %v", cmds)
	}
	app.Navigate("id:site", core.WebNavigation{URL: "https://example.com/", Title: "Example", CanGoBack: true})
	app.Find("text:Example — https://example.com/ (loaded, back true, forward false)")
}

// TestIconBrowserFilters checks that a query rebuilds the bundled pack's
// browser rows, and that clearing it shows the whole catalog again.
func TestIconBrowserFilters(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Icons")
	app.Find("text:1879 of 1879 icons")
	app.Type("id:icon-search", "heart")
	found := 0
	for _, e := range iconCatalog {
		if strings.Contains(e.name, "heart") {
			found++
		}
	}
	app.Find(fmt.Sprintf("text:%d of 1879 icons", found))
	if !app.Exists("text:heart") {
		t.Fatal("the filtered browser shows no heart")
	}
	app.Type("id:icon-search", "no-such-icon")
	app.Find("text:No icon matches.")
	if app.Exists("text:heart") {
		t.Fatal("an emptied browser still shows icons")
	}
	app.Type("id:icon-search", "")
	app.Find("text:1879 of 1879 icons")
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

func TestWelcome(t *testing.T) {
	app := startDesktop(t)
	app.Find("id:welcome-name")
	app.Press("id:welcome-repository")
	if app.Clipboard != "https://"+repository {
		t.Fatalf("clipboard = %q, want the repository's link", app.Clipboard)
	}
	app.Find("id:toast >> text:Link copied")
	app.Press("id:welcome-search")
	app.Find("id:command-search")
	app.Back()
	if app.Exists("id:command-search") {
		t.Fatal("Escape did not close the palette")
	}
}

func TestCommandPalette(t *testing.T) {
	app := startDesktop(t)
	app.Key("mod+k")
	app.Find("id:palette >> role:header Elements")
	app.Type("id:command-search", "stor")
	if !app.Find("id:command-page:Storage").Props.Selected {
		t.Fatal("the matching page is not active")
	}
	app.Submit("id:command-search")
	app.Find("id:section-Storage")
	if app.Exists("id:command-search") {
		t.Fatal("running a command did not close the palette")
	}

	app.Press("id:search")
	app.Type("id:command-search", "dark appearance")
	app.Submit("id:command-search")
	if v, _ := app.Storage().GetItem(keyAppearance); v != "dark" {
		t.Fatalf("appearance = %q, want dark", v)
	}
	if got, want := app.Find("id:sidebar").Style.Background, sidebarColor(ui.DefaultTheme().Tokens(true)); got != want {
		t.Fatalf("sidebar = %x, want the dark one %x", got, want)
	}
}

func TestSettingsDialog(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Text")
	app.Find("id:about")
	app.Key("mod+,")
	app.Find("id:settings >> role:header General")
	app.Press("role:switch Page descriptions")
	if app.Exists("id:about") {
		t.Fatal("turning page descriptions off left the description")
	}

	app.Press("id:settings-category-appearance")
	app.Press("label:Appearance >> text:System")
	app.Press("label:Light")
	if v, _ := app.Storage().GetItem(keyAppearance); v != "light" {
		t.Fatalf("appearance = %q, want light", v)
	}

	app.Type("id:settings-search", "gutter")
	app.Find("id:setting-line-numbers")
	app.Press("id:settings-close")
	if app.Exists("id:settings") {
		t.Fatal("the close button did not close the settings")
	}
	app.Press("id:open-settings")
	if got := app.Find("id:settings-search").Text; got != "" {
		t.Fatalf("reopening kept the search %q", got)
	}
}

func TestPreferencesPersist(t *testing.T) {
	app := startDesktop(t, nsgotest.Storage(map[string]string{
		keyStart: "last", keySection: "Storage", keySidebar: "false", keyAppearance: "dark",
	}))
	app.Find("id:section-Storage")
	if app.Exists("id:sidebar") {
		t.Fatal("the sidebar shows though it was hidden")
	}
	app.Press("id:show-sidebar")
	app.Advance(300 * time.Millisecond)
	if w := app.Find("id:sidebar").Frame.Size.Width; w != sidebarWidth {
		t.Fatalf("sidebar = %v wide after opening, want %v", w, sidebarWidth)
	}
	if v, _ := app.Storage().GetItem(keySidebar); v != "true" {
		t.Fatalf("kept sidebar = %q", v)
	}
	app.Press("id:hide-sidebar")
	app.Advance(300 * time.Millisecond)
	if app.Exists("id:sidebar") {
		t.Fatal("the sidebar did not close")
	}

	// Opening on Welcome, the default, ignores the last page.
	fresh := startDesktop(t, nsgotest.Storage(map[string]string{keySection: "Storage"}))
	fresh.Find("id:welcome-name")
}

func TestComponentPages(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:tab-ui")
	app.Press("id:nav-Fields")
	app.Type("id:field-name", "Ada")
	app.Find("text:Hello, Ada.")
	app.Press("id:select-size")
	app.Press("label:Large")
	app.Find("text:Size large, no city yet.")

	app.Press("id:nav-Feedback")
	app.Press("role:button Saved")
	app.Find("id:toast >> text:Changes saved")

	app.Press("id:nav-Settings")
	app.Find("id:embedded-settings >> role:header Wi-Fi")
	app.Press("role:switch Hide sidebar")
	app.Find("id:embedded-settings >> role:header Sound")

	app.Press("id:nav-Command Palette")
	app.Press("id:open-files")
	app.Key("down")
	app.Submit("id:command-search")
	app.Find("text:Ran open.")

	app.Press("id:nav-Sidebar")
	app.Press("id:add-label")
	app.Press("id:mail-Label 3")
	if !app.Find("id:mail-Label 3").Props.Selected {
		t.Fatal("the new label is not selected")
	}
}

func TestCategories(t *testing.T) {
	app := startDesktop(t)
	if !app.Find("id:tab-base").Props.Selected || app.Exists("id:nav-Fields") {
		t.Fatal("the sidebar does not start on Base")
	}
	// Opening a UI page elsewhere moves the sidebar to its category.
	app.ChooseMenu("View", "Fields")
	if !app.Find("id:tab-ui").Props.Selected || app.Exists("id:nav-Text") {
		t.Fatal("the sidebar did not follow the page to UI")
	}
	app.Find("id:header >> text:UI / Components /")
	// The Theme page swaps the sample's theme.
	app.Press("id:nav-Theme")
	sample := func() core.Color { return app.Find("id:theme-sample").Style.Background }
	if sample() != ui.DefaultTheme().Light.Background {
		t.Fatal("the sample does not start in NSGo's theme")
	}
	app.Press("id:theme-teal")
	if sample() != tealTheme().Light.Background {
		t.Fatal("the sample did not take the teal theme")
	}
}

func TestNativeViewAndFileDrop(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Native View")
	slider := app.Find("id:native-slider")
	if slider.Kind != core.KindNative || slider.Component != sliderComponent || slider.NativeProps["value"] != 0.4 {
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
	if !mid.Has(core.PropTranslateX) || mid.TranslateX <= 0 || mid.TranslateX >= 220 {
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
}

func TestKeyboardAccessory(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Controls")
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

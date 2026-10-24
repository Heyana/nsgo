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

// TestSidebarGroups keeps the sidebar grouped as Onevra's: Overview, the
// icon library, then the component categories.
func TestSidebarGroups(t *testing.T) {
	app := startDesktop(t)
	groups := []struct{ heading, first string }{
		{"Overview", "nav-Welcome"},
		{"Icon library", "nav-Lucide"},
		{"Components", "nav-Buttons"},
		{"Composites", "nav-Sidebar"},
		{"Desktop", "nav-Window"},
		{"Theming", "nav-Theme"},
	}
	last := core.Points(-1)
	for _, group := range groups {
		root := app.Find("id:nav-group-" + group.heading)
		app.Find("id:nav-group-" + group.heading + " >> text:" + group.heading)
		app.Find("id:nav-group-" + group.heading + " >> id:" + group.first)
		if root.Frame.Origin.Y <= last {
			t.Fatalf("the %s group is above the group before it", group.heading)
		}
		last = root.Frame.Origin.Y
	}
	welcome := app.Find("id:nav-group-Overview >> id:nav-Welcome")
	docs := app.Find("id:nav-group-Overview >> id:nav-Documentation")
	if welcome.Frame.Origin.Y >= docs.Frame.Origin.Y {
		t.Fatal("Documentation does not follow Welcome in Overview")
	}
}

func TestSidebarSwitchesSections(t *testing.T) {
	app := startDesktop(t)
	if got := app.Find("id:title").Text; got != "Welcome" {
		t.Fatalf("first section = %q, want Welcome", got)
	}
	app.Press("id:nav-Buttons")
	app.Find("id:section-Buttons")
	if !app.Find("id:nav-Buttons").Props.Selected || app.Find("id:nav-Welcome").Props.Selected {
		t.Fatal("the sidebar does not highlight the selected section")
	}
	app.Find("id:header >> text:Components /")
	if app.Title != "NSGo Gallery — Buttons" {
		t.Fatalf("title = %q", app.Title)
	}
	before := app.Find("role:checkbox Accept the terms").Props.Checked
	app.Press("label:Accept the terms")
	if app.Find("role:checkbox Accept the terms").Props.Checked == before {
		t.Fatal("the checkbox did not toggle")
	}
	app.Press("label:L")
	if !app.Find("role:radio L").Props.Checked {
		t.Fatal("the radio did not select")
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
	app.Press("id:nav-Fields")
	content := app.Find("id:content")
	if content.Frame.Size.Height > 760 || content.Content.Height <= content.Frame.Size.Height {
		t.Fatalf("content frame %v holds %v, want it within the window and scrolling", content.Frame, content.Content)
	}
}

// TestSidebarScrollsWithinTheWindow keeps the sidebar's list inside the
// window, so its later sections scroll rather than overflow.
func TestSidebarScrollsWithinTheWindow(t *testing.T) {
	app := startDesktop(t)
	nav := app.Find("id:nav-sections")
	if bottom := nav.Frame.Origin.Y + nav.Frame.Size.Height; bottom > 760 {
		t.Fatalf("sidebar list ends at %v, past the window", bottom)
	}
	if nav.Content.Height <= nav.Frame.Size.Height {
		t.Fatalf("sidebar list frame %v holds %v, want it scrolling", nav.Frame, nav.Content)
	}
	app.Scroll("id:nav-sections", core.Point{Y: nav.Content.Height})
	if nav.Offset.Y == 0 {
		t.Fatal("the sidebar list did not scroll")
	}
	// The top entries scroll with the groups, not above them.
	for _, id := range []string{"nav-Welcome", "nav-Lucide", "nav-Documentation"} {
		inside := false
		for v := app.Find("id:" + id); v != nil; v = v.Parent {
			if v.Props.TestID == "nav-sections" {
				inside = true
				break
			}
		}
		if !inside {
			t.Fatalf("%s stays above the sidebar's scroll view", id)
		}
	}
}

func TestWindowMaterialTintsTheSidebar(t *testing.T) {
	env := desktopEnv
	env.WindowMaterial = core.WindowMaterialSidebar
	app := nsgotest.Start(t, Root, nsgotest.Size(1100, 760), nsgotest.Environment(env))
	want := ui.Fade(sidebarColor(ui.DefaultTheme().Tokens(false)), 0.72)
	if got := app.Find("id:sidebar").Style.Background; got != want {
		t.Fatalf("sidebar = %x, want the material's tint %x", got, want)
	}
	if got, want := app.Find("id:page").Style.Background, ui.DefaultTheme().Light.Background; got != want {
		t.Fatalf("page = %x, want the opaque background %x", got, want)
	}
}

func TestPhoneStack(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("id:index")
	app.Press("id:nav-Overlays")
	app.Find("id:screen-Overlays")
	app.Press("role:button Dialog")
	app.Find("text:Discard changes?")
	app.Back()
	if app.Exists("text:Discard changes?") {
		t.Fatal("back did not close the dialog")
	}
	app.Back()
	app.Find("id:index")

	// Widening the window keeps the section last open.
	app.Press("id:nav-Feedback")
	app.Resize(1000, 700)
	app.Find("id:section-Feedback")
}

func TestPhoneSystemHeader(t *testing.T) {
	ios := core.Environment{OS: core.OSIOS, Locale: "en-US", SafeArea: core.Edges{Top: 47, Bottom: 34},
		HeaderHeight: 44, LargeHeaderHeight: 96}
	app := nsgotest.Start(t, Root, nsgotest.Environment(ios))
	index := app.Find("id:index").Screen
	if index.Title != "Gallery" || index.HeaderHeight != 91 || index.Right == 0 || index.CanGoBack {
		t.Fatalf("index header = %+v", index)
	}
	app.Press("id:nav-Buttons")
	buttons := app.Find("id:screen-Buttons").Screen
	if !buttons.CanGoBack || buttons.BackTitle != "Gallery" || buttons.Background == 0 {
		t.Fatalf("section header = %+v", buttons)
	}
	app.HeaderBack("id:screen-Buttons")
	app.Find("id:index")
	if app.Exists("id:screen-Buttons") {
		t.Error("the header's back button did not pop the section")
	}
}

func TestApplicationMenu(t *testing.T) {
	app := startDesktop(t)
	app.ChooseMenu("View", "Fields")
	app.Find("id:section-Fields")
	view := app.Menu[slices.IndexFunc(app.Menu, func(e core.MenuEntry) bool { return e.Label == "View" })]
	for _, e := range view.Items {
		if e.Label == "Fields" && (!e.Checked || e.Shortcut != "mod+5") {
			t.Fatalf("View › Fields = %+v, want checked with mod+5", e)
		}
	}
	app.ChooseMenu("View", "Next Section")
	app.Find("id:section-Feedback")

	app.Find("id:about")
	app.ChooseMenu("View", "Show Descriptions")
	if app.Exists("id:about") {
		t.Fatal("Show Descriptions did not hide the description")
	}
	if app.Menu[0].Items[0].Role != core.MenuAbout || app.Menu[0].Items[0].Label != "About" {
		t.Fatalf("app menu starts with %+v", app.Menu[0].Items[0])
	}
}

func TestTitleBar(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Window")
	app.Press("role:button Hide title bar")
	app.Press("role:button Show title bar")
	hidden := core.WindowCommand{Action: core.WindowSetTitleBar, TitleBar: core.TitleBarHidden, Height: headerHeight}
	want := []core.WindowCommand{
		// The gallery hides the title bar as it starts.
		hidden, hidden, {Action: core.WindowSetTitleBar, TitleBar: core.TitleBarDefault},
	}
	if !slices.Equal(app.Window, want) {
		t.Fatalf("window commands = %+v, want %+v", app.Window, want)
	}

	// The platform reports the hidden title bar: the floating title row
	// clears the window controls and floats over both regions.
	env := desktopEnv
	env.TitleBar = core.Edges{Top: headerHeight, Left: 78}
	app.SetEnvironment(env)
	bar := app.Find("id:header")
	if bar.Props.WindowDrag != core.WindowDragOn || bar.Frame.Origin.Y != 0 || bar.Frame.Size.Height != headerHeight {
		t.Fatalf("header = %+v", bar)
	}
	x := core.Points(0)
	for v := app.Find("id:hide-sidebar"); v != nil; v = v.Parent {
		x += v.Frame.Origin.X
	}
	if x < 78+8 {
		t.Fatalf("the sidebar's toggle is at %v, under the window controls", x)
	}
	if bar.Style.Cursor != core.CursorDefault {
		t.Fatalf("the title row leaves the pointer to %v, not the default", bar.Style.Cursor)
	}
	// The regions paint the window's top edge behind the row; their
	// content keeps clear of it.
	surfaceY := func(v *nsgotest.View) core.Points {
		y := core.Points(0)
		for ; v != nil; v = v.Parent {
			y += v.Frame.Origin.Y
		}
		return y
	}
	if y := surfaceY(app.Find("id:search")); y < headerHeight {
		t.Fatalf("the search field starts at %v, under the title row", y)
	}
	if y := surfaceY(app.Find("id:content")); y != headerHeight {
		t.Fatalf("the content starts at %v, want below the title row", y)
	}
	// The title sits 16 points into the page beside the sidebar.
	surfaceX := func(v *nsgotest.View) core.Points {
		x := core.Points(0)
		for ; v != nil; v = v.Parent {
			x += v.Frame.Origin.X
		}
		return x
	}
	if got, want := surfaceX(app.Find("id:header-title")), sidebarWidth+16; got != want {
		t.Fatalf("the title starts at %v, want %v", got, want)
	}
	// A platform whose controls keep a taller bar gets a taller row.
	env.TitleBar.Top = 64
	app.SetEnvironment(env)
	if got := app.Find("id:header").Frame.Size.Height; got != 64 {
		t.Fatalf("header = %v tall, want the platform's 64", got)
	}

	app.SetEnvironment(core.Environment{Idiom: core.IdiomTablet})
	app.Find("text:Desktop only")
}

// TestLinuxWindowButtons keeps Linux's drawn controls working: the title
// row carries Onevra's buttons, and a maximized window shows restore.
func TestLinuxWindowButtons(t *testing.T) {
	app := startDesktop(t)
	env := desktopEnv
	env.OS = core.OSLinux
	env.TitleBar = core.Edges{Top: headerHeight}
	app.SetEnvironment(env)
	app.Find("id:window-controls")
	app.Press("id:window-controls-minimize")
	app.Press("id:window-controls-maximize")
	app.Press("id:window-controls-close")
	want := []core.WindowCommand{{Action: core.WindowMinimize}, {Action: core.WindowToggleMaximize}, {Action: core.WindowClose}}
	if got := app.Window[len(app.Window)-3:]; !slices.Equal(got, want) {
		t.Fatalf("window commands = %+v, want %+v", got, want)
	}
	env.Maximized = true
	app.SetEnvironment(env)
	app.Find("id:window-controls-restore")
	if app.Exists("id:window-controls-maximize") {
		t.Fatal("the maximize button stays while the window is maximized")
	}
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

func TestTextViewStreams(t *testing.T) {
	app := startDesktop(t)
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

// TestIconBrowserFilters checks that a query rebuilds the bundled pack's
// browser rows, and that clearing it shows the whole catalog again.
func TestIconBrowserFilters(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:nav-Lucide")
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
	app.Find("id:palette >> role:header Components")
	app.Type("id:command-search", "feedb")
	if !app.Find("id:command-page:Feedback").Props.Selected {
		t.Fatal("the matching page is not active")
	}
	app.Submit("id:command-search")
	app.Find("id:section-Feedback")
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
	app.Press("id:nav-Fields")
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
		keyStart: "last", keySection: "Feedback", keySidebar: "false", keyAppearance: "dark",
	}))
	app.Find("id:section-Feedback")
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
	fresh := startDesktop(t, nsgotest.Storage(map[string]string{keySection: "Feedback"}))
	fresh.Find("id:welcome-name")
}

func TestComponentPages(t *testing.T) {
	app := startDesktop(t)
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

func TestThemePageSwapsTheTheme(t *testing.T) {
	app := startDesktop(t)
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

// TestSidebarCollapsesUnderThePage keeps the sidebar's content its full
// width while it hides, so the page covers it, as Onevra's does.
func TestSidebarCollapsesUnderThePage(t *testing.T) {
	app := startDesktop(t)
	app.Press("id:hide-sidebar")
	app.Advance(110 * time.Millisecond)
	if w := app.Find("id:sidebar").Frame.Size.Width; w <= 0 || w >= sidebarWidth {
		t.Fatalf("sidebar = %v wide mid-animation, want between 0 and %v", w, sidebarWidth)
	}
	if w := app.Find("id:sidebar-content").Frame.Size.Width; w != sidebarWidth {
		t.Fatalf("sidebar content = %v wide mid-animation, want %v", w, sidebarWidth)
	}
}

func TestSidebarResizesByDraggingItsDivider(t *testing.T) {
	app := startDesktop(t)
	width := func() core.Points { return app.Find("id:sidebar").Frame.Size.Width }
	if width() != sidebarWidth {
		t.Fatalf("sidebar = %v wide, want %v", width(), sidebarWidth)
	}
	page := app.Find("id:page").Frame.Size.Width
	app.Pan("id:sidebar-split-divider", core.Point{X: 100})
	if width() != sidebarWidth+100 {
		t.Fatalf("sidebar = %v wide after the drag, want %v", width(), sidebarWidth+100)
	}
	if got := app.Find("id:page").Frame.Size.Width; got != page-100 {
		t.Fatalf("page = %v wide after the drag, want %v", got, page-100)
	}
	app.Press("id:sidebar-split-divider")
	app.Press("id:sidebar-split-divider")
	if width() != sidebarWidth {
		t.Fatalf("sidebar = %v wide after a double click, want the default %v", width(), sidebarWidth)
	}

	// The width the user chose comes back when the sidebar does.
	app.Pan("id:sidebar-split-divider", core.Point{X: 40})
	app.Press("id:hide-sidebar")
	app.Advance(300 * time.Millisecond)
	if app.Exists("id:sidebar") {
		t.Fatal("the sidebar did not close")
	}
	app.Press("id:show-sidebar")
	app.Advance(300 * time.Millisecond)
	if width() != sidebarWidth+40 {
		t.Fatalf("sidebar = %v wide after reopening, want the resized %v", width(), sidebarWidth+40)
	}
}

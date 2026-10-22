package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/nsgotest"
)

var desktopEnv = core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en-US"}

// startDesktop runs the gallery in a wide desktop window.
func startDesktop(t *testing.T) *nsgotest.App {
	app := nsgotest.Start(t, Root, nsgotest.Size(1100, 760))
	app.SetEnvironment(desktopEnv)
	return app
}

func TestSidebarSwitchesSections(t *testing.T) {
	app := startDesktop(t)
	if got := app.Find("id:title").Text; got != "Text" {
		t.Fatalf("first section = %q, want Text", got)
	}
	app.Press("id:nav-Controls")
	app.Find("id:section-Controls")
	if !app.Find("id:nav-Controls").Props.Selected || app.Find("id:nav-Text").Props.Selected {
		t.Fatal("the sidebar does not highlight the selected section")
	}
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
	app.ChooseMenu("View", "Canvas")
	app.Find("id:section-Canvas")
	view := app.Menu[slices.IndexFunc(app.Menu, func(e core.MenuEntry) bool { return e.Label == "View" })]
	for _, e := range view.Items {
		if e.Label == "Canvas" && (!e.Checked || e.Shortcut != "mod+8") {
			t.Fatalf("View › Canvas = %+v, want checked with mod+8", e)
		}
	}
	app.ChooseMenu("View", "Next Section")
	app.Find("id:section-Animation")

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
	want := []core.WindowCommand{
		{Action: core.WindowSetTitleBar, TitleBar: core.TitleBarHidden, Height: titleBarHeight},
		{Action: core.WindowMinimize}, {Action: core.WindowToggleMaximize}, {Action: core.WindowToggleFullScreen}, {Action: core.WindowClose},
	}
	if !slices.Equal(app.Window, want) {
		t.Fatalf("window commands = %+v, want %+v", app.Window, want)
	}

	// The platform reports the hidden title bar: the gallery draws its own.
	env := desktopEnv
	env.TitleBar = core.Edges{Top: titleBarHeight, Left: 78}
	app.SetEnvironment(env)
	bar := app.Find("id:titlebar")
	if bar.Props.WindowDrag != core.WindowDragOn || bar.Frame.Size.Height != titleBarHeight {
		t.Fatalf("title bar = %+v", bar)
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
}

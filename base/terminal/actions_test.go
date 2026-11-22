package terminal

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func actionSize(app *nsgotest.App, ref *Ref) core.Points {
	var size core.Points
	app.Post(func() { size = ref.FontSize() })
	app.Settle()
	return size
}

func actionInput(t *testing.T, app *nsgotest.App, term *Terminal, conn *recordingConn, want string) {
	t.Helper()
	const marker = "\x00action-barrier\x00"
	term.Send([]byte(marker))
	var got string
	app.WaitUntil(5*time.Second, func() bool { got += conn.take(); return strings.Contains(got, marker) })
	if got = strings.TrimSuffix(got, marker); got != want {
		t.Fatalf("input=%q, want %q", got, want)
	}
}

func TestTerminalKeybindOverridesZoomAndFocus(t *testing.T) {
	conn := newRecordingConn()
	term := newTerminal(t, Options{Conn: conn})
	term.Feed([]byte("alpha\r\nbeta> "))
	ref := &Ref{}
	var show *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		show = core.NewSignal(cx, true)
		return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}, Children: []core.Node{
			core.Show{When: show.Get, Then: func(*core.Scope) core.Node {
				return View{Terminal: term, Ref: ref, AutoFocus: true, TestID: "term", Style: core.Style{Width: core.Pt(400), Height: core.Pt(200)},
					Keybinds: []Keybind{{Key: "mod+=", Action: DecreaseFontSize, Amount: 2}, {Key: "ctrl+c", Action: CopyToClipboard}, {Key: "f3", Action: Ignore}, {Key: "f2", Action: IncreaseFontSize, Amount: 2}}}
			}},
			core.Input{TestID: "other"},
		}}
	}, nsgotest.Size(640, 400), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.WaitUntil(5*time.Second, func() bool { cols, _ := term.Size(); return cols > 0 && ref.v != nil })
	id, frame := app.Find("id:term").ID, app.Find("id:term").Frame
	cols, _ := term.Size()
	app.Key("mod+=")
	if got := actionSize(app, ref); got != 11 {
		t.Fatalf("custom binding did not override default zoom: %v", got)
	}
	app.Key("mod+-")
	if got := actionSize(app, ref); got != 10 {
		t.Fatalf("default decrease=%v", got)
	}
	app.Key("mod+0")
	if got := actionSize(app, ref); got != 13 {
		t.Fatalf("reset=%v", got)
	}
	app.Post(func() { ref.SetFontSize(20) })
	app.Settle()
	if after, _ := term.Size(); after >= cols {
		t.Fatalf("zoom did not resize grid: %d -> %d columns", cols, after)
	}
	if got := app.Find("id:term"); got.ID != id || got.Frame != frame {
		t.Fatalf("zoom replaced or resized its outer view: %v", got.Frame)
	}
	app.Post(func() { ref.Perform(SelectAll, 0) })
	app.Settle()
	app.Key("ctrl+c")
	if !strings.Contains(app.Clipboard, "alpha") {
		t.Fatalf("custom Copy instead of Ctrl+C input=%q", app.Clipboard)
	}
	app.Key("f3")
	actionInput(t, app, term, conn, "")
	app.Press("id:other")
	app.Key("f2")
	if got := actionSize(app, ref); got != 20 {
		t.Fatalf("unfocused binding changed font=%v", got)
	}
	app.Post(ref.Focus)
	app.Settle()
	app.Post(func() { ref.v.composing.Set("世") })
	app.Settle()
	app.Key("f2")
	if got := actionSize(app, ref); got != 20 {
		t.Fatalf("binding interrupted composition: %v", got)
	}
	app.Post(func() { ref.v.composing.Set("") })
	app.Settle()
	app.Key("f2")
	if got := actionSize(app, ref); got != 22 {
		t.Fatalf("binding did not resume after composition: %v", got)
	}
	app.Post(func() {
		ref.SetFontSize(1000)
		if got := ref.FontSize(); got != 255 {
			t.Errorf("upper zoom bound=%v", got)
		}
		ref.SetFontSize(-3)
		if got := ref.FontSize(); got != 1 {
			t.Errorf("lower zoom bound=%v", got)
		}
		ref.SetFontSize(core.Points(math.NaN()))
		if ref.Perform(SetFontSize, core.Points(math.Inf(1))) || ref.Perform(IncreaseFontSize, core.Points(math.Inf(1))) || ref.Perform(Action(255), 0) || ref.FontSize() != 1 {
			t.Error("invalid action or nonfinite size was accepted")
		}
		ref.SetFontSize(22)
	})
	app.Settle()
	app.Screenshot("terminal-actions/zoom")
	app.Post(func() { show.Set(false) })
	app.Settle()
	if ref.FontSize() != 0 || ref.Perform(IncreaseFontSize, 1) {
		t.Fatal("an unmounted ref still reached its old view")
	}
}

func TestTerminalDefaultContextMenuActionsAndLanguage(t *testing.T) {
	i18n.SetApp(i18n.NewCatalog("en", "zh-Hans"))
	defer i18n.SetApp(nil)
	conn := newRecordingConn()
	term := newTerminal(t, Options{Conn: conn})
	term.Feed([]byte("alpha\r\nbeta> \x1b[?2004h"))
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return View{Terminal: term, TestID: "term", AutoFocus: true,
			MenuStyle: core.Style{Background: core.RGB(0xffffff), Color: core.RGB(0), Padding: core.All(6)}, MenuItemStyle: core.Style{Padding: core.Symmetric(6, 10)},
			Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}}
	}, nsgotest.Size(400, 300), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en-US"}))
	app.RightClick("id:term-screen")
	for _, label := range []string{"Copy", "Paste", "Select All", "Clear"} {
		app.Find("role:menuitem " + label)
	}
	app.Screenshot("terminal-actions/menu")
	app.Press("id:term-menu-select-all")
	app.RightClick("id:term-screen")
	app.Press("id:term-menu-copy")
	if !strings.Contains(app.Clipboard, "alpha") {
		t.Fatalf("menu Copy=%q", app.Clipboard)
	}
	app.Post(func() { scope.SetClipboard("pasted") })
	app.Settle()
	app.RightClick("id:term-screen")
	app.Press("id:term-menu-paste")
	var got string
	app.WaitUntil(5*time.Second, func() bool { got += conn.take(); return strings.Contains(got, "\x1b[200~pasted\x1b[201~") })
	app.RightClick("id:term-screen")
	app.Press("id:term-menu-clear")
	if got := term.Text(); strings.Contains(got, "alpha") || !strings.Contains(got, "beta>") {
		t.Fatalf("Clear did not preserve prompt: %q", got)
	}
	app.SetEnvironment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "zh-CN"})
	app.RightClick("id:term-screen")
	for _, label := range []string{"复制", "粘贴", "全选", "清空"} {
		app.Find("role:menuitem " + label)
	}
	app.Screenshot("terminal-actions/menu-zh")
}

func TestTerminalSecondaryClickPolicyAndCustomMenu(t *testing.T) {
	conn := newRecordingConn()
	term := newTerminal(t, Options{Conn: conn})
	var policy *core.Signal[RightClickAction]
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		policy = core.NewSignal(cx, RightClickPaste)
		return core.Dynamic(func(*core.Scope) core.Node {
			return base.ContextMenu{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
				Trigger:  View{Terminal: term, RightClickAction: policy.Get(), TestID: "term", Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}},
				Children: []core.Node{base.MenuItem{Label: "App menu", Children: []core.Node{core.Text{Content: "App menu"}}}}}
		})
	}, nsgotest.Size(400, 300), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.Post(func() { scope.SetClipboard("secondary paste") })
	app.Settle()
	app.RightClick("id:term-screen")
	var got string
	app.WaitUntil(5*time.Second, func() bool { got += conn.take(); return got == "secondary paste" })
	if len(app.Query("role:menuitem")) != 0 {
		t.Fatal("Paste action also opened a menu")
	}
	app.Post(func() { policy.Set(RightClickIgnore) })
	app.Settle()
	app.RightClick("id:term-screen")
	app.Find("role:menuitem App menu")
}

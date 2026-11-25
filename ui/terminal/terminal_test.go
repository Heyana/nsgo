package terminal_test

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	baseterminal "github.com/nsgo-dev/nsgo/base/terminal"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
	uiterminal "github.com/nsgo-dev/nsgo/ui/terminal"
)

type pipeConn struct {
	*io.PipeReader
	show *io.PipeWriter
	mu   sync.Mutex
	sent bytes.Buffer
}

func newPipeConn() *pipeConn {
	r, w := io.Pipe()
	return &pipeConn{PipeReader: r, show: w}
}

func (c *pipeConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sent.Write(p)
}

func (c *pipeConn) take() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.sent.String()
	c.sent.Reset()
	return s
}

func (c *pipeConn) Close() error { return c.show.Close() }

func newTestTerminal(t *testing.T, conn *pipeConn) *baseterminal.Terminal {
	t.Helper()
	term, err := baseterminal.New(baseterminal.Options{Conn: conn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { term.Close() })
	return term
}

func iconStrokeColor(v *nsgotest.View) core.Color {
	for _, op := range v.Ops {
		if op.Kind == core.DrawStrokeColor {
			return op.Color
		}
	}
	return 0
}

var (
	desktopLight = core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en-US"}
	desktopDark  = core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en-US", Dark: true}
	phoneLight   = core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone, Locale: "en-US"}
	phoneDark    = core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone, Locale: "en-US", Dark: true}
)

func TestContextMenuStylingStatesAndThemeReactivity(t *testing.T) {
	conn := newPipeConn()
	term := newTestTerminal(t, conn)
	term.Feed([]byte("alpha\r\nbeta> \x1b[?2004h"))

	var scope *core.Scope
	var colors ui.Colors
	var menuFin ui.Finish
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		cx.SetTheme(nil)
		colors = ui.ColorsOf(cx)
		return core.View{
			Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			StyleFunc: func() core.Style {
				menuFin = ui.FinOf(cx).Menu()
				return core.Style{}
			},
			Children: []core.Node{
				uiterminal.View{
					Terminal:  term,
					Label:     "Terminal",
					TestID:    "term",
					AutoFocus: true,
					Style:     core.Style{Width: core.Pct(100), Height: core.Pct(100)},
				},
			},
		}
	}, nsgotest.Size(480, 320), nsgotest.Environment(desktopLight))
	app.Settle()

	lightMenuFin := menuFin
	app.RightClick("id:term-screen")
	menu := app.Find("role:menu")
	if menu.Style.Background != lightMenuFin.Background || menu.Style.BorderWidth != 1 || menu.Style.BorderColor != lightMenuFin.Border ||
		menu.Style.CornerRadius != ui.RadiusControl+2 || menu.Style.Effects == nil || len(menu.Style.Effects.Shadows) == 0 ||
		menu.Frame.Size.Width < 180 {
		t.Fatalf("light menu surface = %+v frame=%+v, want menu finish %+v", menu.Style, menu.Frame, lightMenuFin)
	}

	copyItem := app.Find("id:term-menu-copy")
	if !copyItem.Props.Disabled {
		t.Fatal("Copy should be disabled without a selection")
	}
	if copyItem.Style.Opacity != 0.45 {
		t.Fatalf("disabled Copy opacity = %v, want 0.45", copyItem.Style.Opacity)
	}
	if len(copyItem.Children) != 2 || copyItem.Children[0].Kind != nsgotest.KindCanvas ||
		copyItem.Children[0].Frame.Size != (core.Size{Width: 14, Height: 14}) {
		t.Fatalf("Copy leading icon children = %+v, want 14x14 icon before text", copyItem.Children)
	}
	var lightMuted core.Color
	app.Post(func() { lightMuted = colors.Muted() })
	app.Settle()
	if got := iconStrokeColor(copyItem.Children[0]); got != lightMuted {
		t.Fatalf("Copy icon color = %#x, want muted %#x", got, lightMuted)
	}

	pasteItem := app.Find("id:term-menu-paste")
	if len(pasteItem.Children) != 1 || pasteItem.Children[0].Kind != nsgotest.KindText {
		t.Fatalf("Paste should have no leading icon, got %+v", pasteItem.Children)
	}
	if pasteItem.Props.Disabled || pasteItem.Style.Opacity != 1 {
		t.Fatalf("Paste disabled=%v opacity=%v, want enabled at opacity 1", pasteItem.Props.Disabled, pasteItem.Style.Opacity)
	}
	pasteText := app.Find("id:term-menu-paste >> text:Paste")
	if pasteItem.Style.CornerRadius != ui.RadiusControl-2 || pasteItem.Frame.Size.Height < 26 ||
		pasteText.Frame.Origin.X != 8 {
		t.Fatalf("menu item layout style = %+v frame=%+v textFrame=%+v", pasteItem.Style, pasteItem.Frame, pasteText.Frame)
	}
	var lightText, lightHover core.Color
	app.Post(func() {
		lightText = colors.Text()
		lightHover = colors.Hover()
	})
	app.Settle()
	if pasteText.TextStyle.Color != lightText || pasteText.TextStyle.Font.Size != 13 {
		t.Fatalf("light item text = %+v, want color %#x size 13", pasteText.TextStyle, lightText)
	}

	app.Hover("id:term-menu-paste")
	if got := app.Find("id:term-menu-paste").Style.Background; got != lightHover {
		t.Fatalf("hovered item background = %#x, want %#x", got, lightHover)
	}

	// Switch appearance to dark while the menu stays open.
	app.SetEnvironment(desktopDark)
	var darkText, darkMuted, darkHover core.Color
	var darkMenuFin ui.Finish
	app.Post(func() {
		darkText = colors.Text()
		darkMuted = colors.Muted()
		darkHover = colors.Hover()
		darkMenuFin = ui.FinOf(scope).Menu()
	})
	app.Settle()
	menuDark := app.Find("role:menu")
	if menuDark.Style.Background != darkMenuFin.Background || menuDark.Style.Background == lightMenuFin.Background ||
		menuDark.Style.BorderColor != darkMenuFin.Border {
		t.Fatalf("dark menu surface = %+v, want %+v", menuDark.Style, darkMenuFin)
	}
	if got := iconStrokeColor(app.Find("id:term-menu-copy").Children[0]); got != darkMuted || got == lightMuted {
		t.Fatalf("dark Copy icon color = %#x, want %#x (light %#x)", got, darkMuted, lightMuted)
	}
	if got := app.Find("id:term-menu-paste >> text:Paste").TextStyle.Color; got != darkText || got == lightText {
		t.Fatalf("dark item text color = %#x, want %#x (light %#x)", got, darkText, lightText)
	}
	app.Hover("id:term-menu-paste")
	if got := app.Find("id:term-menu-paste").Style.Background; got != darkHover || got == lightHover {
		t.Fatalf("dark hovered item background = %#x, want %#x", got, darkHover)
	}

	// Switch theme to WarmDusk while keeping the view mounted.
	app.Post(func() { scope.SetTheme(ui.WarmDuskTheme()) })
	app.Settle()
	var warmMenuFin ui.Finish
	var warmText core.Color
	app.Post(func() {
		warmMenuFin = ui.FinOf(scope).Menu()
		warmText = ui.ColorsOf(scope).Text()
	})
	app.Settle()
	if got := app.Find("role:menu").Style.Background; got != warmMenuFin.Background || got == darkMenuFin.Background {
		t.Fatalf("WarmDusk menu background = %#x, want %#x", got, warmMenuFin.Background)
	}
	if got := app.Find("id:term-menu-paste >> text:Paste").TextStyle.Color; got != warmText {
		t.Fatalf("WarmDusk item text color = %#x, want %#x", got, warmText)
	}

	// Actions work through ui/terminal.View and enable Copy once text is selected.
	app.Press("id:term-menu-select-all")
	app.RightClick("id:term-screen")
	copyEnabled := app.Find("id:term-menu-copy")
	if copyEnabled.Props.Disabled || copyEnabled.Style.Opacity != 1 {
		t.Fatalf("Copy after Select All: disabled=%v opacity=%v", copyEnabled.Props.Disabled, copyEnabled.Style.Opacity)
	}
	app.Press("id:term-menu-copy")
	if !strings.Contains(app.Clipboard, "alpha") {
		t.Fatalf("clipboard = %q, want alpha", app.Clipboard)
	}
}

func TestKeyBarStylingLatchedModifiersAndThemeReactivity(t *testing.T) {
	conn := newPipeConn()
	term := newTestTerminal(t, conn)

	var scope *core.Scope
	var colors ui.Colors
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		cx.SetTheme(nil)
		colors = ui.ColorsOf(cx)
		return uiterminal.View{
			Terminal:  term,
			Label:     "Terminal",
			TestID:    "term",
			AutoFocus: true,
			Style:     core.Style{Width: core.Pct(100), Height: core.Pct(100)},
		}
	}, nsgotest.Size(390, 700), nsgotest.Environment(phoneLight))
	go conn.show.Write([]byte("ready> "))
	app.WaitUntil(5*time.Second, func() bool { return strings.Contains(term.Text(), "ready>") })

	readColors := func() (raised, line, surface, border, text, accent, onAccent core.Color) {
		app.Post(func() {
			raised = colors.Raised()
			line = colors.Line()
			surface = colors.Surface()
			border = colors.Border()
			text = colors.Text()
			accent = colors.AccentFill()
			onAccent = colors.OnAccent()
		})
		app.Settle()
		return
	}

	raised, line, surface, border, text, accent, onAccent := readColors()
	esc := app.Find("id:term-key-escape")
	keyBar := esc.Parent.Parent
	if keyBar.Style.Background != raised || keyBar.Style.Borders.Top != 1 || keyBar.Style.BorderColors.Top != line ||
		keyBar.Frame.Size.Height != 47 || esc.Parent.Frame.Origin.X != 6 || esc.Parent.Frame.Origin.Y != 7 {
		t.Fatalf("light key bar style = %+v frame=%+v rowFrame=%+v, want bg %#x top line %#x", keyBar.Style, keyBar.Frame, esc.Parent.Frame, raised, line)
	}
	if esc.Style.Background != surface || esc.Style.BorderWidth != 1 || esc.Style.BorderColor != border ||
		esc.Style.CornerRadius != 6 || esc.Frame.Size.Height != 34 {
		t.Fatalf("light key style = %+v frame=%+v, want bg %#x border %#x", esc.Style, esc.Frame, surface, border)
	}
	if escText := app.Find("id:term-key-escape >> text:Esc").TextStyle; escText.Color != text || escText.Font.Size != 15 {
		t.Fatalf("light key text style = %+v, want color %#x size 15", escText, text)
	}

	// Latched Ctrl and Alt look distinct from normal keys.
	ctrl := app.Find("id:term-key-ctrl")
	alt := app.Find("id:term-key-alt")
	if ctrl.Props.Pressed || alt.Props.Pressed || ctrl.Style.Background != surface || alt.Style.Background != surface {
		t.Fatalf("initial ctrl/alt should be unlatched: ctrl=%+v alt=%+v", ctrl, alt)
	}
	app.Press("id:term-key-ctrl")
	ctrlOn := app.Find("id:term-key-ctrl")
	if !ctrlOn.Props.Pressed || ctrlOn.Style.Background != accent || ctrlOn.Style.BorderColor != accent {
		t.Fatalf("latched ctrl = %+v, want Pressed and accent %#x", ctrlOn.Style, accent)
	}
	if ctrlText := app.Find("id:term-key-ctrl >> text:Ctrl").TextStyle.Color; ctrlText != onAccent {
		t.Fatalf("latched ctrl text color = %#x, want OnAccent %#x", ctrlText, onAccent)
	}
	// Alt remains unlatched while Ctrl is latched.
	if altOff := app.Find("id:term-key-alt"); altOff.Props.Pressed || altOff.Style.Background != surface {
		t.Fatalf("alt changed unexpectedly while only ctrl latched: %+v", altOff.Style)
	}

	app.Press("id:term-key-alt")
	altOn := app.Find("id:term-key-alt")
	if !altOn.Props.Pressed || altOn.Style.Background != accent ||
		app.Find("id:term-key-alt >> text:Alt").TextStyle.Color != onAccent {
		t.Fatalf("latched alt = %+v, want Pressed and accent %#x", altOn.Style, accent)
	}

	// Switch to dark appearance while Alt stays latched; colors update in place.
	app.SetEnvironment(phoneDark)
	dRaised, dLine, dSurface, dBorder, dText, dAccent, _ := readColors()
	if dRaised == raised || dSurface == surface || dText == text || dAccent == accent {
		t.Fatalf("expected dark palette to differ from light")
	}
	if got := app.Find("id:term-key-escape").Parent.Parent.Style; got.Background != dRaised || got.BorderColors.Top != dLine {
		t.Fatalf("dark key bar = %+v, want bg %#x line %#x", got, dRaised, dLine)
	}
	if got := app.Find("id:term-key-escape").Style; got.Background != dSurface || got.BorderColor != dBorder {
		t.Fatalf("dark key = %+v, want bg %#x border %#x", got, dSurface, dBorder)
	}
	if got := app.Find("id:term-key-escape >> text:Esc").TextStyle.Color; got != dText {
		t.Fatalf("dark key text color = %#x, want %#x", got, dText)
	}
	if got := app.Find("id:term-key-alt").Style; got.Background != dAccent || got.BorderColor != dAccent {
		t.Fatalf("dark latched alt = %+v, want accent %#x", got, dAccent)
	}

	// Switch to WarmDusk theme and check custom AccentFill on latched keys.
	app.Post(func() { scope.SetTheme(ui.WarmDuskTheme()) })
	app.Settle()
	wRaised, _, wSurface, _, _, wAccent, _ := readColors()
	if wAccent == dAccent {
		t.Fatalf("expected WarmDusk accent %#x to differ from default dark %#x", wAccent, dAccent)
	}
	if got := app.Find("id:term-key-escape").Parent.Parent.Style.Background; got != wRaised {
		t.Fatalf("WarmDusk key bar bg = %#x, want %#x", got, wRaised)
	}
	if got := app.Find("id:term-key-escape").Style.Background; got != wSurface {
		t.Fatalf("WarmDusk key bg = %#x, want %#x", got, wSurface)
	}
	if got := app.Find("id:term-key-alt").Style.Background; got != wAccent {
		t.Fatalf("WarmDusk latched alt bg = %#x, want %#x", got, wAccent)
	}

	// Typing a character consumes latched Ctrl+Alt and reverts both keys to unlatched look.
	app.Type("label:Terminal", "x")
	app.WaitUntil(2*time.Second, func() bool { return conn.take() != "" })
	if app.Find("id:term-key-ctrl").Props.Pressed || app.Find("id:term-key-alt").Props.Pressed {
		t.Fatal("ctrl/alt stayed latched after key input")
	}
	if got := app.Find("id:term-key-ctrl").Style.Background; got != wSurface {
		t.Fatalf("unlatched ctrl bg = %#x, want %#x", got, wSurface)
	}
}

func TestAppStylesOverrideUIStyles(t *testing.T) {
	conn := newPipeConn()
	term := newTestTerminal(t, conn)
	customMenuBg := core.RGB(0x123456)
	customItemColor := core.RGB(0x654321)
	customBarBg := core.RGB(0xabcdef)
	customKeyBg := core.RGB(0x223344)
	customKeyOnBg := core.RGB(0x556677)

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return uiterminal.View{
			Terminal:       term,
			Label:          "Terminal",
			TestID:         "term",
			AutoFocus:      true,
			MenuStyle:      core.Style{Background: customMenuBg},
			MenuItemStyle:  core.Style{Color: customItemColor},
			KeyBarStyle:    core.Style{Background: customBarBg},
			KeyStyle:       core.Style{Background: customKeyBg},
			KeyOnStyleFunc: func() core.Style { return core.Style{Background: customKeyOnBg} },
			Style:          core.Style{Width: core.Pct(100), Height: core.Pct(100)},
		}
	}, nsgotest.Size(390, 700), nsgotest.Environment(phoneLight))
	app.Settle()

	esc := app.Find("id:term-key-escape")
	if got := esc.Parent.Parent.Style.Background; got != customBarBg {
		t.Fatalf("key bar bg = %#x, want %#x", got, customBarBg)
	}
	if got := esc.Style.Background; got != customKeyBg {
		t.Fatalf("key bg = %#x, want %#x", got, customKeyBg)
	}
	app.Press("id:term-key-ctrl")
	if got := app.Find("id:term-key-ctrl").Style.Background; got != customKeyOnBg {
		t.Fatalf("latched key bg = %#x, want %#x", got, customKeyOnBg)
	}

	app.LongPress("id:term-screen")
	app.Advance(time.Second)
	app.WaitFor("role:menu", 2*time.Second)
	if got := app.Find("role:menu").Style.Background; got != customMenuBg {
		t.Fatalf("menu bg = %#x, want %#x", got, customMenuBg)
	}
	if got := app.Find("id:term-menu-paste >> text:Paste").TextStyle.Color; got != customItemColor {
		t.Fatalf("item color = %#x, want %#x", got, customItemColor)
	}
}

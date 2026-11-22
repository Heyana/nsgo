// Package terminal gives TSX nsgo/base's Terminal, base/terminal's View
// over a session of its own, which links libghostty-vt's binding: an app
// whose TSX uses it imports this package for its effect.
package terminal

import (
	"io"

	"github.com/nsgo-dev/nsgo/base/terminal"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
)

// Terminal is a terminal running a program, the user's shell by default,
// or connected to Conn. Its session's props start it, and a change of one
// starts another; the view's follow their props, the session going on.
type Terminal struct {
	// The session, as terminal.Options.
	Command     []string
	Dir         string
	Env         []string
	Conn        io.ReadWriteCloser
	Scrollback  int
	ImageLimit  int64
	Cursor      terminal.CursorStyle
	CursorBlink bool
	// OnTitle, OnExit and OnBell run as the program sets the title, ends
	// or rings the bell.
	OnTitle func(title string)
	OnExit  func(code int)
	OnBell  func()
	// Unavailable renders when its session cannot start, such as a browser session without Conn, with
	// why.
	Unavailable func(message string) core.Node

	// The view, as terminal.View: Theme and DarkTheme name Ghostty's
	// themes, DarkTheme being Theme when unset.
	Theme, DarkTheme                        func() string
	FontFamily                              func() string
	FontSize                                func() float64
	PaddingX, PaddingY                      func() float64
	AllowClipboardRead, AllowClipboardWrite func() bool
	PaddingColor                            func() terminal.PaddingColor
	MinimumContrast                         func() float64
	AutoFocus                               bool
	Transparent                             bool
	MouseReporting                          func() bool
	MouseScrollPrecision                    func() float64
	MouseScrollDiscrete                     func() float64
	Keybinds                                func() []terminal.Keybind
	RightClickAction                        func() terminal.RightClickAction
	MenuClass                               string
	MenuStyle                               core.Style
	MenuItemClass                           string
	MenuItemStyle                           core.Style
	NoKeyBar                                func() bool
	KeyBarKeys                              []string
	KeyBarClass                             string
	KeyBarStyle                             core.Style
	KeyClass                                string
	KeyStyle                                core.Style
	KeyOnClass                              string
	KeyOnStyle                              core.Style
	OnSizeChange                            func(cols, rows int)
	OnFocus, OnBlur                         func()
	Class, Label, TestID                    string
	Style                                   core.Style
	Ref                                     *Ref
}

// Ref is a Terminal's ref: the view's selection and clipboard, and the
// session's cursor, title and directory.
type Ref struct {
	view terminal.Ref
	t    *terminal.Terminal
}

func (r *Ref) Selected() bool  { return r.view.Selected() }
func (r *Ref) Copy()           { r.view.Copy() }
func (r *Ref) Paste()          { r.view.Paste() }
func (r *Ref) SelectAll()      { r.view.SelectAll() }
func (r *Ref) ClearSelection() { r.view.ClearSelection() }
func (r *Ref) Focus()          { r.view.Focus() }

func (r *Ref) FontSize() core.Points        { return r.view.FontSize() }
func (r *Ref) SetFontSize(size core.Points) { r.view.SetFontSize(size) }
func (r *Ref) Perform(action terminal.Action, amount core.Points) bool {
	return r.view.Perform(action, amount)
}

// SetCursor sets the cursor's shape until a program changes it.
func (r *Ref) SetCursor(c terminal.CursorStyle) {
	if r.t != nil {
		r.t.SetCursor(c)
	}
}

// Title is what the program last set as its title.
func (r *Ref) Title() string {
	if r.t == nil {
		return ""
	}
	return r.t.Title()
}

// Dir is the program's working directory, as the shell reports it.
func (r *Ref) Dir() string {
	if r.t == nil {
		return ""
	}
	return r.t.Dir()
}

// Send writes text to the program as typed.
func (r *Ref) Send(text string) {
	if r.t != nil {
		r.t.Send([]byte(text))
	}
}

// Render implements core.Node.
func (p Terminal) Render(cx *core.Scope) core.Node {
	// The terminal's goroutines report on the engine's.
	post := func(fn func()) {
		if fn != nil {
			cx.Post(fn)
		}
	}
	opts := terminal.Options{Command: p.Command, Dir: p.Dir, Env: p.Env, Conn: p.Conn, Scrollback: p.Scrollback, ImageLimit: p.ImageLimit, Cursor: p.Cursor, CursorBlink: p.CursorBlink}
	if p.OnTitle != nil {
		opts.OnTitle = func(s string) { post(func() { p.OnTitle(s) }) }
	}
	if p.OnExit != nil {
		opts.OnExit = func(code int) { post(func() { p.OnExit(code) }) }
	}
	if p.OnBell != nil {
		opts.OnBell = func() { post(p.OnBell) }
	}
	t, err := terminal.New(opts)
	if err != nil {
		if p.Unavailable != nil {
			return p.Unavailable(err.Error())
		}
		return core.Group{}
	}
	cx.Cleanup(func() { t.Close() })
	ref := p.Ref
	if ref == nil {
		ref = &Ref{}
	}
	ref.t = t
	read := func(f func() string) string {
		if f == nil {
			return ""
		}
		return f()
	}
	num := func(f func() float64) float64 {
		if f == nil {
			return 0
		}
		return f()
	}
	return core.Dynamic(func(cx *core.Scope) core.Node {
		var keybinds []terminal.Keybind
		if p.Keybinds != nil {
			keybinds = p.Keybinds()
		}
		rightClick := terminal.RightClickContextMenu
		if p.RightClickAction != nil {
			rightClick = p.RightClickAction()
		}
		var theme, dark *terminal.Theme
		paddingColor := terminal.PaddingBackground
		if p.PaddingColor != nil {
			paddingColor = p.PaddingColor()
		}
		if name := read(p.Theme); name != "" {
			theme, _ = terminal.GhosttyTheme(name)
		}
		dark = theme
		if name := read(p.DarkTheme); name != "" {
			dark, _ = terminal.GhosttyTheme(name)
		}
		return terminal.View{
			PaddingColor: paddingColor, MinimumContrast: num(p.MinimumContrast),
			Terminal: t, Theme: theme, DarkTheme: dark,
			Font:      core.Font{Family: read(p.FontFamily), Size: core.Points(num(p.FontSize))},
			Padding:   terminal.Padding{X: core.Points(num(p.PaddingX)), Y: core.Points(num(p.PaddingY))},
			AutoFocus: p.AutoFocus, AllowClipboardRead: p.AllowClipboardRead != nil && p.AllowClipboardRead(), AllowClipboardWrite: p.AllowClipboardWrite != nil && p.AllowClipboardWrite(), Transparent: p.Transparent,
			Keybinds: keybinds, RightClickAction: rightClick,
			MouseReporting:        p.MouseReporting,
			MouseScrollMultiplier: terminal.MouseScrollMultiplier{Precision: num(p.MouseScrollPrecision), Discrete: num(p.MouseScrollDiscrete)},
			MenuClass:             p.MenuClass, MenuStyle: p.MenuStyle, MenuItemClass: p.MenuItemClass, MenuItemStyle: p.MenuItemStyle,
			Class: p.Class, Style: p.Style, Label: p.Label, TestID: p.TestID,
			OnFocus: p.OnFocus, OnBlur: p.OnBlur, OnSizeChange: p.OnSizeChange, Ref: &ref.view,
			KeyBarKeys: p.KeyBarKeys, NoKeyBar: p.NoKeyBar != nil && p.NoKeyBar(),
			KeyBarClass: p.KeyBarClass, KeyBarStyle: p.KeyBarStyle,
			KeyClass: p.KeyClass, KeyStyle: p.KeyStyle, KeyOnClass: p.KeyOnClass, KeyOnStyle: p.KeyOnStyle,
		}
	})
}

func init() {
	tsx.Register("nsgo/base", map[string]core.Node{"Terminal": Terminal{}},
		tsx.Default("MouseReporting", true),
		tsx.Enum(map[string]terminal.Action{
			"copy_to_clipboard": terminal.CopyToClipboard, "paste_from_clipboard": terminal.PasteFromClipboard,
			"select_all": terminal.SelectAll, "clear_screen": terminal.ClearScreen,
			"increase_font_size": terminal.IncreaseFontSize, "decrease_font_size": terminal.DecreaseFontSize,
			"reset_font_size": terminal.ResetFontSize, "set_font_size": terminal.SetFontSize, "ignore": terminal.Ignore,
		}),
		tsx.Enum(map[string]terminal.RightClickAction{
			"context-menu": terminal.RightClickContextMenu, "paste": terminal.RightClickPaste,
			"copy": terminal.RightClickCopy, "copy-or-paste": terminal.RightClickCopyOrPaste, "ignore": terminal.RightClickIgnore,
		}),
		tsx.Enum(map[string]terminal.CursorStyle{"block": terminal.CursorBlock, "bar": terminal.CursorBar, "underline": terminal.CursorUnderline}),
		tsx.Enum(map[string]terminal.PaddingColor{"background": terminal.PaddingBackground, "extend": terminal.PaddingExtend, "extend-always": terminal.PaddingExtendAlways}),
		// It starts programs, past any capability, so it is not SandboxSafe.
		tsx.Event("OnTitle", "title"), tsx.Event("OnExit", "code"), tsx.Event("OnSizeChange", "cols", "rows"),
	)
	tsx.RegisterFunctions("nsgo/base", map[string]any{
		// terminalThemes are the names of Ghostty's themes.
		"terminalThemes": terminal.GhosttyThemes,
	})
}

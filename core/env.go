package core

import "time"

// Env is the environment as signals, one per part, so a binding runs
// again only when a part it read changes.
type Env struct {
	window                    *Signal[Size]
	safe, titleBar            *Signal[Edges]
	maximized                 *Signal[bool]
	keyboard                  *Signal[Points]
	keyboardDuration          *Signal[time.Duration]
	dark, reduce, rtl, reader *Signal[bool]
	fontScale, ratio          *Signal[float32]
	locale                    *Signal[string]
	os                        *Signal[OS]
	version                   *Signal[OSVersion]
	idiom                     *Signal[Idiom]
	appearance                *Signal[Appearance]
	// systemDark is the system's appearance, which a forced one hides.
	systemDark bool
	material   *Signal[WindowMaterial]
	appleKeys  *Signal[bool]
	tokens     *Signal[Tokens]
	// headers are the system header's heights, which only the tree reads.
	headers [2]Points
}

func newEnv(cx *Scope) *Env {
	return &Env{
		window:           NewSignal(cx, Size{}),
		safe:             NewSignal(cx, Edges{}),
		titleBar:         NewSignal(cx, Edges{}),
		maximized:        NewSignal(cx, false),
		keyboard:         NewSignal(cx, Points(0)),
		keyboardDuration: NewSignal(cx, 250*time.Millisecond),
		dark:             NewSignal(cx, false),
		reduce:           NewSignal(cx, false),
		rtl:              NewSignal(cx, false),
		reader:           NewSignal(cx, false),
		fontScale:        NewSignal(cx, float32(1)),
		ratio:            NewSignal(cx, float32(1)),
		locale:           NewSignal(cx, ""),
		os:               NewSignal(cx, OS("")),
		version:          NewSignal(cx, OSVersion{}),
		idiom:            NewSignal(cx, IdiomPhone),
		appearance:       NewSignal(cx, AppearanceSystem),
		appleKeys:        NewSignal(cx, false),
		material:         NewSignal(cx, WindowMaterialNone),
		tokens:           NewSignalFunc[Tokens](cx, Tokens{}, nil),
	}
}

// Env returns the app's environment.
func (cx *Scope) Env() *Env {
	rt := cx.rt
	if rt.env == nil {
		root := cx
		for root.parent != nil {
			root = root.parent
		}
		rt.env = newEnv(root)
	}
	return rt.env
}

func (e *Env) set(v Environment) {
	if v.FontScale <= 0 {
		v.FontScale = 1
	}
	if v.PixelRatio <= 0 {
		v.PixelRatio = 1
	}
	if v.KeyboardDuration <= 0 {
		v.KeyboardDuration = 250 * time.Millisecond
	}
	e.safe.Set(v.SafeArea)
	e.titleBar.Set(v.TitleBar)
	e.maximized.Set(v.Maximized)
	e.keyboard.Set(v.KeyboardHeight)
	e.keyboardDuration.Set(v.KeyboardDuration)
	e.systemDark = v.Dark
	e.dark.Set(e.shownDark())
	e.reduce.Set(v.ReduceMotion)
	e.rtl.Set(v.RTL)
	e.reader.Set(v.ScreenReader)
	e.fontScale.Set(v.FontScale)
	e.ratio.Set(v.PixelRatio)
	e.locale.Set(v.Locale)
	e.os.Set(v.OS)
	e.version.Set(v.OSVersion)
	e.appleKeys.Set(v.OS == OSMacOS || v.OS == OSIOS || v.AppleKeyboard)
	e.idiom.Set(v.Idiom)
	e.material.Set(v.WindowMaterial)
	e.headers = [2]Points{v.HeaderHeight, max(v.LargeHeaderHeight, v.HeaderHeight)}
}

// Window returns the surface's size.
func (e *Env) Window() Size { return e.window.Get() }

// SafeArea returns the insets system UI covers on each edge.
func (e *Env) SafeArea() Edges { return e.safe.Get() }

// KeyboardHeight returns how much of the bottom the keyboard covers.
func (e *Env) KeyboardHeight() Points { return e.keyboard.Get() }

// TitleBar returns what a hidden desktop title bar covers: the core's
// part of desktop.TitleBar, which apps use.
func (e *Env) TitleBar() Edges { return e.titleBar.Get() }

// Maximized reports whether the window is maximized or zoomed: the
// core's part of desktop.Maximized, which apps use.
func (e *Env) Maximized() bool { return e.maximized.Get() }

// KeyboardDuration returns how long the keyboard takes to move.
func (e *Env) KeyboardDuration() time.Duration { return e.keyboardDuration.Get() }

// shownDark is whether the app shows dark: as SetAppearance forces it,
// else as the system's appearance is.
func (e *Env) shownDark() bool {
	switch e.appearance.GetUntracked() {
	case AppearanceLight:
		return false
	case AppearanceDark:
		return true
	}
	return e.systemDark
}

// Dark reports whether the app shows dark: the appearance Scope.SetAppearance
// forces, else the system's.
func (e *Env) Dark() bool { return e.dark.Get() }

// Appearance returns the appearance Scope.SetAppearance forces, or
// AppearanceSystem.
func (e *Env) Appearance() Appearance { return e.appearance.Get() }

// Token returns the value an embedding host gives token name (Tokens) in
// the appearance the app shows, and whether it gives one.
func (e *Env) Token(name string) (string, bool) {
	t := e.tokens.Get()
	if e.dark.Get() {
		if v, ok := t.Dark[name]; ok {
			return v, true
		}
	}
	v, ok := t.Values[name]
	return v, ok
}

// FontScale returns the user's text size multiplier.
func (e *Env) FontScale() float32 { return e.fontScale.Get() }

// ReduceMotion reports whether the user asked for less motion.
func (e *Env) ReduceMotion() bool { return e.reduce.Get() }

// ScreenReader reports whether a screen reader is running.
func (e *Env) ScreenReader() bool { return e.reader.Get() }

// RTL reports whether the app lays out right to left.
func (e *Env) RTL() bool { return e.rtl.Get() }

// PixelRatio returns device pixels per point.
func (e *Env) PixelRatio() float32 { return e.ratio.Get() }

// Locale returns the user's language tag.
func (e *Env) Locale() string { return e.locale.Get() }

// OS returns the operating system.
func (e *Env) OS() OS { return e.os.Get() }

// OSVersion returns the system's version: Android's API level as its
// major, zero on the web.
func (e *Env) OSVersion() OSVersion { return e.version.Get() }

// Idiom returns the kind of device.
func (e *Env) Idiom() Idiom { return e.idiom.Get() }

// AppleKeys reports whether shortcuts follow Apple's conventions: "mod"
// is Command and labels use ⌘.
func (e *Env) AppleKeys() bool { return e.appleKeys.Get() }

// WindowMaterial returns the material behind the window's content, or
// WindowMaterialNone when the window is opaque: the core's part of
// desktop.ShowsMaterial, which apps use.
func (e *Env) WindowMaterial() WindowMaterial { return e.material.Get() }

// Desktop reports whether the device is a desktop.
func (e *Env) Desktop() bool { return e.idiom.Get() == IdiomDesktop }

// insetsKey is provided by screens and modals: the insets their content
// keeps clear of, below their headers.
var insetsKey = NewKey[func() Edges]("insets", nil)

// Insets returns what content under cx must keep clear of: the insets of
// the nearest screen or modal, or the surface's safe area.
func Insets(cx *Scope) Edges {
	if f := insetsKey.Use(cx); f != nil {
		return f()
	}
	return cx.Env().SafeArea()
}

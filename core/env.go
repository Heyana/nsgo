package core

import "time"

// Environment is what the platform reports about where the app runs. The
// surface size is not part of it: Tree.Resize reports that.
type Environment struct {
	// SafeArea is the part of each edge that system bars, notches and
	// rounded corners cover.
	SafeArea Edges
	// TitleBar is what a hidden desktop title bar still covers: its
	// height on top, its buttons on the left or right.
	TitleBar Edges
	// KeyboardHeight is how much of the bottom the software keyboard
	// covers; KeyboardDuration is how long it takes to move.
	KeyboardHeight   Points
	KeyboardDuration time.Duration
	Dark             bool
	// FontScale multiplies text sizes, as the user's text size setting
	// asks; 0 means 1.
	FontScale    float32
	ReduceMotion bool
	ScreenReader bool
	// RTL lays the app out right to left.
	RTL bool
	// PixelRatio is device pixels per point; 0 means 1.
	PixelRatio float32
	// Locale is a BCP 47 language tag, such as "en-US".
	Locale string
	OS     OS
	Idiom  Idiom
}

// OS names the operating system the app runs on.
type OS string

const (
	OSMacOS   OS = "macos"
	OSIOS     OS = "ios"
	OSAndroid OS = "android"
	OSWindows OS = "windows"
	OSLinux   OS = "linux"
	OSWeb     OS = "web"
)

// Idiom is the kind of device, which decides how components present.
type Idiom uint8

const (
	IdiomPhone Idiom = iota
	IdiomTablet
	IdiomDesktop
)

// Env is the environment as signals, one per part, so a binding runs
// again only when a part it read changes.
type Env struct {
	window                    *Signal[Size]
	safe, titleBar            *Signal[Edges]
	keyboard                  *Signal[Points]
	keyboardDuration          *Signal[time.Duration]
	dark, reduce, rtl, reader *Signal[bool]
	fontScale, ratio          *Signal[float32]
	locale                    *Signal[string]
	os                        *Signal[OS]
	idiom                     *Signal[Idiom]
}

func newEnv(cx *Scope) *Env {
	return &Env{
		window:           NewSignal(cx, Size{}),
		safe:             NewSignal(cx, Edges{}),
		titleBar:         NewSignal(cx, Edges{}),
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
		idiom:            NewSignal(cx, IdiomPhone),
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
	e.keyboard.Set(v.KeyboardHeight)
	e.keyboardDuration.Set(v.KeyboardDuration)
	e.dark.Set(v.Dark)
	e.reduce.Set(v.ReduceMotion)
	e.rtl.Set(v.RTL)
	e.reader.Set(v.ScreenReader)
	e.fontScale.Set(v.FontScale)
	e.ratio.Set(v.PixelRatio)
	e.locale.Set(v.Locale)
	e.os.Set(v.OS)
	e.idiom.Set(v.Idiom)
}

// Window returns the surface's size.
func (e *Env) Window() Size { return e.window.Get() }

// SafeArea returns the insets system UI covers on each edge.
func (e *Env) SafeArea() Edges { return e.safe.Get() }

// KeyboardHeight returns how much of the bottom the keyboard covers.
func (e *Env) KeyboardHeight() Points { return e.keyboard.Get() }

// TitleBar returns what a hidden desktop title bar covers.
func (e *Env) TitleBar() Edges { return e.titleBar.Get() }

// KeyboardDuration returns how long the keyboard takes to move.
func (e *Env) KeyboardDuration() time.Duration { return e.keyboardDuration.Get() }

// Dark reports whether the system appearance is dark.
func (e *Env) Dark() bool { return e.dark.Get() }

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

// Idiom returns the kind of device.
func (e *Env) Idiom() Idiom { return e.idiom.Get() }

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

package core

// Props only one platform has are grouped in a field named after it, as
// NSH's ios, android and desktop objects: other platforms ignore them.
// What two platforms have stays a field of the element.

// IOSViewProps are a View's props only iOS has.
type IOSViewProps struct {
	// BackdropMaterial is the system material iOS draws the view's backdrop blur with. macOS draws its closest material for one other than BackdropUltraThin, and blurs by the radius for that.
	BackdropMaterial BackdropMaterial
}

// AndroidViewProps are a View's props only Android has.
type AndroidViewProps struct {
	// Ripple spreads from the touch while the view is pressed; it shows
	// on views that take presses. Keep one value for the view's life.
	Ripple *Ripple
	// Elevation lifts the view, which casts the system's shadow below it.
	Elevation Points
}

// DesktopViewProps are a View's props only the desktops have.
type DesktopViewProps struct {
	// WindowDrag makes the view a handle that moves the desktop window.
	WindowDrag WindowDrag
}

// WebViewProps are a View's props only the web has.
type WebViewProps struct {
	// Href gives a link its browser address. Ordinary presses still use
	// OnPress; a middle click or modified click opens the browser's link.
	Href     string
	HrefFunc func() string
}

func (p WebViewProps) href(owner *Scope) string {
	// A confined plugin cannot turn a native browser link into a path
	// around its host's OpenURL access checks. Its OnPress remains scoped.
	if p.HrefFunc != nil && p.Href != "" {
		panic("core.View.Web sets both Href and HrefFunc, which replaces it")
	}
	if confinement(owner) != nil {
		return ""
	}
	if p.HrefFunc != nil {
		return p.HrefFunc()
	}
	return p.Href
}

// IOSInputProps are an Input's props only iOS has.
type IOSInputProps struct {
	// KeyboardAppearance picks iOS's light or dark keyboard.
	KeyboardAppearance KeyboardAppearance
}

// IOSModalProps are a Modal's props only iOS has.
type IOSModalProps struct {
	// SupportedOrientations are the orientations iOS allows while the
	// modal shows; 0 keeps the app's.
	SupportedOrientations Orientation
}

// IOSScreenProps are a Screen's props only iOS has.
type IOSScreenProps struct {
	// HeaderRightSystemItem shows UIKit's own bar button, in the system's
	// look, at the header's end in place of HeaderRight's views. A press
	// on it presses HeaderRight's first view that takes presses, so keep
	// one there; it is what the other platforms show.
	HeaderRightSystemItem HeaderSystemItem
	// HeaderRightSymbols shows UIKit's own bar buttons, each on glass of
	// its own, in place of HeaderRight's views, none while it returns none.
	// A press on one presses that of HeaderRight's views that take presses,
	// in order: keep one there for each.
	HeaderRightSymbols func() []HeaderSymbol
}

// AndroidScreenProps are base.NavPage's Android header options, carried by Screen.
type AndroidScreenProps struct {
	// HeaderRightWithLargeTitle places HeaderRight beside the expanded title, moving it into the compact bar as the large title collapses.
	HeaderRightWithLargeTitle bool
}

// HeaderSymbol is a button of iOS's header: Name is its SF Symbol, and Label
// what assistive technology reads.
type HeaderSymbol struct{ Name, Label string }

// AndroidModalProps are a Modal's props only Android has.
type AndroidModalProps struct {
	// StatusBarTranslucent lets the content reach under Android's status
	// bar; otherwise the modal starts below it.
	StatusBarTranslucent bool
}

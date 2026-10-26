package core

// Props only one platform has are grouped in a field named after it, as
// NSH's ios, android and desktop objects: other platforms ignore them.
// What two platforms have stays a field of the element.

// IOSViewProps are a View's props only iOS has.
type IOSViewProps struct {
	// BackdropMaterial is the system material iOS draws the view's
	// backdrop blur with.
	BackdropMaterial BackdropMaterial
}

// DesktopViewProps are a View's props only the desktops have.
type DesktopViewProps struct {
	// WindowDrag makes the view a handle that moves the desktop window.
	WindowDrag WindowDrag
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

// AndroidModalProps are a Modal's props only Android has.
type AndroidModalProps struct {
	// StatusBarTranslucent lets the content reach under Android's status
	// bar; otherwise the modal starts below it.
	StatusBarTranslucent bool
}

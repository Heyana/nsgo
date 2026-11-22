package nsgotest

import "github.com/nsgo-dev/nsgo/internal/driver"

// ViewKind identifies the native view a mounted model represents.
// Its zero value describes no known view kind.
type ViewKind uint8

const (
	// KindView identifies a container.
	KindView ViewKind = iota + 1
	// KindText identifies prepared text.
	KindText
	// KindInput identifies a native text editor.
	KindInput
	// KindScroll identifies a scroll view.
	KindScroll
	// KindImage identifies an image.
	KindImage
	// KindModal identifies a presented layer.
	KindModal
	// KindCanvas identifies a recorded drawing.
	KindCanvas
	// KindWeb identifies a web view.
	KindWeb
	// KindVideo identifies a video player.
	KindVideo
	// KindNative identifies an app-registered native component.
	KindNative
	// KindShader identifies a fragment shader.
	KindShader
	// KindGPU identifies a 3D GPU surface.
	KindGPU
)

// viewKinds maps the mount protocol to the kinds the test model owns.
var viewKinds = map[driver.ViewKind]ViewKind{
	driver.KindView:   KindView,
	driver.KindText:   KindText,
	driver.KindInput:  KindInput,
	driver.KindScroll: KindScroll,
	driver.KindImage:  KindImage,
	driver.KindModal:  KindModal,
	driver.KindCanvas: KindCanvas,
	driver.KindWeb:    KindWeb,
	driver.KindVideo:  KindVideo,
	driver.KindNative: KindNative,
	driver.KindShader: KindShader,
	driver.KindGPU:    KindGPU,
}

func viewKind(kind driver.ViewKind) ViewKind { return viewKinds[kind] }

func (kind ViewKind) driverKind() driver.ViewKind {
	switch kind {
	case KindView:
		return driver.KindView
	case KindText:
		return driver.KindText
	case KindInput:
		return driver.KindInput
	case KindScroll:
		return driver.KindScroll
	case KindImage:
		return driver.KindImage
	case KindModal:
		return driver.KindModal
	case KindCanvas:
		return driver.KindCanvas
	case KindWeb:
		return driver.KindWeb
	case KindVideo:
		return driver.KindVideo
	case KindNative:
		return driver.KindNative
	case KindShader:
		return driver.KindShader
	case KindGPU:
		return driver.KindGPU
	}
	return 0
}

// String returns the name used in view dumps and diagnostics.
func (kind ViewKind) String() string { return kind.driverKind().String() }

//go:build ios && nsgotest

package ios

import (
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// RetainInputForTest keeps an input's UIKit view alive as an outgoing transition can. The first closure lays it out once it is unmounted and reports whether its editor has also been unregistered; the second releases the retained view. Neither closure accesses UIKit off main.
func RetainInputForTest(testID string) (func() bool, func()) {
	var native, editor objc.ID
	onMainSync(func() {
		for _, v := range runner.views {
			if v != nil && v.input != nil && v.props.TestID == testID {
				native, editor = v.native.Send(sel("retain")), v.input.field
				break
			}
		}
	})
	if native == 0 {
		return nil, nil
	}
	return func() bool {
		var gone bool
		onMainSync(func() {
			if views[native] == nil {
				// Deliver the real Objective-C callback after unmount, independent of UIKit's transition timing.
				native.Send(sel("layoutSubviews"))
				gone = views[editor] == nil
			}
		})
		return gone
	}, func() { onMainSync(func() { native.Send(selRelease) }) }
}

// InputBaselinesForTest draws a multiline input's view and returns the baselines of the lines of dark ink in it, from its top, with those a core.Text of the input's style puts the text it shows on, its value or else its placeholder, in its content box. ok is false while the input has not mounted.
func InputBaselinesForTest(testID string) (got, want []float64, ok bool) {
	onMainSync(func() {
		var v *view
		for _, w := range runner.views {
			if w != nil && w.input != nil && w.props.TestID == testID {
				v = w
				break
			}
		}
		if v == nil {
			return
		}
		ok = true
		v.native.Send(sel("layoutIfNeeded"))
		size := objc.Send[apple.CGRect](v.native, sel("bounds")).Size
		got = inkBaselines(v.native.Send(sel("layer")), size.Width, size.Height)
		c := &v.input.fs.Config
		text := v.input.text()
		if text == "" {
			text = c.Placeholder
		}
		p := apple.TextEngine{}.Prepare(&driver.AttributedText{Text: text, Style: c.Style})
		l := p.Layout(p.Measure(v.inset.Size.Width)).(*apple.Layout)
		for _, line := range l.TextLines() {
			b := line.Bounds
			want = append(want, float64(v.inset.Origin.Y+b.Origin.Y+line.Ascender+(b.Size.Height-line.Ascender-line.Descender)/2))
		}
		l.Release()
		p.Release()
	})
	return got, want, ok
}

// InputAccessibilityState holds the native UIAccessibility state of an input's editor and visual placeholder label.
type InputAccessibilityState struct {
	EditorHint          string
	HasPlaceholderLabel bool
	LabelIsElement      bool
	LabelElementsHidden bool
	ContainerIsElement  bool
}

// InputAccessibilityForTest reads the native UIAccessibility state of the mounted input with testID.
func InputAccessibilityForTest(testID string) (state InputAccessibilityState, ok bool) {
	onMainSync(func() {
		for _, v := range runner.views {
			if v == nil || v.input == nil || v.props.TestID != testID {
				continue
			}
			ok = true
			state.ContainerIsElement = objc.Send[bool](v.native, sel("isAccessibilityElement"))
			state.EditorHint = apple.GoString(v.input.field.Send(sel("accessibilityHint")))
			if v.input.label != 0 {
				state.HasPlaceholderLabel = true
				state.LabelIsElement = objc.Send[bool](v.input.label, sel("isAccessibilityElement"))
				state.LabelElementsHidden = objc.Send[bool](v.input.label, sel("accessibilityElementsHidden"))
			}
			return
		}
	})
	return state, ok
}

// inkBaselines draws layer's tree on white, w by h points at 3 pixels a point, and returns the bottom of each band of rows with dark ink, in points: where capitals stand.
func inkBaselines(layer objc.ID, w, h float64) []float64 {
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_GLOBAL)
	if err != nil {
		return nil
	}
	var (
		colorSpace func() uintptr
		bitmap     func(data unsafe.Pointer, w, h, bits, row, space uintptr, info uint32) uintptr
		translate  func(ctx uintptr, x, y float64)
		scale      func(ctx uintptr, x, y float64)
	)
	purego.RegisterLibFunc(&colorSpace, cg, "CGColorSpaceCreateDeviceRGB")
	purego.RegisterLibFunc(&bitmap, cg, "CGBitmapContextCreate")
	purego.RegisterLibFunc(&translate, cg, "CGContextTranslateCTM")
	purego.RegisterLibFunc(&scale, cg, "CGContextScaleCTM")
	const k = 3
	pw, ph := int(w*k), int(h*k)
	pix := make([]byte, pw*ph*4)
	for i := range pix {
		pix[i] = 0xff
	}
	space := colorSpace()
	defer apple.Release(space)
	const premultipliedLast = 1
	ctx := bitmap(unsafe.Pointer(&pix[0]), uintptr(pw), uintptr(ph), 8, uintptr(pw*4), space, premultipliedLast)
	defer apple.Release(ctx)
	// Layers draw with y down from the top, as the bitmap's rows go.
	translate(ctx, 0, float64(ph))
	scale(ctx, k, -k)
	layer.Send(sel("renderInContext:"), ctx)
	inked := func(y int) bool {
		for x := range pw {
			if p := pix[(y*pw+x)*4:]; p[0] < 100 && p[1] < 100 && p[2] < 100 {
				return true
			}
		}
		return false
	}
	var out []float64
	for y := range ph {
		if inked(y) && (y+1 == ph || !inked(y+1)) {
			out = append(out, float64(y+1)/k)
		}
	}
	return out
}

//go:build darwin

package apple

import (
	"runtime"

	"github.com/ebitengine/purego/objc"
)

var pipPlayerHostClass objc.Class

// AVPlayer can vend video to only one layer at a time. Present the second
// layer in the controller's public contentOverlayView, between its own
// video and its controls; putting it behind the controller would obscure
// the frames with the controller's now-empty video surface. A flexible
// view follows the controls into full screen and lays the layer out.
func newPiPPlayerHost(overlay, layer objc.ID) objc.ID {
	if pipPlayerHostClass == 0 {
		name, layout := "UIView", "layoutSubviews"
		var methods []objc.MethodDef
		if runtime.GOOS != "ios" {
			name, layout = "NSView", "layout"
			methods = append(methods,
				objc.MethodDef{Cmd: sel("isFlipped"), Fn: func(objc.ID, objc.SEL) bool { return true }},
				objc.MethodDef{Cmd: sel("hitTest:"), Fn: func(objc.ID, objc.SEL, CGPoint) objc.ID { return 0 }})
		}
		methods = append(methods,
			objc.MethodDef{Cmd: sel(layout), Fn: func(self objc.ID, cmd objc.SEL) {
				Super(self, cmd)
				parent := self.Send(sel("layer"))
				layers := parent.Send(sel("sublayers"))
				if objc.Send[uint](layers, sel("count")) > 0 {
					WithoutAnimation(func() {
						layers.Send(sel("objectAtIndex:"), uint(0)).Send(sel("setFrame:"), objc.Send[CGRect](self, sel("bounds")))
					})
				}
			}})
		pipPlayerHostClass = RegisterClass("NsgoPiPPlayerHost", Class(name), nil, methods)
	}
	host := objc.ID(pipPlayerHostClass).Send(sel("alloc")).Send(sel("initWithFrame:"), objc.Send[CGRect](overlay, sel("bounds")))
	if runtime.GOOS == "ios" {
		host.Send(sel("setUserInteractionEnabled:"), false)
	} else {
		host.Send(sel("setWantsLayer:"), true)
	}
	host.Send(sel("setAutoresizingMask:"), uint(2|16))
	layer.Send(sel("setFrame:"), objc.Send[CGRect](host, sel("bounds")))
	host.Send(sel("layer")).Send(sel("addSublayer:"), layer)
	overlay.Send(sel("addSubview:"), host)
	return host
}

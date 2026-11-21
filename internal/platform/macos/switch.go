//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// nsSwitch is core.SystemSwitch's component, which the backend registers
// itself: an NSSwitch reporting the user's choice as "change".
type nsSwitch struct {
	cx      *driver.NativeContext
	view    objc.ID
	invalid bool
}

var (
	classSwitch  objc.Class
	nsSwitches   = map[objc.ID]*nsSwitch{}
	switchTarget objc.ID
)

func newSwitch(cx *driver.NativeContext) driver.NativeComponent {
	if switchTarget == 0 {
		class := register("NsgoSwitchTarget", apple.Class("NSObject"), nil, []objc.MethodDef{
			{Cmd: sel("changed:"), Fn: func(_ objc.ID, _ objc.SEL, sender objc.ID) {
				if s := nsSwitches[sender]; s != nil {
					s.cx.Emit("change", s.on())
				}
			}},
		})
		switchTarget = objc.ID(class).Send(selNew)
	}
	if classSwitch == 0 {
		classSwitch = register("NsgoSwitch", apple.Class("NSSwitch"), nil, []objc.MethodDef{
			{Cmd: sel("accessibilityAttributeNames"), Fn: func(self objc.ID, cmd objc.SEL) objc.ID {
				names := apple.SendSuper[objc.ID](self, cmd)
				if nsSwitches[self] == nil || names == 0 {
					return names
				}
				return axAddAttr(names, axAttrInvalid)
			}},
			{Cmd: sel("accessibilityAttributeValue:"), Fn: func(self objc.ID, cmd objc.SEL, name objc.ID) objc.ID {
				if s := nsSwitches[self]; s != nil && apple.GoString(name) == axAttrInvalid {
					return axInvalidValue(s.invalid)
				}
				return apple.SendSuper[objc.ID](self, cmd, name)
			}},
			{Cmd: sel("accessibilityIsAttributeSettable:"), Fn: func(self objc.ID, cmd objc.SEL, name objc.ID) bool {
				if nsSwitches[self] != nil && apple.GoString(name) == axAttrInvalid {
					return false
				}
				return apple.SendSuper[bool](self, cmd, name)
			}},
		})
	}
	s := &nsSwitch{cx: cx, view: objc.ID(classSwitch).Send(selNew)}
	s.view.Send(sel("setTarget:"), switchTarget)
	s.view.Send(sel("setAction:"), sel("changed:"))
	nsSwitches[s.view] = s
	s.measure()
	return s
}

func (s *nsSwitch) on() bool { return objc.Send[int](s.view, sel("state")) == 1 }

// measure asks for the switch's size at its control size.
func (s *nsSwitch) measure() {
	fit := objc.Send[apple.CGSize](s.view, sel("intrinsicContentSize"))
	if fit.Width <= 0 || fit.Height <= 0 {
		fit = objc.Send[apple.CGSize](s.view, sel("fittingSize"))
	}
	s.cx.SetSize(driver.Size{Width: driver.Points(fit.Width), Height: driver.Points(fit.Height)})
}

func (s *nsSwitch) View() any { return s.view }

func (s *nsSwitch) SetProp(name string, value any) {
	switch name {
	case "checked":
		on, _ := value.(bool)
		if s.on() != on {
			state := 0
			if on {
				state = 1
			}
			s.view.Send(sel("setState:"), state)
		}
	case "disabled":
		off, _ := value.(bool)
		s.view.Send(sel("setEnabled:"), !off)
	case "size":
		// NSControlSize: regular, small, mini, large.
		size := 0
		switch value {
		case "small":
			size = 1
		case "large":
			size = 3
		}
		s.view.Send(sel("setControlSize:"), uint(size))
		s.measure()
	}
}

func (s *nsSwitch) Command(name string, _ any) {
	if name == "activate" && objc.Send[bool](s.view, sel("isEnabled")) {
		s.view.Send(sel("window")).Send(sel("makeFirstResponder:"), s.view)
		s.view.Send(sel("performClick:"), objc.ID(0))
	}
}

func (s *nsSwitch) Close() {
	delete(nsSwitches, s.view)
	s.view.Send(sel("setTarget:"), objc.ID(0))
	s.view.Send(selRelease)
}

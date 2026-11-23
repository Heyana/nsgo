//go:build android || nsgoandroidhost

package android

import "github.com/nsgo-dev/nsgo/internal/driver"

// systemSwitch is Android's Material 3 switch, implemented by the committed framework view so it follows the platform density, theme and accessibility stack rather than ui's drawn capsule.
type systemSwitch struct{ cx *driver.NativeContext }

func newSystemSwitch(cx *driver.NativeContext) driver.NativeComponent {
	cx.SetSize(switchSize(""))
	return &systemSwitch{cx: cx}
}

// switchSize is the switch's track at size, as Switch.java measures it.
func switchSize(size string) driver.Size {
	switch size {
	case "small":
		return driver.Size{Width: 44, Height: 28}
	case "large":
		return driver.Size{Width: 60, Height: 36}
	}
	return driver.Size{Width: 52, Height: 32}
}

func (*systemSwitch) View() any { return "dev.oneo.nsgo.Switch" }

// SetProp follows the size, which the view measures by.
func (s *systemSwitch) SetProp(name string, value any) {
	if name == "size" {
		size, _ := value.(string)
		s.cx.SetSize(switchSize(size))
	}
}

func (s *systemSwitch) ViewEvent(name string, args []any) {
	if name != "onCheckedChanged" || len(args) < 2 {
		return
	}
	if checked, ok := args[1].(bool); ok {
		s.cx.Emit("change", checked)
	}
}

func (s *systemSwitch) Command(string, any) {}
func (*systemSwitch) Close()                {}

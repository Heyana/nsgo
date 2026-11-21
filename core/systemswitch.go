package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// SystemSwitch is the platform's own switch, NSSwitch on macOS,
// UISwitch on iOS and Material 3's framework switch on Android, native
// components they register; ui.Switch's
// SwitchSystem variant shows it there. Elsewhere it shows nothing, and
// SystemSwitchAvailable says whether it shows. The switch shows the
// user's choice at once, as the system's does, and its size is the
// system's where the style leaves it open.
type SystemSwitch struct {
	Ref        *Ref
	LabelledBy []*Ref
	Invalid    func() bool
	Checked    func() bool
	Disabled   func() bool
	// OnChange receives the value the user chose, which the native component reports as its "change" event with a bool: a test reports one with nsgotest's Emit(selector, "change", value), or flips the switch with Press.
	OnChange func(checked bool)
	// Size is macOS's control size, "small" or "large", else the regular
	// one; UISwitch has one size.
	Size      string
	Label     string
	LabelFunc func() string
	Style     Style
	TestID    string
}

// SystemSwitchAvailable reports whether the platform has a SystemSwitch: macOS, iOS (not a watch), or Android's Material 3 switch, or a web build that runs as them.
func SystemSwitchAvailable(cx *Scope) bool {
	os := cx.Env().host()
	return os == OSMacOS || os == OSAndroid || (os == OSIOS && cx.Env().Idiom() != IdiomWatch)
}

// Render implements Node.
func (s SystemSwitch) Render(cx *Scope) Node {
	return Show{When: func() bool { return SystemSwitchAvailable(cx) }, Then: s.native,
		Else: func(*Scope) Node { return View{Style: Style{Display: DisplayNone}, TestID: s.TestID} }}
}

func (s SystemSwitch) native(*Scope) Node {
	style := s.Style
	// The system draws its switch at one size, so its parent centers it
	// rather than stretching it, unless the style places it.
	if style.AlignSelf == 0 {
		style.AlignSelf = AlignCenter
	}
	return Native{Component: driver.SwitchComponent, Style: style, Label: s.Label, LabelFunc: s.LabelFunc, TestID: s.TestID,
		Ref: s.Ref, LabelledBy: s.LabelledBy, Invalid: s.Invalid,
		PropsFunc: func() map[string]any {
			props := map[string]any{"checked": s.Checked != nil && s.Checked(), "disabled": s.Disabled != nil && s.Disabled()}
			if s.Size != "" {
				props["size"] = s.Size
			}
			return props
		},
		OnEvent: func(name string, data any) {
			if on, ok := data.(bool); ok && name == "change" && s.OnChange != nil {
				s.OnChange(on)
			}
		}}
}

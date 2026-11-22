package harmonyos

import "github.com/nsgo-dev/nsgo/internal/driver"

// harmonyAccess is the accessibility state sent to Index.ets in op "vp".
type harmonyAccess struct {
	label    string
	desc     string
	disabled bool
	level    string
	group    bool
}

// harmonyAccessOf maps ViewProps to ArkUI's accessibilityText,
// accessibilityDescription, enabled, accessibilityLevel and accessibilityGroup.
func harmonyAccessOf(p *driver.ViewProps) harmonyAccess {
	desc := p.HelpText()
	if p.Invalid {
		if desc == "" {
			desc = "invalid data"
		} else {
			desc += ", invalid data"
		}
	}
	level := "auto"
	group := false
	switch {
	case p.ElementsHidden:
		level = "no-hide-descendants"
	case p.Accessible == driver.AccessibleNo:
		level = "no"
	case p.Accessible == driver.AccessibleYes || p.LabelFor != 0:
		level = "yes"
		group = true
	}
	return harmonyAccess{
		label:    p.Label,
		desc:     desc,
		disabled: p.Disabled,
		level:    level,
		group:    group,
	}
}

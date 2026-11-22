package harmonyos

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestHarmonyAccessResolvedLabelsAndStates(t *testing.T) {
	// Visible label with projected disabled and invalid states.
	label := harmonyAccessOf(&driver.ViewProps{
		Role:       driver.RoleText,
		Label:      "Email",
		LabelFor:   2,
		LabelPress: true,
		Disabled:   true,
		Invalid:    true,
	})
	if label.label != "Email" || label.desc != "invalid data" || !label.disabled || label.level != "yes" || !label.group {
		t.Fatalf("label access = %+v", label)
	}

	// Control named by LabelledBy with description, hint, disabled and invalid.
	ids := []driver.NodeID{1}
	field := harmonyAccessOf(&driver.ViewProps{
		Role:        driver.RoleTextField,
		Label:       "Email",
		LabelledBy:  &ids,
		Description: "Work address",
		Hint:        "Enter email",
		Disabled:    true,
		Invalid:     true,
	})
	if field.label != "Email" || field.desc != "Work address. Enter email, invalid data" || !field.disabled || field.level != "auto" || field.group {
		t.Fatalf("field access = %+v", field)
	}

	// Hidden and non-accessible nodes.
	hidden := harmonyAccessOf(&driver.ViewProps{
		Label:          "Tooltip",
		Accessible:     driver.AccessibleNo,
		ElementsHidden: true,
	})
	if hidden.level != "no-hide-descendants" || hidden.group {
		t.Fatalf("hidden access = %+v", hidden)
	}

	nonAcc := harmonyAccessOf(&driver.ViewProps{
		Accessible: driver.AccessibleNo,
	})
	if nonAcc.level != "no" || nonAcc.group {
		t.Fatalf("non-accessible access = %+v", nonAcc)
	}

	// Grouped AccessibleYes container.
	grouped := harmonyAccessOf(&driver.ViewProps{
		Accessible: driver.AccessibleYes,
		Label:      "Profile card",
	})
	if grouped.level != "yes" || !grouped.group || grouped.label != "Profile card" {
		t.Fatalf("grouped access = %+v", grouped)
	}

	// Cleared props restore defaults.
	cleared := harmonyAccessOf(&driver.ViewProps{})
	if cleared != (harmonyAccess{level: "auto"}) {
		t.Fatalf("cleared access = %+v", cleared)
	}
}

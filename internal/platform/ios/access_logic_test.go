package ios

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestHeadingLevelAndSpokenLevel(t *testing.T) {
	for _, c := range []struct {
		name  string
		props driver.ViewProps
		want  int
	}{
		{"unlevelled heading", driver.ViewProps{Role: driver.RoleHeader}, 0},
		{"heading level 1", driver.ViewProps{Role: driver.RoleHeader, Level: 1}, 1},
		{"heading level 3", driver.ViewProps{Role: driver.RoleHeader, Level: 3}, 3},
		{"heading level clamped to 6", driver.ViewProps{Role: driver.RoleHeader, Level: 9}, 6},
		{"tree item level is not a heading level", driver.ViewProps{Role: driver.RoleTreeItem, Level: 2}, 0},
	} {
		if got := headingLevel(&c.props); got != c.want {
			t.Errorf("%s: headingLevel = %d, want %d", c.name, got, c.want)
		}
	}

	if got := spokenValue(&driver.ViewProps{Role: driver.RoleHeader, Level: 2}); got != "" {
		t.Errorf("levelled heading spokenValue = %q, want empty (heading level goes via attributed label)", got)
	}
	if got := spokenValue(&driver.ViewProps{Role: driver.RoleTreeItem, Level: 2, Expandable: true, Expanded: true}); got != "level 2, expanded" {
		t.Errorf("tree item spokenValue = %q, want %q", got, "level 2, expanded")
	}
	if got := spokenValue(&driver.ViewProps{Role: driver.RoleTreeItem, Level: 1}); got != "level 1" {
		t.Errorf("root tree item spokenValue = %q, want %q", got, "level 1")
	}
}

func TestEditorTraitsAndValue(t *testing.T) {
	const base, search, adjustable uint64 = 1 << 0, 1 << 1, 1 << 2
	actions := &[]driver.AccessibilityAction{{Name: driver.ActionIncrement}, {Name: driver.ActionDecrement}}
	for _, c := range []struct {
		name  string
		props driver.ViewProps
		want  uint64
	}{
		{"plain input", driver.ViewProps{Role: driver.RoleTextField}, base},
		{"search field", driver.ViewProps{Role: driver.RoleSearchField}, base | search},
		{"spin button without actions", driver.ViewProps{Role: driver.RoleSpinButton}, base | adjustable},
		{"spin button with actions", driver.ViewProps{Role: driver.RoleSpinButton, Actions: actions}, base | adjustable},
		{"input with increment/decrement actions", driver.ViewProps{Role: driver.RoleTextField, Actions: actions}, base | adjustable},
	} {
		if got := editorTraits(base, &c.props, search, adjustable); got != c.want {
			t.Errorf("%s: editorTraits = %b, want %b", c.name, got, c.want)
		}
	}
	if got := editorValue("12", &driver.ViewProps{Role: driver.RoleSpinButton, Value: driver.AccessibilityValue{Min: 0, Max: 20, Now: 12, Text: "12"}}); got != "12" {
		t.Errorf("spin button matching text: editorValue = %q, want 12", got)
	}
	if got := editorValue("12", &driver.ViewProps{Role: driver.RoleSpinButton, Invalid: true, Value: driver.AccessibilityValue{Text: "12"}}); got != "12, invalid data" {
		t.Errorf("invalid spin button: editorValue = %q, want %q", got, "12, invalid data")
	}
}

func TestCanvasNodeSpokenValue(t *testing.T) {
	for _, tc := range []struct {
		name string
		node driver.CanvasNode
		want string
	}{
		{"range percentage", driver.CanvasNode{Role: driver.RoleAdjustable, Min: 0, Max: 200, Value: 50}, "25%"},
		{"value text overrides percentage", driver.CanvasNode{Role: driver.RoleAdjustable, Min: 0, Max: 100, Value: 50, ValueText: "Half"}, "Half"},
		{"mixed checkbox", driver.CanvasNode{Role: driver.RoleCheckbox, Checked: driver.CheckedMixed}, "mixed"},
		{"checked switch", driver.CanvasNode{Role: driver.RoleSwitch, Checked: driver.CheckedTrue}, "1"},
		{"collapsed disclosure", driver.CanvasNode{Role: driver.RoleButton, Expanded: driver.ExpandedFalse}, "collapsed"},
		{"expanded disclosure", driver.CanvasNode{Role: driver.RoleButton, Expanded: driver.ExpandedTrue}, "expanded"},
	} {
		vp := driver.CanvasNodeViewProps(tc.node)
		if got := spokenValue(&vp); got != tc.want {
			t.Errorf("%s: spokenValue = %q, want %q", tc.name, got, tc.want)
		}
	}
}

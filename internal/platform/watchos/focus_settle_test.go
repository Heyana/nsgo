package watchos

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestFieldCanFocus(t *testing.T) {
	if !fieldCanFocus(driver.InputConfig{}) {
		t.Fatal("plain input rejected focus")
	}
	if fieldCanFocus(driver.InputConfig{Disabled: true}) {
		t.Fatal("disabled input accepted focus")
	}
	if fieldCanFocus(driver.InputConfig{Readonly: true}) {
		t.Fatal("readonly input accepted focus")
	}
}

func TestResolvePressFocus(t *testing.T) {
	tests := []struct {
		name        string
		focused     driver.NodeID
		hitField    driver.NodeID
		hitCanFocus bool
		want        pressFocusAction
	}{
		{
			name:        "tapping an enabled field from unfocused focuses it",
			hitField:    2,
			hitCanFocus: true,
			want:        pressFocusAction{focus: 2},
		},
		{
			name:        "tapping another enabled field blurs the previous field first",
			focused:     1,
			hitField:    2,
			hitCanFocus: true,
			want:        pressFocusAction{blur: 1, focus: 2},
		},
		{
			name:        "tapping the already focused field is a no-op",
			focused:     2,
			hitField:    2,
			hitCanFocus: true,
			want:        pressFocusAction{},
		},
		{
			name:        "tapping a disabled field blurs the previous field without focusing it",
			focused:     1,
			hitField:    2,
			hitCanFocus: false,
			want:        pressFocusAction{blur: 1},
		},
		{
			name:    "tapping a non-field view blurs the focused field before OnPress runs",
			focused: 1,
			want:    pressFocusAction{blur: 1},
		},
		{
			name: "tapping a non-field view with nothing focused does nothing",
			want: pressFocusAction{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolvePressFocus(tc.focused, tc.hitField, tc.hitCanFocus); got != tc.want {
				t.Fatalf("resolvePressFocus(%d, %d, %v) = %+v, want %+v", tc.focused, tc.hitField, tc.hitCanFocus, got, tc.want)
			}
		})
	}
}

func TestWatchTestSurfaceBusy(t *testing.T) {
	if testSurfaceBusy(0, false, 0, false) {
		t.Fatal("idle watch surface reported busy")
	}
	for _, tc := range []struct {
		name string
		busy bool
	}{
		{"queued mount batch", testSurfaceBusy(1, false, 0, false)},
		{"ending animator", testSurfaceBusy(0, true, 0, false)},
		{"layout motion", testSurfaceBusy(0, false, 1, false)},
		{"gliding scroll", testSurfaceBusy(0, false, 0, true)},
	} {
		if !tc.busy {
			t.Fatalf("%s: expected busy=true", tc.name)
		}
	}
}

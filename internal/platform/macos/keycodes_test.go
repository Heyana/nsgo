//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// TestLayoutCodeNamesFollowTheLayout translates the French (AZERTY) and German (QWERTZ) keyboard layouts by their input sources, selecting neither: the key in QWERTY's W position keeps its physical code KeyW while the name the layout gives it follows the layout (#1242).
func TestLayoutCodeNamesFollowTheLayout(t *testing.T) {
	onMain(func() { layoutCodeNamesFollowTheLayout(t) })
}

func layoutCodeNamesFollowTheLayout(t *testing.T) {
	for _, c := range []struct {
		source string
		want   map[string]string
	}{
		{"com.apple.keylayout.US", map[string]string{"KeyW": "w", "KeyQ": "q", "KeyY": "y", "KeyZ": "z", "ArrowLeft": "left", "ShiftRight": "shift"}},
		{"com.apple.keylayout.French", map[string]string{"KeyW": "z", "KeyQ": "a", "KeyA": "q", "KeyZ": "w", "ArrowLeft": "left", "ShiftRight": "shift"}},
		{"com.apple.keylayout.German", map[string]string{"KeyW": "w", "KeyY": "z", "KeyZ": "y", "ArrowLeft": "left", "ShiftRight": "shift"}},
	} {
		t.Run(c.source, func(t *testing.T) {
			tis := inputSources()
			filter := objc.ID(apple.Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"), apple.NSString(c.source), tis.id)
			found := objc.ID(tis.find(uintptr(filter), true))
			if found == 0 || found.Send(sel("count")) == 0 {
				t.Skipf("the %s input source is not installed", c.source)
			}
			defer apple.Release(uintptr(found))
			names := layoutCodeNames(uintptr(found.Send(sel("firstObject"))), keyboardLayout().typeOf())
			for code, want := range c.want {
				if got := names[code]; got != want {
					t.Errorf("%s names %s %q, want %q", c.source, code, got, want)
				}
			}
		})
	}
}

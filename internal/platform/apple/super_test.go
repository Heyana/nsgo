//go:build darwin

package apple

import (
	"strings"
	"testing"

	"github.com/ebitengine/purego/objc"
)

// TestRegisterClassRejectsDuplicateSelector checks that a class defining a selector twice panics naming it, before the runtime refuses the second method with only its index, as NsgoWindowDelegate once did with windowDidFailToEnterFullScreen: at every macOS app's launch.
func TestRegisterClassRejectsDuplicateSelector(t *testing.T) {
	nop := func(objc.ID, objc.SEL, objc.ID) {}
	cmd := objc.RegisterName("nsgoTestDuplicate:")
	defer func() {
		msg, _ := recover().(string)
		if !strings.Contains(msg, "NsgoTestDuplicateSelector") || !strings.Contains(msg, "nsgoTestDuplicate:") {
			t.Errorf("a duplicate selector panics with %q, want the class and selector named", msg)
		}
	}()
	RegisterClass("NsgoTestDuplicateSelector", Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: cmd, Fn: nop},
		{Cmd: objc.RegisterName("nsgoTestOther:"), Fn: nop},
		{Cmd: cmd, Fn: nop},
	})
	t.Error("a class defining a selector twice registered")
}

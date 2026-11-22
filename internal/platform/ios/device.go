//go:build ios

package ios

import (
	"sync"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// hardwareModel is the hardware's model identifier, such as "iPhone15,2". A simulator's sysctl names the Mac's architecture, so there it is the simulated device's identifier from SIMULATOR_MODEL_IDENTIFIER; UIDevice's generic model, such as "iPhone", when neither says.
var hardwareModel = sync.OnceValue(func() string {
	if model := apple.ModelIdentifier(); model != "" {
		return model
	}
	return apple.GoString(objc.ID(apple.Class("UIDevice")).Send(sel("currentDevice")).Send(sel("model")))
})

// deviceIdentity returns the name the user gave the device and its model identifier. Without the user-assigned-device-name entitlement, iOS 16 and later answer UIDevice's name with its generic model, such as "iPhone", which names no device, so the name is empty then.
func deviceIdentity() (name, model string) {
	if device := objc.ID(apple.Class("UIDevice")).Send(sel("currentDevice")); device != 0 {
		name = apple.GoString(device.Send(sel("name")))
		if name == apple.GoString(device.Send(sel("model"))) {
			name = ""
		}
	}
	return name, hardwareModel()
}

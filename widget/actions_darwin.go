//go:build darwin

package widget

import (
	"sync"

	"github.com/ebitengine/purego"
)

var actionListening sync.Once

// listen has the library report presses and push tokens, which it keeps until then: the press that started the app in the background waits for it.
func listen() {
	if _, err := containerDir(); err != nil {
		return
	}
	actionListening.Do(func() {
		lib, err := kit()
		if err != nil {
			return
		}
		fn, err := purego.Dlsym(lib, "nsgo_activity_listen")
		if err != nil {
			return
		}
		cb := purego.NewCallback(func(event int32, id, value uintptr) {
			a, v := cString(id), cString(value)
			switch event {
			case eventPress:
				pressed(ActivityPress{Activity: activityByID(a), Action: v})
			case eventToken:
				tokenChanged(PushToken{Activity: activityByID(a), Token: v})
			case eventStartToken:
				tokenChanged(PushToken{Token: v})
			}
		})
		purego.SyscallN(fn, cb)
	})
}

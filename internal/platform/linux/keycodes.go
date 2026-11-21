//go:build linux && !android

package linux

import (
	"strings"
	"sync"
	"unicode"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

var keyLayoutAPI = sync.OnceValue(func() (api struct {
	defaultSeat func(uintptr) uintptr
	keyboard    func(uintptr) uintptr
	active      func(uintptr) int32
	layout      func(uintptr) uint32
	translate   func(uintptr, uint32, uint32, int32, *uint32, *int32, *int32, *uint32) int32
	unicode     func(uint32) uint32
}) {
	purego.RegisterLibFunc(&api.defaultSeat, gtkLib, "gdk_display_get_default_seat")
	purego.RegisterLibFunc(&api.keyboard, gtkLib, "gdk_seat_get_keyboard")
	if active, err := purego.Dlsym(gtkLib, "gdk_device_get_active_layout_index"); err == nil {
		purego.RegisterFunc(&api.active, active)
	}
	purego.RegisterLibFunc(&api.layout, gtkLib, "gdk_key_event_get_layout")
	purego.RegisterLibFunc(&api.translate, gtkLib, "gdk_display_translate_key")
	purego.RegisterLibFunc(&api.unicode, gtkLib, "gdk_keyval_to_unicode")
	return api
})

func (w *window) keyLayoutChanged(controller uintptr) {
	if !w.heldKeys || controller == 0 {
		return
	}
	if event := gtkEventControllerGetCurrentEvent(controller); event != 0 {
		if group := int32(keyLayoutAPI().layout(event)); group != w.keyLayoutGroup {
			w.keyLayoutGroup = group
			w.updateEnvironment()
		}
	}
}

func (w *window) keyboardCodeNames() map[string]string {
	if !w.heldKeys {
		return nil
	}
	display := gdkDisplayGetDefault()
	if display == 0 {
		return nil
	}
	api := keyLayoutAPI()
	group := w.keyLayoutGroup
	if api.active != nil {
		if seat := api.defaultSeat(display); seat != 0 {
			if keyboard := api.keyboard(seat); keyboard != 0 {
				group = api.active(keyboard)
			}
		}
	}
	names := map[string]string{}
	for code := range uint16(256) {
		position := driver.XKBKeyCode(code)
		if position == "" {
			continue
		}
		var value uint32
		if api.translate(display, uint32(code), 0, group, &value, nil, nil, nil) == 0 {
			continue
		}
		if modifier, ok := modifierKeyvals[value]; ok {
			names[position] = modifier.name
		} else if ch := rune(api.unicode(value)); unicode.IsPrint(ch) && ch != ' ' {
			names[position] = strings.ToLower(string(ch))
		} else if name := keyName(value, false); name != "" {
			names[position] = name
		}
	}
	return names
}

// keyCode returns the position for a hardware keycode. The test host uses synthetic identities outside this range for its layout-dependent presses.
func keyCode(code uint32) string {
	if code > 0xffff {
		return ""
	}
	return driver.XKBKeyCode(uint16(code))
}

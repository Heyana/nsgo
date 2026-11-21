//go:build darwin && !ios

package macos

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf16"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

var keyboardLayout = sync.OnceValue(func() (layout struct {
	current   func() uintptr
	bytes     func(uintptr) uintptr
	translate func(uintptr, uint16, uint16, uint32, uint32, uint32, *uint32, int, *int, *uint16) int32
	typeOf    func() uint8
	property  uintptr
}) {
	carbon, err := purego.Dlopen(carbonPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	purego.RegisterLibFunc(&layout.current, carbon, "TISCopyCurrentKeyboardLayoutInputSource")
	purego.RegisterLibFunc(&layout.translate, carbon, "UCKeyTranslate")
	purego.RegisterLibFunc(&layout.typeOf, carbon, "LMGetKbdType")
	purego.RegisterLibFunc(&layout.bytes, cf, "CFDataGetBytePtr")
	layout.property = uintptr(apple.Global(carbonPath, "kTISPropertyUnicodeKeyLayoutData"))
	return layout
})

// keyboardCodeNames translates physical keys in the current keyboard layout.
func (s *surface) keyboardCodeNames() map[string]string {
	if !s.heldKeys {
		return nil
	}
	api := keyboardLayout()
	source := api.current()
	if source == 0 {
		return nil
	}
	defer apple.Release(source)
	tis := inputSources()
	key := apple.GoString(objc.ID(tis.name(source, tis.id)))
	typ := api.typeOf()
	if key == s.keyLayoutID && typ == s.keyLayoutType && s.env.KeyCodeNames != nil {
		return s.env.KeyCodeNames
	}
	s.keyLayoutID, s.keyLayoutType = key, typ
	names := map[string]string{}
	if data := tis.name(source, api.property); data != 0 {
		layout := api.bytes(data)
		for code := range uint16(128) {
			position := driver.AppKitKeyCode(code)
			if position == "" {
				continue
			}
			if name := keyNames[code]; name != "" {
				names[position] = name
				continue
			}
			if modifier, ok := modifierKeys[code]; ok {
				names[position] = modifier.name
				continue
			}
			const display, noDeadKeys = 3, 1
			var dead uint32
			var chars [8]uint16
			var n int
			if api.translate(layout, code, display, 0, uint32(typ), noDeadKeys, &dead, len(chars), &n, &chars[0]) == 0 && n > 0 && n <= len(chars) {
				runes := utf16.Decode(chars[:n])
				if unicode.IsPrint(runes[0]) {
					names[position] = strings.ToLower(string(runes))
				}
			}
		}
	}
	return names
}

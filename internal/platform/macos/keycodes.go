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
	return layoutCodeNames(source, typ)
}

// layoutCodeNames names the physical keys of an input source's keyboard layout, of keyboard type typ, as keyName names them: a key of keyNames or a modifier by its name, and another by the character it types without modifiers.
func layoutCodeNames(source uintptr, typ uint8) map[string]string {
	names := map[string]string{}
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
		if chars := layoutChars(source, typ, code); chars != "" {
			if runes := []rune(chars); unicode.IsPrint(runes[0]) {
				names[position] = strings.ToLower(chars)
			}
		}
	}
	return names
}

// layoutChars returns what the key with AppKit key code code types without modifiers in an input source's keyboard layout, of keyboard type typ, a dead key's own character included, or "" for none.
func layoutChars(source uintptr, typ uint8, code uint16) string {
	api := keyboardLayout()
	data := inputSources().name(source, api.property)
	if data == 0 {
		return ""
	}
	const display, noDeadKeys = 3, 1
	var dead uint32
	var chars [8]uint16
	var n int
	if api.translate(api.bytes(data), code, display, 0, uint32(typ), noDeadKeys, &dead, len(chars), &n, &chars[0]) != 0 || n <= 0 || n > len(chars) {
		return ""
	}
	return string(utf16.Decode(chars[:n]))
}

// currentLayoutChars returns what the key with AppKit key code code types without modifiers in the current keyboard layout, as a real keyboard's key there does.
func currentLayoutChars(code uint16) string {
	api := keyboardLayout()
	source := api.current()
	if source == 0 {
		return ""
	}
	defer apple.Release(source)
	return layoutChars(source, api.typeOf(), code)
}

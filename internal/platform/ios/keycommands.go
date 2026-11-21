//go:build ios

package ios

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func keyCommandMethods() []objc.MethodDef {
	return []objc.MethodDef{
		{Cmd: sel("keyCommands"), Fn: responderKeyCommands},
		{Cmd: sel("nsgoKeyCommand:"), Fn: invokeKeyCommand},
	}
}

func responderKeyCommands(self objc.ID, cmd objc.SEL) objc.ID {
	inherited := apple.SendSuper[objc.ID](self, cmd)
	s := pressSurface(self)
	if s == nil || onTV() {
		return inherited
	}
	keys := s.availableKeyCommands()
	if len(keys) == 0 {
		return inherited
	}
	commands := objc.ID(apple.Class("NSMutableArray")).Send(sel("array"))
	if inherited != 0 {
		commands.Send(sel("addObjectsFromArray:"), inherited)
	}
	for _, b := range keys {
		if command := makeKeyCommand(b); command != 0 {
			commands.Send(sel("addObject:"), command)
		}
	}
	return commands
}

func (s *surface) availableKeyCommands() []driver.KeyBinding {
	return driver.KeyCommands(s.keys, s.focusKeyUse(), func(key string) []string {
		name, flags := commandParts(key)
		if keyInputNames[name] == "" && literalCommandInput(name) == "" {
			return nil
		}
		return apple.KeyNames(name, flags)
	})
}

func commandParts(key string) (name string, flags uint) {
	parts := strings.Split(key, "+")
	for _, mod := range parts[:len(parts)-1] {
		switch mod {
		case "shift":
			flags |= 1 << 17
		case "ctrl":
			flags |= 1 << 18
		case "alt":
			flags |= 1 << 19
		case "mod", "meta":
			flags |= 1 << 20
		}
	}
	return parts[len(parts)-1], flags
}

var keyInputNames = map[string]string{
	"up": "UpArrow", "down": "DownArrow", "left": "LeftArrow", "right": "RightArrow",
	"escape": "Escape", "pageup": "PageUp", "pagedown": "PageDown", "home": "Home", "end": "End",
	"delete": "Delete", "f1": "F1", "f2": "F2", "f3": "F3", "f4": "F4", "f5": "F5", "f6": "F6",
	"f7": "F7", "f8": "F8", "f9": "F9", "f10": "F10", "f11": "F11", "f12": "F12",
}

func literalCommandInput(name string) string {
	switch name {
	case "enter":
		return "\r"
	case "tab":
		return "\t"
	case "space":
		return " "
	case "backspace":
		return "\b"
	default:
		if utf8.RuneCountInString(name) == 1 {
			return name
		}
		return ""
	}
}

// UIKit's special input strings are constant objects.
var keyInputs = map[string]objc.ID{}

func commandInput(name string) objc.ID {
	if symbol := keyInputNames[name]; symbol != "" {
		if input, ok := keyInputs[name]; ok {
			return input
		}
		input := apple.Global(uikitPath, "UIKeyInput"+symbol)
		keyInputs[name] = input
		return input
	}
	return apple.NSString(literalCommandInput(name))
}

func makeKeyCommand(b driver.KeyBinding) objc.ID {
	name, flags := commandParts(b.Key)
	title := apple.NSString(b.Title)
	input := commandInput(name)
	if input == 0 {
		return 0
	}
	id := objc.ID(apple.Class("NSNumber")).Send(sel("numberWithUnsignedInt:"), b.ID)
	command := objc.ID(apple.Class("UIKeyCommand")).Send(
		sel("commandWithTitle:image:action:input:modifierFlags:propertyList:"),
		title, objc.ID(0), sel("nsgoKeyCommand:"), input, flags, id)
	command.Send(sel("setDiscoverabilityTitle:"), title)
	command.Send(sel("setWantsPriorityOverSystemBehavior:"), b.Priority)
	command.Send(sel("setAllowsAutomaticLocalization:"), false)
	command.Send(sel("setAllowsAutomaticMirroring:"), false)
	return command
}

// keyCommandID rechecks a UIKit command's binding in case a menu open
// during a batch still holds a command from the previous keymap.
func (s *surface) keyCommandID(command objc.ID) uint32 {
	if s == nil || s.engine == nil || command == 0 ||
		!objc.Send[bool](command, sel("isKindOfClass:"), apple.Class("UIKeyCommand")) {
		return 0
	}
	id := objc.Send[uint32](command.Send(sel("propertyList")), sel("unsignedIntValue"))
	i := slices.IndexFunc(s.keys, func(b driver.KeyBinding) bool { return b.ID == id })
	if i < 0 || s.keys[i].Title == "" {
		return 0
	}
	name, flags := commandParts(s.keys[i].Key)
	if winner, _ := driver.FindKey(s.keys, apple.KeyNames(name, flags), s.focusKeyUse(), false); winner != id {
		return 0
	}
	return id
}

func invokeKeyCommand(self objc.ID, _ objc.SEL, command objc.ID) {
	s := pressSurface(self)
	if id := s.keyCommandID(command); id != 0 {
		s.engine.InputModality(driver.ModalityKeyboard)
		s.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
	}
}

// refreshKeyCommands runs after a keymap or focus update on the main
// thread. UIKit reads the responder's current commands and rebuilds any
// groups in the menu bar.
func (s *surface) refreshKeyCommands() {
	if onTV() || s.embedded || (len(s.commandMenuKeys) == 0 && !slices.ContainsFunc(s.keys, func(b driver.KeyBinding) bool { return b.Title != "" })) {
		return
	}
	next := s.availableKeyCommands()
	if !slices.Equal(next, s.commandMenuKeys) {
		s.commandMenuKeys = next
		objc.ID(apple.Class("UIMenuSystem")).Send(sel("mainSystem")).Send(sel("setNeedsRebuild"))
	}
}

// UIKit builds its main menu through the application delegate, separate
// from the responder chain that owns the key commands.
func buildKeyMenus(self objc.ID, cmd objc.SEL, builder objc.ID) {
	apple.Super(self, cmd, builder)
	s := runner
	if s == nil || onTV() || s.env.Idiom != driver.IdiomTablet || s.env.OSVersion.Major < 26 ||
		builder.Send(sel("system")) != objc.ID(apple.Class("UIMenuSystem")).Send(sel("mainSystem")) {
		return
	}
	menus := s.keyCommandMenus()
	if len(menus) == 0 {
		return
	}
	root := apple.Global(uikitPath, "UIMenuRoot")
	for _, menu := range menus {
		builder.Send(sel("insertChildMenu:atEndOfMenuForIdentifier:"), menu, root)
	}
}

func (s *surface) keyCommandMenus() []objc.ID {
	var groups []string
	children := make(map[string]objc.ID)
	for _, b := range s.availableKeyCommands() {
		if b.Group == "" {
			continue
		}
		commands := children[b.Group]
		if commands == 0 {
			commands = objc.ID(apple.Class("NSMutableArray")).Send(sel("array"))
			children[b.Group] = commands
			groups = append(groups, b.Group)
		}
		if command := makeKeyCommand(b); command != 0 {
			commands.Send(sel("addObject:"), command)
		}
	}
	var menus []objc.ID
	for i, group := range groups {
		id := apple.NSString(fmt.Sprintf("dev.oneo.nsgo.keyCommands.%d", i))
		menu := objc.ID(apple.Class("UIMenu")).Send(sel("menuWithTitle:image:identifier:options:children:"),
			apple.NSString(group), objc.ID(0), id, uint(0), children[group])
		menus = append(menus, menu)
	}
	return menus
}

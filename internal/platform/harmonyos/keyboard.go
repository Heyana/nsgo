//go:build js && wasm

package harmonyos

import "github.com/nsgo-dev/nsgo/internal/driver"

type harmonyHeldKey struct {
	id   driver.NodeID
	name string
	// code is the key's physical position, as KeyboardEvent.code.
	code string
}

type harmonyKey struct {
	code         driver.NodeID
	action, mods int
	text         string
	target       driver.NodeID
	editing      bool
	observe      bool
}

// dispatchKey orders Go callbacks and the Go keymap. ArkTS currently returns false before its asynchronous bridge delivers the event, so consumption here prevents only Go keymap actions, not an ArkUI native action already taken.
func (m *mounter) dispatchKey(key harmonyKey) {
	if key.action == 2 { // lifecycle loss is queued after any pending down
		clear(m.pressed)
		for code, held := range m.held {
			delete(m.held, code)
			m.s.dispatch(driver.Event{Kind: driver.EventKeyUp, ID: held.id, Text: held.name, KeyCode: held.code})
		}
		return
	}
	if key.action == 1 {
		delete(m.pressed, key.code)
		if held, ok := m.held[key.code]; ok {
			delete(m.held, key.code)
			mods := m.keyModifiers(key.mods &^ harmonyModifier(held.name))
			m.s.dispatch(driver.Event{Kind: driver.EventKeyUp, ID: held.id, Text: held.name, KeyCode: held.code, Modifiers: harmonyModifiers(mods)})
		}
		return
	}
	name := keyName(key.code, key.text)
	if name == "" {
		return
	}
	repeat := m.pressed[key.code]
	if m.pressed == nil {
		m.pressed = map[driver.NodeID]bool{}
	}
	m.pressed[key.code] = true
	if !key.editing {
		id := key.target
		_, observed := m.held[key.code]
		if id != 0 || key.observe || observed {
			if m.held == nil {
				m.held = map[driver.NodeID]harmonyHeldKey{}
			}
			if held, ok := m.held[key.code]; ok {
				id, name = held.id, held.name
			} else {
				m.held[key.code] = harmonyHeldKey{id, name, keyPosition(key.code)}
			}
			mods := m.keyModifiers(key.mods)
			ev := driver.Event{Kind: driver.EventKeyDown, ID: id, Text: name, KeyCode: keyPosition(key.code), Modifiers: harmonyModifiers(mods), Repeat: repeat}
			if id != 0 && harmonyModifier(name) == 0 {
				if <-m.s.engine.DispatchKeyDownAsync(ev) {
					return
				}
			} else {
				m.s.dispatch(ev)
			}
		}
	}
	m.matchKey(name, m.keyModifiers(key.mods), repeat, key.editing)
}

func (m *mounter) keyModifiers(mods int) int {
	for _, held := range m.held {
		mods |= harmonyModifier(held.name)
	}
	return mods
}

func harmonyModifier(name string) int {
	switch name {
	case "ctrl":
		return 1
	case "alt":
		return 2
	case "shift":
		return 4
	case "meta":
		return 8
	}
	return 0
}

func harmonyModifiers(mods int) driver.Modifiers {
	return driver.Modifiers{Control: mods&1 != 0, Alt: mods&2 != 0, Shift: mods&4 != 0, Platform: mods&8 != 0}
}

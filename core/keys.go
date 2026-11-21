package core

import (
	"log"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// KeyEvent is a key going down on a hardware keyboard while a view has the keyboard focus (View.OnKeyDown), or coming up after it went down there (View.OnKeyUp).
type KeyEvent struct {
	// Key names the key as OnKey does, without modifiers: "k", "1", "space", "enter", "f5", "left", "escape"; a modifier pressed alone is "shift", "ctrl", "alt" or "meta", which is Command on macOS and the Windows or Super key elsewhere. A letter or a digit is the one the keyboard layout types unshifted.
	Key string
	// Modifiers are the modifier keys held: going down, the one pressed alone included; coming up, those still held, the one released not among them.
	Modifiers Modifiers
	// Repeat is whether the press is a held key's auto-repeat, which iOS never reports; it is false for a key coming up.
	Repeat bool
}

// KeysDown returns the names of the keys held on cx's window, sorted, as KeyEvent.Key names them: "shift" for either Shift, "left", "space", "a". It is a tracked read, so what reads it renders again as a key goes down or comes up but not as a held key repeats, and a RequestAnimationFrame callback, which runs untracked, reads the same keys, as a game's frame loop polls them. A key held as the window loses the keyboard focus comes up then.
//
// The first read on a window asks its platform for every key, not only those a focused view that hears keys (View.OnKeyDown) has, and that stays on for the life of the window; a key already held then is unknown until it is pressed again. Keys typed while a text input or a web view has the focus, or while an input method composes, are never held. macOS, Windows, Linux, the web and iOS with a hardware keyboard report them; Android, HarmonyOS, watchOS and mini programs do not, so none is ever held there.
func KeysDown(cx *Scope) []string {
	t := cx.tree()
	if t == nil {
		return nil
	}
	return slices.Clone(t.keysHeld().Get())
}

// KeyHeld reports whether key is held on cx's window, a tracked read of KeysDown's keys: key is one name, as OnKey writes it ("left", "space", "shift"), "mod" being "meta" where the platform modifier is Command (Env.AppleKeys) and "ctrl" elsewhere, or several joined by "+" ("shift+a"), held together.
func KeyHeld(cx *Scope, key string) bool {
	t := cx.tree()
	if t == nil {
		return false
	}
	held := t.keysHeld().Get()
	for _, name := range strings.Split(driver.NormalizeKey(key), "+") {
		switch name {
		case "mod":
			name = "ctrl"
			if cx.Env().AppleKeys() {
				name = "meta"
			}
		case "control":
			name = "ctrl"
		case "option":
			name = "alt"
		case "cmd", "command", "super", "win":
			name = "meta"
		}
		if !slices.Contains(held, name) {
			return false
		}
	}
	return true
}

// keysHeld is the signal KeysDown reads, made by the first read, which asks the platform for every key going down and up (driver.MountItem.HeldKeys) with the next keymap.
func (t *Tree) keysHeld() *Signal[[]string] {
	if t.heldRead == nil {
		t.heldRead = NewSignalFunc(t.root, t.heldNames(), slices.Equal[[]string])
		t.heldKeys = true
		t.keysChanged, t.changed = true, true
	}
	return t.heldRead
}

// holdKey counts key name going down (delta 1) or coming up (-1). A count never goes below none: the platform reports an up for a key it saw go down only as a repeat, as one held before the window took the focus, which was never counted.
func (t *Tree) holdKey(name string, delta int) {
	n := t.held[name] + delta
	if name == "" || n < 0 {
		return
	}
	if n == 0 {
		delete(t.held, name)
	} else {
		if t.held == nil {
			t.held = map[string]int{}
		}
		t.held[name] = n
	}
	if t.heldRead != nil {
		t.heldRead.Set(t.heldNames())
	}
}

// heldNames returns the names of the keys held, sorted.
func (t *Tree) heldNames() []string {
	names := make([]string, 0, len(t.held))
	for name := range t.held {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Shortcut returns the press in OnKey's and desktop.RegisterHotKey's form, with each modifier by its own name, as driver.NormalizeKey writes it: "ctrl+shift+k" or "alt+space" on Windows and Linux, "shift+meta+k" for Command+Shift+K on macOS. A modifier pressed alone is "".
func (e KeyEvent) Shortcut() string {
	switch e.Key {
	case "", "shift", "ctrl", "alt", "meta":
		return ""
	}
	var parts []string
	for _, m := range []struct {
		on   bool
		name string
	}{{e.Modifiers.Control, "ctrl"}, {e.Modifiers.Alt, "alt"}, {e.Modifiers.Shift, "shift"}, {e.Modifiers.Platform, "meta"}} {
		if m.on {
			parts = append(parts, m.name)
		}
	}
	return driver.NormalizeKey(strings.Join(append(parts, e.Key), "+"))
}

// Keyboard shortcuts: the surface's table of bindings goes to the platform, which matches each key press on the main thread, consumes the presses that match and reports the binding each matched with EventKey, which runs it.

type keyBinding struct {
	driver.KeyBinding
	owner *Scope
	// within is the scope Confine confined it to, nil for none.
	within *Scope
	// overModals is KeyOptions.OverModals.
	overModals bool
	// yieldsToMenu is KeyOptions.YieldsToMenu.
	yieldsToMenu bool
	// home is, for an app-wide binding (KeyOptions.AllSurfaces) given to a surface other than the one it was bound in, that surface, and nil for a binding of this surface's own. Such a foreign binding comes before this surface's own bindings, follows whether its owner shows in its home surface, and rests while a modal shows in this one unless overModals (keyActive).
	home *Tree
	fn   func()
}

// globalKey is a binding made with KeyOptions.AllSurfaces: the binding it gives each surface's tree, the trees opened since included (newTree).
type globalKey struct {
	owner        *Scope
	home         *Tree
	key          string
	inText       bool
	yields       driver.KeyUse
	opts         KeyOptions
	fn           func()
	bindings     map[*Tree]*keyBinding
	title, group string
	// shown is whether its owner showed in its home surface when that surface last sent its keymap, which the other surfaces' copies follow (followHome); known is whether it was looked at yet.
	shown, known bool
}

// bind gives tree t the binding: as OnKey's in the tree it was bound in, else before the tree's own bindings, which win over it whenever they were made.
func (g *globalKey) bind(t *Tree, own bool) {
	var home *Tree
	if !own {
		home = g.home
	}
	b := t.addKeyBinding(g.owner, g.key, g.inText, g.yields, g.opts, g.fn, home)
	b.Title, b.Group = g.title, g.group
	g.bindings[t] = b
}

// KeyWhen says when a binding fires.
type KeyWhen uint8

const (
	// KeyAuto fires it unless the view with the keyboard focus uses the key itself: a text input or a web view the keys EditsText reports, a view that takes presses (a button, a menu row, a switch) Return and Space, a slider or spin button the arrows, Page Up, Page Down, Home and End, and a tab bar, radio group, list, menu or tree, or one of their items, the same keys to move in it. Modified keys, such as Shift+Space, are none of these but a text input's.
	KeyAuto KeyWhen = iota
	// KeyAlways fires it wherever the keyboard focus is.
	KeyAlways
	// KeyNotEditing fires it only while no text input or web view has the keyboard focus, and takes its key from any other focused view: a control binds its own keys with it while it has the focus, as a select does Return while its trigger keeps the focus, where KeyAuto would leave them to the control.
	KeyNotEditing
)

// KeyOptions are how a binding fires.
type KeyOptions struct {
	When KeyWhen
	// Repeat fires it again on the auto-repeats of a held key.
	Repeat bool
	// OverModals keeps it firing while a modal its owner's views are not in covers them, for an app-wide shortcut such as a menu's.
	OverModals bool
	// YieldsToMenu leaves the key to the application menu while an enabled item takes the same shortcut where the focus is on macOS, Windows and Linux, which show the menu, for a default shortcut an app's menu may take over. An item that leaves its shortcut to a focused text input or web view (MenuNotEditing) does not suppress the binding, nor do the items after it with the same shortcut, which the desktops never reach past it, and on Linux no item takes a key pressed without Control, Alt or a logo key from a focused input or web view. Else a binding takes its key before the menu does.
	YieldsToMenu bool
	// AllSurfaces binds the key in every surface of the app rather than only the one it is bound in: the app's own and each window, panel and tray view it opens, those it opens later too, for an app-wide shortcut. In the surface it is bound in it is as OnKey says; in each other one it comes before that surface's own bindings, so one of the same key there wins over it, whenever it was made, and rests while a modal shows there unless OverModals keeps it. It rests in all of them while what its owner shows is hidden in the surface it is bound in, a modal covering it there aside, so a component's shortcut does not run while the user cannot see the component; one bound at the app's root, which shows nothing, fires everywhere. It ends in all of them as its owner goes. Where the platform opens no windows it is OnKey's.
	AllSurfaces bool
	// Title gives a shortcut's localized name in the iOS keyboard menu.
	// A nil function or an empty title leaves the shortcut out of the menu.
	Title func() string
	// Group gives the localized title of the shortcut's iPadOS menu.
	Group func() string
}

// OnKey runs fn when key is pressed, while the current owner lives and the user can see what it shows. key names modifiers and a key joined by "+": "mod+s" (Command on Apple platforms, Control elsewhere), "ctrl+shift+z", "alt+left", "escape", "enter", "f5", "a". Of the bindings of a key that fire where the focus is, the newest wins, and only it runs. It fires once while the key is held, and not while the view with the keyboard focus uses the key itself, a text input its editing keys, a button Return and Space, a slider or a tab bar its arrows (KeyAuto); OnKeyWith says else. A control that binds its own keys while it has the focus binds them with KeyNotEditing, or the focus it holds would keep them from it.
//
// What the owner shows are the views of the nearest owner at or above it that has any. The binding rests while none of them shows: in a Holder's content no place shows, under a DisplayNone view such as a navigation stack's screen below the top, in a dismissed modal. While a modal is shown it rests too if any of them is outside the topmost modal, as the page under a dialog is, unless KeyOptions.OverModals. It fires again once they show, without binding it again.
func (cx *Scope) OnKey(key string, fn func()) { cx.OnKeyWith(key, KeyOptions{}, fn) }

// OnKeyWith is OnKey firing as opts say.
func (cx *Scope) OnKeyWith(key string, opts KeyOptions, fn func()) {
	t := cx.tree()
	if t == nil {
		return
	}
	k := driver.NormalizeKey(key)
	inText := opts.When == KeyAlways || opts.When == KeyAuto && !EditsText(k)
	var yields driver.KeyUse
	if opts.When == KeyAuto {
		yields = driver.KeyUsedBy(k)
	}
	owner := cx.current()
	if opts.AllSurfaces {
		rt := cx.rt
		g := &globalKey{owner: owner, key: k, inText: inText, yields: yields, opts: opts, fn: fn, home: t, bindings: map[*Tree]*keyBinding{}}
		rt.globalKeys = append(rt.globalKeys, g)
		g.bind(rt.tree, rt.tree == t)
		for _, s := range rt.surfaces {
			if !s.closed {
				g.bind(s, s == t)
			}
		}
		cx.observeKeyTitle(opts, func(title, group string) {
			g.title, g.group = title, group
			for tree, b := range g.bindings {
				tree.setKeyTitle(b, title, group)
			}
		})
		owner.addCleanup(func() {
			rt.globalKeys = slices.DeleteFunc(rt.globalKeys, func(o *globalKey) bool { return o == g })
			for tree, b := range g.bindings {
				tree.removeKeyBinding(b)
			}
		})
		return
	}
	b := t.addKeyBinding(owner, k, inText, yields, opts, fn, nil)
	cx.observeKeyTitle(opts, func(title, group string) { t.setKeyTitle(b, title, group) })
	owner.addCleanup(func() { t.removeKeyBinding(b) })
}

func (cx *Scope) observeKeyTitle(opts KeyOptions, update func(title, group string)) {
	if opts.Title == nil {
		return
	}
	cx.renderEffect(func() {
		// The app's language can change even if the title reads a catalog indirectly.
		_ = cx.Env().Locale()
		title := opts.Title()
		var group string
		if title != "" && opts.Group != nil {
			group = opts.Group()
		}
		update(title, group)
	})
}

func (t *Tree) setKeyTitle(b *keyBinding, title, group string) {
	if b.Title != title || b.Group != group {
		b.Title, b.Group = title, group
		t.keysChanged, t.changed = true, true
	}
}

// addKeyBinding adds a binding of the normalized key to the tree's table, as its newest, or as its oldest if home, the surface of an app-wide binding made in another one, is set.
func (t *Tree) addKeyBinding(owner *Scope, key string, inText bool, yields driver.KeyUse, opts KeyOptions, fn func(), home *Tree) *keyBinding {
	// IDs are never reused, so a press the platform matched to a binding since removed runs nothing (pressKey).
	t.keyID++
	if t.keyID == 0 {
		t.keyID++
	}
	b := &keyBinding{
		KeyBinding: driver.KeyBinding{Key: key, InText: inText, Repeat: opts.Repeat, Yields: yields, ID: t.keyID, Priority: opts.Title != nil && opts.When != KeyAuto},
		owner:      owner, within: confinement(owner), overModals: opts.OverModals, yieldsToMenu: opts.YieldsToMenu, home: home, fn: fn,
	}
	if home != nil {
		t.keys = slices.Insert(t.keys, 0, b)
	} else {
		t.keys = append(t.keys, b)
	}
	t.keysChanged = true
	t.changed = true
	return b
}

// removeKeyBinding takes b out of the tree's table.
func (t *Tree) removeKeyBinding(b *keyBinding) {
	t.keys = slices.DeleteFunc(t.keys, func(k *keyBinding) bool { return k == b })
	t.keysChanged = true
	t.changed = true
}

type backHandler struct {
	owner, within *Scope
	fn            func() bool
}

// confineKey is where Confine marks an owner.
type confineKey struct{}

// Confine holds what the current owner shows, whoever makes it, to its place in the window, as a browser holds a cross-origin frame. Its key bindings (OnKey) and back handlers (OnBackPress) fire only while the keyboard focus is on a view it owns, itself or through the owners below it: never while the focus is elsewhere or nowhere, so what it shows cannot take the keys or back presses of the views around it; a press they leave goes on to the others, as if they were not there. Its views draw and take input only within its frame, the nearest view around them it does not own, which clips them: its overlays and Fixed views are hosted, sized and placed there rather than in the root view or a modal, and its overlays take back only under the focus rule above. Give it a view of its own, as tsx's sandbox does, or the views beside it in that frame stay within its reach. What a frame cannot hold, and the focus, wait for the user: a Ref's Focus and SetAccessibilityFocus, an input's AutoFocus, DismissKeyboard, a Modal, ShowAlert, PopupMenu, LookUp and Announce act only while the focus is already in its views or the user's input just landed on one (confinedMay), else they are refused, and the first refusal of each kind logs; a modal so shown stays and hears back wherever the focus is, and back takes it away whether or not it removes the modal. Its navigation stacks pop on back only under the focus rule above. Confine it before it renders. tsx's sandbox confines a bundle's scope.
func (cx *Scope) Confine() {
	o := cx.current()
	if o.values == nil {
		o.values = map[any]any{}
	}
	o.values[confineKey{}] = true
	o.rt.confined = true
}

// confinement is the owner nearest o that Confine marked, nil for none. Apps that never confine a scope skip the walk, which every hoisted view, back press and gated request takes.
func confinement(o *Scope) *Scope {
	if o == nil || !o.rt.confined {
		return nil
	}
	for s := o; s != nil; s = s.parent {
		if _, ok := s.values[confineKey{}]; ok {
			return s
		}
	}
	return nil
}

// focusWithin reports whether the keyboard focus is on a view s owns, itself or through the owners below it; any focus, or none, for a nil s.
func (t *Tree) focusWithin(s *Scope) bool {
	if s == nil {
		return true
	}
	return t.focus != nil && owns(s, t.focus.owner)
}

// owns reports whether s is o or an owner above it.
func owns(s, o *Scope) bool {
	for ; o != nil; o = o.parent {
		if o == s {
			return true
		}
	}
	return false
}

// confinedRefusal is where a confined owner notes that a request of a kind was refused, which logs once.
type confinedRefusal uint8

const (
	refusedFocus confinedRefusal = iota
	refusedPresent
	refusedAnnounce
)

// askedToPresent is what a refused presentation logs: a Modal, ShowAlert, PopupMenu or LookUp, which the platform shows over the window, where no frame clips it.
const askedToPresent = "asked to present a modal, an alert, a context menu or a dictionary panel"

// mayMoveFocus reports whether a focus request of owner o, a Ref's Focus or SetAccessibilityFocus or an input's AutoFocus on a view it owns or its DismissKeyboard, may move the keyboard focus (confinedMay): else a confined owner's views could take the focus, and with it the keys, from the views around them whenever they like.
func (t *Tree) mayMoveFocus(o *Scope) bool {
	return t.confinedMay(o, refusedFocus, "asked for the keyboard focus")
}

// confinedMay reports whether owner o may act as the user's own input would let it. One Confine confined may only while the keyboard focus is already in a view it owns, or until the commit after the user's own input (activated) landed on one, as a browser lets a cross-origin frame take the focus or open a popup only on the user's activation. Any other owner always may. The first refusal of each kind of each confined owner logs what it asked.
func (t *Tree) confinedMay(o *Scope, kind confinedRefusal, what string) bool {
	c := confinement(o)
	if c == nil || t.focusWithin(c) || owns(c, t.activated) {
		return true
	}
	if _, ok := c.values[kind]; !ok {
		c.values[kind] = true
		log.Printf("nsgo: a confined view %s while the keyboard focus was elsewhere and the user's input had not landed on the view; refused", what)
	}
	return false
}

// focusKeys reports whether a binding's part in the keymap follows the focus: one confined to its owner's views or yielding to a menu item that may leave its shortcut to text.
func (t *Tree) focusKeys() bool {
	return slices.ContainsFunc(t.keys, func(b *keyBinding) bool { return b.within != nil || b.yieldsToMenu })
}

// OnBackPress runs fn when back is pressed (Escape, the back button or gesture, the browser's back) while the current owner lives and the user can see what it shows, as for OnKey: after the overlays and modals, which take it first, and before the navigation stacks, the newest handler first. fn returns true to take it; when none does, the top stack pops, else the platform goes back itself.
func (cx *Scope) OnBackPress(fn func() bool) {
	t := cx.tree()
	if t == nil {
		return
	}
	owner := cx.current()
	b := &backHandler{owner: owner, within: confinement(owner), fn: fn}
	t.backs = append(t.backs, b)
	b.owner.addCleanup(func() { t.backs = slices.DeleteFunc(t.backs, func(o *backHandler) bool { return o == b }) })
}

// ExitApp is the core's part of mobile.ExitApp, which apps use: it
// leaves the app as Android's back button does when nothing takes it.
func (cx *Scope) ExitApp() {
	if t := cx.tree(); t != nil {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemBackUnhandled})
		t.changed = true
	}
}

// keymap returns the bindings that may fire where the focus is and as the views show, oldest first: the newest of a key wins.
func (t *Tree) keymap() []driver.KeyBinding {
	out := make([]driver.KeyBinding, 0, len(t.keys))
	if len(t.keys) == 0 {
		return out
	}
	places := t.keyPlaces()
	for _, b := range t.keys {
		if t.keyActive(b, places) {
			out = append(out, b.KeyBinding)
		}
	}
	return out
}

// keyActive reports whether binding b may fire: its owner lives and shows (keyPlaces.shows), above any modal unless it is OverModals, the focus is within what it is confined to, and if it is YieldsToMenu, no enabled item of the application menu takes its key where the focus is (menuHasKey). A foreign binding's owner shows in its home surface, not in this one: it rests while its owner is hidden there, whatever modal covers it there, and while a modal shows in this surface unless it is OverModals (#712).
func (t *Tree) keyActive(b *keyBinding, places *keyPlaces) bool {
	if b.owner.disposed {
		return false
	}
	if b.home != nil {
		if !b.home.keyPlaces().shows(b.owner, true) || !b.overModals && places.modal {
			return false
		}
	} else if !places.shows(b.owner, b.overModals) {
		return false
	}
	return t.focusWithin(b.within) && !(b.yieldsToMenu && t.menuHasKey(b.Key))
}

// followHome has the other surfaces send their keymaps again as the owner of an app-wide binding made in t comes to show in t or stops showing there, which their copies of the binding follow (keyActive). t sends its own keymap then, as what shows in it changed. A surface the platform commits before t takes it in the next pass, which a post asks for.
func (t *Tree) followHome() {
	var places *keyPlaces
	moved := false
	for _, g := range t.rt.globalKeys {
		if g.home != t || g.owner.disposed {
			continue
		}
		if places == nil {
			places = t.keyPlaces()
		}
		shown := places.shows(g.owner, true)
		if g.known && shown == g.shown {
			continue
		}
		g.known, g.shown = true, shown
		for o := range g.bindings {
			if o != t && !o.closed {
				o.keysChanged, o.changed = true, true
				moved = true
			}
		}
	}
	if moved && t.rt.post != nil {
		t.rt.post(func() {})
	}
}

// menuHasKey reports whether an enabled item of the application menu takes the normalized key as its shortcut where the focus is (driver.MenuShortcut), "mod" and the platform's own Command or Control being the same press. Only macOS, Windows and Linux show the menu, not a web build that runs as them; elsewhere it takes no key.
func (t *Tree) menuHasKey(key string) bool {
	env := t.root.Env()
	menu := t.menu
	os := env.hostUntracked()
	switch os {
	case OSMacOS:
		// One menu bar shows the menu the surface that sent one last sent, the app's own surface's while the window the focus is in sets none.
		menu = nil
		if bar := t.rt.menuBar; bar != nil && !bar.closed {
			menu = bar.menu
		}
	case OSWindows, OSLinux:
	default:
		return false
	}
	if len(menu) == 0 {
		return false
	}
	command := "ctrl"
	if env.appleKeys.GetUntracked() {
		command = "meta"
	}
	parts := strings.Split(key, "+")
	mods, name := parts[:len(parts)-1], parts[len(parts)-1]
	names := []string{key}
	for _, s := range [][2]string{{"mod", command}, {command, "mod"}} {
		if i := slices.Index(mods, s[0]); i >= 0 {
			swapped := slices.Clone(mods)
			swapped[i] = s[1]
			names = append(names, driver.NormalizeKey(strings.Join(append(swapped, name), "+")))
		}
	}
	editing := t.focus != nil && (t.focus.kind == driver.KindInput || t.focus.kind == driver.KindWeb)
	_, ok := driver.MenuShortcut(menu, names, os, editing, env.speak.GetUntracked())
	return ok
}

// keyPlaces is where the owners of views show, which decides the key bindings and back handlers that take part: read from the views each time a keymap, a key or a back press asks, so it follows a Holder's content in and out of its places, a modal shown or dismissed, and DisplayNone.
type keyPlaces struct {
	of map[*Scope]keyPlace
	// modal is whether a modal shows in the tree.
	modal bool
}

// keyPlace is what an owner's own views are: some live, some shown, some shown under the topmost modal.
type keyPlace uint8

const (
	ownsView keyPlace = 1 << iota
	viewShown
	viewCovered
)

// keyPlaces finds where the owners of the views show. A view shows when the root view holds it, through the places of Holders, and neither it nor a view around it is DisplayNone (a stack's screen below the top, a Suspense's waiting content) or a modal dismissed. A modal covers what shows outside it; nothing covers the topmost modal, nor a Topmost overlay of a host from the root view to it, which shows above that host's modals.
func (t *Tree) keyPlaces() *keyPlaces {
	p := &keyPlaces{of: make(map[*Scope]keyPlace)}
	for _, n := range t.nodes {
		if n != nil && !n.deleted && n.owner != nil {
			p.of[n.owner] |= ownsView
		}
	}
	for n := range t.regions {
		if n.owner != nil {
			p.of[n.owner] |= ownsView
		}
	}
	hosts := []*node{t.view}
	for top := topModal(t.view); top != nil; top = topModal(top) {
		hosts = append(hosts, top)
	}
	top := hosts[len(hosts)-1]
	p.modal = len(hosts) > 1
	var walk func(n *node, above bool)
	walk = func(n *node, above bool) {
		if n.deleted || n.style.Display == DisplayNone || n.hoist != nil && n.hoist.dismissed {
			return
		}
		if n == top || n.hoist != nil && n.hoist.topmost() && slices.Contains(hosts, n.hoist.host) {
			above = true
		}
		if n.owner != nil {
			f := p.of[n.owner] | ownsView | viewShown
			if !above {
				f |= viewCovered
			}
			p.of[n.owner] = f
		}
		for _, c := range n.children {
			walk(c, above)
		}
	}
	walk(t.view, len(hosts) == 1)
	return p
}

// shows reports whether what owner o binds takes part: the nearest owner at or above o that has a view, as the owner of a binding often has none of its own (a Show's that binds while a control has the focus), has one shown and, unless overModals, none of them under a modal. An owner with no view around it takes part.
func (p *keyPlaces) shows(o *Scope, overModals bool) bool {
	for s := o; s != nil; s = s.parent {
		if f := p.of[s]; f != 0 {
			return f&viewShown != 0 && (overModals || f&viewCovered == 0)
		}
	}
	return true
}

// pressKey runs the binding the platform matched a press to (EventKey), by its ID: never another in its place, though a binding of the same key may have come or gone since, as IDs are not reused. It is checked again, as the platform may have matched the press by a keymap the focus or the views have since left; then nothing runs.
func (t *Tree) pressKey(id uint32) {
	if id == 0 {
		return
	}
	i := slices.IndexFunc(t.keys, func(b *keyBinding) bool { return b.ID == id })
	if i < 0 {
		return
	}
	if b := t.keys[i]; t.keyActive(b, t.keyPlaces()) {
		t.within(b.owner, b.fn)
	}
}

// EditsText reports whether a text input with the keyboard focus uses
// the normalized key itself: keys that type, move the caret or delete,
// and select all, cut, copy, paste, undo and redo. A binding of it fires
// in text inputs only with KeyAlways.
func EditsText(key string) bool {
	parts := strings.Split(key, "+")
	name, mods := parts[len(parts)-1], parts[:len(parts)-1]
	command := slices.Contains(mods, "mod") || slices.Contains(mods, "ctrl") || slices.Contains(mods, "meta")
	switch {
	case len(name) > 1 && name[0] == 'f' && name[1] >= '1' && name[1] <= '9':
		return false
	case len(name) == 1:
		return !command || strings.Contains("acvxyz", name)
	}
	return true
}

// KeyLabel is key as menus and hints show it: ⌃⌥⇧⌘K where "mod" is
// Command (apple), else Ctrl+Alt+Shift+K, with "meta" as Win on Windows,
// Meta on the web and Super elsewhere.
func KeyLabel(key string, os OS, apple bool) string {
	parts := strings.Split(driver.NormalizeKey(key), "+")
	name, mods := parts[len(parts)-1], parts[:len(parts)-1]
	has := func(m string) bool { return slices.Contains(mods, m) }
	ctrl, cmd := has("ctrl"), has("meta")
	if has("mod") {
		if apple {
			cmd = true
		} else {
			ctrl = true
		}
	}
	var b strings.Builder
	if apple {
		for _, m := range []struct {
			on    bool
			glyph string
		}{{ctrl, "⌃"}, {has("alt"), "⌥"}, {has("shift"), "⇧"}, {cmd, "⌘"}} {
			if m.on {
				b.WriteString(m.glyph)
			}
		}
	} else {
		meta := map[OS]string{OSWindows: "Win+", OSWeb: "Meta+"}[os]
		if meta == "" {
			meta = "Super+"
		}
		for _, m := range []struct {
			on   bool
			name string
		}{{ctrl, "Ctrl+"}, {has("alt"), "Alt+"}, {has("shift"), "Shift+"}, {cmd, meta}} {
			if m.on {
				b.WriteString(m.name)
			}
		}
	}
	named := map[string][2]string{
		"enter": {"↩", "Enter"}, "escape": {"⎋", "Esc"}, "tab": {"⇥", "Tab"}, "backspace": {"⌫", "Backspace"},
		"delete": {"⌦", "Del"}, "up": {"↑", "↑"}, "down": {"↓", "↓"}, "left": {"←", "←"}, "right": {"→", "→"},
		"home": {"↖", "Home"}, "end": {"↘", "End"}, "pageup": {"⇞", "PgUp"}, "pagedown": {"⇟", "PgDn"},
		"space": {"Space", "Space"},
	}
	switch n, ok := named[name]; {
	case ok && apple:
		b.WriteString(n[0])
	case ok:
		b.WriteString(n[1])
	default:
		b.WriteString(strings.ToUpper(name))
	}
	return b.String()
}

// KeyLabel is key as the platform the app runs on shows it, as
// core.KeyLabel says.
func (cx *Scope) KeyLabel(key string) string {
	env := cx.Env()
	var os OS
	var apple bool
	Untracked(cx, func() bool {
		os, apple = env.OS(), env.AppleKeys()
		return true
	})
	return KeyLabel(key, os, apple)
}

// SetTitle is the core's part of desktop.SetTitle, which apps use.
func (cx *Scope) SetTitle(title string) {
	if t := cx.tree(); t != nil {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemTitle, Value: title})
		t.changed = true
	}
}

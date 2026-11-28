//go:build ios

package ios

import (
	"runtime"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Accessibility: VoiceOver reads each view's UIAccessibility properties,
// set here from its props, events and text.
//
// - A view is an element when its role says so; without a role, text and
//   pressable, labelled or Accessible views are. An element's subviews
//   are not read, so an Accessible view is one element (the core labels
//   it with its text). AccessibleNo views are none, and hidden ones hide
//   their subtree.
// - The value is Text, else a range's percentage, then the checked and
//   expanded states, which UIKit has no traits for.
// - Increment and decrement are the adjustable trait's swipes, escape
//   and magic tap VoiceOver's gestures; custom actions are
//   UIAccessibilityCustomActions.
// - Live regions are announced from here: a changed label, or a changed
//   text inside one.

var (
	selSetAccessibilityHint           = sel("setAccessibilityHint:")
	selSetAccessibilityElementsHidden = sel("setAccessibilityElementsHidden:")
	selSetAccessibilityViewIsModal    = sel("setAccessibilityViewIsModal:")
	selCustomAction                   = sel("nsgoAccessibilityAction:")
	layoutChanged                     uint32  // UIAccessibilityLayoutChangedNotification
	headingLevelKey                   objc.ID // UIAccessibilityTextAttributeHeadingLevel
)

var traitNames = []string{"Button", "Link", "Header", "Image", "StaticText", "Selected", "NotEnabled", "TabBar", "SearchField",
	"Adjustable", "UpdatesFrequently"}

func loadAccessibility(uikit uintptr) {
	for _, name := range []string{"UIAccessibilityAnnouncementNotification", "UIAccessibilityLayoutChangedNotification"} {
		if addr, err := purego.Dlsym(uikit, name); err == nil {
			n := **(**uint32)(unsafe.Pointer(&addr))
			if name == "UIAccessibilityAnnouncementNotification" {
				announcement = n
			} else {
				layoutChanged = n
			}
		}
	}
	for _, name := range traitNames {
		addr, err := purego.Dlsym(uikit, "UIAccessibilityTrait"+name)
		if err != nil {
			panic(err)
		}
		traits[name] = **(**uint64)(unsafe.Pointer(&addr))
	}
	if addr, err := purego.Dlsym(uikit, "UIAccessibilityTextAttributeHeadingLevel"); err == nil && addr != 0 {
		headingLevelKey = **(**objc.ID)(unsafe.Pointer(&addr))
	}
}

// accessMethods are what every view answers VoiceOver's actions with.
func accessMethods() []objc.MethodDef {
	standard := func(name string) func(objc.ID, objc.SEL) bool {
		return func(self objc.ID, _ objc.SEL) bool {
			v := views[self]
			return v != nil && v.action(name)
		}
	}
	return []objc.MethodDef{
		{Cmd: sel("accessibilityActivate"), Fn: accessibilityActivate},
		{Cmd: sel("accessibilityIncrement"), Fn: func(self objc.ID, _ objc.SEL) {
			if v := views[self]; v != nil {
				v.action(driver.ActionIncrement)
			}
		}},
		{Cmd: sel("accessibilityDecrement"), Fn: func(self objc.ID, _ objc.SEL) {
			if v := views[self]; v != nil {
				v.action(driver.ActionDecrement)
			}
		}},
		{Cmd: sel("accessibilityPerformEscape"), Fn: accessibilityEscape},
		{Cmd: sel("accessibilityPerformMagicTap"), Fn: standard(driver.ActionMagicTap)},
		{Cmd: selCustomAction, Fn: accessibilityCustom},
		{Cmd: sel("accessibilityLabel"), Fn: accessibilityLabel},
		{Cmd: sel("accessibilityAttributedLabel"), Fn: accessibilityAttributedLabel},
	}
}

func nsOrNil(s string) objc.ID { return apple.OptionalNSString(s) }

// setProps applies new props, announcing a live region's new label.
func (v *view) setProps(p *driver.ViewProps) {
	old := v.props
	v.props = *p
	if p.TestID != old.TestID {
		apple.Msg(v.native, selSetAccessibilityIdentifier, uintptr(nsOrNil(p.TestID)))
	}
	if p.Appearance != old.Appearance {
		apple.Msg(v.native, sel("setOverrideUserInterfaceStyle:"), uintptr(uiStyle(p.Appearance)))
	}
	v.updateAccessibility()
	if v.propsSet && p.Label != old.Label {
		v.announceLive(p.Label)
	}
	v.propsSet = true
}

// setLabel is ItemAccessibilityLabel.
func (v *view) setLabel(label string) {
	if label == v.props.Label {
		return
	}
	v.props.Label = label
	if switchViewOf(v.component) != 0 {
		v.updateAccessibility()
	}
	v.announceLive(label)
}

// label is what VoiceOver reads for v: its label, else a text's text.
func (v *view) label() string {
	if v.props.Label == "" && v.kind == driver.KindText {
		return v.plain
	}
	return v.props.Label
}

// spokenLabel is what VoiceOver reads for v: its label, a text's text,
// or the texts inside it, as React Native's elements without a label.
func (v *view) spokenLabel() string {
	if l := v.label(); l != "" || v.kind == driver.KindText || !v.axElement {
		return l
	}
	var parts []string
	collectText(v.native, &parts)
	return strings.Join(parts, " ")
}

func collectText(native objc.ID, out *[]string) {
	apple.CollectText(native, out, func(child objc.ID) (string, bool) {
		if v := views[child]; v != nil && v.kind == driver.KindText {
			return v.label(), true
		}
		return "", false
	})
}

// accessibilityLabel answers VoiceOver with spokenLabel, read when asked
// so that it follows the texts inside.
func accessibilityLabel(self objc.ID, cmd objc.SEL) objc.ID {
	if v := views[self]; v != nil && v.id != 0 {
		return nsOrNil(v.spokenLabel())
	}
	return apple.SendSuper[objc.ID](self, cmd)
}

// accessibilityAttributedLabel attaches UIAccessibilityTextAttributeHeadingLevel to a levelled heading's label so VoiceOver announces its level and includes it in the heading-level rotor.
func accessibilityAttributedLabel(self objc.ID, cmd objc.SEL) objc.ID {
	if v := views[self]; v != nil && v.id != 0 && headingLevelKey != 0 {
		if level := headingLevel(&v.props); level > 0 {
			if label := v.spokenLabel(); label != "" {
				n := objc.ID(apple.Class("NSNumber")).Send(sel("numberWithInt:"), level)
				attrs := objc.ID(apple.Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"), n, headingLevelKey)
				return objc.ID(apple.Class("NSAttributedString")).Send(selAlloc).Send(
					sel("initWithString:attributes:"), apple.NSString(label), attrs).Send(sel("autorelease"))
			}
		}
	}
	return apple.SendSuper[objc.ID](self, cmd)
}

// textChanged follows ItemText: a text inside a live region is announced.
func (v *view) textChanged(text string) {
	if text == v.plain {
		return
	}
	first := !v.textSet
	v.plain, v.textSet = text, true
	if first {
		return
	}
	for p := v; p != nil; p = p.parent() {
		if p.props.LiveRegion != driver.LiveRegionNone {
			if p.props.Role == driver.RoleStatus {
				// A status reads its whole text, as ARIA's status is atomic; its view is a container, so spokenLabel reads only its label.
				text = p.label()
				if text == "" {
					var parts []string
					collectText(p.native, &parts)
					text = strings.Join(parts, " ")
				}
			}
			uiAccessibilityPost(announcement, apple.NSString(text))
			return
		}
	}
}

// parent returns the view v is mounted in, if any.
func (v *view) parent() *view {
	for n := v.native.Send(selSuperview); n != 0; n = n.Send(selSuperview) {
		if p := views[n]; p != nil {
			if p.id == 0 {
				return nil // the surface
			}
			return p
		}
	}
	return nil
}

func (v *view) announceLive(text string) {
	if v.props.LiveRegion != driver.LiveRegionNone && v.kind != driver.KindText && text != "" {
		uiAccessibilityPost(announcement, apple.NSString(text))
	}
}

// updateAccessibility sets v's properties from its props, events and text.
func (v *view) updateAccessibility() {
	native, p := v.native, &v.props
	if sw := switchViewOf(v.component); sw != 0 {
		native.Send(selSetIsAccessibilityElement, false)
		v.axElement = false
		sw.Send(sel("setAccessibilityLabel:"), nsOrNil(p.Label))
		return
	}
	// VoiceOver keeps to a dialog while it shows.
	if modal := p.Role == driver.RoleDialog || p.Role == driver.RoleAlertDialog; modal != v.axModal {
		v.axModal = modal
		var m uintptr
		if modal {
			m = 1
		}
		apple.Msg(native, selSetAccessibilityViewIsModal, m)
	}
	if p.ElementsHidden != v.axHidden {
		v.axHidden = p.ElementsHidden
		var h uintptr
		if p.ElementsHidden {
			h = 1
		}
		apple.Msg(native, selSetAccessibilityElementsHidden, h)
	}
	const noContainer, semanticGroup = 0, 4
	pressable := !p.LabelPress && (v.events.Has(driver.EventPress) || v.events.Has(driver.EventLongPress))
	var t uint64
	role := p.Role
	if role == driver.RoleStatus && v.kind == driver.KindText {
		// A status that is a text is read as one, not a container of none.
		role = driver.RoleText
	}
	switch role {
	case driver.RoleRadioGroup, driver.RoleTabList, driver.RoleMenu, driver.RoleDialog, driver.RoleAlertDialog, driver.RoleAlert, driver.RoleStatus,
		driver.RoleTable, driver.RoleGrid, driver.RoleRowGroup, driver.RoleRow, driver.RoleListbox, driver.RoleGroup,
		driver.RoleRegion, driver.RoleNavigation, driver.RoleTree, driver.RoleToolbar:
		// Groups are containers, so their children stay accessible.
		if v.axElement {
			apple.Msg(native, selSetIsAccessibilityElement, 0)
			v.axElement = false
		}
		if v.axContainer != semanticGroup {
			v.axContainer = semanticGroup
			apple.Msg(native, selSetAccessibilityContainerType, semanticGroup)
		}
		if p.Role == driver.RoleTabList {
			apple.Msg(native, selSetAccessibilityTraits, uintptr(traits["TabBar"]))
		}
		return
	case driver.RoleNone:
		if pressable {
			t = traits["Button"]
		}
	case driver.RoleButton, driver.RoleCheckbox, driver.RoleSwitch, driver.RoleRadio, driver.RoleTab, driver.RoleMenuItem,
		driver.RoleMenuItemCheckbox, driver.RoleMenuItemRadio, driver.RoleOption, driver.RoleTreeItem, driver.RoleCombobox:
		// UIKit has no option or tree item trait: they are activated, and
		// the chosen one carries Selected.
		t = traits["Button"]
	case driver.RoleLink:
		t = traits["Link"]
	case driver.RoleHeader, driver.RoleColumnHeader:
		t = traits["Header"]
	case driver.RoleImage:
		t = traits["Image"]
	case driver.RoleText:
		t = traits["StaticText"]
	case driver.RoleAdjustable, driver.RoleSpinButton:
		t = traits["Adjustable"]
	case driver.RoleProgressBar:
		t = traits["UpdatesFrequently"]
	case driver.RoleSearchField:
		t = traits["SearchField"]
	}
	if p.HasAction(driver.ActionIncrement) || p.HasAction(driver.ActionDecrement) {
		t |= traits["Adjustable"]
	}
	// A toggle button that is on reads as selected, as WebKit reads
	// aria-pressed.
	if p.Selected || p.Pressed {
		t |= traits["Selected"]
	}
	if p.Busy {
		t |= traits["UpdatesFrequently"]
	}
	if p.Disabled {
		t |= traits["NotEnabled"]
	}
	element := p.Role != driver.RoleNone || v.kind == driver.KindText || pressable || p.Label != "" ||
		p.Accessible == driver.AccessibleYes
	// An input's editor is its element, whatever its role (editorAccess): a combobox's field stays a text field.
	if p.Accessible == driver.AccessibleNo || p.Role == driver.RoleTextField || v.input != nil {
		element = false
	}
	v.axElement = element
	if len(v.axNodes) > 0 {
		// A canvas with named parts is their container.
		element = false
	}
	var el uintptr
	if element {
		el = 1
	}
	apple.Msg(native, selSetIsAccessibilityElement, el)
	if v.axContainer != noContainer {
		v.axContainer = noContainer
		apple.Msg(native, selSetAccessibilityContainerType, noContainer)
	}
	if !element {
		return
	}
	apple.Msg(native, selSetAccessibilityTraits, uintptr(t))
	if hint := p.HelpText(); hint != v.axHint {
		v.axHint = hint
		apple.Msg(native, selSetAccessibilityHint, uintptr(nsOrNil(hint)))
	}
	if val := spokenValue(p); val != v.axValue {
		v.axValue = val
		apple.Msg(native, selSetAccessibilityValue, uintptr(nsOrNil(val)))
	}
	var custom []objc.ID
	for _, a := range p.ActionList() {
		if driver.IsStandardAction(a.Name) {
			continue
		}
		label := a.Label
		if label == "" {
			label = a.Name
		}
		act := objc.ID(apple.Class("UIAccessibilityCustomAction")).Send(selAlloc).Send(
			sel("initWithName:target:selector:"), apple.NSString(label), native, selCustomAction)
		custom = append(custom, act)
	}
	if len(custom) > 0 || v.axCustom {
		v.axCustom = len(custom) > 0
		arr := objc.ID(0)
		if len(custom) > 0 {
			arr = objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObjects:count:"),
				unsafe.Pointer(&custom[0]), uint(len(custom)))
			runtime.KeepAlive(custom)
		}
		native.Send(sel("setAccessibilityCustomActions:"), arr)
		for _, a := range custom {
			a.Send(selRelease)
		}
	}
}

// editorAccess are an input's editor's answers to VoiceOver. The editor is the input's element, as UIKit's text field or text view, so it can be edited whatever the input's role; it reads the input's label and hint (including a multiline text view's placeholder while empty), the value editorValue gives, the search field trait for RoleSearchField, and the adjustable trait with increment and decrement actions for RoleSpinButton.
func editorAccess() []objc.MethodDef {
	return []objc.MethodDef{
		{Cmd: sel("accessibilityLabel"), Fn: func(self objc.ID, cmd objc.SEL) objc.ID {
			if v := views[self]; v != nil && v.props.Label != "" {
				return apple.NSString(v.props.Label)
			}
			return apple.SendSuper[objc.ID](self, cmd)
		}},
		{Cmd: sel("accessibilityHint"), Fn: func(self objc.ID, cmd objc.SEL) objc.ID {
			if v := views[self]; v != nil {
				var multiline bool
				var placeholder, text string
				if in := v.input; in != nil {
					multiline = in.multiline
					if multiline {
						placeholder = in.fs.Config.Placeholder
						if placeholder != "" {
							text = in.text()
						}
					}
				}
				if h := editorHint(&v.props, multiline, placeholder, text); h != "" {
					return apple.NSString(h)
				}
			}
			return apple.SendSuper[objc.ID](self, cmd)
		}},
		{Cmd: sel("accessibilityTraits"), Fn: func(self objc.ID, cmd objc.SEL) uint64 {
			t := apple.SendSuper[uint64](self, cmd)
			if v := views[self]; v != nil {
				t = editorTraits(t, &v.props, traits["SearchField"], traits["Adjustable"])
			}
			return t
		}},
		{Cmd: sel("accessibilityValue"), Fn: func(self objc.ID, cmd objc.SEL) objc.ID {
			own := apple.SendSuper[objc.ID](self, cmd)
			v := views[self]
			if v == nil {
				return own
			}
			text := apple.GoString(own)
			if value := editorValue(text, &v.props); value != text {
				return apple.NSString(value)
			}
			return own
		}},
		{Cmd: sel("accessibilityIncrement"), Fn: func(self objc.ID, _ objc.SEL) {
			if v := views[self]; v != nil {
				v.action(driver.ActionIncrement)
			}
		}},
		{Cmd: sel("accessibilityDecrement"), Fn: func(self objc.ID, _ objc.SEL) {
			if v := views[self]; v != nil {
				v.action(driver.ActionDecrement)
			}
		}},
	}
}

// action reports the accessibility action name if v offers it.
func (v *view) action(name string) bool {
	if !v.props.HasAction(name) || !v.events.Has(driver.EventAccessibilityAction) {
		return false
	}
	v.report(driver.Event{Kind: driver.EventAccessibilityAction, Text: name})
	return true
}

// accessibilityEscape answers VoiceOver's escape, the two-finger scrub,
// which UIKit asks of the focused element and then of the views around
// it: a view with the escape action takes it, and the outermost view of
// the surface, the root or a modal UIKit presents apart, goes back as
// Escape and the back button do, closing popups and popping screens.
func accessibilityEscape(self objc.ID, _ objc.SEL) bool {
	v := views[self]
	if v == nil {
		return false
	}
	if v.action(driver.ActionEscape) {
		return true
	}
	if up := views[self.Send(selSuperview)]; up != nil && up.surface == v.surface && up.id != 0 {
		return false
	}
	v.surface.engine.Dispatch(driver.Event{Kind: driver.EventBack})
	return true
}

func accessibilityActivate(self objc.ID, _ objc.SEL) bool {
	v := views[self]
	switch {
	case v == nil:
		return false
	case v.action(driver.ActionActivate):
		return true
	case !v.events.Has(driver.EventPress):
		return false
	}
	v.report(driver.Event{Kind: driver.EventPress, Index: -1})
	return true
}

// accessibilityCustom runs the custom action whose name VoiceOver chose.
// nsgo:allow-duplicate The callback resolves a backend-owned view; action lookup lives in apple.CustomAccessibilityAction.
func accessibilityCustom(self objc.ID, _ objc.SEL, action objc.ID) bool {
	v := views[self]
	if v == nil {
		return false
	}
	return apple.CustomAccessibilityAction(action, v.props.ActionList(), v.action)
}

// focusAccessibility moves VoiceOver to v.
func (v *view) focusAccessibility() {
	target := v.native
	if v.input != nil {
		target = v.input.field
	}
	uiAccessibilityPost(layoutChanged, target)
}

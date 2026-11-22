//go:build darwin && !ios

package macos

import (
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Accessibility: VoiceOver reads each view's NSAccessibility attributes,
// set here from its props, events and text.
//
// - A view is an element when its role says so; without a role, text is
//   static text, a pressable view a button, and a labelled or Accessible
//   view a group. AccessibleNo and hidden views are none.
// - An Accessible view or a hidden one has no accessibility children, so
//   VoiceOver reads it as one element (the core labels it with its text).
// - Ranges set the value and its bounds, Text its description; the
//   increment, decrement and escape actions are AXIncrement, AXDecrement
//   and AXCancel, custom ones NSAccessibilityCustomActions.
// - Live regions are announced from here: a changed label, or a changed
//   text inside one.
// - Invalid views, switches and input editors expose AXInvalid ("true" or
//   "false") through accessibilityAttributeNames and
//   accessibilityAttributeValue:, posting AXInvalidStatusChanged as it
//   changes.
// - An input's editor is its element and answers for the input (editorAccessMethods); one with RoleCombobox is an editable combobox, AXComboBox, where a view with RoleCombobox is a select's pop-up button, and one with RoleSpinButton an editable AXIncrementor with the input's actions, where a view with RoleSpinButton is an incrementor of its own.

const (
	appKitPath             = "/System/Library/Frameworks/AppKit.framework/AppKit"
	axAttrInvalid          = "AXInvalid"
	axInvalidStatusChanged = "AXInvalidStatusChanged"
)

var (
	selSetAccessibilityHidden       = sel("setAccessibilityHidden:")
	selSetAccessibilityHelp         = sel("setAccessibilityHelp:")
	selSetAccessibilityExpanded     = sel("setAccessibilityExpanded:")
	selAccessibilityDisclosureLevel = sel("accessibilityDisclosureLevel")
	selCustomAction                 = sel("nsgoAccessibilityAction:")

	axPerform = map[objc.SEL]string{
		sel("accessibilityPerformIncrement"): driver.ActionIncrement,
		sel("accessibilityPerformDecrement"): driver.ActionDecrement,
		sel("accessibilityPerformCancel"):    driver.ActionEscape,
	}
	// axActions are the same actions by their names, as an input's editor answers them (editorAccessMethods).
	axActions = map[string]string{
		"AXIncrement": driver.ActionIncrement,
		"AXDecrement": driver.ActionDecrement,
		"AXCancel":    driver.ActionEscape,
	}
)

// axPost is NSAccessibilityPostNotificationWithUserInfo.
var axPost = sync.OnceValue(func() func(element, name, info objc.ID) {
	lib, err := purego.Dlopen(appKitPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return func(_, _, _ objc.ID) {}
	}
	var post func(element, name, info objc.ID)
	purego.RegisterLibFunc(&post, lib, "NSAccessibilityPostNotificationWithUserInfo")
	return post
})

// accessMethods are what every view answers VoiceOver's actions and attributes with.
func accessMethods() []objc.MethodDef {
	defs := []objc.MethodDef{
		{Cmd: sel("accessibilityPerformPress"), Fn: accessibilityPress},
		{Cmd: sel("isAccessibilitySelectorAllowed:"), Fn: accessibilityAllowed},
		{Cmd: selCustomAction, Fn: accessibilityCustom},
		{Cmd: sel("accessibilityLabel"), Fn: accessibilityLabel},
		{Cmd: sel("accessibilityChildren"), Fn: accessibilityChildren},
		{Cmd: sel("accessibilityAttributeNames"), Fn: func(self objc.ID, cmd objc.SEL) objc.ID {
			names := apple.SendSuper[objc.ID](self, cmd)
			v := views[self]
			if v == nil || !v.axElement || names == 0 {
				return names
			}
			return axAddAttr(names, axAttrInvalid)
		}},
		{Cmd: sel("accessibilityAttributeValue:"), Fn: func(self objc.ID, cmd objc.SEL, name objc.ID) objc.ID {
			if v := views[self]; v != nil && v.axElement && apple.GoString(name) == axAttrInvalid {
				return axInvalidValue(v.props.Invalid)
			}
			return apple.SendSuper[objc.ID](self, cmd, name)
		}},
		{Cmd: sel("accessibilityIsAttributeSettable:"), Fn: func(self objc.ID, cmd objc.SEL, name objc.ID) bool {
			if v := views[self]; v != nil && v.axElement && apple.GoString(name) == axAttrInvalid {
				return false
			}
			return apple.SendSuper[bool](self, cmd, name)
		}},
		{Cmd: selAccessibilityDisclosureLevel, Fn: func(self objc.ID, _ objc.SEL) int {
			if v := views[self]; v != nil && v.props.Role != driver.RoleHeader && v.props.Level > 0 {
				return v.props.Level - 1
			}
			return 0
		}},
		{Cmd: sel("accessibilityTitleUIElement"), Fn: func(self objc.ID, _ objc.SEL) objc.ID {
			if v := views[self]; v != nil {
				return v.titleUIElement()
			}
			return 0
		}},
		{Cmd: sel("accessibilityServesAsTitleForUIElements"), Fn: func(self objc.ID, _ objc.SEL) objc.ID {
			v := views[self]
			if v == nil || v.surface == nil {
				return nsArray(nil)
			}
			var targets []objc.ID
			for _, target := range v.surface.views {
				if target != nil && target.props.LabelledBy != nil {
					for _, id := range *target.props.LabelledBy {
						if id == v.id {
							targets = append(targets, target.accessElement())
						}
					}
				}
			}
			return nsArray(targets)
		}},
	}
	for s, name := range axPerform {
		defs = append(defs, objc.MethodDef{Cmd: s, Fn: func(self objc.ID, _ objc.SEL) bool {
			v := views[self]
			return v != nil && v.action(name)
		}})
	}
	return defs
}

// AppKit roles of core roles: role and subrole.
var roles = map[driver.Role][2]string{
	driver.RoleButton:      {"AXButton", ""},
	driver.RoleLink:        {"AXLink", ""},
	driver.RoleHeader:      {"AXHeading", ""},
	driver.RoleImage:       {"AXImage", ""},
	driver.RoleCheckbox:    {"AXCheckBox", ""},
	driver.RoleSwitch:      {"AXCheckBox", "AXSwitch"},
	driver.RoleRadio:       {"AXRadioButton", ""},
	driver.RoleTab:         {"AXRadioButton", "AXTabButton"},
	driver.RoleText:        {"AXStaticText", ""},
	driver.RoleRadioGroup:  {"AXRadioGroup", ""},
	driver.RoleTabList:     {"AXTabGroup", ""},
	driver.RoleMenu:        {"AXMenu", ""},
	driver.RoleMenuItem:    {"AXMenuItem", ""},
	driver.RoleAdjustable:  {"AXSlider", ""},
	driver.RoleSpinButton:  {"AXIncrementor", ""},
	driver.RoleCombobox:    {"AXPopUpButton", ""},
	driver.RoleProgressBar: {"AXProgressIndicator", ""},
	// WebKit's roles for ARIA's tables.
	driver.RoleTable:        {"AXTable", ""},
	driver.RoleGrid:         {"AXTable", ""},
	driver.RoleRowGroup:     {"AXGroup", ""},
	driver.RoleRow:          {"AXRow", ""},
	driver.RoleColumnHeader: {"AXCell", ""},
	driver.RoleCell:         {"AXCell", ""},
	// WebKit's subroles for ARIA's dialog and alert.
	driver.RoleDialog: {"AXGroup", "AXApplicationDialog"},
	driver.RoleAlert:  {"AXGroup", "AXApplicationAlert"},
	// And for alertdialog.
	driver.RoleAlertDialog: {"AXGroup", "AXApplicationAlertDialog"},
	// And for status; a search field is its editor (editorAttribute), as a text field is.
	driver.RoleStatus: {"AXGroup", "AXApplicationStatus"},
	// A listbox and a tree are lists, as WebKit maps ARIA's. Their items
	// are static text that reports its state: AppKit's rows need a
	// table's columns, and its menu items a menu.
	driver.RoleListbox:  {"AXList", ""},
	driver.RoleTree:     {"AXList", ""},
	driver.RoleOption:   {"AXStaticText", ""},
	driver.RoleTreeItem: {"AXStaticText", ""},
	driver.RoleGroup:    {"AXGroup", ""},
	// ARIA's landmark subroles, as WebKit maps them.
	driver.RoleRegion:           {"AXGroup", "AXLandmarkRegion"},
	driver.RoleNavigation:       {"AXGroup", "AXLandmarkNavigation"},
	driver.RoleToolbar:          {"AXToolbar", ""},
	driver.RoleMenuItemCheckbox: {"AXMenuItem", ""},
	driver.RoleMenuItemRadio:    {"AXMenuItem", ""},
}

// axRole returns the AppKit role of v, or none.
func (v *view) axRole() [2]string {
	p := &v.props
	if len(v.axNodes) > 0 {
		// A canvas with named parts holds them.
		return [2]string{"AXGroup", ""}
	}
	if p.Role == driver.RoleButton && p.Pressed {
		// A button that stays on, as browsers make ARIA's aria-pressed.
		return [2]string{"AXCheckBox", "AXToggle"}
	}
	if v.input != nil && (p.Role == driver.RoleCombobox || p.Role == driver.RoleSpinButton || p.Role == driver.RoleSearchField) {
		// An editable combobox, spin button or search field: its editor is the element (editorAccessMethods).
		return [2]string{}
	}
	if r, ok := roles[p.Role]; ok {
		return r
	}
	if r, ok := v.axScrollRole(); ok {
		return r
	}
	switch {
	case v.kind == driver.KindText:
		return roles[driver.RoleText]
	case p.Accessible == driver.AccessibleNo || p.Role == driver.RoleTextField:
		return [2]string{}
	case !p.LabelPress && (v.events.Has(driver.EventPress) || v.events.Has(driver.EventLongPress)):
		return roles[driver.RoleButton]
	case p.Label != "" || p.Accessible == driver.AccessibleYes:
		return [2]string{"AXGroup", ""}
	}
	return [2]string{}
}

func nsNumber(f float64) objc.ID {
	return objc.ID(apple.Class("NSNumber")).Send(sel("numberWithDouble:"), f)
}

func nsOrNil(s string) objc.ID { return apple.OptionalNSString(s) }

func axInvalidValue(invalid bool) objc.ID {
	if invalid {
		return apple.NSString("true")
	}
	return apple.NSString("false")
}

func axAddAttr(names objc.ID, name string) objc.ID {
	if ns := apple.NSString(name); !objc.Send[bool](names, sel("containsObject:"), ns) {
		return names.Send(sel("arrayByAddingObject:"), ns)
	}
	return names
}

// accessElement is the control's exposed element, an input's cell or text editor rather than its container.
func (v *view) accessElement() objc.ID {
	if s, ok := v.component.(*nsSwitch); ok {
		return s.view
	}
	if in := v.input; in != nil {
		if in.multiline {
			return in.field
		}
		return in.field.Send(sel("cell"))
	}
	return v.native
}

// invalidElement is the exposed element that answers AXInvalid and takes AXInvalidStatusChanged, or 0 when v exposes none.
func (v *view) invalidElement() objc.ID {
	if v.props.ElementsHidden {
		return 0
	}
	if s, ok := v.component.(*nsSwitch); ok {
		return s.view
	}
	if in := v.input; in != nil && !v.axElement {
		if in.field == 0 {
			return 0
		}
		if in.multiline {
			return in.field
		}
		return in.field.Send(sel("cell"))
	}
	if v.axElement {
		return v.native
	}
	return 0
}

// AppKit and UI Automation expose one naming element. Label carries the complete resolved name when the app supplies several.
func (v *view) titleUIElement() objc.ID {
	if v.surface != nil && v.props.LabelledBy != nil {
		for _, id := range *v.props.LabelledBy {
			if int(id) < len(v.surface.views) {
				if label := v.surface.views[id]; label != nil {
					return label.accessElement()
				}
			}
		}
	}
	return 0
}

// nsArray returns an autoreleased array of items.
func nsArray(items []objc.ID) objc.ID {
	class := objc.ID(apple.Class("NSArray"))
	if len(items) == 0 {
		return class.Send(sel("array"))
	}
	a := class.Send(sel("arrayWithObjects:count:"), unsafe.Pointer(&items[0]), uint(len(items)))
	runtime.KeepAlive(items)
	return a
}

// setProps applies new props, announcing a live region's new label.
func (v *view) setProps(p *driver.ViewProps) {
	old := v.props
	v.props = *p
	if p.TestID != old.TestID {
		v.native.Send(selSetAccessibilityIdentifier, nsOrNil(p.TestID))
	}
	if p.Appearance != old.Appearance {
		v.native.Send(sel("setAppearance:"), nsAppearance(p.Appearance))
	}
	v.updateAccessibility()
	if v.propsSet && p.Invalid != old.Invalid {
		if el := v.invalidElement(); el != 0 {
			axPost()(el, apple.NSString(axInvalidStatusChanged), 0)
		}
	}
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
	if _, ok := v.component.(*nsSwitch); ok {
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

// accessibilityChildren hides a group's or a hidden view's subtree.
func accessibilityChildren(self objc.ID, cmd objc.SEL) objc.ID {
	if v := views[self]; v != nil && v.axGrouped {
		return nsArray(nil)
	} else if v != nil && len(v.axNodes) > 0 {
		return nsArray(v.axNodes)
	} else if v != nil && v.sc != nil && v.axElement {
		// A scroll area's bar goes with its content.
		kids := apple.SendSuper[objc.ID](self, cmd)
		if kids == 0 {
			return nsArray([]objc.ID{v.axScrollBar()})
		}
		return kids.Send(sel("arrayByAddingObject:"), v.axScrollBar())
	}
	return apple.SendSuper[objc.ID](self, cmd)
}

// axRoleDescription is NSAccessibilityRoleDescription.
var axRoleDescription = sync.OnceValue(func() func(role, subrole objc.ID) objc.ID {
	lib, err := purego.Dlopen(appKitPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return func(_, _ objc.ID) objc.ID { return 0 }
	}
	var describe func(role, subrole objc.ID) objc.ID
	purego.RegisterLibFunc(&describe, lib, "NSAccessibilityRoleDescription")
	return describe
})

// editorAccessMethods are what an input's editor, a field's cell (cell) or a text view, answers VoiceOver with. AppKit's text classes answer through accessibilityAttributeValue:, which reads none of the attributes set on them, and a field's element is its cell rather than the field, so the editor answers here for its input's view, which is no element: the input's label, help, test ID, value text and standard actions, for RoleCombobox the role of an editable combobox and the popup's state, for RoleSpinButton the role of an incrementor, whose AXIncrement and AXDecrement step the input, and for RoleSearchField a search field's subrole. Its text, value and selection stay its class's.
func editorAccessMethods(cell bool) []objc.MethodDef {
	owner := func(self objc.ID) *view {
		if cell {
			self = self.Send(sel("controlView"))
		}
		if v := views[self]; v != nil && v.input != nil && v.input.field == self && !v.axElement {
			return v
		}
		return nil
	}
	return []objc.MethodDef{
		{Cmd: sel("accessibilityTitleUIElement"), Fn: func(self objc.ID, cmd objc.SEL) objc.ID {
			if v := owner(self); v != nil {
				return v.titleUIElement()
			}
			return apple.SendSuper[objc.ID](self, cmd)
		}},
		{Cmd: sel("accessibilityPlaceholderValue"), Fn: func(self objc.ID, cmd objc.SEL) objc.ID {
			if v := owner(self); v != nil {
				return nsOrNil(v.input.fs.Config.Placeholder)
			}
			return apple.SendSuper[objc.ID](self, cmd)
		}},
		{Cmd: sel("accessibilityAttributeValue:"), Fn: func(self objc.ID, cmd objc.SEL, name objc.ID) objc.ID {
			if v := owner(self); v != nil {
				if value, ok := v.editorAttribute(apple.GoString(name)); ok {
					return value
				}
			}
			return apple.SendSuper[objc.ID](self, cmd, name)
		}},
		{Cmd: sel("accessibilityIsAttributeSettable:"), Fn: func(self objc.ID, cmd objc.SEL, name objc.ID) bool {
			if v := owner(self); v != nil && apple.GoString(name) == axAttrInvalid {
				return false
			}
			return apple.SendSuper[bool](self, cmd, name)
		}},
		{Cmd: sel("accessibilityAttributeNames"), Fn: func(self objc.ID, cmd objc.SEL) objc.ID {
			names := apple.SendSuper[objc.ID](self, cmd)
			v := owner(self)
			if v == nil || names == 0 {
				return names
			}
			for _, name := range []string{"AXSubrole", "AXDescription", "AXIdentifier", "AXExpanded", "AXValueDescription", "AXPlaceholderValue", "AXTitleUIElement", axAttrInvalid} {
				if _, ok := v.editorAttribute(name); !ok {
					continue
				}
				names = axAddAttr(names, name)
			}
			return names
		}},
		{Cmd: sel("accessibilityActionNames"), Fn: func(self objc.ID, cmd objc.SEL) objc.ID {
			names := apple.SendSuper[objc.ID](self, cmd)
			v := owner(self)
			if v == nil {
				return names
			}
			if names == 0 {
				names = nsArray(nil)
			}
			for _, name := range []string{"AXIncrement", "AXDecrement", "AXCancel"} {
				if !v.props.HasAction(axActions[name]) {
					continue
				}
				if ns := apple.NSString(name); !objc.Send[bool](names, sel("containsObject:"), ns) {
					names = names.Send(sel("arrayByAddingObject:"), ns)
				}
			}
			return names
		}},
		{Cmd: sel("accessibilityPerformAction:"), Fn: func(self objc.ID, cmd objc.SEL, name objc.ID) {
			if v := owner(self); v != nil {
				if action, ok := axActions[apple.GoString(name)]; ok && v.action(action) {
					return
				}
			}
			apple.Super(self, cmd, name)
		}},
	}
}

// editorAttribute is what the editor of v, an input whose view is no element, answers for an attribute, or false for what its class answers.
func (v *view) editorAttribute(name string) (objc.ID, bool) {
	p := &v.props
	combobox := p.Role == driver.RoleCombobox
	role, subrole := "", ""
	switch p.Role {
	case driver.RoleCombobox:
		role = "AXComboBox"
	case driver.RoleSpinButton:
		// As browsers make a number field, ARIA's spinbutton.
		role = "AXIncrementor"
	case driver.RoleSearchField:
		// As AppKit's NSSearchField and WebKit's searchbox are.
		role, subrole = "AXTextField", "AXSearchField"
	}
	switch name {
	case "AXTitleUIElement":
		return v.titleUIElement(), true
	case "AXRole":
		if role != "" {
			return apple.NSString(role), true
		}
	case "AXSubrole":
		if subrole != "" {
			return apple.NSString(subrole), true
		}
	case "AXRoleDescription":
		if role != "" {
			return axRoleDescription()(apple.NSString(role), nsOrNil(subrole)), true
		}
	case "AXDescription":
		if p.Label != "" {
			return apple.NSString(p.Label), true
		}
	case "AXPlaceholderValue":
		return nsOrNil(v.input.fs.Config.Placeholder), true
	case "AXHelp":
		if help := p.HelpText(); help != "" {
			return apple.NSString(help), true
		}
	case "AXIdentifier":
		if p.TestID != "" {
			return apple.NSString(p.TestID), true
		}
	case "AXExpanded":
		if combobox && p.Expandable {
			return objc.ID(apple.Class("NSNumber")).Send(sel("numberWithBool:"), p.Expanded), true
		}
	case "AXValueDescription":
		// The input's value text where it is not the editor's, such as a multiple combobox's chosen items: its description, as a range's text is.
		if t := p.Value.Text; t != "" && t != v.input.text() {
			return apple.NSString(t), true
		}
	case axAttrInvalid:
		return axInvalidValue(p.Invalid), true
	}
	return 0, false
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
	for p := v; p != nil; p = p.up {
		if p.props.LiveRegion != driver.LiveRegionNone {
			if p.props.Role == driver.RoleStatus {
				text = p.spokenLabel()
			}
			announceWith(text, p.props.LiveRegion)
			return
		}
	}
}

func (v *view) announceLive(text string) {
	if v.props.LiveRegion != driver.LiveRegionNone && v.kind != driver.KindText && text != "" {
		announceWith(text, v.props.LiveRegion)
	}
}

// updateAccessibility sets v's attributes from its props, events and text.
func (v *view) updateAccessibility() {
	native, p := v.native, &v.props
	if s, ok := v.component.(*nsSwitch); ok {
		native.Send(selSetAccessibilityElement, false)
		v.axElement = false
		s.invalid = p.Invalid
		s.view.Send(sel("setAccessibilityLabel:"), nsOrNil(p.Label))
		s.view.Send(sel("setAccessibilityTitleUIElement:"), v.titleUIElement())
		return
	}
	role := v.axRole()
	element := role[0] != "" && !p.ElementsHidden
	native.Send(selSetAccessibilityElement, element)
	v.axElement = element
	native.Send(selSetAccessibilityHidden, p.ElementsHidden)
	v.axGrouped = p.ElementsHidden || (p.Accessible == driver.AccessibleYes && v.kind != driver.KindText)
	if !element {
		return
	}
	native.Send(selSetAccessibilityRole, apple.NSString(role[0]))
	native.Send(selSetAccessibilitySubrole, nsOrNil(role[1]))
	native.Send(selSetAccessibilityHelp, nsOrNil(p.HelpText()))
	native.Send(selSetAccessibilityEnabled, !p.Disabled)
	native.Send(selSetAccessibilitySelected, p.Selected)
	if p.Expandable {
		native.Send(selSetAccessibilityExpanded, p.Expanded)
	}
	val := p.Value
	switch {
	case val.IsRange() || p.Role == driver.RoleAdjustable || p.Role == driver.RoleProgressBar:
		native.Send(sel("setAccessibilityMinValue:"), nsNumber(float64(val.Min)))
		native.Send(sel("setAccessibilityMaxValue:"), nsNumber(float64(val.Max)))
		native.Send(selSetAccessibilityValue, nsNumber(float64(val.Now)))
		native.Send(sel("setAccessibilityValueDescription:"), nsOrNil(val.Text))
	case p.Role == driver.RoleCheckbox || p.Role == driver.RoleSwitch || p.Role == driver.RoleRadio ||
		p.Role == driver.RoleMenuItemCheckbox || p.Role == driver.RoleMenuItemRadio || p.Pressed:
		n := 0
		switch {
		case p.Indeterminate:
			n = 2 // mixed
		case p.Checked || p.Selected || p.Pressed:
			n = 1
		}
		native.Send(selSetAccessibilityValue, objc.ID(apple.Class("NSNumber")).Send(sel("numberWithInt:"), n))
	case p.Role == driver.RoleHeader && p.Level > 0:
		// VoiceOver reads an AXHeading's level from AXValue as a 1-based NSNumber.
		native.Send(selSetAccessibilityValue, objc.ID(apple.Class("NSNumber")).Send(sel("numberWithInt:"), p.Level))
	case val.Text != "":
		native.Send(selSetAccessibilityValue, apple.NSString(val.Text))
	default:
		native.Send(selSetAccessibilityValue, 0)
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
		act := objc.ID(apple.Class("NSAccessibilityCustomAction")).Send(selAlloc).Send(
			sel("initWithName:target:selector:"), apple.NSString(label), native, selCustomAction)
		custom = append(custom, act)
	}
	if len(custom) > 0 || v.axCustom {
		v.axCustom = len(custom) > 0
		native.Send(sel("setAccessibilityCustomActions:"), nsArray(custom))
		for _, a := range custom {
			a.Send(selRelease)
		}
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

func accessibilityPress(self objc.ID, _ objc.SEL) bool {
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

// accessibilityAllowed offers VoiceOver only the actions and level attributes a view has.
func accessibilityAllowed(self objc.ID, cmd objc.SEL, selector objc.SEL) bool {
	if selector == selAccessibilityDisclosureLevel {
		v := views[self]
		return v != nil && v.props.Role != driver.RoleHeader && v.props.Level > 0
	}
	if name, ok := axPerform[selector]; ok {
		v := views[self]
		return v != nil && v.props.HasAction(name)
	}
	return apple.SendSuper[bool](self, cmd, selector)
}

// accessibilityCustom runs the custom action whose label VoiceOver chose.
// nsgo:allow-duplicate The callback resolves a backend-owned view; action lookup lives in apple.CustomAccessibilityAction.
func accessibilityCustom(self objc.ID, _ objc.SEL, action objc.ID) bool {
	v := views[self]
	if v == nil {
		return false
	}
	return apple.CustomAccessibilityAction(action, v.props.ActionList(), v.action)
}

// focusAccessibility moves VoiceOver to v, and keyboard focus to its stop.
func (v *view) focusAccessibility() {
	target := v.native
	if v.input != nil {
		target = v.input.field
	}
	v.focus()
	info := objc.ID(apple.Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"),
		nsArray([]objc.ID{target}), apple.Global(appKitPath, "NSAccessibilityUIElementsKey"))
	axPost()(target, apple.Global(appKitPath, "NSAccessibilityLayoutChangedNotification"), info)
}

// announce has VoiceOver say text.
func announce(text string) { announceWith(text, driver.LiveRegionAssertive) }

func announceWith(text string, live driver.LiveRegion) {
	priority := 50 // NSAccessibilityPriorityMedium
	if live == driver.LiveRegionAssertive {
		priority = 90 // NSAccessibilityPriorityHigh
	}
	keys := []objc.ID{apple.Global(appKitPath, "NSAccessibilityAnnouncementKey"), apple.Global(appKitPath, "NSAccessibilityPriorityKey")}
	values := []objc.ID{apple.NSString(text), objc.ID(apple.Class("NSNumber")).Send(sel("numberWithInt:"), priority)}
	info := objc.ID(apple.Class("NSDictionary")).Send(sel("dictionaryWithObjects:forKeys:count:"),
		unsafe.Pointer(&values[0]), unsafe.Pointer(&keys[0]), uint(len(keys)))
	app := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
	axPost()(app, apple.Global(appKitPath, "NSAccessibilityAnnouncementRequestedNotification"), info)
	runtime.KeepAlive(keys)
	runtime.KeepAlive(values)
}

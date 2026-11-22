//go:build linux && !android

package linux

import (
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Accessibility goes through GTK's GtkAccessible, which AT-SPI exposes,
// as NSH's Linux backend does: each view's widget gets a role, its label
// (a text's content when it has none), its hint as the description, its
// value and its states, and its TestID as the widget's name.
//
// GTK keeps a widget's role from its construction, so the views of a
// batch get theirs as they are made, from the props and events the batch
// carries (prepareRoles). A canvas's named parts are accessible children:
// empty labels of their role at their bounds, which take no pointer.
// Live regions and ItemAnnounce are announcements
// (gtk_accessible_announce). GTK has no custom actions, nor grouping.

// gValue is GLib's GValue: a type and two words of data.
type gValue struct {
	gType uintptr
	data  [2]uint64
}

// gEnumValue is the start of GLib's GEnumValue.
type gEnumValue struct {
	value int32
}

// G_TYPE_* fundamental types.
const (
	gTypeBoolean = 5 << 2
	gTypeInt     = 6 << 2
	gTypeDouble  = 15 << 2
)

var (
	gValueSetString                  func(*gValue, *byte)
	gValueSetDouble                  func(*gValue, float64)
	gValueSetBoolean                 func(*gValue, int32)
	gValueSetInt                     func(*gValue, int32)
	gTypeClassRef                    func(uintptr) uintptr
	gEnumGetValueByNick              func(uintptr, *byte) *gEnumValue
	gtkAccessibleRoleGetType         func() uintptr
	gtkAccessiblePropertyGetType     func() uintptr
	gtkAccessibleStateGetType        func() uintptr
	gtkAccessibleTristateGetType     func() uintptr
	gtkAccessibleUpdatePropertyValue func(uintptr, int32, *int32, *gValue)
	gtkAccessibleUpdateStateValue    func(uintptr, int32, *int32, *gValue)
	gtkAccessibleResetProperty       func(uintptr, int32)
	gtkAccessibleResetState          func(uintptr, int32)
	gtkAccessibleAnnounce            func(uintptr, *byte, int32)
	gtkLabelGetType                  func() uintptr
	gtkAccessibleRangeGetType        func() uintptr
	gtkAccessibleRelationGetType     func() uintptr
	gtkAccessibleUpdateRelationValue func(uintptr, int32, *int32, *gValue)
	gtkAccessibleResetRelation       func(uintptr, int32)
	gtkAccessibleListGetType         func() uintptr
	gtkAccessibleListNewFromList     func(uintptr) uintptr
	gListAppend                      func(uintptr, uintptr) uintptr
	gListFree                        func(uintptr)
	gTypeAddInterfaceStatic          func(uintptr, uintptr, *gInterfaceInfo)
)

// gInterfaceInfo is GObject's GInterfaceInfo: an init, a finalizer and
// their data, none here.
type gInterfaceInfo struct {
	init, finalize, data uintptr
}

func bindAccess() {
	purego.RegisterLibFunc(&gtkAccessibleRelationGetType, gtkLib, "gtk_accessible_relation_get_type")
	purego.RegisterLibFunc(&gtkAccessibleUpdateRelationValue, gtkLib, "gtk_accessible_update_relation_value")
	purego.RegisterLibFunc(&gtkAccessibleResetRelation, gtkLib, "gtk_accessible_reset_relation")
	purego.RegisterLibFunc(&gtkAccessibleListGetType, gtkLib, "gtk_accessible_list_get_type")
	purego.RegisterLibFunc(&gtkAccessibleListNewFromList, gtkLib, "gtk_accessible_list_new_from_list")
	purego.RegisterLibFunc(&gListAppend, glibLib, "g_list_append")
	purego.RegisterLibFunc(&gListFree, glibLib, "g_list_free")
	purego.RegisterLibFunc(&gValueSetString, gobjectLib, "g_value_set_string")
	purego.RegisterLibFunc(&gValueSetDouble, gobjectLib, "g_value_set_double")
	purego.RegisterLibFunc(&gValueSetBoolean, gobjectLib, "g_value_set_boolean")
	purego.RegisterLibFunc(&gValueSetInt, gobjectLib, "g_value_set_int")
	purego.RegisterLibFunc(&gTypeClassRef, gobjectLib, "g_type_class_ref")
	purego.RegisterLibFunc(&gEnumGetValueByNick, gobjectLib, "g_enum_get_value_by_nick")
	purego.RegisterLibFunc(&gtkAccessibleRoleGetType, gtkLib, "gtk_accessible_role_get_type")
	purego.RegisterLibFunc(&gtkAccessiblePropertyGetType, gtkLib, "gtk_accessible_property_get_type")
	purego.RegisterLibFunc(&gtkAccessibleStateGetType, gtkLib, "gtk_accessible_state_get_type")
	purego.RegisterLibFunc(&gtkAccessibleTristateGetType, gtkLib, "gtk_accessible_tristate_get_type")
	purego.RegisterLibFunc(&gtkAccessibleUpdatePropertyValue, gtkLib, "gtk_accessible_update_property_value")
	purego.RegisterLibFunc(&gtkAccessibleUpdateStateValue, gtkLib, "gtk_accessible_update_state_value")
	purego.RegisterLibFunc(&gtkAccessibleResetProperty, gtkLib, "gtk_accessible_reset_property")
	purego.RegisterLibFunc(&gtkAccessibleResetState, gtkLib, "gtk_accessible_reset_state")
	purego.RegisterLibFunc(&gtkAccessibleAnnounce, gtkLib, "gtk_accessible_announce")
	purego.RegisterLibFunc(&gtkLabelGetType, gtkLib, "gtk_label_get_type")
	purego.RegisterLibFunc(&gtkAccessibleRangeGetType, gtkLib, "gtk_accessible_range_get_type")
	purego.RegisterLibFunc(&gTypeAddInterfaceStatic, gobjectLib, "g_type_add_interface_static")
}

// enums are GTK's accessibility enums by nick, read once from GLib's type
// system rather than copied: their values grew between GTK releases.
var enums map[string]map[string]int32

// enumValue returns the value of nick in GTK's enum kind ("role",
// "property", "state", "tristate"); -1 for one this GTK lacks.
func enumValue(kind, nick string) int32 {
	if enums == nil {
		enums = map[string]map[string]int32{}
	}
	m := enums[kind]
	if m == nil {
		m = map[string]int32{}
		enums[kind] = m
	}
	if v, ok := m[nick]; ok {
		return v
	}
	var t uintptr
	switch kind {
	case "role":
		t = gtkAccessibleRoleGetType()
	case "property":
		t = gtkAccessiblePropertyGetType()
	case "state":
		t = gtkAccessibleStateGetType()
	case "relation":
		t = gtkAccessibleRelationGetType()
	case "tristate":
		t = gtkAccessibleTristateGetType()
	}
	v := int32(-1)
	if e := gEnumGetValueByNick(gTypeClassRef(t), cstr(nick)); e != nil {
		v = e.value
	}
	m[nick] = v
	return v
}

// roleNicks are GTK's roles of NSGo's.
var roleNicks = [...]string{
	driver.RoleButton: "button", driver.RoleLink: "link", driver.RoleHeader: "heading", driver.RoleImage: "img",
	driver.RoleCheckbox: "checkbox", driver.RoleSwitch: "switch", driver.RoleRadio: "radio", driver.RoleTab: "tab",
	driver.RoleText: "label", driver.RoleTextField: "text-box", driver.RoleRadioGroup: "radio-group",
	driver.RoleTabList: "tab-list", driver.RoleMenu: "menu", driver.RoleMenuItem: "menu-item",
	driver.RoleDialog: "dialog", driver.RoleAlertDialog: "alert-dialog", driver.RoleAlert: "alert", driver.RoleAdjustable: "slider", driver.RoleSpinButton: "spin-button", driver.RoleCombobox: "combo-box",
	driver.RoleProgressBar: "progress-bar", driver.RoleTable: "table", driver.RoleGrid: "grid",
	driver.RoleRowGroup: "row-group", driver.RoleRow: "row", driver.RoleColumnHeader: "column-header",
	driver.RoleCell: "cell", driver.RoleListbox: "list-box", driver.RoleOption: "option", driver.RoleGroup: "group",
	driver.RoleRegion: "region", driver.RoleNavigation: "navigation", driver.RoleTree: "tree",
	driver.RoleTreeItem: "tree-item", driver.RoleToolbar: "toolbar",
	driver.RoleMenuItemCheckbox: "menu-item-checkbox", driver.RoleMenuItemRadio: "menu-item-radio",
	driver.RoleStatus: "status", driver.RoleSearchField: "search-box",
}

// roleNick returns the GTK role of a view: its Role's, else a text's
// label, a pressable view's button, a labelled image's img, another
// labelled view's group, and generic for the rest.
func roleNick(kind driver.ViewKind, p *driver.ViewProps, pressable bool) string {
	if int(p.Role) < len(roleNicks) && roleNicks[p.Role] != "" {
		return roleNicks[p.Role]
	}
	switch {
	case kind == driver.KindText:
		return "label"
	case kind == driver.KindInput:
		return "text-box"
	case pressable && !p.LabelPress:
		return "button"
	case kind == driver.KindImage && p.Label != "":
		return "img"
	case p.Label != "":
		return "group"
	}
	return "generic"
}

func pressable(m driver.EventMask) bool {
	return m.Has(driver.EventPress) || m.Has(driver.EventLongPress)
}

// prepareRoles finds the roles of the views b creates, from its props and
// events, before they are made.
func (w *window) prepareRoles(b *driver.MountBatch) {
	type draft struct {
		kind   driver.ViewKind
		props  *driver.ViewProps
		events driver.EventMask
	}
	drafts := map[driver.NodeID]*draft{}
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case driver.ItemCreate:
			drafts[it.ID] = &draft{kind: it.View}
		case driver.ItemViewProps:
			if d := drafts[it.ID]; d != nil {
				d.props = it.ViewProps
			}
		case driver.ItemEvents:
			if d := drafts[it.ID]; d != nil {
				d.events = it.Events
			}
		}
	}
	w.roles = map[driver.NodeID]int32{}
	none := driver.ViewProps{}
	for id, d := range drafts {
		p := d.props
		if p == nil {
			p = &none
		}
		if nick := roleNick(d.kind, p, pressable(d.events)); nick != "generic" {
			w.roles[id] = enumValue("role", nick)
		}
	}
}

// newAccessibleWidget makes an instance of t with role, unless it is -1.
func newAccessibleWidget(t uintptr, role int32) uintptr {
	if role < 0 {
		return gObjectNew(t, nil)
	}
	return gObjectNew(t, cstr("accessible-role"), role, uintptr(0))
}

// accessState is what was last given to GTK of a view.
type accessState struct {
	label, description, valueText string
	level                         int
	multiSelectable               bool
	min, max, now                 float32
	ranged                        bool
	states                        [8]int8 // disabled, selected, checked, pressed, expanded, hidden, busy, invalid: -1 unset, 0, 1, 2 mixed
	testID                        string
	set                           bool
}

// updateAccess brings what GTK knows of v up to date, and reports
// whether its label changed after it was first set.
func (w *window) updateAccess(v *view) bool {
	if v.widget == 0 {
		return false
	}
	p := &v.props
	label := p.Label
	if label == "" && v.kind == driver.KindText {
		label = v.plain
	}
	old := v.access
	n := accessState{label: label, description: p.HelpText(), valueText: p.Value.Text, level: max(p.Level, 0), multiSelectable: p.MultiSelectable, set: true, testID: p.TestID}
	if p.Value.IsRange() {
		n.ranged, n.min, n.max, n.now = true, p.Value.Min, p.Value.Max, p.Value.Now
	}
	n.states = [8]int8{-1, -1, -1, -1, -1, -1, -1, -1}
	bit := func(b bool) int8 {
		if b {
			return 1
		}
		return 0
	}
	n.states[0] = bit(p.Disabled)
	if p.Selected {
		n.states[1] = 1
	}
	checkable := p.Role == driver.RoleCheckbox || p.Role == driver.RoleSwitch || p.Role == driver.RoleRadio ||
		p.Role == driver.RoleMenuItemCheckbox || p.Role == driver.RoleMenuItemRadio
	toggle := p.Checked || p.Indeterminate || p.Pressed
	switch {
	case checkable:
		n.states[2] = bit(p.Checked)
		if p.Indeterminate {
			n.states[2] = 2
		}
	case toggle && (p.Role == driver.RoleButton || pressable(v.events)):
		n.states[3] = bit(p.Checked || p.Pressed)
		if p.Indeterminate {
			n.states[3] = 2
		}
	}
	if p.Expandable {
		n.states[4] = bit(p.Expanded)
	}
	n.states[5] = bit(p.ElementsHidden || v.style.Hidden)
	n.states[6] = bit(p.Busy)
	n.states[7] = bit(p.Invalid)
	if n == old {
		return false
	}
	v.access = n
	acc := v.widget
	setString := func(nick, s, was string) {
		if s == was && old.set {
			return
		}
		setAccessibleString(acc, nick, s)
	}
	setString("label", n.label, old.label)
	setString("description", n.description, old.description)
	setString("value-text", n.valueText, old.valueText)
	if n.level != old.level {
		// GTK_ACCESSIBLE_PROPERTY_LEVEL takes a 1-based int, reset when 0.
		if prop := enumValue("property", "level"); prop >= 0 {
			if n.level > 0 {
				var val gValue
				gValueInit(uintptr(unsafe.Pointer(&val)), gTypeInt)
				gValueSetInt(&val, int32(n.level))
				gtkAccessibleUpdatePropertyValue(acc, 1, &prop, &val)
				gValueUnset(uintptr(unsafe.Pointer(&val)))
			} else {
				gtkAccessibleResetProperty(acc, prop)
			}
		}
	}
	if n.multiSelectable != old.multiSelectable {
		// GTK_ACCESSIBLE_PROPERTY_MULTI_SELECTABLE, whose nick has a hyphen.
		prop := enumValue("property", "multi-selectable")
		if n.multiSelectable {
			var val gValue
			gValueInit(uintptr(unsafe.Pointer(&val)), gTypeBoolean)
			gValueSetBoolean(&val, 1)
			gtkAccessibleUpdatePropertyValue(acc, 1, &prop, &val)
			gValueUnset(uintptr(unsafe.Pointer(&val)))
		} else {
			gtkAccessibleResetProperty(acc, prop)
		}
	}
	if n.ranged != old.ranged || n.min != old.min || n.max != old.max || n.now != old.now {
		for i, nick := range []string{"value-min", "value-max", "value-now"} {
			prop := enumValue("property", nick)
			if !n.ranged {
				gtkAccessibleResetProperty(acc, prop)
				continue
			}
			var val gValue
			gValueInit(uintptr(unsafe.Pointer(&val)), gTypeDouble)
			gValueSetDouble(&val, float64([]float32{n.min, n.max, n.now}[i]))
			gtkAccessibleUpdatePropertyValue(acc, 1, &prop, &val)
			gValueUnset(uintptr(unsafe.Pointer(&val)))
		}
	}
	for i, nick := range []string{"disabled", "selected", "checked", "pressed", "expanded", "hidden", "busy", "invalid"} {
		s := n.states[i]
		if old.set && s == old.states[i] {
			continue
		}
		state := enumValue("state", nick)
		if state < 0 {
			continue
		}
		if s < 0 {
			gtkAccessibleResetState(acc, state)
			continue
		}
		var val gValue
		switch nick {
		case "checked", "pressed":
			// GTK reads a tristate as an int.
			gValueInit(uintptr(unsafe.Pointer(&val)), gTypeInt)
			gValueSetInt(&val, enumValue("tristate", [...]string{"false", "true", "mixed"}[s]))
		case "selected", "expanded", "invalid":
			// GTK collects a boolean state that may be undefined (selected and expanded) with g_value_get_int, as it does GtkAccessibleInvalidState, whose true is 1; a boolean GValue there is rejected and read as false.
			gValueInit(uintptr(unsafe.Pointer(&val)), gTypeInt)
			gValueSetInt(&val, int32(s))
		default:
			gValueInit(uintptr(unsafe.Pointer(&val)), gTypeBoolean)
			gValueSetBoolean(&val, int32(s))
		}
		gtkAccessibleUpdateStateValue(acc, 1, &state, &val)
		gValueUnset(uintptr(unsafe.Pointer(&val)))
	}
	if n.testID != old.testID {
		name := n.testID
		if name == "" {
			name = "NsgoView"
		}
		gtkWidgetSetName(v.widget, cstr(name))
	}
	return old.set && n.label != old.label
}

// setAccessibleString sets string accessible property nick on acc, or resets it when s is empty.
func setAccessibleString(acc uintptr, nick, s string) {
	prop := enumValue("property", nick)
	if prop < 0 {
		return
	}
	if s == "" {
		gtkAccessibleResetProperty(acc, prop)
		return
	}
	var val gValue
	gValueInit(uintptr(unsafe.Pointer(&val)), gTypeString)
	gValueSetString(&val, cstr(s))
	gtkAccessibleUpdatePropertyValue(acc, 1, &prop, &val)
	gValueUnset(uintptr(unsafe.Pointer(&val)))
}

// updateEditorPlaceholder keeps a multiline GtkTextView's GtkAccessible
// placeholder property aligned with the input's placeholder, updating on
// change and resetting when cleared.
func (w *window) updateEditorPlaceholder(v *view) {
	if v.textView == 0 {
		return
	}
	switch v.editorPlaceholder.update(v.input.Placeholder) {
	case editorPlaceholderSet:
		setAccessibleString(v.textView, editorPlaceholderProperty, v.input.Placeholder)
	case editorPlaceholderReset:
		setAccessibleString(v.textView, editorPlaceholderProperty, "")
	}
}

// updateLabelRelation runs after the batch, when every referenced widget has been created. The labels go as a GtkAccessibleList (GTK 4.14), which GTK copies: a plain GList in a G_TYPE_POINTER value is one GTK up to 4.18 takes ownership of and later versions copy, so neither freeing nor keeping it is right on every version.
func (w *window) updateLabelRelation(v *view) {
	if v.deleted || v.widget == 0 {
		return
	}
	relation := enumValue("relation", "labelled-by")
	var list uintptr
	if v.props.LabelledBy != nil {
		for _, id := range *v.props.LabelledBy {
			if label := w.view(id); label != nil && !label.deleted && label.widget != 0 {
				list = gListAppend(list, label.widget)
			}
		}
	}
	if list == 0 {
		gtkAccessibleResetRelation(v.widget, relation)
		return
	}
	var value gValue
	gValueInit(uintptr(unsafe.Pointer(&value)), gtkAccessibleListGetType())
	gValueTakeBoxed(uintptr(unsafe.Pointer(&value)), gtkAccessibleListNewFromList(list))
	gtkAccessibleUpdateRelationValue(v.widget, 1, &relation, &value)
	gValueUnset(uintptr(unsafe.Pointer(&value)))
	gListFree(list)
}

// accessChanged follows a change of what v tells assistive technology:
// a changed label or text inside a live region is announced.
func (w *window) accessChanged(v *view) {
	if !w.updateAccess(v) {
		return
	}
	for p := v; p != nil; p = p.parent {
		if p.props.LiveRegion != driver.LiveRegionNone {
			text := v.access.label
			if p.props.Role == driver.RoleStatus {
				// A status reads its whole text, as ARIA's status is atomic.
				text = statusText(p)
			}
			w.announce(text, p.props.LiveRegion == driver.LiveRegionAssertive)
			return
		}
	}
}

// statusText is what status v reads: its label, else its text or the labels and texts inside it, in tree order.
func statusText(v *view) string {
	var parts []string
	var walk func(*view)
	walk = func(n *view) {
		switch {
		case n.props.Label != "":
			parts = append(parts, n.props.Label)
		case n.kind == driver.KindText:
			if n.plain != "" {
				parts = append(parts, n.plain)
			}
		default:
			for _, c := range n.children {
				if !c.props.ElementsHidden && !c.style.Hidden && !c.deleted {
					walk(c)
				}
			}
		}
	}
	walk(v)
	return strings.Join(parts, " ")
}

// announce has assistive technology read text.
func (w *window) announce(text string, urgent bool) {
	if text == "" || w.surface == 0 {
		return
	}
	priority := int32(1) // medium
	if urgent {
		priority = 2
	}
	gtkAccessibleAnnounce(w.surface, cstr(text), priority)
}

// setCanvasParts makes a canvas's named parts its accessible children.
func (w *window) setCanvasParts(v *view, nodes []driver.CanvasNode) {
	if len(nodes) == 0 && len(v.parts) == 0 {
		return
	}
	for _, c := range v.parts {
		gtkWidgetUnparent(c.widget)
		gObjectUnref(c.widget)
	}
	v.parts = v.parts[:0]
	for _, n := range nodes {
		nick := "group"
		if int(n.Role) < len(roleNicks) && roleNicks[n.Role] != "" {
			nick = roleNicks[n.Role]
		}
		c := newAccessibleWidget(gtkLabelGetType(), enumValue("role", nick))
		gObjectRefSink(c)
		gtkWidgetSetCanTarget(c, 0)
		gtkWidgetSetParent(c, v.widget)
		part := &view{widget: c, props: driver.ViewProps{Label: n.Label, TestID: n.TestID, Selected: n.Selected}}
		w.updateAccess(part)
		v.parts = append(v.parts, canvasPart{widget: c, bounds: n.Bounds})
	}
	gtkWidgetQueueAllocate(v.widget)
}

// canvasPart is a canvas's named part: its widget and its bounds.
type canvasPart struct {
	widget uintptr
	bounds driver.Bounds
}

// part returns the canvas part whose widget is c, if any.
func (v *view) part(c uintptr) *canvasPart {
	for i := range v.parts {
		if v.parts[i].widget == c {
			return &v.parts[i]
		}
	}
	return nil
}

// releaseCanvasParts lets a deleted canvas's parts go.
func (v *view) releaseCanvasParts() {
	for _, c := range v.parts {
		gtkWidgetUnparent(c.widget)
		gObjectUnref(c.widget)
	}
	v.parts = nil
}

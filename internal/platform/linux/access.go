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
	bindSyscall(&gtkAccessibleUpdateRelationValue, gtkLib, "gtk_accessible_update_relation_value")
	bindSyscall(&gtkAccessibleResetRelation, gtkLib, "gtk_accessible_reset_relation")
	purego.RegisterLibFunc(&gtkAccessibleListGetType, gtkLib, "gtk_accessible_list_get_type")
	bindSyscall(&gtkAccessibleListNewFromList, gtkLib, "gtk_accessible_list_new_from_list")
	bindSyscall(&gListAppend, glibLib, "g_list_append")
	bindSyscall(&gListFree, glibLib, "g_list_free")
	bindSyscall(&gValueSetString, gobjectLib, "g_value_set_string")
	purego.RegisterLibFunc(&gValueSetDouble, gobjectLib, "g_value_set_double")
	bindSyscall(&gValueSetBoolean, gobjectLib, "g_value_set_boolean")
	bindSyscall(&gValueSetInt, gobjectLib, "g_value_set_int")
	purego.RegisterLibFunc(&gTypeClassRef, gobjectLib, "g_type_class_ref")
	purego.RegisterLibFunc(&gEnumGetValueByNick, gobjectLib, "g_enum_get_value_by_nick")
	purego.RegisterLibFunc(&gtkAccessibleRoleGetType, gtkLib, "gtk_accessible_role_get_type")
	purego.RegisterLibFunc(&gtkAccessiblePropertyGetType, gtkLib, "gtk_accessible_property_get_type")
	purego.RegisterLibFunc(&gtkAccessibleStateGetType, gtkLib, "gtk_accessible_state_get_type")
	purego.RegisterLibFunc(&gtkAccessibleTristateGetType, gtkLib, "gtk_accessible_tristate_get_type")
	bindSyscall(&gtkAccessibleUpdatePropertyValue, gtkLib, "gtk_accessible_update_property_value")
	bindSyscall(&gtkAccessibleUpdateStateValue, gtkLib, "gtk_accessible_update_state_value")
	bindSyscall(&gtkAccessibleResetProperty, gtkLib, "gtk_accessible_reset_property")
	bindSyscall(&gtkAccessibleResetState, gtkLib, "gtk_accessible_reset_state")
	purego.RegisterLibFunc(&gtkAccessibleAnnounce, gtkLib, "gtk_accessible_announce")
	purego.RegisterLibFunc(&gtkLabelGetType, gtkLib, "gtk_label_get_type")
	purego.RegisterLibFunc(&gtkAccessibleRangeGetType, gtkLib, "gtk_accessible_range_get_type")
	purego.RegisterLibFunc(&gTypeAddInterfaceStatic, gobjectLib, "g_type_add_interface_static")
}

// accEnums caches GTK's frequently used accessibility enum values so
// updateAccess never performs nested map lookups on hot text/prop updates.
// enumsPtr tracks the backing map pointer of enums so unit tests that swap
// enums invalidate the cache automatically.
var accEnums struct {
	ready           bool
	enumsPtr        uintptr
	propLabel       int32
	propDescription int32
	propValueText   int32
	propLevel       int32
	propMultiSel    int32
	propValueMin    int32
	propValueMax    int32
	propValueNow    int32
	states          [8]int32
	tristate        [3]int32
}

func initAccessEnums() {
	ptr := *(*uintptr)(unsafe.Pointer(&enums))
	if accEnums.ready && accEnums.enumsPtr == ptr {
		return
	}
	accEnums.propLabel = enumValue("property", "label")
	accEnums.propDescription = enumValue("property", "description")
	accEnums.propValueText = enumValue("property", "value-text")
	accEnums.propLevel = enumValue("property", "level")
	accEnums.propMultiSel = enumValue("property", "multi-selectable")
	accEnums.propValueMin = enumValue("property", "value-min")
	accEnums.propValueMax = enumValue("property", "value-max")
	accEnums.propValueNow = enumValue("property", "value-now")
	for i, nick := range [...]string{"disabled", "selected", "checked", "pressed", "expanded", "hidden", "busy", "invalid"} {
		accEnums.states[i] = enumValue("state", nick)
	}
	for i, nick := range [...]string{"false", "true", "mixed"} {
		accEnums.tristate[i] = enumValue("tristate", nick)
	}
	accEnums.enumsPtr = *(*uintptr)(unsafe.Pointer(&enums))
	accEnums.ready = accEnums.enumsPtr != 0
}

// defaultAccessStates matches a freshly constructed GtkWidget's initial
// GtkAccessible state values: boolean states false (0) and optional/tristate
// states unset (-1).
var defaultAccessStates = [8]int8{0, -1, -1, -1, -1, 0, 0, 0}

// enums are GTK's accessibility enums by nick, read once from GLib's type
// system rather than copied: their values grew between GTK releases.
var enums map[string]map[string]int32

// enumValue returns the value of nick in GTK's enum kind ("role",
// "property", "state", "tristate"); -1 for one this GTK lacks.
func enumValue(kind, nick string) int32 {
	if m := enums[kind]; m != nil {
		if v, ok := m[nick]; ok {
			return v
		}
	}
	if gTypeClassRef == nil || gEnumGetValueByNick == nil {
		return -1
	}
	var t uintptr
	switch kind {
	case "role":
		if gtkAccessibleRoleGetType == nil {
			return -1
		}
		t = gtkAccessibleRoleGetType()
	case "property":
		if gtkAccessiblePropertyGetType == nil {
			return -1
		}
		t = gtkAccessiblePropertyGetType()
	case "state":
		if gtkAccessibleStateGetType == nil {
			return -1
		}
		t = gtkAccessibleStateGetType()
	case "relation":
		if gtkAccessibleRelationGetType == nil {
			return -1
		}
		t = gtkAccessibleRelationGetType()
	case "tristate":
		if gtkAccessibleTristateGetType == nil {
			return -1
		}
		t = gtkAccessibleTristateGetType()
	}
	if t == 0 {
		return -1
	}
	if enums == nil {
		enums = map[string]map[string]int32{}
	}
	m := enums[kind]
	if m == nil {
		m = map[string]int32{}
		enums[kind] = m
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
	if len(w.roles) > 0 {
		clear(w.roles)
	}
	type draft struct {
		kind   driver.ViewKind
		props  *driver.ViewProps
		events driver.EventMask
	}
	var drafts map[driver.NodeID]*draft
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case driver.ItemCreate:
			if drafts == nil {
				drafts = map[driver.NodeID]*draft{}
			}
			drafts[it.ID] = &draft{kind: it.View}
		case driver.ItemViewProps:
			if drafts != nil {
				if d := drafts[it.ID]; d != nil {
					d.props = it.ViewProps
				}
			}
		case driver.ItemEvents:
			if drafts != nil {
				if d := drafts[it.ID]; d != nil {
					d.events = it.Events
				}
			}
		}
	}
	if len(drafts) == 0 {
		return
	}
	if w.roles == nil {
		w.roles = map[driver.NodeID]int32{}
	}
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
	initAccessEnums()
	acc := v.widget
	setString := func(prop int32, s, was string) {
		if s == was && (old.set || s == "") {
			return
		}
		setAccessiblePropString(acc, prop, s)
	}
	setString(accEnums.propLabel, n.label, old.label)
	setString(accEnums.propDescription, n.description, old.description)
	setString(accEnums.propValueText, n.valueText, old.valueText)
	if n.level != old.level {
		// GTK_ACCESSIBLE_PROPERTY_LEVEL takes a 1-based int, reset when 0.
		if prop := accEnums.propLevel; prop >= 0 {
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
		if prop := accEnums.propMultiSel; prop >= 0 {
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
	}
	if n.ranged != old.ranged || n.min != old.min || n.max != old.max || n.now != old.now {
		props := [3]int32{accEnums.propValueMin, accEnums.propValueMax, accEnums.propValueNow}
		vals := [3]float32{n.min, n.max, n.now}
		for i, prop := range props {
			if prop < 0 {
				continue
			}
			if !n.ranged {
				gtkAccessibleResetProperty(acc, prop)
				continue
			}
			var val gValue
			gValueInit(uintptr(unsafe.Pointer(&val)), gTypeDouble)
			gValueSetDouble(&val, float64(vals[i]))
			gtkAccessibleUpdatePropertyValue(acc, 1, &prop, &val)
			gValueUnset(uintptr(unsafe.Pointer(&val)))
		}
	}
	prevStates := old.states
	if !old.set {
		prevStates = defaultAccessStates
	}
	for i := range n.states {
		s := n.states[i]
		if s == prevStates[i] {
			continue
		}
		state := accEnums.states[i]
		if state < 0 {
			continue
		}
		if s < 0 {
			gtkAccessibleResetState(acc, state)
			continue
		}
		var val gValue
		switch i {
		case 2, 3: // checked, pressed: GTK reads a tristate as an int.
			gValueInit(uintptr(unsafe.Pointer(&val)), gTypeInt)
			gValueSetInt(&val, accEnums.tristate[s])
		case 1, 4, 7: // selected, expanded, invalid: GTK collects with g_value_get_int.
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
		if n.testID == "" {
			gtkWidgetSetName(v.widget, cstrNsgoViewName)
		} else {
			gtkWidgetSetName(v.widget, cstr(n.testID))
		}
	}
	return old.set && n.label != old.label
}

// setAccessiblePropString sets string accessible property prop on acc, or resets it when s is empty.
func setAccessiblePropString(acc uintptr, prop int32, s string) {
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

// setAccessibleString sets string accessible property nick on acc, or resets it when s is empty.
func setAccessibleString(acc uintptr, nick, s string) {
	setAccessiblePropString(acc, enumValue("property", nick), s)
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
	w.queueAllocate(v.widget)
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

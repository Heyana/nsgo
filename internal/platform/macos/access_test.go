//go:build darwin && !ios

package macos

import (
	"bytes"
	"os"
	"os/exec"
	"slices"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// axAttr is an element's attribute as VoiceOver reads it, through accessibilityAttributeValue:, described.
func axAttr(e objc.ID, name string) string {
	value := e.Send(sel("accessibilityAttributeValue:"), apple.NSString(name))
	if value == 0 {
		return ""
	}
	return apple.GoString(value.Send(sel("description")))
}

func axAttrNames(e objc.ID) []string { return axList(e.Send(sel("accessibilityAttributeNames"))) }

func axActionNames(e objc.ID) []string { return axList(e.Send(sel("accessibilityActionNames"))) }

func axList(list objc.ID) []string {
	var out []string
	for i := range objc.Send[int](list, sel("count")) {
		out = append(out, apple.GoString(list.Send(sel("objectAtIndex:"), i)))
	}
	return out
}

// TestInputAccessibilityLabel checks, by walking a window's accessibility tree as VoiceOver does, that a field, a masked field and a text view are found by their native role and label, that the label follows updates and clearing, and that each editor answers the input's placeholder through accessibilityPlaceholderValue and AXPlaceholderValue while a text view's drawn placeholder is no element of its own (#371).
func TestInputAccessibilityLabel(t *testing.T) {
	// Native accessibility initializes process-wide AppKit state, so the window-tree inspection runs apart from the suite's application run loop.
	if os.Getenv("NSGO_TEST_INPUT_ACCESS_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=30s")
		cmd.Env = append(os.Environ(), "NSGO_TEST_INPUT_ACCESS_PROCESS="+t.Name())
		if out, err := cmd.CombinedOutput(); err != nil || !bytes.Contains(out, []byte("\nPASS\n")) {
			t.Fatalf("native accessibility test did not pass (%v):\n%s", err, out)
		}
		return
	}
	onMain(func() {
		loadAppKit()
		for _, c := range []struct {
			name              string
			multiline, masked bool
			role              string
		}{
			{"field", false, false, "AXTextField"},
			{"secure field", false, true, "AXTextField"},
			{"text view", true, false, "AXTextArea"},
		} {
			window := objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
				apple.Rect(0, 0, 300, 120), uint(0), uint(2), false)
			window.Send(sel("setReleasedWhenClosed:"), false)
			v := &view{id: 1, kind: driver.KindInput, native: objc.ID(classInput).Send(selNew), input: &input{}}
			views[v.native] = v
			v.setEditor(c.multiline, c.masked)
			v.native.Send(sel("setFrame:"), apple.Rect(0, 0, 280, 100))
			v.input.host.Send(sel("setFrame:"), apple.Rect(0, 0, 280, 100))
			window.Send(sel("contentView")).Send(selAddSubview, v.native)
			v.setProps(&driver.ViewProps{Role: driver.RoleTextField, Label: "Search", TestID: "search"})
			config := driver.InputConfig{Multiline: c.multiline, Masked: c.masked, Placeholder: "Search messages",
				Style: driver.TextStyle{Font: driver.Font{Size: 14}, Color: core.RGB(0)}}
			v.configure(&config)
			if got := findAX(window, axIs(c.role, "Search")); got == 0 {
				t.Errorf("%s: no native accessibility element with role %s and label Search", c.name, c.role)
			}
			field := v.input.field
			if !c.multiline {
				field = field.Send(sel("cell"))
			}
			if got := apple.GoString(field.Send(sel("accessibilityRole"))); got != c.role {
				t.Errorf("%s: native role = %q, want %q", c.name, got, c.role)
			}
			if got := apple.GoString(field.Send(sel("accessibilityLabel"))); got != "Search" {
				t.Errorf("%s: native label = %q, want Search", c.name, got)
			}
			if got := apple.GoString(field.Send(sel("accessibilityPlaceholderValue"))); got != "Search messages" {
				t.Errorf("%s: native placeholder = %q", c.name, got)
			}
			if got := axAttr(field, "AXPlaceholderValue"); got != "Search messages" {
				t.Errorf("%s: AXPlaceholderValue = %q", c.name, got)
			}
			if !slices.Contains(axAttrNames(field), "AXPlaceholderValue") {
				t.Errorf("%s: native attributes lack AXPlaceholderValue", c.name)
			}
			if findAX(window, func(e objc.ID) bool {
				return apple.GoString(e.Send(sel("accessibilityRole"))) == "AXStaticText" && apple.GoString(e.Send(sel("accessibilityValue"))) == "Search messages"
			}) != 0 {
				t.Errorf("%s: the drawn placeholder is an accessibility element of its own", c.name)
			}
			v.setLabel("Find messages")
			if got := apple.GoString(field.Send(sel("accessibilityLabel"))); got != "Find messages" {
				t.Errorf("%s: updated native label = %q", c.name, got)
			}
			if findAX(window, axIs(c.role, "Find messages")) == 0 {
				t.Errorf("%s: updated label is absent from the window's accessibility tree", c.name)
			}
			for _, placeholder := range []string{"Type a query", ""} {
				config.Placeholder = placeholder
				v.configure(&config)
				if got := apple.GoString(field.Send(sel("accessibilityPlaceholderValue"))); got != placeholder {
					t.Errorf("%s: updated native placeholder = %q, want %q", c.name, got, placeholder)
				}
				if got := axAttr(field, "AXPlaceholderValue"); got != placeholder {
					t.Errorf("%s: updated AXPlaceholderValue = %q, want %q", c.name, got, placeholder)
				}
			}
			v.setLabel("")
			if got := apple.GoString(field.Send(sel("accessibilityLabel"))); got != "" {
				t.Errorf("%s: cleared native label = %q", c.name, got)
			}
			delete(views, v.native)
			delete(views, v.input.field)
			delete(views, v.input.host)
			v.native.Send(selRelease)
			window.Send(selRelease)
		}
	})
}

// axIs matches an element by its role and label.
func axIs(role, label string) func(objc.ID) bool {
	return func(e objc.ID) bool {
		return apple.GoString(e.Send(sel("accessibilityRole"))) == role && apple.GoString(e.Send(sel("accessibilityLabel"))) == label
	}
}

// findAX is the first element under root, as accessibility clients walk it through accessibilityChildren, that match takes.
func findAX(root objc.ID, match func(objc.ID) bool) objc.ID {
	seen := make(map[objc.ID]bool)
	var find func(objc.ID) objc.ID
	find = func(e objc.ID) objc.ID {
		if e == 0 || seen[e] {
			return 0
		}
		seen[e] = true
		if match(e) {
			return e
		}
		children := e.Send(sel("accessibilityChildren"))
		for i := range objc.Send[int](children, sel("count")) {
			if found := find(children.Send(sel("objectAtIndex:"), i)); found != 0 {
				return found
			}
		}
		return 0
	}
	return find(root)
}

// TestEditableCombobox checks, through accessibilityAttributeValue: as VoiceOver reads them, that an input with RoleCombobox is an editable combobox on its editor, the field's cell or the text view, with its text, label, value text and the popup's state, its view no element; that a plain input's editor, masked too, takes its label; and that a view with RoleCombobox stays a pop-up button (#246).
func TestEditableCombobox(t *testing.T) { onMain(func() { editableCombobox(t) }) }

func editableCombobox(t *testing.T) {
	loadAppKit()
	attr, names := axAttr, axAttrNames
	for _, c := range []struct {
		name              string
		multiline, masked bool
		role, plainRole   string
	}{
		{"field", false, false, "AXComboBox", "AXTextField"},
		{"masked field", false, true, "", "AXTextField"},
		{"text view", true, false, "AXComboBox", "AXTextArea"},
	} {
		v := &view{kind: driver.KindInput, native: objc.ID(classInput).Send(selNew), input: &input{}}
		v.setEditor(c.multiline, c.masked)
		v.input.setText("Banana")
		editor := v.input.field
		if !c.multiline {
			editor = editor.Send(sel("cell"))
		}
		plain := attr(editor, "AXRoleDescription")
		v.setProps(&driver.ViewProps{Role: driver.RoleTextField, Label: "Name", TestID: "name"})
		if got := attr(editor, "AXRole"); got != c.plainRole {
			t.Errorf("%s: a plain input's role %q, want %q", c.name, got, c.plainRole)
		}
		if got := attr(editor, "AXDescription"); got != "Name" {
			t.Errorf("%s: a plain input's label %q", c.name, got)
		}
		if got := attr(editor, "AXIdentifier"); got != "name" {
			t.Errorf("%s: a plain input's identifier %q", c.name, got)
		}
		if got := attr(editor, "AXExpanded"); got != "" {
			t.Errorf("%s: a plain input is expanded: %q", c.name, got)
		}
		if !slices.Contains(names(editor), "AXDescription") {
			t.Errorf("%s: a plain input's attributes lack AXDescription: %v", c.name, names(editor))
		}
		if c.role == "" {
			v.native.Send(selRelease)
			delete(views, v.input.field)
			delete(views, v.input.host)
			continue
		}
		v.setProps(&driver.ViewProps{Role: driver.RoleCombobox, Label: "Fruit", TestID: "fruit", Expandable: true, Expanded: true})
		if objc.Send[bool](v.native, sel("isAccessibilityElement")) {
			t.Errorf("%s: a combobox input's view is an element", c.name)
		}
		for name, want := range map[string]string{"AXRole": c.role, "AXDescription": "Fruit", "AXIdentifier": "fruit", "AXExpanded": "1", "AXValue": "Banana"} {
			if got := attr(editor, name); got != want {
				t.Errorf("%s: %s %q, want %q", c.name, name, got, want)
			}
		}
		if got := attr(editor, "AXRoleDescription"); got == "" || got == plain {
			t.Errorf("%s: a combobox's role description %q, a plain input's %q", c.name, got, plain)
		}
		if got := names(editor); !slices.Contains(got, "AXExpanded") || !slices.Contains(got, "AXValue") {
			t.Errorf("%s: a combobox's attributes %v", c.name, got)
		}
		if got := attr(editor, "AXValueDescription"); got != "" {
			t.Errorf("%s: a combobox without a value text describes its value as %q", c.name, got)
		}
		v.setProps(&driver.ViewProps{Role: driver.RoleCombobox, Label: "Fruit", Expandable: true, Value: driver.AccessibilityValue{Text: "Banana"}})
		if got := attr(editor, "AXExpanded"); got != "0" {
			t.Errorf("%s: a closed combobox's AXExpanded %q", c.name, got)
		}
		if got := attr(editor, "AXValueDescription"); got != "" {
			t.Errorf("%s: a value text that is the editor's text is described again as %q", c.name, got)
		}
		// A multiple combobox's chosen items, while its editor holds the query.
		v.setProps(&driver.ViewProps{Role: driver.RoleCombobox, Label: "Fruits", Expandable: true, Value: driver.AccessibilityValue{Text: "Apple, Cherry"}})
		if got := attr(editor, "AXValueDescription"); got != "Apple, Cherry" {
			t.Errorf("%s: a multiple combobox's value description %q, want its chosen items", c.name, got)
		}
		if got := attr(editor, "AXValue"); got != "Banana" {
			t.Errorf("%s: a multiple combobox's value %q, want the editor's text", c.name, got)
		}
		if got := names(editor); !slices.Contains(got, "AXValueDescription") {
			t.Errorf("%s: a described combobox's attributes %v", c.name, got)
		}
		v.native.Send(selRelease)
		delete(views, v.input.field)
		delete(views, v.input.host)
	}

	trigger := &view{id: 1, kind: driver.KindView, native: objc.ID(classView).Send(selNew)}
	views[trigger.native] = trigger
	trigger.setProps(&driver.ViewProps{Role: driver.RoleCombobox, Label: "Kind", Expandable: true})
	// A view outside a window answers accessibilityAttributeValue: with AXUnknown: read its properties.
	if got := apple.GoString(trigger.native.Send(sel("accessibilityRole"))); got != "AXPopUpButton" {
		t.Errorf("a select's trigger is %q, want AXPopUpButton", got)
	}
	if got := apple.GoString(trigger.native.Send(sel("accessibilityLabel"))); got != "Kind" {
		t.Errorf("a select's trigger's label %q", got)
	}
	delete(views, trigger.native)
	trigger.native.Send(selRelease)
}

// actionEngine is a platform that shows nothing, for an engine that only hears the events a test's views report.
type actionEngine struct{ apple.TextEngine }

func (actionEngine) Mount(*driver.MountBatch)  {}
func (actionEngine) RequestFrame()             {}
func (actionEngine) SubmitGPU(driver.GPUFrame) {}

// TestEditableSpinButton checks, through the accessibility API VoiceOver uses, that an input with RoleSpinButton is an editable incrementor on its editor, the field's cell or the text view, with its text, label and test ID, whose AXIncrement and AXDecrement report the input's actions, its view no element; that an input without the actions offers none; and that a view with RoleSpinButton stays an incrementor of its own (#254).
func TestEditableSpinButton(t *testing.T) { onMain(func() { editableSpinButton(t) }) }

func editableSpinButton(t *testing.T) {
	loadAppKit()
	var reported []driver.Event
	eng := engine.Start(actionEngine{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{Event: func(_ uint32, e driver.Event) {
		if e.Kind == driver.EventAccessibilityAction {
			reported = append(reported, e)
		}
	}})
	defer eng.Close()
	s := &surface{engine: eng}
	s.animator = driver.NewAnimator(func(driver.Event) {})
	actions := &[]driver.AccessibilityAction{{Name: driver.ActionIncrement}, {Name: driver.ActionDecrement}}
	for _, c := range []struct {
		name      string
		multiline bool
		plainRole string
	}{
		{"field", false, "AXTextField"},
		{"text view", true, "AXTextArea"},
	} {
		v := &view{id: 7, kind: driver.KindInput, surface: s, native: objc.ID(classInput).Send(selNew), input: &input{}}
		v.events = 1 << driver.EventAccessibilityAction
		v.setEditor(c.multiline, false)
		v.input.setText("12")
		editor := v.input.field
		if !c.multiline {
			editor = editor.Send(sel("cell"))
		}
		plain := axAttr(editor, "AXRoleDescription")
		v.setProps(&driver.ViewProps{Role: driver.RoleTextField, Label: "Count"})
		if got := axActionNames(editor); slices.Contains(got, "AXIncrement") || slices.Contains(got, "AXDecrement") {
			t.Errorf("%s: a plain input offers %v", c.name, got)
		}
		if got := axAttr(editor, "AXRole"); got != c.plainRole {
			t.Errorf("%s: a plain input's role %q, want %q", c.name, got, c.plainRole)
		}

		v.setProps(&driver.ViewProps{Role: driver.RoleSpinButton, Label: "Count", TestID: "count", Actions: actions,
			Value: driver.AccessibilityValue{Min: 0, Max: 20, Now: 12, Text: "12"}})
		if objc.Send[bool](v.native, sel("isAccessibilityElement")) {
			t.Errorf("%s: a spin button input's view is an element", c.name)
		}
		for name, want := range map[string]string{"AXRole": "AXIncrementor", "AXDescription": "Count", "AXIdentifier": "count", "AXValue": "12", "AXValueDescription": ""} {
			if got := axAttr(editor, name); got != want {
				t.Errorf("%s: %s %q, want %q", c.name, name, got, want)
			}
		}
		if got := axAttr(editor, "AXRoleDescription"); got == "" || got == plain {
			t.Errorf("%s: a spin button's role description %q, a plain input's %q", c.name, got, plain)
		}
		got := axActionNames(editor)
		if !slices.Contains(got, "AXIncrement") || !slices.Contains(got, "AXDecrement") || slices.Contains(got, "AXCancel") {
			t.Errorf("%s: a spin button's actions %v", c.name, got)
		}
		if d := apple.GoString(editor.Send(sel("accessibilityActionDescription:"), apple.NSString("AXIncrement"))); d == "" {
			t.Errorf("%s: AXIncrement has no description", c.name)
		}
		reported = nil
		editor.Send(sel("accessibilityPerformAction:"), apple.NSString("AXIncrement"))
		editor.Send(sel("accessibilityPerformAction:"), apple.NSString("AXDecrement"))
		editor.Send(sel("accessibilityPerformAction:"), apple.NSString("AXCancel"))
		var names []string
		for _, e := range reported {
			if e.ID != v.id {
				t.Errorf("%s: an action reported for view %d", c.name, e.ID)
			}
			names = append(names, e.Text)
		}
		if want := []string{driver.ActionIncrement, driver.ActionDecrement}; !slices.Equal(names, want) {
			t.Errorf("%s: performing AXIncrement, AXDecrement and AXCancel reported %v, want %v", c.name, names, want)
		}
		v.native.Send(selRelease)
		delete(views, v.input.field)
		delete(views, v.input.host)
	}

	stepper := &view{id: 1, kind: driver.KindView, native: objc.ID(classView).Send(selNew)}
	views[stepper.native] = stepper
	stepper.setProps(&driver.ViewProps{Role: driver.RoleSpinButton, Label: "Count", Actions: actions})
	if got := apple.GoString(stepper.native.Send(sel("accessibilityRole"))); got != "AXIncrementor" {
		t.Errorf("a spin button view is %q, want AXIncrementor", got)
	}
	if !objc.Send[bool](stepper.native, sel("isAccessibilityElement")) {
		t.Error("a spin button view is no element")
	}
	delete(views, stepper.native)
	stepper.native.Send(selRelease)
}

// TestSearchFieldAndStatus checks, through the accessibility API VoiceOver uses, that an input with RoleSearchField stays an editable text field on its editor, with AXSearchField's subrole and role description, its view no element, and that a view with RoleStatus is a group with AXApplicationStatus's subrole whose children stay elements.
func TestSearchFieldAndStatus(t *testing.T) { onMain(func() { searchFieldAndStatus(t) }) }

func searchFieldAndStatus(t *testing.T) {
	loadAppKit()
	v := &view{kind: driver.KindInput, native: objc.ID(classInput).Send(selNew), input: &input{}}
	v.setEditor(false, false)
	v.input.setText("query")
	editor := v.input.field.Send(sel("cell"))
	v.setProps(&driver.ViewProps{Role: driver.RoleTextField, Label: "Find"})
	plain := axAttr(editor, "AXRoleDescription")
	v.setProps(&driver.ViewProps{Role: driver.RoleSearchField, Label: "Find", TestID: "find"})
	if objc.Send[bool](v.native, sel("isAccessibilityElement")) {
		t.Error("a search field's view is an element")
	}
	for name, want := range map[string]string{"AXRole": "AXTextField", "AXSubrole": "AXSearchField", "AXDescription": "Find", "AXIdentifier": "find", "AXValue": "query"} {
		if got := axAttr(editor, name); got != want {
			t.Errorf("a search field's %s %q, want %q", name, got, want)
		}
	}
	if got := axAttr(editor, "AXRoleDescription"); got == "" || got == plain {
		t.Errorf("a search field's role description %q, a plain input's %q", got, plain)
	}
	if got := axAttrNames(editor); !slices.Contains(got, "AXSubrole") {
		t.Errorf("a search field's attributes %v", got)
	}
	v.native.Send(selRelease)
	delete(views, v.input.field)
	delete(views, v.input.host)

	status := &view{id: 1, kind: driver.KindView, native: objc.ID(classView).Send(selNew)}
	views[status.native] = status
	status.setProps(&driver.ViewProps{Role: driver.RoleStatus, LiveRegion: driver.LiveRegionPolite})
	if got := apple.GoString(status.native.Send(sel("accessibilityRole"))); got != "AXGroup" {
		t.Errorf("a status is %q, want AXGroup", got)
	}
	if got := apple.GoString(status.native.Send(sel("accessibilitySubrole"))); got != "AXApplicationStatus" {
		t.Errorf("a status's subrole %q, want AXApplicationStatus", got)
	}
	if status.axGrouped {
		t.Error("a status hides its children")
	}
	delete(views, status.native)
	status.native.Send(selRelease)
}

// TestInvalidAccessibility checks, through accessibilityAttributeNames and accessibilityAttributeValue: in a window's accessibility tree, that views, input editors (plain, masked and multiline) and SystemSwitch answer AXInvalid with NSString "true"/"false" and post AXInvalidStatusChanged only when Invalid transitions, while non-element and hidden views neither expose AXInvalid nor post notifications (#1592).
func TestInvalidAccessibility(t *testing.T) {
	if os.Getenv("NSGO_TEST_INVALID_ACCESS_PROCESS") != t.Name() {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=30s")
		cmd.Env = append(os.Environ(), "NSGO_TEST_INVALID_ACCESS_PROCESS="+t.Name())
		if out, err := cmd.CombinedOutput(); err != nil || !bytes.Contains(out, []byte("\nPASS\n")) {
			t.Fatalf("native invalid accessibility test did not pass (%v):\n%s", err, out)
		}
		return
	}
	onMain(func() {
		loadAppKit()
		type posted struct {
			element objc.ID
			name    string
		}
		var notes []posted
		origPost := axPost
		realPost := origPost()
		axPost = func() func(element, name, info objc.ID) {
			return func(element, name, info objc.ID) {
				notes = append(notes, posted{element: element, name: apple.GoString(name)})
				realPost(element, name, info)
			}
		}
		defer func() { axPost = origPost }()

		nsStringClass := objc.ID(apple.Class("NSString"))
		checkAXInvalid := func(label string, el objc.ID, want string) {
			t.Helper()
			if names := axAttrNames(el); !slices.Contains(names, axAttrInvalid) {
				t.Errorf("%s: accessibilityAttributeNames lacks %s: %v", label, axAttrInvalid, names)
			}
			raw := el.Send(sel("accessibilityAttributeValue:"), apple.NSString(axAttrInvalid))
			if raw == 0 {
				t.Errorf("%s: AXInvalid value is nil, want %q", label, want)
				return
			}
			if !objc.Send[bool](raw, sel("isKindOfClass:"), nsStringClass) {
				t.Errorf("%s: AXInvalid value is not an NSString", label)
			}
			if got := apple.GoString(raw); got != want {
				t.Errorf("%s: AXInvalid = %q, want %q", label, got, want)
			}
			if objc.Send[bool](el, sel("accessibilityIsAttributeSettable:"), apple.NSString(axAttrInvalid)) {
				t.Errorf("%s: AXInvalid reported settable", label)
			}
		}
		checkNotes := func(label string, want ...posted) {
			t.Helper()
			if !slices.Equal(notes, want) {
				t.Errorf("%s: notifications = %+v, want %+v", label, notes, want)
			}
			notes = nil
		}

		window := objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(0, 0, 320, 200), uint(0), uint(2), false)
		window.Send(sel("setReleasedWhenClosed:"), false)
		defer window.Send(selRelease)
		content := window.Send(sel("contentView"))

		// 1. Regular accessible view (checkbox) inside a window.
		box := &view{id: 1, kind: driver.KindView, native: objc.ID(classView).Send(selNew)}
		views[box.native] = box
		box.native.Send(sel("setFrame:"), apple.Rect(0, 0, 120, 30))
		content.Send(selAddSubview, box.native)
		box.setProps(&driver.ViewProps{Role: driver.RoleCheckbox, Label: "Terms", Checked: true})
		checkNotes("view mount valid")
		if got := findAX(window, axIs("AXCheckBox", "Terms")); got != box.native {
			t.Fatalf("view: findAX returned %v, want %v", got, box.native)
		}
		if got := apple.GoString(box.native.Send(sel("accessibilityRole"))); got != "AXCheckBox" {
			t.Errorf("view: accessibilityRole = %q, want AXCheckBox", got)
		}
		if got := apple.GoString(box.native.Send(sel("accessibilityValue")).Send(sel("description"))); got != "1" {
			t.Errorf("view: accessibilityValue = %q, want 1", got)
		}
		checkAXInvalid("view valid", box.native, "false")

		box.setProps(&driver.ViewProps{Role: driver.RoleCheckbox, Label: "Terms", Checked: true, Invalid: true})
		checkAXInvalid("view invalid", box.native, "true")
		checkNotes("view false->true", posted{element: box.native, name: axInvalidStatusChanged})

		// Unchanged Invalid should not re-post.
		box.setProps(&driver.ViewProps{Role: driver.RoleCheckbox, Label: "Accept terms", Checked: true, Invalid: true})
		checkAXInvalid("view still invalid", box.native, "true")
		checkNotes("view true->true")

		box.setProps(&driver.ViewProps{Role: driver.RoleCheckbox, Label: "Accept terms", Checked: true, Invalid: false})
		checkAXInvalid("view valid again", box.native, "false")
		checkNotes("view true->false", posted{element: box.native, name: axInvalidStatusChanged})

		box.setProps(&driver.ViewProps{Role: driver.RoleCheckbox, Label: "Accept terms", Checked: false, Invalid: false})
		checkNotes("view false->false")

		box.native.Send(sel("removeFromSuperview"))
		delete(views, box.native)
		box.native.Send(selRelease)

		// 2. View mounted already invalid: exposes "true" without posting on mount.
		mountedBad := &view{id: 2, kind: driver.KindView, native: objc.ID(classView).Send(selNew)}
		views[mountedBad.native] = mountedBad
		mountedBad.native.Send(sel("setFrame:"), apple.Rect(0, 0, 120, 30))
		content.Send(selAddSubview, mountedBad.native)
		mountedBad.setProps(&driver.ViewProps{Role: driver.RoleButton, Label: "Submit", Invalid: true})
		checkAXInvalid("mounted invalid view", mountedBad.native, "true")
		checkNotes("view mount invalid")
		mountedBad.setProps(&driver.ViewProps{Role: driver.RoleButton, Label: "Submit", Invalid: false})
		checkAXInvalid("mounted invalid view cleared", mountedBad.native, "false")
		checkNotes("mounted invalid true->false", posted{element: mountedBad.native, name: axInvalidStatusChanged})
		mountedBad.native.Send(sel("removeFromSuperview"))
		delete(views, mountedBad.native)
		mountedBad.native.Send(selRelease)

		// 3. Non-element and hidden views neither expose AXInvalid nor post notifications.
		for _, c := range []struct {
			name  string
			props driver.ViewProps
		}{
			{"plain container", driver.ViewProps{}},
			{"accessible no", driver.ViewProps{Accessible: driver.AccessibleNo, Label: "Ignored"}},
			{"hidden", driver.ViewProps{Role: driver.RoleButton, Label: "Hidden", ElementsHidden: true}},
		} {
			nv := &view{id: 3, kind: driver.KindView, native: objc.ID(classView).Send(selNew)}
			views[nv.native] = nv
			nv.native.Send(sel("setFrame:"), apple.Rect(0, 0, 100, 30))
			content.Send(selAddSubview, nv.native)
			p := c.props
			nv.setProps(&p)
			if names := axAttrNames(nv.native); slices.Contains(names, axAttrInvalid) {
				t.Errorf("%s: non-element unexpectedly exposes %s", c.name, axAttrInvalid)
			}
			p.Invalid = true
			nv.setProps(&p)
			checkNotes(c.name + " false->true")
			p.Invalid = false
			nv.setProps(&p)
			checkNotes(c.name + " true->false")
			nv.native.Send(sel("removeFromSuperview"))
			delete(views, nv.native)
			nv.native.Send(selRelease)
		}

		// 4. Text inputs: plain field, masked field, and multiline text view.
		for _, c := range []struct {
			name              string
			multiline, masked bool
			role              string
		}{
			{"field", false, false, "AXTextField"},
			{"secure field", false, true, "AXTextField"},
			{"text view", true, false, "AXTextArea"},
		} {
			v := &view{id: 4, kind: driver.KindInput, native: objc.ID(classInput).Send(selNew), input: &input{}}
			views[v.native] = v
			v.setEditor(c.multiline, c.masked)
			v.native.Send(sel("setFrame:"), apple.Rect(0, 0, 280, 100))
			v.input.host.Send(sel("setFrame:"), apple.Rect(0, 0, 280, 100))
			content.Send(selAddSubview, v.native)
			v.setProps(&driver.ViewProps{Role: driver.RoleTextField, Label: "Email"})
			checkNotes(c.name + " mount")
			editor := v.accessElement()
			if got := findAX(window, axIs(c.role, "Email")); got != editor {
				t.Fatalf("%s: findAX returned %v, want editor %v", c.name, got, editor)
			}
			if names := axAttrNames(v.native); slices.Contains(names, axAttrInvalid) {
				t.Errorf("%s: input container view unexpectedly exposes %s", c.name, axAttrInvalid)
			}
			checkAXInvalid(c.name+" valid", editor, "false")

			v.setProps(&driver.ViewProps{Role: driver.RoleTextField, Label: "Email", Invalid: true})
			checkAXInvalid(c.name+" invalid", editor, "true")
			checkNotes(c.name+" false->true", posted{element: editor, name: axInvalidStatusChanged})

			v.setProps(&driver.ViewProps{Role: driver.RoleTextField, Label: "Work email", Invalid: true})
			checkAXInvalid(c.name+" still invalid", editor, "true")
			checkNotes(c.name + " true->true")

			v.setProps(&driver.ViewProps{Role: driver.RoleTextField, Label: "Work email", Invalid: false})
			checkAXInvalid(c.name+" valid again", editor, "false")
			checkNotes(c.name+" true->false", posted{element: editor, name: axInvalidStatusChanged})

			v.native.Send(sel("removeFromSuperview"))
			delete(views, v.native)
			delete(views, v.input.field)
			delete(views, v.input.host)
			v.native.Send(selRelease)
		}

		// 5. Native SystemSwitch (NSSwitch).
		swView := &view{id: 5, kind: driver.KindNative, native: objc.ID(classView).Send(selNew)}
		views[swView.native] = swView
		swComp := newSwitch(driver.NewNativeContext(func(string, any) {}, func(driver.Size) {})).(*nsSwitch)
		swView.component = swComp
		swView.componentView = swComp.view
		swView.native.Send(sel("setFrame:"), apple.Rect(0, 0, 60, 30))
		swComp.view.Send(sel("setFrame:"), apple.Rect(0, 0, 60, 30))
		swView.native.Send(selAddSubview, swComp.view)
		content.Send(selAddSubview, swView.native)
		swView.setProps(&driver.ViewProps{Label: "Notifications"})
		checkNotes("switch mount")
		swEl := swView.accessElement()
		if got := findAX(window, func(e objc.ID) bool {
			return apple.GoString(e.Send(sel("accessibilityLabel"))) == "Notifications"
		}); got != swEl {
			t.Fatalf("switch: findAX returned %v, want %v", got, swEl)
		}
		checkAXInvalid("switch valid", swEl, "false")

		swView.setProps(&driver.ViewProps{Label: "Notifications", Invalid: true})
		checkAXInvalid("switch invalid", swEl, "true")
		checkNotes("switch false->true", posted{element: swEl, name: axInvalidStatusChanged})

		swView.setProps(&driver.ViewProps{Label: "Notifications", Invalid: false})
		checkAXInvalid("switch valid again", swEl, "false")
		checkNotes("switch true->false", posted{element: swEl, name: axInvalidStatusChanged})

		swView.native.Send(sel("removeFromSuperview"))
		swComp.Close()
		delete(views, swView.native)
		swView.native.Send(selRelease)
	})
}

// TestAccessibilityLevel checks, through the NSAccessibility properties VoiceOver reads, that a heading exposes ViewProps.Level as a 1-based accessibilityValue without disclosure level, that a tree item exposes it as a 0-based accessibilityDisclosureLevel, and that clearing Level removes both (#1594).
func TestAccessibilityLevel(t *testing.T) { onMain(func() { accessibilityLevel(t) }) }

func accessibilityLevel(t *testing.T) {
	loadAppKit()
	heading := &view{id: 1, kind: driver.KindView, native: objc.ID(classView).Send(selNew)}
	views[heading.native] = heading
	heading.setProps(&driver.ViewProps{Role: driver.RoleHeader, Label: "Section", Level: 3})
	if got := apple.GoString(heading.native.Send(sel("accessibilityRole"))); got != "AXHeading" {
		t.Errorf("heading role = %q, want AXHeading", got)
	}
	if val := heading.native.Send(sel("accessibilityValue")); val == 0 || objc.Send[int](val, sel("intValue")) != 3 {
		t.Errorf("heading level 3 accessibilityValue = %v, want 3", val)
	}
	if objc.Send[bool](heading.native, sel("isAccessibilitySelectorAllowed:"), selAccessibilityDisclosureLevel) {
		t.Error("heading allowed accessibilityDisclosureLevel")
	}
	heading.setProps(&driver.ViewProps{Role: driver.RoleHeader, Label: "Section", Level: 0})
	if val := heading.native.Send(sel("accessibilityValue")); val != 0 {
		t.Errorf("cleared heading accessibilityValue = %v, want nil", val)
	}

	item := &view{id: 2, kind: driver.KindView, native: objc.ID(classView).Send(selNew)}
	views[item.native] = item
	item.setProps(&driver.ViewProps{Role: driver.RoleTreeItem, Label: "Child", Level: 2})
	if !objc.Send[bool](item.native, sel("isAccessibilitySelectorAllowed:"), selAccessibilityDisclosureLevel) {
		t.Error("tree item with level 2 disallowed accessibilityDisclosureLevel")
	}
	if got := objc.Send[int](item.native, selAccessibilityDisclosureLevel); got != 1 {
		t.Errorf("tree item level 2 accessibilityDisclosureLevel = %d, want 1", got)
	}
	item.setProps(&driver.ViewProps{Role: driver.RoleTreeItem, Label: "Root", Level: 1})
	if got := objc.Send[int](item.native, selAccessibilityDisclosureLevel); got != 0 {
		t.Errorf("tree item level 1 accessibilityDisclosureLevel = %d, want 0", got)
	}
	item.setProps(&driver.ViewProps{Role: driver.RoleTreeItem, Label: "Plain", Level: 0})
	if objc.Send[bool](item.native, sel("isAccessibilitySelectorAllowed:"), selAccessibilityDisclosureLevel) {
		t.Error("cleared tree item still allowed accessibilityDisclosureLevel")
	}

	delete(views, heading.native)
	delete(views, item.native)
	heading.native.Send(selRelease)
	item.native.Send(selRelease)
}

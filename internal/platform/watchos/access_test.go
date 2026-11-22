package watchos

import (
	"encoding/binary"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWatchAccessVisibleLabelAndControl(t *testing.T) {
	// A visible label (Text.LabelFor) has EventPress solely to forward
	// activation, so LabelPress must prevent inferring a button trait while
	// exposing the resolved name and projected disabled/invalid states.
	labelProps := &driver.ViewProps{
		Role:       driver.RoleText,
		Label:      "Email",
		LabelFor:   2,
		LabelPress: true,
		Disabled:   true,
		Invalid:    true,
		NoFocus:    true,
	}
	la := watchAccessOf(driver.KindText, labelProps, 1<<driver.EventPress, "Email", false)
	if la.flags&accessElement == 0 {
		t.Fatal("visible label is not an accessibility element")
	}
	if la.flags&accessDisabled == 0 {
		t.Fatal("visible label missing projected disabled state")
	}
	if la.traits&traitButton != 0 {
		t.Fatalf("visible label inferred button trait: %#x", la.traits)
	}
	if la.traits&traitStaticText == 0 {
		t.Fatalf("visible text label missing staticText trait: %#x", la.traits)
	}
	if la.label != "Email" {
		t.Fatalf("label = %q, want Email", la.label)
	}
	if la.value != "invalid data" {
		t.Fatalf("value = %q, want invalid data", la.value)
	}

	// A container View with LabelFor exposes the resolved name and projected
	// states as one element without inferring a button trait.
	viewLabelProps := &driver.ViewProps{
		Label:      "Account email",
		LabelFor:   2,
		LabelPress: true,
		Disabled:   true,
		Invalid:    true,
	}
	va := watchAccessOf(driver.KindView, viewLabelProps, 1<<driver.EventPress, "", false)
	if va.flags&accessElement == 0 || va.flags&accessDisabled == 0 {
		t.Fatalf("view label flags = %#x, want element and disabled", va.flags)
	}
	if va.traits&traitButton != 0 {
		t.Fatalf("view label inferred button trait: %#x", va.traits)
	}
	if va.label != "Account email" || va.value != "invalid data" {
		t.Fatalf("view label = %+v", va)
	}

	// An input labelled by visible labels gets the resolved name, hint,
	// disabled and invalid states on its native FieldNode (element=false on
	// wrapper so SwiftUI keeps FieldNode's TextField as the element).
	ids := []driver.NodeID{1}
	fieldProps := &driver.ViewProps{
		Role:        driver.RoleTextField,
		Label:       "Email",
		LabelledBy:  &ids,
		Description: "Work address",
		Hint:        "Double tap to edit",
		Disabled:    true,
		Invalid:     true,
	}
	fa := watchAccessOf(driver.KindInput, fieldProps, 0, "", true)
	if fa.flags&accessElement != 0 {
		t.Fatalf("input wrapper should not shadow TextField element: flags=%#x", fa.flags)
	}
	if fa.flags&accessDisabled == 0 {
		t.Fatal("input missing disabled state")
	}
	if fa.label != "Email" {
		t.Fatalf("input label = %q, want Email", fa.label)
	}
	if fa.hint != "Work address. Double tap to edit" {
		t.Fatalf("input hint = %q", fa.hint)
	}
	if fa.value != "invalid data" {
		t.Fatalf("input value = %q, want invalid data", fa.value)
	}
}

func TestWatchAccessUpdatesHiddenAndNonAccessible(t *testing.T) {
	// Dynamic label & hint changes, plus clearing invalid state.
	p := &driver.ViewProps{
		Role:    driver.RoleCheckbox,
		Label:   "Terms",
		Hint:    "Accept terms",
		Checked: true,
		Invalid: true,
	}
	a := watchAccessOf(driver.KindView, p, 1<<driver.EventPress, "", false)
	if a.traits&traitButton == 0 || a.label != "Terms" || a.hint != "Accept terms" || a.value != "1, invalid data" {
		t.Fatalf("checkbox access = %+v", a)
	}
	p.Label = "Privacy policy"
	p.Hint = ""
	p.Invalid = false
	a2 := watchAccessOf(driver.KindView, p, 1<<driver.EventPress, "", false)
	if a2.label != "Privacy policy" || a2.hint != "" || a2.value != "1" {
		t.Fatalf("updated checkbox access = %+v", a2)
	}

	// Hidden subtree (ElementsHidden) and non-accessible node (AccessibleNo).
	hidden := watchAccessOf(driver.KindView, &driver.ViewProps{
		Label:          "Tooltip",
		Accessible:     driver.AccessibleNo,
		ElementsHidden: true,
	}, 0, "", false)
	if hidden.flags&accessHidden == 0 || hidden.flags&accessElement != 0 {
		t.Fatalf("hidden node flags = %#x, want hidden and non-element", hidden.flags)
	}

	nonAcc := watchAccessOf(driver.KindView, &driver.ViewProps{
		Label:      "Decorative",
		Accessible: driver.AccessibleNo,
	}, 0, "", false)
	if nonAcc.flags&accessHidden != 0 || nonAcc.flags&accessElement != 0 {
		t.Fatalf("non-accessible node flags = %#x, want neither hidden nor element", nonAcc.flags)
	}

	// Plain Text without explicit Label falls back to its text content.
	plain := watchAccessOf(driver.KindText, &driver.ViewProps{}, 0, "Hello watch", false)
	if plain.flags&accessElement == 0 || plain.label != "Hello watch" || plain.traits&traitStaticText == 0 {
		t.Fatalf("plain text access = %+v", plain)
	}

	// Dialog container keeps children accessible (element=false) and sets modal.
	dlg := watchAccessOf(driver.KindView, &driver.ViewProps{Role: driver.RoleDialog, Label: "Confirm"}, 0, "", false)
	if dlg.flags&accessElement != 0 || dlg.flags&accessModal == 0 {
		t.Fatalf("dialog flags = %#x, want modal and non-element", dlg.flags)
	}
}

func TestWatchAccessWireFormat(t *testing.T) {
	var w writer
	want := watchAccess{
		flags:  accessElement | accessDisabled,
		traits: traitStaticText,
		label:  "Email",
		hint:   "Required",
		value:  "invalid data",
	}
	w.access(7, want)
	b := w.buf
	if len(b) == 0 || b[0] != opAccess {
		t.Fatalf("op = %v, want %d", b, opAccess)
	}
	pos := 1
	readU32 := func() uint32 {
		v := binary.LittleEndian.Uint32(b[pos : pos+4])
		pos += 4
		return v
	}
	readStr := func() string {
		n := int(readU32())
		s := string(b[pos : pos+n])
		pos += n
		return s
	}
	if id := readU32(); id != 7 {
		t.Fatalf("id = %d, want 7", id)
	}
	flags := b[pos]
	pos++
	traits := readU32()
	label, hint, value := readStr(), readStr(), readStr()
	if pos != len(b) {
		t.Fatalf("trailing bytes: %d of %d", pos, len(b))
	}
	got := watchAccess{flags: flags, traits: traits, label: label, hint: hint, value: value}
	if got != want {
		t.Fatalf("decoded = %+v, want %+v", got, want)
	}
}

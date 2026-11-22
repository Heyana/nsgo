package linux

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeFocusNode struct {
	name          string
	kind          driver.ViewKind
	hidden        bool
	deleted       bool
	props         driver.ViewProps
	events        driver.EventMask
	inputDisabled bool
	children      []*fakeFocusNode

	viewFocusable   bool
	editorFocusable bool
	viewUpdates     int
	editorUpdates   int
}

func (n *fakeFocusNode) focusKind() driver.ViewKind      { return n.kind }
func (n *fakeFocusNode) focusHidden() bool               { return n.hidden }
func (n *fakeFocusNode) focusDeleted() bool              { return n.deleted }
func (n *fakeFocusNode) focusChildren() []*fakeFocusNode { return n.children }
func (n *fakeFocusNode) focusStopSelf() bool {
	return viewCanFocusStop(n.kind, &n.props, n.events, n.inputDisabled)
}

func (n *fakeFocusNode) apply(viewFocus, editorFocus bool) {
	if n.viewFocusable != viewFocus {
		n.viewFocusable = viewFocus
		n.viewUpdates++
	}
	if n.editorFocusable != editorFocus {
		n.editorFocusable = editorFocus
		n.editorUpdates++
	}
}

func TestViewCanFocusStop(t *testing.T) {
	press := driver.EventMask(1 << driver.EventPress)
	longPress := driver.EventMask(1 << driver.EventLongPress)
	pressDown := driver.EventMask(1 << driver.EventPressDown)
	pressUp := driver.EventMask(1 << driver.EventPressUp)

	cases := []struct {
		name          string
		kind          driver.ViewKind
		props         driver.ViewProps
		events        driver.EventMask
		inputDisabled bool
		want          bool
	}{
		{"plain view", driver.KindView, driver.ViewProps{}, 0, false, false},
		{"plain text", driver.KindText, driver.ViewProps{Label: "hello"}, 0, false, false},
		{"pressable button", driver.KindView, driver.ViewProps{}, press, false, true},
		{"long press view", driver.KindView, driver.ViewProps{}, longPress, false, true},
		{"press down view", driver.KindView, driver.ViewProps{}, pressDown, false, true},
		{"press up view", driver.KindView, driver.ViewProps{}, pressUp, false, true},
		{"explicit focusable", driver.KindView, driver.ViewProps{Focusable: true}, 0, false, true},
		{"disabled pressable", driver.KindView, driver.ViewProps{Disabled: true}, press, false, false},
		{"disabled focusable", driver.KindView, driver.ViewProps{Disabled: true, Focusable: true}, 0, false, false},
		{"no-focus pressable", driver.KindView, driver.ViewProps{NoFocus: true}, press, false, false},
		{"no-focus explicit", driver.KindView, driver.ViewProps{NoFocus: true, Focusable: true}, 0, false, false},
		{"label-only press", driver.KindText, driver.ViewProps{LabelPress: true, NoFocus: true}, press, false, false},
		{"enabled input", driver.KindInput, driver.ViewProps{}, 0, false, true},
		{"input disabled by view props", driver.KindInput, driver.ViewProps{Disabled: true}, 0, false, false},
		{"input disabled by config", driver.KindInput, driver.ViewProps{}, 0, true, false},
	}
	for _, tc := range cases {
		if got := viewCanFocusStop(tc.kind, &tc.props, tc.events, tc.inputDisabled); got != tc.want {
			t.Errorf("%s: viewCanFocusStop() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestWidgetFocusableTargets(t *testing.T) {
	if v, e := widgetFocusableTargets(driver.KindView, true); !v || e {
		t.Errorf("KindView stop = (%v, %v), want (true, false)", v, e)
	}
	if v, e := widgetFocusableTargets(driver.KindView, false); v || e {
		t.Errorf("KindView non-stop = (%v, %v), want (false, false)", v, e)
	}
	if v, e := widgetFocusableTargets(driver.KindInput, true); v || !e {
		t.Errorf("KindInput stop = (%v, %v), want (false, true)", v, e)
	}
	if v, e := widgetFocusableTargets(driver.KindInput, false); v || e {
		t.Errorf("KindInput non-stop = (%v, %v), want (false, false)", v, e)
	}
}

func TestSyncFocusTreeScopesVisibilityAndTransitions(t *testing.T) {
	press := driver.EventMask(1 << driver.EventPress)

	btn := &fakeFocusNode{name: "button", kind: driver.KindView, events: press}
	explicit := &fakeFocusNode{name: "explicit", kind: driver.KindView, props: driver.ViewProps{Focusable: true}}
	plain := &fakeFocusNode{name: "plain", kind: driver.KindView}
	noFocus := &fakeFocusNode{name: "noFocus", kind: driver.KindView, props: driver.ViewProps{NoFocus: true}, events: press}
	disabledBtn := &fakeFocusNode{name: "disabledBtn", kind: driver.KindView, props: driver.ViewProps{Disabled: true}, events: press}
	field := &fakeFocusNode{name: "field", kind: driver.KindInput, editorFocusable: true}
	disabledField := &fakeFocusNode{name: "disabledField", kind: driver.KindInput, inputDisabled: true, editorFocusable: true}
	nestedBtn := &fakeFocusNode{name: "nestedBtn", kind: driver.KindView, events: press}
	group := &fakeFocusNode{name: "group", kind: driver.KindView, children: []*fakeFocusNode{nestedBtn}}
	modalBtn := &fakeFocusNode{name: "modalBtn", kind: driver.KindView, events: press}
	modal := &fakeFocusNode{name: "modal", kind: driver.KindModal, hidden: true, children: []*fakeFocusNode{modalBtn}}
	innerModalBtn := &fakeFocusNode{name: "innerModalBtn", kind: driver.KindView, events: press}
	innerModal := &fakeFocusNode{name: "innerModal", kind: driver.KindModal, hidden: true, children: []*fakeFocusNode{innerModalBtn}}

	root := &fakeFocusNode{
		name: "root",
		kind: driver.KindView,
		children: []*fakeFocusNode{
			btn, explicit, plain, noFocus, disabledBtn, field, disabledField, group, modal, innerModal,
		},
	}

	sync := func() {
		syncFocusTree(root, func(v *fakeFocusNode, vf, ef bool) {
			v.apply(vf, ef)
		})
	}
	assertFocus := func(stage string, want map[*fakeFocusNode][2]bool) {
		t.Helper()
		for n, w := range want {
			if n.viewFocusable != w[0] || n.editorFocusable != w[1] {
				t.Errorf("%s: %s focusable = (view=%v, editor=%v), want (view=%v, editor=%v)",
					stage, n.name, n.viewFocusable, n.editorFocusable, w[0], w[1])
			}
		}
	}

	// Initial mount: hidden modals do not steal scope; all active Tab stops are focusable.
	sync()
	assertFocus("initial", map[*fakeFocusNode][2]bool{
		root:          {false, false},
		btn:           {true, false},
		explicit:      {true, false},
		plain:         {false, false},
		noFocus:       {false, false},
		disabledBtn:   {false, false},
		field:         {false, true},
		disabledField: {false, false},
		nestedBtn:     {true, false},
		modalBtn:      {false, false},
		innerModalBtn: {false, false},
	})

	// Second sync without changes emits zero updates.
	prevBtnUpdates, prevFieldUpdates := btn.viewUpdates, field.editorUpdates
	sync()
	if btn.viewUpdates != prevBtnUpdates || field.editorUpdates != prevFieldUpdates {
		t.Errorf("redundant sync updated focusable counts: btn %d->%d, field %d->%d",
			prevBtnUpdates, btn.viewUpdates, prevFieldUpdates, field.editorUpdates)
	}

	// Hiding a container suppresses its descendant stops.
	group.hidden = true
	sync()
	assertFocus("group hidden", map[*fakeFocusNode][2]bool{
		nestedBtn: {false, false},
		btn:       {true, false},
	})
	group.hidden = false
	sync()
	assertFocus("group shown", map[*fakeFocusNode][2]bool{
		nestedBtn: {true, false},
	})

	// Opening the first modal makes only its stops focusable.
	modal.hidden = false
	sync()
	assertFocus("modal open", map[*fakeFocusNode][2]bool{
		btn:           {false, false},
		explicit:      {false, false},
		field:         {false, false},
		nestedBtn:     {false, false},
		modalBtn:      {true, false},
		innerModalBtn: {false, false},
	})

	// Opening a second (newer) modal moves focusable stops exclusively into it.
	innerModal.hidden = false
	sync()
	assertFocus("nested modal open", map[*fakeFocusNode][2]bool{
		btn:           {false, false},
		modalBtn:      {false, false},
		innerModalBtn: {true, false},
	})

	// Closing the newer modal restores the first modal's stops.
	innerModal.hidden = true
	sync()
	assertFocus("nested modal closed", map[*fakeFocusNode][2]bool{
		btn:           {false, false},
		modalBtn:      {true, false},
		innerModalBtn: {false, false},
	})

	// Closing the first modal restores the main window's stops.
	modal.hidden = true
	sync()
	assertFocus("all modals closed", map[*fakeFocusNode][2]bool{
		btn:           {true, false},
		explicit:      {true, false},
		field:         {false, true},
		disabledField: {false, false},
		nestedBtn:     {true, false},
		modalBtn:      {false, false},
	})

	// Dynamic roving focus and disabled toggles update in place.
	explicit.props.Focusable = false
	disabledBtn.props.Disabled = false
	disabledField.inputDisabled = false
	sync()
	assertFocus("dynamic toggles", map[*fakeFocusNode][2]bool{
		explicit:      {false, false},
		disabledBtn:   {true, false},
		disabledField: {false, true},
	})
}

//go:build linux && !android

package linux

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/core"
)

// TestAccessibilitySelectedAndExpandedReachATSPI reads the states through the same AT-SPI interface a screen reader uses, on a private session bus so the test sees only its own window.
func TestAccessibilitySelectedAndExpandedReachATSPI(t *testing.T) {
	if want := os.Getenv("NSGO_TEST_ACCESS_WANT"); want != "" {
		probeAccessibilityStates(t, want == "true")
		return
	}
	if os.Getenv("NSGO_TEST_ACCESS_PROCESS") != t.Name() {
		bus, err := exec.LookPath("dbus-run-session")
		if err != nil {
			t.Skip("needs dbus-run-session")
		}
		cmd := exec.Command(bus, "--", os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=45s")
		cmd.Env = append(os.Environ(), "NSGO_TEST_ACCESS_PROCESS="+t.Name(), "GTK_A11Y=atspi")
		runAccessibilityProcess(t, cmd)
		return
	}
	var update func(bool)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, true)
		update = func(v bool) { cx.Post(func() { on.Set(v) }) }
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("access-states|%v", on.Get())) })
		return core.View{Role: core.RoleTreeItem, Label: accessTestLabel, Selected: on.Get, Expanded: on.Get,
			Style: core.Style{Width: core.Pt(100), Height: core.Pt(100)}}
	}, Options{Title: "access-states", Width: 150, Height: 150})
	waitForTitle(t, w, "access-states|true")
	for _, want := range []bool{true, false, true} {
		update(want)
		waitForTitle(t, w, fmt.Sprintf("access-states|%v", want))
		// AT-SPI's synchronous calls iterate GLib's default context. Keep the client in another process so it cannot dispatch GTK callbacks on the test goroutine while the window's thread uses that context.
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=15s")
		cmd.Env = append(os.Environ(), fmt.Sprintf("NSGO_TEST_ACCESS_WANT=%v", want))
		runAccessibilityProcess(t, cmd)
	}
}

func runAccessibilityProcess(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(string(out), "g_value_get_int:") {
		t.Fatalf("GTK rejected an accessibility state's type:\n%s", out)
	}
	if strings.Contains(string(out), "--- SKIP:") {
		t.Skip(string(out))
	}
}

const accessTestLabel = "NSGo selected and expanded regression"

func probeAccessibilityStates(t *testing.T, want bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	lib, err := purego.Dlopen("libatspi.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Skip("needs libatspi: ", err)
	}
	var init func() int32
	var desktop func(int32) uintptr
	var name func(uintptr, *uintptr) uintptr
	var count func(uintptr, *uintptr) int32
	var child func(uintptr, int32, *uintptr) uintptr
	var states func(uintptr) uintptr
	var contains func(uintptr, int32) int32
	var clear func(uintptr)
	var retain func(uintptr) uintptr
	purego.RegisterLibFunc(&init, lib, "atspi_init")
	purego.RegisterLibFunc(&desktop, lib, "atspi_get_desktop")
	purego.RegisterLibFunc(&name, lib, "atspi_accessible_get_name")
	purego.RegisterLibFunc(&count, lib, "atspi_accessible_get_child_count")
	purego.RegisterLibFunc(&child, lib, "atspi_accessible_get_child_at_index")
	purego.RegisterLibFunc(&states, lib, "atspi_accessible_get_state_set")
	purego.RegisterLibFunc(&contains, lib, "atspi_state_set_contains")
	purego.RegisterLibFunc(&clear, lib, "atspi_accessible_clear_cache")
	purego.RegisterLibFunc(&retain, gobjectLib, "g_object_ref")
	if init() != 0 {
		t.Fatal("AT-SPI could not connect to the private bus")
	}
	// GTK registers with the accessibility bus asynchronously.
	var find func(uintptr, int) uintptr
	find = func(node uintptr, depth int) uintptr {
		if node == 0 || depth > 10 {
			return 0
		}
		clear(node)
		s := name(node, nil)
		got := goString(s)
		gFree(s)
		if got == accessTestLabel {
			retain(node)
			return node
		}
		for i, n := int32(0), count(node, nil); i < n; i++ {
			c := child(node, i, nil)
			found := find(c, depth+1)
			if c != 0 {
				gObjectUnref(c)
			}
			if found != 0 {
				return found
			}
		}
		return 0
	}
	var node uintptr
	for deadline := time.Now().Add(10 * time.Second); node == 0; time.Sleep(20 * time.Millisecond) {
		root := desktop(0)
		node = find(root, 0)
		if root != 0 {
			gObjectUnref(root)
		}
		if time.Now().After(deadline) {
			t.Fatal("the labelled view did not appear on the AT-SPI bus")
		}
	}
	defer gObjectUnref(node)
	// AtspiStateType's stable protocol values.
	const expandable, expanded, selected = 9, 10, 23
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		clear(node)
		set := states(node)
		e, x, s := contains(set, expandable) != 0, contains(set, expanded) != 0, contains(set, selected) != 0
		gObjectUnref(set)
		// A collapsed view must stay expandable: its expanded state is false, not unset.
		if e && x == want && s == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("AT-SPI expandable %v, expanded %v, selected %v; want expandable and both %v", e, x, s, want)
		}
	}
}

const (
	multilinePlaceholderTestLabel = "NSGo multiline placeholder regression"
	multilinePlaceholderTestDesc  = "Supports Markdown"
)

// TestMultilinePlaceholderAccessibilityReachesATSPI checks that a multiline
// GtkTextView exposes its current placeholder through AT-SPI's placeholder-text
// attribute across updates, edits and clearing while preserving its explicit
// description and keeping the visual placeholder GtkLabel out of AT-SPI (#1595).
func TestMultilinePlaceholderAccessibilityReachesATSPI(t *testing.T) {
	if _, ok := os.LookupEnv("NSGO_TEST_PLACEHOLDER_WANT"); ok {
		probeMultilinePlaceholderATSPI(t, os.Getenv("NSGO_TEST_PLACEHOLDER_WANT"))
		return
	}
	if os.Getenv("NSGO_TEST_ACCESS_PROCESS") != t.Name() {
		bus, err := exec.LookPath("dbus-run-session")
		if err != nil {
			t.Skip("needs dbus-run-session")
		}
		cmd := exec.Command(bus, "--", os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=45s")
		cmd.Env = append(os.Environ(), "NSGO_TEST_ACCESS_PROCESS="+t.Name(), "GTK_A11Y=atspi")
		runAccessibilityProcess(t, cmd)
		return
	}
	var setState func(placeholder, value string)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		placeholder := core.NewSignal(cx, "Write a note")
		value := core.NewSignal(cx, "")
		setState = func(p, v string) {
			cx.Post(func() {
				placeholder.Set(p)
				value.Set(v)
			})
		}
		cx.Effect(func() {
			cx.SetTitle(fmt.Sprintf("placeholder-a11y|%s|%s", placeholder.Get(), value.Get()))
		})
		return core.View{
			Style: core.Style{Width: core.Pt(260), Height: core.Pt(140)},
			Children: []core.Node{
				core.Input{
					Multiline:                true,
					Label:                    multilinePlaceholderTestLabel,
					AccessibilityDescription: multilinePlaceholderTestDesc,
					PlaceholderFunc:          placeholder.Get,
					Value:                    value.Get,
					OnChange:                 value.Set,
					Style:                    core.Style{Width: core.Pt(240), Height: core.Pt(100)},
				},
			},
		}
	}, Options{Title: "placeholder-a11y", Width: 280, Height: 160})
	waitForTitle(t, w, "placeholder-a11y|Write a note|")

	// Verify the visual placeholder GtkLabel stays decorative and non-interactive in GTK.
	var (
		getRole      func(uintptr) int32
		getFocusable func(uintptr) int32
		getCanTarget func(uintptr) int32
	)
	purego.RegisterLibFunc(&getRole, gtkLib, "gtk_accessible_get_accessible_role")
	purego.RegisterLibFunc(&getFocusable, gtkLib, "gtk_widget_get_focusable")
	purego.RegisterLibFunc(&getCanTarget, gtkLib, "gtk_widget_get_can_target")
	w.onLoop(t, func() {
		var note *view
		for _, v := range w.entries {
			if v.textView != 0 {
				note = v
				break
			}
		}
		if note == nil || note.hint == 0 {
			t.Fatal("multiline input or its visual placeholder label did not mount")
		}
		if got, want := getRole(note.hint), enumValue("role", placeholderLabelRole); got != want {
			t.Errorf("placeholder label accessible role = %d, want presentation (%d)", got, want)
		}
		if getFocusable(note.hint) != 0 {
			t.Error("placeholder label is focusable")
		}
		if getCanTarget(note.hint) != 0 {
			t.Error("placeholder label takes pointer targets")
		}
	})

	for _, step := range []struct{ placeholder, value string }{
		{"Write a note", ""},
		{"Add details", ""},
		{"Add details", "Draft"},
		{"", ""},
	} {
		setState(step.placeholder, step.value)
		waitForTitle(t, w, fmt.Sprintf("placeholder-a11y|%s|%s", step.placeholder, step.value))
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=15s")
		cmd.Env = append(os.Environ(), "NSGO_TEST_PLACEHOLDER_WANT="+step.placeholder)
		runAccessibilityProcess(t, cmd)
	}
}

func probeMultilinePlaceholderATSPI(t *testing.T, wantPlaceholder string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	lib, err := purego.Dlopen("libatspi.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Skip("needs libatspi: ", err)
	}
	var (
		init        func() int32
		desktop     func(int32) uintptr
		name        func(uintptr, *uintptr) uintptr
		description func(uintptr, *uintptr) uintptr
		roleName    func(uintptr, *uintptr) uintptr
		attributes  func(uintptr, *uintptr) uintptr
		count       func(uintptr, *uintptr) int32
		child       func(uintptr, int32, *uintptr) uintptr
		clear       func(uintptr)
		retain      func(uintptr) uintptr
		hashLookup  func(uintptr, *byte) uintptr
		hashUnref   func(uintptr)
	)
	purego.RegisterLibFunc(&init, lib, "atspi_init")
	purego.RegisterLibFunc(&desktop, lib, "atspi_get_desktop")
	purego.RegisterLibFunc(&name, lib, "atspi_accessible_get_name")
	purego.RegisterLibFunc(&description, lib, "atspi_accessible_get_description")
	purego.RegisterLibFunc(&roleName, lib, "atspi_accessible_get_role_name")
	purego.RegisterLibFunc(&attributes, lib, "atspi_accessible_get_attributes")
	purego.RegisterLibFunc(&count, lib, "atspi_accessible_get_child_count")
	purego.RegisterLibFunc(&child, lib, "atspi_accessible_get_child_at_index")
	purego.RegisterLibFunc(&clear, lib, "atspi_accessible_clear_cache")
	purego.RegisterLibFunc(&retain, gobjectLib, "g_object_ref")
	purego.RegisterLibFunc(&hashLookup, glibLib, "g_hash_table_lookup")
	purego.RegisterLibFunc(&hashUnref, glibLib, "g_hash_table_unref")
	if init() != 0 {
		t.Fatal("AT-SPI could not connect to the private bus")
	}
	readStr := func(fn func(uintptr, *uintptr) uintptr, node uintptr) string {
		s := fn(node, nil)
		got := goString(s)
		gFree(s)
		return got
	}
	var findByName func(uintptr, string, int) uintptr
	findByName = func(node uintptr, target string, depth int) uintptr {
		if node == 0 || depth > 12 {
			return 0
		}
		clear(node)
		if readStr(name, node) == target {
			retain(node)
			return node
		}
		for i, n := int32(0), count(node, nil); i < n; i++ {
			c := child(node, i, nil)
			found := findByName(c, target, depth+1)
			if c != 0 {
				gObjectUnref(c)
			}
			if found != 0 {
				return found
			}
		}
		return 0
	}
	var inputNode uintptr
	for deadline := time.Now().Add(10 * time.Second); inputNode == 0; time.Sleep(20 * time.Millisecond) {
		root := desktop(0)
		inputNode = findByName(root, multilinePlaceholderTestLabel, 0)
		if root != 0 {
			gObjectUnref(root)
		}
		if time.Now().After(deadline) {
			t.Fatal("the multiline input did not appear on the AT-SPI bus")
		}
	}
	defer gObjectUnref(inputNode)

	// Walk inputNode's subtree to find the GtkTextView editor ("text") and ensure no "label" stop exposes the visual placeholder.
	var inspect func(uintptr, int) (editor uintptr, hasLabel bool)
	inspect = func(node uintptr, depth int) (uintptr, bool) {
		if node == 0 || depth > 8 {
			return 0, false
		}
		clear(node)
		var ed uintptr
		var labelFound bool
		if depth > 0 {
			r := readStr(roleName, node)
			if r == "label" {
				labelFound = true
			}
			if r == "text" {
				retain(node)
				ed = node
			}
		}
		for i, n := int32(0), count(node, nil); i < n; i++ {
			c := child(node, i, nil)
			subEd, subLabel := inspect(c, depth+1)
			if c != 0 {
				gObjectUnref(c)
			}
			if subLabel {
				labelFound = true
			}
			if ed == 0 && subEd != 0 {
				ed = subEd
			} else if subEd != 0 {
				gObjectUnref(subEd)
			}
		}
		return ed, labelFound
	}

	var editorNode uintptr
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if editorNode != 0 {
			gObjectUnref(editorNode)
			editorNode = 0
		}
		var hasLabel bool
		editorNode, hasLabel = inspect(inputNode, 0)
		if hasLabel {
			if editorNode != 0 {
				gObjectUnref(editorNode)
			}
			t.Fatal("visual placeholder GtkLabel appeared as a separate AT-SPI label stop")
		}
		if editorNode == 0 {
			if time.Now().After(deadline) {
				t.Fatal("GtkTextView editor did not appear under the multiline input on AT-SPI")
			}
			continue
		}
		clear(inputNode)
		desc := readStr(description, inputNode)
		clear(editorNode)
		var gotPlaceholder string
		if attrs := attributes(editorNode, nil); attrs != 0 {
			if p := hashLookup(attrs, cstr("placeholder-text")); p != 0 {
				gotPlaceholder = goString(p)
			}
			hashUnref(attrs)
		}
		if desc == multilinePlaceholderTestDesc && gotPlaceholder == wantPlaceholder {
			gObjectUnref(editorNode)
			return
		}
		if time.Now().After(deadline) {
			gObjectUnref(editorNode)
			t.Fatalf("AT-SPI description = %q (want %q), placeholder-text = %q (want %q)",
				desc, multilinePlaceholderTestDesc, gotPlaceholder, wantPlaceholder)
		}
	}
}

const (
	focusableButtonLabel   = "NSGo focusable stop button"
	focusableRovingLabel   = "NSGo focusable stop roving"
	focusablePlainLabel    = "NSGo focusable stop plain"
	focusableNoFocusLabel  = "NSGo focusable stop nofocus"
	focusableDisabledLabel = "NSGo focusable stop disabled"
)

// TestAccessibilityFocusableStopsReachATSPI verifies that AT-SPI exposes every
// active Tab stop with ATSPI_STATE_FOCUSABLE even while unfocused (without
// giving a button initial focus on window map), keeps non-stops non-focusable,
// and updates AT-SPI when a roving stop toggles off (#1552).
func TestAccessibilityFocusableStopsReachATSPI(t *testing.T) {
	if phase := os.Getenv("NSGO_TEST_FOCUSABLE_PHASE"); phase != "" {
		probeFocusableStopsATSPI(t, phase)
		return
	}
	if os.Getenv("NSGO_TEST_ACCESS_PROCESS") != t.Name() {
		bus, err := exec.LookPath("dbus-run-session")
		if err != nil {
			t.Skip("needs dbus-run-session")
		}
		cmd := exec.Command(bus, "--", os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=45s")
		cmd.Env = append(os.Environ(), "NSGO_TEST_ACCESS_PROCESS="+t.Name(), "GTK_A11Y=atspi")
		runAccessibilityProcess(t, cmd)
		return
	}
	var setRoving func(bool)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		roving := core.NewSignal(cx, true)
		setRoving = func(on bool) { cx.Post(func() { roving.Set(on) }) }
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("focusable-a11y|%v", roving.Get())) })
		return core.View{
			Style: core.Style{Width: core.Pt(220), Height: core.Pt(220)},
			Children: []core.Node{
				core.View{
					Role:    core.RoleButton,
					Label:   focusableButtonLabel,
					Style:   core.Style{Width: core.Pt(180), Height: core.Pt(30)},
					OnPress: func(core.PressEvent) {},
				},
				core.View{
					Role:      core.RoleButton,
					Label:     focusableRovingLabel,
					Focusable: roving.Get,
					Style:     core.Style{Width: core.Pt(180), Height: core.Pt(30)},
				},
				core.View{
					Role:  core.RoleGroup,
					Label: focusablePlainLabel,
					Style: core.Style{Width: core.Pt(180), Height: core.Pt(30)},
				},
				core.View{
					Role:    core.RoleButton,
					Label:   focusableNoFocusLabel,
					NoFocus: true,
					Style:   core.Style{Width: core.Pt(180), Height: core.Pt(30)},
					OnPress: func(core.PressEvent) {},
				},
				core.View{
					Role:     core.RoleButton,
					Label:    focusableDisabledLabel,
					Disabled: func() bool { return true },
					Style:    core.Style{Width: core.Pt(180), Height: core.Pt(30)},
					OnPress:  func(core.PressEvent) {},
				},
			},
		}
	}, Options{Title: "focusable-a11y", Width: 240, Height: 240})
	waitForTitle(t, w, "focusable-a11y|true")

	w.onLoop(t, func() {
		if f := gtkWindowGetFocus(w.toplevel()); f != 0 {
			t.Fatalf("window map focused %#x, want no initial focus", f)
		}
	})
	probe := func(phase string) {
		t.Helper()
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v", "-test.timeout=15s")
		cmd.Env = append(os.Environ(), "NSGO_TEST_FOCUSABLE_PHASE="+phase)
		runAccessibilityProcess(t, cmd)
	}
	probe("initial")

	w.onLoop(t, func() {
		if !w.tab(false) {
			t.Fatal("Tab did not focus the first button stop")
		}
	})
	probe("focused")

	setRoving(false)
	waitForTitle(t, w, "focusable-a11y|false")
	probe("roving-off")
}

func probeFocusableStopsATSPI(t *testing.T, phase string) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	lib, err := purego.Dlopen("libatspi.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Skip("needs libatspi: ", err)
	}
	var (
		init     func() int32
		desktop  func(int32) uintptr
		name     func(uintptr, *uintptr) uintptr
		count    func(uintptr, *uintptr) int32
		child    func(uintptr, int32, *uintptr) uintptr
		states   func(uintptr) uintptr
		contains func(uintptr, int32) int32
		clear    func(uintptr)
		retain   func(uintptr) uintptr
	)
	purego.RegisterLibFunc(&init, lib, "atspi_init")
	purego.RegisterLibFunc(&desktop, lib, "atspi_get_desktop")
	purego.RegisterLibFunc(&name, lib, "atspi_accessible_get_name")
	purego.RegisterLibFunc(&count, lib, "atspi_accessible_get_child_count")
	purego.RegisterLibFunc(&child, lib, "atspi_accessible_get_child_at_index")
	purego.RegisterLibFunc(&states, lib, "atspi_accessible_get_state_set")
	purego.RegisterLibFunc(&contains, lib, "atspi_state_set_contains")
	purego.RegisterLibFunc(&clear, lib, "atspi_accessible_clear_cache")
	purego.RegisterLibFunc(&retain, gobjectLib, "g_object_ref")
	if init() != 0 {
		t.Fatal("AT-SPI could not connect to the private bus")
	}

	type wantState struct {
		focusable bool
		focused   bool
	}
	want := map[string]wantState{
		focusableButtonLabel:   {focusable: true, focused: phase != "initial"},
		focusableRovingLabel:   {focusable: phase != "roving-off", focused: false},
		focusablePlainLabel:    {focusable: false, focused: false},
		focusableNoFocusLabel:  {focusable: false, focused: false},
		focusableDisabledLabel: {focusable: false, focused: false},
	}

	var collect func(uintptr, int, map[string]uintptr)
	collect = func(node uintptr, depth int, found map[string]uintptr) {
		if node == 0 || depth > 12 {
			return
		}
		clear(node)
		s := name(node, nil)
		got := goString(s)
		gFree(s)
		if _, ok := want[got]; ok && found[got] == 0 {
			retain(node)
			found[got] = node
		}
		for i, n := int32(0), count(node, nil); i < n; i++ {
			c := child(node, i, nil)
			collect(c, depth+1, found)
			if c != 0 {
				gObjectUnref(c)
			}
		}
	}

	found := map[string]uintptr{}
	for deadline := time.Now().Add(10 * time.Second); len(found) < len(want); time.Sleep(20 * time.Millisecond) {
		root := desktop(0)
		collect(root, 0, found)
		if root != 0 {
			gObjectUnref(root)
		}
		if len(found) < len(want) && time.Now().After(deadline) {
			for k, n := range found {
				gObjectUnref(n)
				delete(found, k)
			}
			t.Fatalf("phase %s: found %d of %d accessible nodes on AT-SPI", phase, len(found), len(want))
		}
	}
	defer func() {
		for _, n := range found {
			gObjectUnref(n)
		}
	}()

	// AtspiStateType's stable protocol values.
	const focusableState, focusedState = 11, 12
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		allMatch := true
		var lastMismatch string
		for label, exp := range want {
			node := found[label]
			clear(node)
			set := states(node)
			gotFocusable := contains(set, focusableState) != 0
			gotFocused := contains(set, focusedState) != 0
			gObjectUnref(set)
			if gotFocusable != exp.focusable || gotFocused != exp.focused {
				allMatch = false
				lastMismatch = fmt.Sprintf("%q: focusable=%v focused=%v, want focusable=%v focused=%v",
					label, gotFocusable, gotFocused, exp.focusable, exp.focused)
				break
			}
		}
		if allMatch {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("phase %s: %s", phase, lastMismatch)
		}
	}
}

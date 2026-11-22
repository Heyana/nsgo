//go:build !ios || nsgotest || nsgoautomation

package ios

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSimulatorDetectionAndHIDFallback(t *testing.T) {
	if isSimulatorEnv(func(string) string { return "" }) {
		t.Fatal("empty env reported as simulator")
	}
	if !isSimulatorEnv(func(k string) string {
		if k == "SIMULATOR_MODEL_IDENTIFIER" {
			return "iPhone17,1"
		}
		return ""
	}) {
		t.Fatal("SIMULATOR_MODEL_IDENTIFIER not detected as simulator")
	}
	if !isSimulatorEnv(func(k string) string {
		if k == "SIMULATOR_UDID" {
			return "00000000-0000-0000-0000-000000000000"
		}
		return ""
	}) {
		t.Fatal("SIMULATOR_UDID not detected as simulator")
	}

	called := false
	if shouldUseSimulatorHID(false, func() error {
		called = true
		return nil
	}) {
		t.Fatal("physical device used simulator HID")
	}
	if called {
		t.Fatal("physical device attempted to load private simulator HID")
	}

	if shouldUseSimulatorHID(true, func() error { return errors.New("missing BackBoardServices") }) {
		t.Fatal("simulator with missing HID did not fall back to device path")
	}
	if !shouldUseSimulatorHID(true, func() error { return nil }) {
		t.Fatal("simulator with working HID did not use simulator path")
	}
}

func TestResolveDevicePress(t *testing.T) {
	tests := []struct {
		name  string
		chain []deviceHitNode
		hold  bool
		mods  driver.Modifiers
		want  devicePressTarget
	}{
		{
			name: "text editor wins over enclosing pressable view",
			chain: []deviceHitNode{
				{isTextEditor: true},
				{isNSGoView: true, pressable: true},
			},
			want: devicePressTarget{kind: devicePressFocusText, index: 0},
		},
		{
			name: "native switch activates directly",
			chain: []deviceHitNode{
				{isSwitch: true, isControl: true},
				{isNSGoView: true, pressable: true},
			},
			want: devicePressTarget{kind: devicePressSwitch, index: 0},
		},
		{
			name: "native tab bar selects tapped tab",
			chain: []deviceHitNode{
				{isNSGoView: false, isAxElement: true},
				{isTabBar: true},
			},
			want: devicePressTarget{kind: devicePressTabBar, index: 1},
		},
		{
			name: "native UIControl fires control events",
			chain: []deviceHitNode{
				{isControl: true},
			},
			want: devicePressTarget{kind: devicePressControl, index: 0},
		},
		{
			name: "NSGo view with accessibility activate takes accessibilityActivate on plain tap",
			chain: []deviceHitNode{
				{isNSGoView: true, pressable: true, hasActivate: true},
			},
			want: devicePressTarget{kind: devicePressAccessibility, index: 0},
		},
		{
			name: "NSGo view with accessibility activate uses engine press on long press",
			chain: []deviceHitNode{
				{isNSGoView: true, pressable: true, hasActivate: true},
			},
			hold: true,
			want: devicePressTarget{kind: devicePressView, index: 0},
		},
		{
			name: "NSGo view with link at point uses engine press to preserve span index",
			chain: []deviceHitNode{
				{isNSGoView: true, pressable: true, hasActivate: true, hasLinkAtPoint: true},
			},
			want: devicePressTarget{kind: devicePressView, index: 0},
		},
		{
			name: "NSGo view with modifiers uses engine press to preserve modifiers",
			chain: []deviceHitNode{
				{isNSGoView: true, pressable: true, hasActivate: true},
			},
			mods: driver.Modifiers{Shift: true},
			want: devicePressTarget{kind: devicePressView, index: 0},
		},
		{
			name: "NSGo pressable ancestor found through non-pressable text child",
			chain: []deviceHitNode{
				{isNSGoView: true, isAxElement: true},
				{isNSGoView: true, pressable: true},
			},
			want: devicePressTarget{kind: devicePressView, index: 1},
		},
		{
			name: "non-NSGo accessibility element falls back to accessibilityActivate",
			chain: []deviceHitNode{
				{isAxElement: true},
				{},
			},
			want: devicePressTarget{kind: devicePressAccessibility, index: 0},
		},
		{
			name:  "empty chain resolves to none",
			chain: nil,
			want:  devicePressTarget{kind: devicePressNone, index: -1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveDevicePress(tc.chain, tc.hold, tc.mods)
			if got != tc.want {
				t.Fatalf("resolveDevicePress = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestClampScrollOffset(t *testing.T) {
	// Vertical scroll view with top inset 20, content 500pt tall in a 200pt viewport:
	// valid Y range is [-20, 300].
	to, moved := clampScrollOffset(
		driver.Point{X: 0, Y: 0},
		driver.Point{X: 0, Y: 120},
		driver.Size{Width: 300, Height: 500},
		driver.Size{Width: 300, Height: 200},
		20, 0, 0, 0,
	)
	if !moved || to != (driver.Point{X: 0, Y: 120}) {
		t.Fatalf("scroll down = %+v, moved=%v", to, moved)
	}
	// Already at bottom edge: cannot move further down (lets nested outer scroll view take it).
	to, moved = clampScrollOffset(
		driver.Point{X: 0, Y: 300},
		driver.Point{X: 0, Y: 80},
		driver.Size{Width: 300, Height: 500},
		driver.Size{Width: 300, Height: 200},
		20, 0, 0, 0,
	)
	if moved || to != (driver.Point{X: 0, Y: 300}) {
		t.Fatalf("at bottom edge = %+v, moved=%v", to, moved)
	}
	// Scrolling past top clamps to -insetTop.
	to, moved = clampScrollOffset(
		driver.Point{X: 0, Y: 10},
		driver.Point{X: 0, Y: -100},
		driver.Size{Width: 300, Height: 500},
		driver.Size{Width: 300, Height: 200},
		20, 0, 0, 0,
	)
	if !moved || to != (driver.Point{X: 0, Y: -20}) {
		t.Fatalf("scroll to top inset = %+v, moved=%v", to, moved)
	}
}

func TestResolveAlertAction(t *testing.T) {
	actions := []driver.AlertAction{
		{Label: "Delete", Role: driver.AlertDestructive},
		{Label: "Cancel", Role: driver.AlertCancel},
	}
	if label, idx, err := resolveAlertAction(actions, 0); err != nil || label != "Delete" || idx != 0 {
		t.Fatalf("index 0 = %q, %d, %v", label, idx, err)
	}
	if label, idx, err := resolveAlertAction(actions, -1); err != nil || label != "Cancel" || idx != 1 {
		t.Fatalf("dismiss = %q, %d, %v", label, idx, err)
	}
	if label, idx, err := resolveAlertAction(nil, -1); err != nil || label != "OK" || idx != -1 {
		t.Fatalf("empty actions = %q, %d, %v", label, idx, err)
	}
	if _, _, err := resolveAlertAction([]driver.AlertAction{{Label: "Keep"}}, -1); err == nil {
		t.Fatal("undismissable alert unexpectedly succeeded")
	}
}

func TestParseDeviceKeyAndHeldState(t *testing.T) {
	name, mods, flags, ok := parseDeviceKey("Mod+Shift+K")
	if !ok || name != "k" || !mods.Platform || !mods.Shift || mods.Control || mods.Alt || flags != (deviceModMeta|deviceModShift) {
		t.Fatalf("parse Mod+Shift+K = %q, %+v, %#x, %v", name, mods, flags, ok)
	}
	if name, _, _, ok := parseDeviceKey("Return"); !ok || name != "enter" {
		t.Fatalf("parse Return = %q, %v", name, ok)
	}
	if _, _, _, ok := parseDeviceKey("nonexistent"); ok {
		t.Fatal("unknown key parsed as valid")
	}

	var st deviceKeyState
	mods, flags, _, wasHeld := st.down("shift", 0)
	if wasHeld || !mods.Shift || flags != deviceModShift {
		t.Fatalf("shift down = %+v, %#x, %v", mods, flags, wasHeld)
	}
	mods, flags, _, wasHeld = st.down("a", 7)
	if wasHeld || !mods.Shift || flags != deviceModShift {
		t.Fatalf("a down while shift held = %+v, %#x, %v", mods, flags, wasHeld)
	}
	id, wasHeld, mods, flags := st.up("a")
	if !wasHeld || id != 7 || !mods.Shift || flags != deviceModShift {
		t.Fatalf("a up = id=%d held=%v mods=%+v flags=%#x", id, wasHeld, mods, flags)
	}
	id, wasHeld, mods, flags = st.up("shift")
	if !wasHeld || id != 0 || mods != (driver.Modifiers{}) || flags != 0 {
		t.Fatalf("shift up = id=%d held=%v mods=%+v flags=%#x", id, wasHeld, mods, flags)
	}
}

func TestInterpolateDragPath(t *testing.T) {
	pts := interpolateDragPath([]driver.Point{{X: 10, Y: 20}, {X: 10, Y: 55}})
	if len(pts) < 4 || pts[0] != (driver.Point{X: 10, Y: 20}) || pts[len(pts)-1] != (driver.Point{X: 10, Y: 55}) {
		t.Fatalf("interpolated path = %+v", pts)
	}
}

// TestDataTableHeaderHitTestOnPhone mirrors iOS hitTest's ReachView traversal
// over a DataTable titleCell whose HeaderContent (BoxNone, ZIndex 2, keeping
// clear of the 3pt resize band) sits above ResizeHandle (ZIndex 1, 10pt touch
// slop): taps on the sort button land on it, taps in the 3pt band or a
// non-sortable column's touch slop reach the resize handle, and taps on the
// title reach the header cell.
func TestDataTableHeaderHitTestOnPhone(t *testing.T) {
	type hitNode struct {
		name      string
		frame     driver.Bounds
		props     driver.ViewProps
		pressable bool
		children  []*hitNode
	}
	var hitTest func(n *hitNode, p driver.Point) []*hitNode
	hitTest = func(n *hitNode, p driver.Point) []*hitNode {
		own, kids := driver.ReachView(n.frame.Size, &n.props, driver.Bounds{}, false, p)
		if !own && !kids {
			return nil
		}
		if kids {
			for i := len(n.children) - 1; i >= 0; i-- {
				c := n.children[i]
				local := driver.Point{X: p.X - c.frame.Origin.X, Y: p.Y - c.frame.Origin.Y}
				if chain := hitTest(c, local); chain != nil {
					return append(chain, n)
				}
			}
		}
		if own {
			return []*hitNode{n}
		}
		return nil
	}

	makeHeader := func(sortable bool) *hitNode {
		handle := &hitNode{
			name:      "resize",
			frame:     driver.Bounds{Origin: driver.Point{X: 93, Y: 0}, Size: driver.Size{Width: 3, Height: 32}},
			props:     driver.ViewProps{HitSlop: driver.Edges{Left: 10, Right: 10}},
			pressable: true,
		}
		contentKids := []*hitNode{
			{
				name:  "label",
				frame: driver.Bounds{Origin: driver.Point{X: 0, Y: 6}, Size: driver.Size{Width: 93, Height: 20}},
				props: driver.ViewProps{PointerEvents: driver.PointerEventsNone},
			},
		}
		if sortable {
			contentKids[0].frame.Size.Width = 73
			contentKids = append(contentKids, &hitNode{
				name:      "sort",
				frame:     driver.Bounds{Origin: driver.Point{X: 73, Y: 6}, Size: driver.Size{Width: 20, Height: 20}},
				pressable: true,
				children: []*hitNode{{
					name:  "chevrons",
					frame: driver.Bounds{Origin: driver.Point{X: 4, Y: 4}, Size: driver.Size{Width: 12, Height: 12}},
					props: driver.ViewProps{PointerEvents: driver.PointerEventsNone},
				}},
			})
		}
		content := &hitNode{
			name:     "header-content",
			frame:    driver.Bounds{Origin: driver.Point{X: 0, Y: 0}, Size: driver.Size{Width: 93, Height: 32}},
			props:    driver.ViewProps{PointerEvents: driver.PointerEventsBoxNone},
			children: contentKids,
		}
		return &hitNode{
			name:      "title-cell",
			frame:     driver.Bounds{Size: driver.Size{Width: 96, Height: 32}},
			pressable: true,
			// Native child order by ZIndex: ResizeHandle (1) below HeaderContent (2).
			children: []*hitNode{handle, content},
		}
	}

	resolve := func(root *hitNode, p driver.Point) string {
		chain := hitTest(root, p)
		nodes := make([]deviceHitNode, len(chain))
		for i, n := range chain {
			nodes[i] = deviceHitNode{index: i, isNSGoView: true, pressable: n.pressable}
		}
		got := resolveDevicePress(nodes, false, driver.Modifiers{})
		if got.kind != devicePressView || got.index < 0 {
			return ""
		}
		return chain[got.index].name
	}

	sortable := makeHeader(true)
	if got := resolve(sortable, driver.Point{X: 83, Y: 16}); got != "sort" {
		t.Fatalf("sort button center hit %q, want sort", got)
	}
	if got := resolve(sortable, driver.Point{X: 92, Y: 16}); got != "sort" {
		t.Fatalf("sort button trailing edge hit %q, want sort", got)
	}
	if got := resolve(sortable, driver.Point{X: 94.5, Y: 16}); got != "resize" {
		t.Fatalf("resize handle band hit %q, want resize", got)
	}
	if got := resolve(sortable, driver.Point{X: 30, Y: 16}); got != "title-cell" {
		t.Fatalf("header label hit %q, want title-cell", got)
	}

	plain := makeHeader(false)
	if got := resolve(plain, driver.Point{X: 88, Y: 16}); got != "resize" {
		t.Fatalf("non-sortable handle touch slop hit %q, want resize", got)
	}
	if got := resolve(plain, driver.Point{X: 30, Y: 16}); got != "title-cell" {
		t.Fatalf("non-sortable header label hit %q, want title-cell", got)
	}
}

func TestHIDPressTrackerAndSurfaceBusy(t *testing.T) {
	var tracker hidPressTracker
	if tracker.pending(0) {
		t.Fatal("zero serial reported pending")
	}
	id1 := tracker.mark(0x100)
	if !tracker.pending(id1) {
		t.Fatal("marked press release not reported pending")
	}
	// Another window's event must not clear our window's pending press.
	tracker.delivered(0x200)
	if !tracker.pending(id1) {
		t.Fatal("unrelated window event cleared pending press")
	}
	tracker.delivered(0x100)
	if tracker.pending(id1) {
		t.Fatal("matching window touch end did not clear pending press")
	}

	id2 := tracker.mark(0x100)
	if !tracker.pending(id2) {
		t.Fatal("second marked press release not pending")
	}
	tracker.clear()
	if tracker.pending(id2) {
		t.Fatal("clear did not release pending press")
	}

	if testSurfaceBusy(0, false, 0, 0, false, false) {
		t.Fatal("idle surface reported busy")
	}
	for _, tc := range []struct {
		name string
		busy bool
	}{
		{"queued mount batch", testSurfaceBusy(1, false, 0, 0, false, false)},
		{"ending animator", testSurfaceBusy(0, true, 0, 0, false, false)},
		{"layout motion", testSurfaceBusy(0, false, 1, 0, false, false)},
		{"vanishing ghost", testSurfaceBusy(0, false, 0, 1, false, false)},
		{"pending press release", testSurfaceBusy(0, false, 0, 0, true, false)},
		{"window animation", testSurfaceBusy(0, false, 0, 0, false, true)},
	} {
		if !tc.busy {
			t.Fatalf("%s: expected busy=true", tc.name)
		}
	}
}

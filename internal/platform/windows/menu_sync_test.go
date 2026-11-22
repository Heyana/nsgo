package windows

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeWinMenuItemState struct {
	kind      winMenuKind
	label     string
	fType     uint32
	fState    uint32
	commandID uint32
	subMenu   uintptr
	bitmap    uintptr
}

type fakeWinMenuOps struct {
	nextHandle uintptr
	menus      map[uintptr][]fakeWinMenuItemState
	popups     int
	inserts    int
	updates    int
	deletes    int
	lastDiff   winMenuDiff
	bitmaps    map[uintptr]bool // live HBITMAPs
}

func newFakeWinMenuOps() *fakeWinMenuOps {
	return &fakeWinMenuOps{
		nextHandle: 100,
		menus:      map[uintptr][]fakeWinMenuItemState{},
		bitmaps:    map[uintptr]bool{},
	}
}

func (f *fakeWinMenuOps) newBar() uintptr {
	f.nextHandle++
	h := f.nextHandle
	f.menus[h] = nil
	return h
}

func (f *fakeWinMenuOps) createPopup() uintptr {
	f.popups++
	f.nextHandle++
	h := f.nextHandle
	f.menus[h] = nil
	return h
}

func (f *fakeWinMenuOps) insertItem(menu uintptr, pos uint32, it *winMenuItem) {
	f.inserts++
	item := fakeWinMenuItemState{
		kind:      it.kind,
		label:     it.label,
		fType:     it.fType,
		fState:    it.fState,
		commandID: it.commandID,
		subMenu:   it.handle,
		bitmap:    it.bitmap,
	}
	items := f.menus[menu]
	idx := int(pos)
	items = slices.Insert(items, idx, item)
	f.menus[menu] = items
}

func (f *fakeWinMenuOps) updateItem(menu uintptr, pos uint32, next *winMenuItem, diff winMenuDiff) {
	f.updates++
	f.lastDiff = diff
	item := &f.menus[menu][int(pos)]
	if diff.fTypeChanged {
		item.fType = next.fType
	}
	if diff.fStateChanged {
		item.fState = next.fState
	}
	if diff.iconChanged {
		item.bitmap = next.bitmap
	}
	if diff.idChanged {
		item.commandID = next.commandID
	}
	if diff.labelChanged {
		item.label = next.label
	}
}

func (f *fakeWinMenuOps) deleteItem(menu uintptr, pos uint32) {
	f.deletes++
	items := f.menus[menu]
	removed := items[int(pos)]
	f.menus[menu] = slices.Delete(items, int(pos), int(pos)+1)
	if removed.subMenu != 0 {
		f.destroySubtree(removed.subMenu)
	}
}

func (f *fakeWinMenuOps) iconBitmap(icon driver.MenuIcon) uintptr {
	f.nextHandle++
	f.bitmaps[f.nextHandle] = true
	return f.nextHandle
}

func (f *fakeWinMenuOps) freeBitmap(bitmap uintptr) {
	if !f.bitmaps[bitmap] {
		panic("freeing a bitmap that is not live")
	}
	delete(f.bitmaps, bitmap)
}

func (f *fakeWinMenuOps) destroySubtree(menu uintptr) {
	for _, it := range f.menus[menu] {
		if it.subMenu != 0 {
			f.destroySubtree(it.subMenu)
		}
	}
	delete(f.menus, menu)
}

func TestWinMenuSyncPreservesHandlesOnStateToggle(t *testing.T) {
	ops := newFakeWinMenuOps()
	bar := ops.newBar()

	makeMenu := func(checked, disabled bool, firstID uint32) ([]winMenuItem, []menuRef) {
		entries := []driver.MenuEntry{
			{Label: "File", Items: []driver.MenuEntry{
				{Label: "New", Shortcut: "mod+n", ID: firstID},
			}},
			{Label: "View", Items: []driver.MenuEntry{
				{Label: "Sidebar", Checkable: true, Checked: checked, Disabled: disabled, ID: firstID + 1},
				{Separator: true},
				{Label: "Movies", Checkable: true, Radio: true, Checked: !checked, ID: firstID + 2},
			}},
		}
		var refs []menuRef
		return buildWinMenuTree(entries, "en", mfString, false, winMenuIconLook{size: 16}, &refs), refs
	}

	tree1, refs1 := makeMenu(true, false, 10)
	if !syncWinMenu(ops, bar, nil, tree1) {
		t.Fatal("initial sync reported no top-level change")
	}
	if len(refs1) != 3 {
		t.Fatalf("len(refs1) = %d, want 3", len(refs1))
	}
	fileHandle := tree1[0].handle
	viewHandle := tree1[1].handle
	if fileHandle == 0 || viewHandle == 0 {
		t.Fatal("submenu handles not populated")
	}

	// Reset counters and toggle Sidebar checked + Movies radio checked.
	ops.popups, ops.inserts, ops.updates, ops.deletes = 0, 0, 0, 0
	tree2, refs2 := makeMenu(false, true, 20)
	topChanged := syncWinMenu(ops, bar, tree1, tree2)
	if topChanged {
		t.Error("toggling submenu item states reported top-level bar change")
	}
	if ops.popups != 0 || ops.inserts != 0 || ops.deletes != 0 {
		t.Errorf("expected zero structural mutations, got popups=%d inserts=%d deletes=%d", ops.popups, ops.inserts, ops.deletes)
	}
	if ops.updates != 2 {
		t.Errorf("updates = %d, want 2 (Sidebar and Movies)", ops.updates)
	}
	if tree2[0].handle != fileHandle || tree2[1].handle != viewHandle {
		t.Error("submenu handles were not preserved across in-place sync")
	}
	if got := ops.menus[viewHandle][0].fState; got != mfsGrayed {
		t.Errorf("Sidebar fState = %#x, want %#x (unchecked+grayed)", got, mfsGrayed)
	}
	if got := ops.menus[viewHandle][2].fState; got != mfsChecked {
		t.Errorf("Movies fState = %#x, want %#x (checked)", got, mfsChecked)
	}
	if got := ops.menus[viewHandle][2].fType; got != mfString|mfRadioCheck {
		t.Errorf("Movies fType = %#x, want %#x", got, mfString|mfRadioCheck)
	}
	if refs2[1].id != 21 || refs2[2].id != 22 {
		t.Errorf("refs2 IDs = %+v, want 21 and 22", refs2)
	}
}

func TestWinMenuSyncUpdatesTopologyLabelsRTLAndCommandIDs(t *testing.T) {
	ops := newFakeWinMenuOps()
	bar := ops.newBar()

	var refs1 []menuRef
	tree1 := buildWinMenuTree([]driver.MenuEntry{
		{Label: "File", Items: []driver.MenuEntry{
			{Label: "Open", ID: 1},
		}},
		{Label: "View", Items: []driver.MenuEntry{
			{Label: "Zoom", ID: 2},
		}},
	}, "en", mfString, false, winMenuIconLook{size: 16}, &refs1)
	syncWinMenu(ops, bar, nil, tree1)
	fileHandle, viewHandle := tree1[0].handle, tree1[1].handle

	// Insert an item into File (shifting View's commandID from 2 to 3), rename View -> Display, add a Help menu, and switch to RTL.
	ops.popups, ops.inserts, ops.updates, ops.deletes = 0, 0, 0, 0
	var refs2 []menuRef
	tree2 := buildWinMenuTree([]driver.MenuEntry{
		{Label: "File", Items: []driver.MenuEntry{
			{Label: "New", Shortcut: "mod+n", ID: 10},
			{Label: "Open", ID: 11},
		}},
		{Label: "Display", Items: []driver.MenuEntry{
			{Label: "Zoom", ID: 12},
		}},
		{Label: "Help", Items: []driver.MenuEntry{
			{Label: "Docs", ID: 13},
		}},
	}, "en", mfString|mfRightOrder, false, winMenuIconLook{size: 16}, &refs2)
	topChanged := syncWinMenu(ops, bar, tree1, tree2)
	if !topChanged {
		t.Error("adding top-level menu and renaming View should report top-level change")
	}
	if tree2[0].handle != fileHandle || tree2[1].handle != viewHandle {
		t.Error("existing File and View submenu handles were not preserved")
	}
	if tree2[2].handle == 0 {
		t.Error("added Help submenu got no handle")
	}
	if got := ops.menus[bar][1].label; got != "Display" {
		t.Errorf("renamed top-level menu = %q, want Display", got)
	}
	if got := ops.menus[bar][0].fType; got&mfRightOrder == 0 {
		t.Errorf("RTL top-level item missing MFT_RIGHTORDER: %#x", got)
	}
	if got := ops.menus[fileHandle][0].label; got != "New\tCtrl+N" {
		t.Errorf("File[0] label = %q, want New\\tCtrl+N", got)
	}
	if got := ops.menus[viewHandle][0].commandID; got != 3 {
		t.Errorf("shifted View[0] commandID = %d, want 3", got)
	}
	if len(refs2) != 4 || refs2[2].id != 12 || refs2[3].id != 13 {
		t.Errorf("unexpected refs2: %+v", refs2)
	}

	// Replace top-level submenu with a leaf action and drop Help.
	var refs3 []menuRef
	tree3 := buildWinMenuTree([]driver.MenuEntry{
		{Label: "Sync & Run", ID: 30},
	}, "en", mfString, false, winMenuIconLook{size: 16}, &refs3)
	if !syncWinMenu(ops, bar, tree2, tree3) {
		t.Error("replacing top-level submenus should report top-level change")
	}
	if len(ops.menus[bar]) != 1 || ops.menus[bar][0].kind != winMenuCommand || ops.menus[bar][0].label != "Sync && Run" {
		t.Errorf("bar after replace = %+v", ops.menus[bar])
	}
	if len(ops.menus) != 1 {
		t.Errorf("expected old submenus to be destroyed, still alive: %d", len(ops.menus))
	}
}

func TestWinMenuBuildNormalizesRolesAndSeparators(t *testing.T) {
	entries := []driver.MenuEntry{
		{Separator: true},
		{Label: "App", Items: []driver.MenuEntry{
			{Role: driver.MenuAbout},
			{Separator: true},
			{Role: driver.MenuServices, Items: []driver.MenuEntry{{Label: "Mine", ID: 1}}},
			{Separator: true},
			{Separator: true},
			{Role: driver.MenuQuit, Label: "Quit"},
			{Separator: true},
		}},
		{Label: "Edit", Items: []driver.MenuEntry{
			{Role: driver.MenuDelete, Label: "Delete"},
			{Separator: true},
			{Label: "Speech", Items: []driver.MenuEntry{
				{Role: driver.MenuStartSpeaking},
				{Role: driver.MenuStopSpeaking},
			}},
		}},
		{Label: "Help", Role: driver.MenuHelpMenu},
	}

	var refsNoSpeech []menuRef
	treeNoSpeech := buildWinMenuTree(entries, "en", mfString, false, winMenuIconLook{size: 16}, &refsNoSpeech)
	if len(treeNoSpeech) != 2 {
		t.Fatalf("without speech: got %d top-level menus, want 2", len(treeNoSpeech))
	}
	if len(treeNoSpeech[0].children) != 1 || treeNoSpeech[0].children[0].label != "Quit\tAlt+F4" {
		t.Errorf("App children = %+v, want only Quit\\tAlt+F4", treeNoSpeech[0].children)
	}
	if len(treeNoSpeech[1].children) != 1 || treeNoSpeech[1].children[0].label != "Delete" {
		t.Errorf("Edit children without speech = %+v, want only Delete", treeNoSpeech[1].children)
	}

	var refsSpeech []menuRef
	treeSpeech := buildWinMenuTree(entries, "en", mfString, true, winMenuIconLook{size: 16}, &refsSpeech)
	if len(treeSpeech[1].children) != 3 || treeSpeech[1].children[1].kind != winMenuSeparator || treeSpeech[1].children[2].label != "Speech" {
		t.Errorf("Edit children with speech = %+v", treeSpeech[1].children)
	}
}

// TestWinMenuSyncOwnsIconBitmaps checks that an item keeps its HBITMAP while its icon stays, gets a new one when the icon, its size or a template icon's appearance changes, and that every replaced or deleted item's bitmap is freed.
func TestWinMenuSyncOwnsIconBitmaps(t *testing.T) {
	ops := newFakeWinMenuOps()
	bar := ops.newBar()
	plain := driver.MenuIcon{Data: []byte("plain")}
	template := driver.MenuIcon{Data: []byte("template"), Template: true}
	build := func(open driver.MenuIcon, look winMenuIconLook, extra bool) []winMenuItem {
		items := []driver.MenuEntry{
			{Label: "Open", ID: 1, Icon: open},
			{Label: "Save", ID: 2, Icon: template},
		}
		if extra {
			items = append(items, driver.MenuEntry{Label: "Recent", Icon: plain, Items: []driver.MenuEntry{{Label: "a.txt", ID: 3, Icon: plain}}})
		}
		var refs []menuRef
		return buildWinMenuTree([]driver.MenuEntry{{Label: "File", Icon: plain, Items: items}}, "en", mfString, false, look, &refs)
	}
	light := winMenuIconLook{size: 16}
	tree1 := build(plain, light, true)
	syncWinMenu(ops, bar, nil, tree1)
	file := tree1[0].handle
	if len(ops.bitmaps) != 5 || ops.menus[bar][0].bitmap == 0 || ops.menus[file][0].bitmap == 0 {
		t.Fatalf("initial bitmaps = %d, bar item %+v, Open %+v", len(ops.bitmaps), ops.menus[bar][0], ops.menus[file][0])
	}

	ops.updates = 0
	tree2 := build(plain, light, true)
	syncWinMenu(ops, bar, tree1, tree2)
	if ops.updates != 0 || len(ops.bitmaps) != 5 || tree2[0].children[0].bitmap != tree1[0].children[0].bitmap {
		t.Errorf("unchanged icons: updates=%d bitmaps=%d", ops.updates, len(ops.bitmaps))
	}

	// Dark appearance gives only the template icon a new bitmap.
	ops.updates = 0
	saveOld := tree2[0].children[1].bitmap
	tree3 := build(plain, winMenuIconLook{size: 16, dark: true}, true)
	syncWinMenu(ops, bar, tree2, tree3)
	if ops.updates != 1 || ops.lastDiff != (winMenuDiff{iconChanged: true}) {
		t.Errorf("dark appearance: updates=%d last diff %+v, want one icon update", ops.updates, ops.lastDiff)
	}
	if save := tree3[0].children[1].bitmap; save == saveOld || ops.bitmaps[saveOld] || ops.menus[file][1].bitmap != save {
		t.Errorf("Save bitmap %#x (old %#x, live %v, attached %#x)", save, saveOld, ops.bitmaps[saveOld], ops.menus[file][1].bitmap)
	}

	// Removing an icon detaches and frees its bitmap; dropping a submenu frees its items' bitmaps.
	tree4 := build(driver.MenuIcon{}, winMenuIconLook{size: 16, dark: true}, false)
	syncWinMenu(ops, bar, tree3, tree4)
	if ops.menus[file][0].bitmap != 0 || tree4[0].children[0].bitmap != 0 {
		t.Errorf("Open kept a bitmap after its icon was removed: %+v", ops.menus[file][0])
	}
	if len(ops.bitmaps) != 2 {
		t.Errorf("live bitmaps = %d, want 2 (File and Save)", len(ops.bitmaps))
	}

	releaseWinMenuTree(ops, tree4)
	if len(ops.bitmaps) != 0 {
		t.Errorf("bitmaps leaked after release: %d", len(ops.bitmaps))
	}
}

func TestWinFirstShowStateTransitions(t *testing.T) {
	var fs firstShow
	if fs.consume(false) {
		t.Fatal("unarmed firstShow consumed true")
	}

	// Non-hidden window waits for its first batch, surviving non-show commands.
	fs.arm(false)
	for _, action := range []driver.WindowAction{
		driver.WindowSetTitleBar, driver.WindowSetMenuBar, driver.WindowSetFrame,
		driver.WindowSetConstraints, driver.WindowSetLevel, driver.WindowFitSize,
		driver.WindowSetAnchor, driver.WindowToggleFullScreen, driver.WindowSetTransparent,
	} {
		fs.command(action)
	}
	if !fs.consume(false) {
		t.Fatal("expected first batch to consume pending show")
	}
	if fs.consume(false) {
		t.Fatal("second batch should not consume pending show again")
	}

	// Hidden window never shows on its first batch.
	fs.arm(true)
	if fs.consume(false) {
		t.Fatal("hidden window consumed show")
	}

	// Explicit hide/show/close/destroy cancels deferred first show.
	for _, action := range []driver.WindowAction{
		driver.WindowHide, driver.WindowShow, driver.WindowOrderFront,
		driver.WindowClose, driver.WindowDestroy,
	} {
		fs.arm(false)
		fs.command(action)
		if fs.consume(false) {
			t.Errorf("command %v did not cancel deferred first show", action)
		}
	}

	// Destroying window does not present even if still armed.
	fs.arm(false)
	if fs.consume(true) {
		t.Fatal("destroying window consumed show")
	}
	if fs.consume(false) {
		t.Fatal("consumed flag was not cleared when closed")
	}
}

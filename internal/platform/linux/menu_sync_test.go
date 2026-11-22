package linux

import (
	"fmt"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeGTKEntry struct {
	kind       string // "submenu", "section", "item"
	label      string
	childMenu  uintptr
	actionName string
	radio      bool
	iconKey    string
}

type fakeGTKAction struct {
	name     string
	kind     gtkMenuActionKind
	id       uint32
	role     driver.MenuRole
	disabled bool
	checked  bool
}

type fakeGTKMenuOps struct {
	nextMenu   uintptr
	nextAction uintptr
	counter    int
	menus      map[uintptr][]fakeGTKEntry
	menuRefs   map[uintptr]int
	actions    map[uintptr]*fakeGTKAction
	byName     map[string]uintptr

	menuMutations int
	actionAdds    int
	actionUpdates int
	actionRemoves int
}

func newFakeGTKMenuOps() *fakeGTKMenuOps {
	return &fakeGTKMenuOps{
		nextMenu:   100,
		nextAction: 500,
		menus:      map[uintptr][]fakeGTKEntry{},
		menuRefs:   map[uintptr]int{},
		actions:    map[uintptr]*fakeGTKAction{},
		byName:     map[string]uintptr{},
	}
}

func (f *fakeGTKMenuOps) newMenu() uintptr {
	f.nextMenu++
	h := f.nextMenu
	f.menus[h] = nil
	f.menuRefs[h] = 1
	return h
}

func (f *fakeGTKMenuOps) refMenu(menu uintptr) {
	f.menuRefs[menu]++
}

func (f *fakeGTKMenuOps) unrefMenu(menu uintptr) {
	f.menuRefs[menu]--
	if f.menuRefs[menu] < 0 {
		panic(fmt.Sprintf("menu %d refcount went negative", menu))
	}
	if f.menuRefs[menu] == 0 {
		entries := f.menus[menu]
		delete(f.menus, menu)
		delete(f.menuRefs, menu)
		for _, e := range entries {
			if e.childMenu != 0 {
				f.unrefMenu(e.childMenu)
			}
		}
	}
}

func (f *fakeGTKMenuOps) insertSubmenu(parent uintptr, pos int32, n *gtkMenuNode) {
	f.menuMutations++
	f.refMenu(n.menu)
	f.menus[parent] = slices.Insert(f.menus[parent], int(pos), fakeGTKEntry{
		kind:      "submenu",
		label:     n.label,
		childMenu: n.menu,
		iconKey:   n.iconKey,
	})
}

func (f *fakeGTKMenuOps) insertSection(parent uintptr, pos int32, section uintptr) {
	f.menuMutations++
	f.refMenu(section)
	f.menus[parent] = slices.Insert(f.menus[parent], int(pos), fakeGTKEntry{
		kind:      "section",
		childMenu: section,
	})
}

func (f *fakeGTKMenuOps) insertItem(parent uintptr, pos int32, n *gtkMenuNode) {
	f.menuMutations++
	f.menus[parent] = slices.Insert(f.menus[parent], int(pos), fakeGTKEntry{
		kind:       "item",
		label:      n.label,
		actionName: n.actionName,
		radio:      n.actionKind == gtkMenuActionRadio,
		iconKey:    n.iconKey,
	})
}

func (f *fakeGTKMenuOps) removeAt(parent uintptr, pos int32) {
	f.menuMutations++
	items := f.menus[parent]
	removed := items[int(pos)]
	f.menus[parent] = slices.Delete(items, int(pos), int(pos)+1)
	if removed.childMenu != 0 {
		f.unrefMenu(removed.childMenu)
	}
}

func (f *fakeGTKMenuOps) addAction(n *gtkMenuNode) (uintptr, string) {
	f.actionAdds++
	f.nextAction++
	f.counter++
	h := f.nextAction
	name := fmt.Sprintf("nsgo.item-%d", f.counter)
	f.actions[h] = &fakeGTKAction{
		name:     name,
		kind:     n.actionKind,
		id:       n.id,
		role:     n.role,
		disabled: n.disabled,
		checked:  n.checked,
	}
	f.byName[name] = h
	return h, name
}

func (f *fakeGTKMenuOps) updateAction(action uintptr, old, next *gtkMenuNode) {
	f.actionUpdates++
	a := f.actions[action]
	if a == nil {
		panic(fmt.Sprintf("updateAction on unknown action %d", action))
	}
	a.disabled = next.disabled
	a.checked = next.checked
	a.id = next.id
	a.role = next.role
}

func (f *fakeGTKMenuOps) removeAction(action uintptr, name string) {
	f.actionRemoves++
	delete(f.actions, action)
	delete(f.byName, name)
}

func TestGTKMenuSyncPreservesMenusAndActionsOnStateToggle(t *testing.T) {
	ops := newFakeGTKMenuOps()
	root := ops.newMenu()

	makeTree := func(sidebarChecked, sidebarDisabled, moviesChecked bool, firstID uint32) []gtkMenuNode {
		return buildGTKMenuTree([]driver.MenuEntry{
			{Label: "View", Items: []driver.MenuEntry{
				{Label: "Sidebar", Checkable: true, Checked: sidebarChecked, Disabled: sidebarDisabled, ID: firstID},
				{Separator: true},
				{Label: "Movies", Checkable: true, Radio: true, Checked: moviesChecked, ID: firstID + 1},
				{Label: "Courses", Checkable: true, Radio: true, Checked: !moviesChecked, ID: firstID + 2},
			}},
		}, "en", false, gtkMenuIconLook{size: 16})
	}

	tree1 := makeTree(true, false, true, 1)
	syncGTKMenuItems(ops, root, nil, tree1)

	viewMenu := tree1[0].menu
	sec0Menu := tree1[0].sections[0].menu
	sec1Menu := tree1[0].sections[1].menu
	sidebarAct := tree1[0].sections[0].items[0].action
	moviesAct := tree1[0].sections[1].items[0].action
	coursesAct := tree1[0].sections[1].items[1].action
	if viewMenu == 0 || sec0Menu == 0 || sec1Menu == 0 || sidebarAct == 0 || moviesAct == 0 || coursesAct == 0 {
		t.Fatal("initial sync did not populate handles")
	}

	// Toggle checkbox, disable it, switch radio selection, and bump handler IDs.
	ops.menuMutations, ops.actionAdds, ops.actionUpdates, ops.actionRemoves = 0, 0, 0, 0
	tree2 := makeTree(false, true, false, 10)
	syncGTKMenuItems(ops, root, tree1, tree2)

	if ops.menuMutations != 0 {
		t.Errorf("expected zero GMenu mutations on state/ID toggle, got %d", ops.menuMutations)
	}
	if ops.actionAdds != 0 || ops.actionRemoves != 0 {
		t.Errorf("expected zero action adds/removes, got adds=%d removes=%d", ops.actionAdds, ops.actionRemoves)
	}
	if ops.actionUpdates != 3 {
		t.Errorf("actionUpdates = %d, want 3", ops.actionUpdates)
	}
	if tree2[0].menu != viewMenu || tree2[0].sections[0].menu != sec0Menu || tree2[0].sections[1].menu != sec1Menu {
		t.Error("submenu or section GMenu handles were not preserved")
	}
	if tree2[0].sections[0].items[0].action != sidebarAct ||
		tree2[0].sections[1].items[0].action != moviesAct ||
		tree2[0].sections[1].items[1].action != coursesAct {
		t.Error("GSimpleAction handles were not preserved")
	}
	if a := ops.actions[sidebarAct]; a.checked || !a.disabled || a.id != 10 {
		t.Errorf("Sidebar action = %+v, want unchecked, disabled, id=10", a)
	}
	if a := ops.actions[moviesAct]; a.checked || a.id != 11 {
		t.Errorf("Movies action = %+v, want unchecked, id=11", a)
	}
	if a := ops.actions[coursesAct]; !a.checked || a.id != 12 {
		t.Errorf("Courses action = %+v, want checked, id=12", a)
	}

	// Teardown releases every GMenu and GSimpleAction cleanly.
	releaseGTKMenuTree(ops, tree2)
	ops.unrefMenu(root)
	if len(ops.menus) != 0 || len(ops.actions) != 0 {
		t.Errorf("leaked resources after teardown: menus=%d actions=%d", len(ops.menus), len(ops.actions))
	}
}

func TestGTKMenuSyncLabelActionKindSectionsAndTopology(t *testing.T) {
	ops := newFakeGTKMenuOps()
	root := ops.newMenu()

	tree1 := buildGTKMenuTree([]driver.MenuEntry{
		{Label: "View", Items: []driver.MenuEntry{
			{Label: "Sidebar", ID: 1},
			{Label: "Inspector", Checkable: true, Checked: true, ID: 2},
		}},
		{Label: "Help", Items: []driver.MenuEntry{
			{Label: "Docs", ID: 3},
		}},
	}, "en", false, gtkMenuIconLook{size: 16})
	syncGTKMenuItems(ops, root, nil, tree1)

	viewMenu := tree1[0].menu
	sidebarAct := tree1[0].sections[0].items[0].action
	inspectorAct := tree1[0].sections[0].items[1].action

	// Rename View -> Display (submenu GMenu stays!), rename Sidebar -> Left Panel (same action kind keeps action!), change Inspector from Checkable to Radio (replaces action!), add a second section, and remove Help.
	ops.menuMutations, ops.actionAdds, ops.actionUpdates, ops.actionRemoves = 0, 0, 0, 0
	tree2 := buildGTKMenuTree([]driver.MenuEntry{
		{Label: "Display", Items: []driver.MenuEntry{
			{Label: "Left Panel", ID: 1},
			{Label: "Inspector", Checkable: true, Radio: true, Checked: true, ID: 2},
			{Separator: true},
			{Label: "Zoom", ID: 4},
		}},
	}, "en", false, gtkMenuIconLook{size: 16})
	syncGTKMenuItems(ops, root, tree1, tree2)

	if tree2[0].menu != viewMenu {
		t.Error("renaming submenu replaced its GMenu instead of preserving it")
	}
	if ops.menus[root][0].label != "Display" || ops.menus[root][0].childMenu != viewMenu {
		t.Errorf("root[0] = %+v, want label Display with preserved GMenu", ops.menus[root][0])
	}
	if tree2[0].sections[0].items[0].action != sidebarAct {
		t.Error("renaming item with same action kind replaced its GSimpleAction")
	}
	if ops.menus[tree2[0].sections[0].menu][0].label != "Left Panel" {
		t.Errorf("renamed item label = %q, want Left Panel", ops.menus[tree2[0].sections[0].menu][0].label)
	}
	if tree2[0].sections[0].items[1].action == inspectorAct {
		t.Error("changing checkable to radio reused the old boolean GSimpleAction")
	}
	if _, exists := ops.actions[inspectorAct]; exists {
		t.Error("old boolean GSimpleAction was not removed")
	}
	if !ops.menus[tree2[0].sections[0].menu][1].radio {
		t.Error("replaced radio item missing radio target attribute")
	}
	if len(tree2[0].sections) != 2 || len(ops.menus[viewMenu]) != 2 {
		t.Errorf("expected 2 sections in Display submenu, got %d", len(tree2[0].sections))
	}
	if len(ops.menus[root]) != 1 {
		t.Errorf("expected Help menu removed from root, got %d top-level entries", len(ops.menus[root]))
	}
	if len(ops.actions) != 3 {
		t.Errorf("expected 3 live actions (Left Panel, Inspector, Zoom), got %d", len(ops.actions))
	}

	// Swap submenu for a leaf item at index 0.
	tree3 := buildGTKMenuTree([]driver.MenuEntry{
		{Label: "Sync", ID: 99},
	}, "en", false, gtkMenuIconLook{size: 16})
	syncGTKMenuItems(ops, root, tree2, tree3)
	if len(ops.menus[root]) != 1 || ops.menus[root][0].kind != "item" || ops.menus[root][0].label != "Sync" {
		t.Errorf("root after swap = %+v", ops.menus[root])
	}
	if len(ops.actions) != 1 {
		t.Errorf("expected 1 live action after swap, got %d", len(ops.actions))
	}

	releaseGTKMenuTree(ops, tree3)
	ops.unrefMenu(root)
	if len(ops.menus) != 0 || len(ops.actions) != 0 {
		t.Errorf("leaked resources after teardown: menus=%d actions=%d", len(ops.menus), len(ops.actions))
	}
}

func TestGTKMenuBuildNormalizesRolesAndSeparators(t *testing.T) {
	entries := []driver.MenuEntry{
		{Separator: true},
		{Label: "Library", Items: []driver.MenuEntry{
			{Separator: true},
			{Label: "Movies", Radio: true, Checkable: true, Checked: true, ID: 1},
			{Separator: true},
			{Separator: true},
			{Role: driver.MenuServices},
			{Role: driver.MenuStartSpeaking},
			{Role: driver.MenuBringAllToFront},
			{Separator: true},
		}},
		{Label: "Help", Role: driver.MenuHelpMenu},
	}

	treeNoSpeech := buildGTKMenuTree(entries, "en", false, gtkMenuIconLook{size: 16})
	if len(treeNoSpeech) != 1 {
		t.Fatalf("without speech: got %d top-level items, want 1", len(treeNoSpeech))
	}
	if len(treeNoSpeech[0].sections) != 1 || len(treeNoSpeech[0].sections[0].items) != 1 {
		t.Errorf("expected 1 non-empty section with 1 item, got %+v", treeNoSpeech[0].sections)
	}

	treeSpeech := buildGTKMenuTree(entries, "en", true, gtkMenuIconLook{size: 16})
	if len(treeSpeech[0].sections) != 2 || treeSpeech[0].sections[1].items[0].role != driver.MenuStartSpeaking {
		t.Errorf("with speech: expected 2 sections with Start Speaking, got %+v", treeSpeech[0].sections)
	}
}

// TestGTKMenuSyncReplacesRowsWhoseIconChanges checks that an icon, its tint for a template icon or its size replaces only the rows showing it, keeping their actions and submenus.
func TestGTKMenuSyncReplacesRowsWhoseIconChanges(t *testing.T) {
	ops := newFakeGTKMenuOps()
	root := ops.newMenu()
	plain := driver.MenuIcon{Data: []byte("plain")}
	template := driver.MenuIcon{Data: []byte("template"), Template: true}
	entries := func(open driver.MenuIcon) []driver.MenuEntry {
		return []driver.MenuEntry{
			{Label: "File", Icon: plain, Items: []driver.MenuEntry{
				{Label: "Open", ID: 1, Icon: open},
				{Label: "Save", ID: 2, Icon: template},
			}},
		}
	}
	light := gtkMenuIconLook{size: 16}
	tree1 := buildGTKMenuTree(entries(plain), "en", false, light)
	syncGTKMenuItems(ops, root, nil, tree1)
	file, section := tree1[0].menu, tree1[0].sections[0].menu
	open, save := tree1[0].sections[0].items[0].action, tree1[0].sections[0].items[1].action
	if ops.menus[root][0].iconKey == "" || ops.menus[section][0].iconKey == "" {
		t.Fatalf("icons not passed to inserted rows: root=%+v section=%+v", ops.menus[root], ops.menus[section])
	}

	ops.menuMutations = 0
	tree2 := buildGTKMenuTree(entries(plain), "en", false, light)
	syncGTKMenuItems(ops, root, tree1, tree2)
	if ops.menuMutations != 0 {
		t.Errorf("unchanged icons mutated the menu %d times", ops.menuMutations)
	}

	// Dark appearance retints only the template icon's row.
	ops.menuMutations = 0
	dark := gtkMenuIconLook{size: 16, dark: true}
	tree3 := buildGTKMenuTree(entries(plain), "en", false, dark)
	syncGTKMenuItems(ops, root, tree2, tree3)
	if ops.menuMutations != 2 {
		t.Errorf("dark appearance: menu mutations = %d, want 2 (remove and insert Save)", ops.menuMutations)
	}
	if got := ops.menus[section][1].iconKey; got != tree3[0].sections[0].items[1].iconKey || got == tree2[0].sections[0].items[1].iconKey {
		t.Errorf("Save row icon key = %q, want the dark one", got)
	}

	// A new icon replaces its row and keeps its action.
	tree4 := buildGTKMenuTree(entries(template), "en", false, dark)
	syncGTKMenuItems(ops, root, tree3, tree4)
	if tree4[0].menu != file || tree4[0].sections[0].menu != section {
		t.Error("an item's icon change replaced its submenu or section")
	}
	if tree4[0].sections[0].items[0].action != open || tree4[0].sections[0].items[1].action != save {
		t.Error("an icon change replaced an item's action")
	}
	if ops.menus[section][0].iconKey != tree4[0].sections[0].items[0].iconKey || ops.menus[section][0].label != "Open" {
		t.Errorf("Open row = %+v, want the template icon", ops.menus[section][0])
	}

	// A larger display scale replaces every row with an icon, the submenu's included, keeping its GMenu.
	ops.menuMutations = 0
	tree5 := buildGTKMenuTree(entries(template), "en", false, gtkMenuIconLook{size: 32, dark: true})
	syncGTKMenuItems(ops, root, tree4, tree5)
	if ops.menuMutations != 6 {
		t.Errorf("scale change: menu mutations = %d, want 6", ops.menuMutations)
	}
	if ops.menus[root][0].childMenu != file {
		t.Error("an icon change on a submenu replaced its GMenu")
	}

	releaseGTKMenuTree(ops, tree5)
	ops.unrefMenu(root)
	if len(ops.menus) != 0 || len(ops.actions) != 0 {
		t.Errorf("leaked resources after teardown: menus=%d actions=%d", len(ops.menus), len(ops.actions))
	}
}

func TestLinuxFirstShowStateTransitions(t *testing.T) {
	var fs firstShow
	if fs.consume(false) {
		t.Fatal("unarmed firstShow consumed true")
	}

	fs.arm(false)
	for _, action := range []driver.WindowAction{
		driver.WindowSetTitleBar, driver.WindowSetMenuBar, driver.WindowSetFrame,
		driver.WindowSetConstraints, driver.WindowSetLevel, driver.WindowSetWorkspaces,
		driver.WindowFitSize, driver.WindowSetAnchor, driver.WindowToggleFullScreen,
		driver.WindowSetTransparent,
	} {
		fs.command(action)
	}
	if !fs.consume(false) {
		t.Fatal("expected first batch to consume pending show")
	}
	if fs.consume(false) {
		t.Fatal("second batch should not consume pending show again")
	}

	fs.arm(true)
	if fs.consume(false) {
		t.Fatal("hidden window consumed show")
	}

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

	fs.arm(false)
	if fs.consume(true) {
		t.Fatal("closing window consumed show")
	}
	if fs.consume(false) {
		t.Fatal("consumed flag was not cleared when closing")
	}
}

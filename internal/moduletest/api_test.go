package moduletest

import (
	"bytes"
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
)

// publicPackages lists what apps may import: every library package of the module this platform builds, but those under internal/, cmd/, tools/, examples/ and testdata. It is derived from the module so a new public package cannot slip past the API checks.
func publicPackages(t *testing.T) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "-e", "-f", "{{if and (ne .Name \"main\") (or .GoFiles .CgoFiles)}}{{.ImportPath}}{{end}}", "./...")
	cmd.Dir = moduleRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list public packages: %v\n%s", err, stderr.Bytes())
	}
	var packages []string
	for path := range strings.SplitSeq(string(out), "\n") {
		if rel, ok := inModule(strings.TrimSpace(path)); ok && publicPackage(rel) {
			packages = append(packages, rel)
		}
	}
	slices.Sort(packages)
	// The root, core and nsgotest are what every app links; a listing without them went wrong.
	for _, rel := range []string{"", "core", "nsgotest", "base", "ui"} {
		if !slices.Contains(packages, rel) {
			t.Fatalf("go list found no %q among the public packages %v", rel, packages)
		}
	}
	return packages
}

// publicPackage reports whether rel, a package path inside the module, is one apps may import.
func publicPackage(rel string) bool {
	for part := range strings.SplitSeq(rel, "/") {
		switch part {
		case "internal", "cmd", "tools", "examples", "testdata":
			return false
		}
	}
	return true
}

// exportImporter loads the module's packages, and what they import, from the compiler's export data, which go list -export leaves in the build cache: type-checking some fifty public packages and their dependencies from source takes minutes. It is made once, for every test that reads the packages' types.
var exportImporter = sync.OnceValues(func() (exportTypes, error) {
	cmd := exec.Command("go", "list", "-e", "-export", "-deps", "-f", "{{if .Export}}{{.ImportPath}}={{.Export}}{{end}}", "./...")
	cmd.Dir = moduleRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return exportTypes{}, fmt.Errorf("go list -export: %v\n%s", err, stderr.Bytes())
	}
	exports := map[string]string{}
	for line := range strings.SplitSeq(string(out), "\n") {
		if path, file, ok := strings.Cut(strings.TrimSpace(line), "="); ok {
			exports[path] = file
		}
	}
	files := token.NewFileSet()
	return exportTypes{files, importer.ForCompiler(files, "gc", func(path string) (io.ReadCloser, error) {
		file, ok := exports[path]
		if !ok {
			return nil, fmt.Errorf("go list -export gave no export data for %s", path)
		}
		return os.Open(file)
	})}, nil
})

// exportTypes are exportImporter's file set and importer.
type exportTypes struct {
	files *token.FileSet
	imp   types.Importer
}

// typesImporter is exportImporter for a test, which it fails when go list does.
func typesImporter(t *testing.T) (*token.FileSet, types.Importer) {
	t.Helper()
	e, err := exportImporter()
	if err != nil {
		t.Fatal(err)
	}
	return e.files, e.imp
}

// runtimeAPI is what the engine and nsgotest drive core's tree with. Apps
// never call it, so the internal types it takes need no alias.
var runtimeAPI = map[string]bool{
	"core.Tree": true, "core.NewTree": true, "core.ApproxTextEngine": true,
}

// TestPublicAPINamesNoInternalType checks the driver allowlist and requires
// other reachable internal types to have an alias in the public API.
func TestPublicAPINamesNoInternalType(t *testing.T) {
	if testing.Short() {
		t.Skip("lists and loads every public package")
	}
	_, imp := typesImporter(t)
	load := func(rel string) *types.Package {
		path := modulePath
		if rel != "" {
			path += "/" + rel
		}
		p, err := imp.Import(path)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	aliases := func(p *types.Package, into map[*types.TypeName]bool) {
		for _, name := range p.Scope().Names() {
			if tn, ok := p.Scope().Lookup(name).(*types.TypeName); ok && tn.IsAlias() && tn.Exported() {
				if n, ok := types.Unalias(tn.Type()).(*types.Named); ok {
					into[n.Obj()] = true
				}
			}
		}
	}
	core := load("core")
	internal := func(p *types.Package) bool {
		return p != nil && strings.HasPrefix(p.Path(), modulePath+"/internal/")
	}
	for _, rel := range publicPackages(t) {
		pkg := load(rel)
		named := map[*types.TypeName]bool{}
		aliases(core, named)
		aliases(pkg, named)
		missing := map[string][]string{}
		seen := map[types.Type]bool{}
		var walk func(types.Type, string)
		walk = func(typ types.Type, from string) {
			typ = types.Unalias(typ)
			if seen[typ] {
				return
			}
			seen[typ] = true
			switch typ := typ.(type) {
			case *types.Named:
				obj := typ.Obj()
				if obj.Exported() && obj.Pkg() != nil && obj.Pkg().Path() == modulePath+"/internal/driver" &&
					!allowedPublicDriverType(obj.Name()) {
					t.Errorf("%s reaches driver.%s through %s, which is outside the public driver allowlist", pkg.Path(), obj.Name(), from)
					return
				}
				if obj.Pkg() == nil || !strings.HasPrefix(obj.Pkg().Path(), modulePath) {
					return
				}
				if internal(obj.Pkg()) && obj.Exported() && !named[obj] {
					missing[obj.Pkg().Name()+"."+obj.Name()] = append(missing[obj.Pkg().Name()+"."+obj.Name()], from)
				}
				if st, ok := typ.Underlying().(*types.Struct); ok {
					for i := range st.NumFields() {
						if f := st.Field(i); f.Exported() {
							walk(f.Type(), obj.Name()+"."+f.Name())
						}
					}
				} else {
					walk(typ.Underlying(), obj.Name())
				}
				for i := range typ.NumMethods() {
					if m := typ.Method(i); m.Exported() {
						walk(m.Type(), obj.Name()+"."+m.Name())
					}
				}
			case *types.Pointer:
				walk(typ.Elem(), from)
			case *types.Slice:
				walk(typ.Elem(), from)
			case *types.Array:
				walk(typ.Elem(), from)
			case *types.Map:
				walk(typ.Key(), from)
				walk(typ.Elem(), from)
			case *types.Chan:
				walk(typ.Elem(), from)
			case *types.Signature:
				for i := range typ.Params().Len() {
					walk(typ.Params().At(i).Type(), from)
				}
				for i := range typ.Results().Len() {
					walk(typ.Results().At(i).Type(), from)
				}
			case *types.Interface:
				for i := range typ.NumMethods() {
					walk(typ.Method(i).Type(), from)
				}
			case *types.Struct:
				for i := range typ.NumFields() {
					if f := typ.Field(i); f.Exported() {
						walk(f.Type(), from)
					}
				}
			}
		}
		for _, name := range pkg.Scope().Names() {
			obj := pkg.Scope().Lookup(name)
			if obj.Exported() && !runtimeAPI[pkg.Name()+"."+name] {
				walk(obj.Type(), pkg.Name()+"."+name)
			}
		}
		var names []string
		for n := range missing {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			from := missing[n]
			slices.Sort(from)
			t.Errorf("%s reaches %s through %v; alias it in core or in %s", pkg.Path(), n, from[:min(len(from), 3)], pkg.Name())
		}
	}
}

// twinned lists part styles that follow layout (#294) and range labels
// (#145). Text with a general-purpose prop name is checked separately.
var twinned = []string{"base.List.ListStyle", "base.Slider.StartLabel", "base.Tree.ListStyle", "ui.Slider.HighLabel", "ui.Slider.LowLabel"}

// textFuncExceptions records one-shot options, data snapshots and existing
// component props still awaiting bindings. Remove a field when adding its
// twin.
var textFuncExceptions = strings.Fields(`
core.Composition.Text core.HeaderSymbol.Label core.MenuItem.Label core.Notification.Title
core.PickDirectoriesOptions.Title core.PickFilesOptions.Title core.ShareOptions.Text core.TabBarItem.Title
core.SelectionBlock.Label core.SelectionBlock.Title core.SelectionInline.Label core.SelectionInline.Text core.SelectionInline.Title
core.TextSelection.Text core.TrayOptions.Title core.WindowOptions.Title

base.Accordion.Label base.AccordionPanel.Label base.AccordionTrigger.Label base.AccordionTrigger.Hint
base.AlertDialog.Title base.AlertDialog.Description base.Avatar.Label base.AvatarGroup.Label
base.Carousel.Label base.CarouselItem.Label base.CarouselNext.Label base.CarouselPagination.Label
base.CarouselPaginationItem.Label base.CarouselPrevious.Label base.ColorArea.Label base.ColorChoice.Text
base.ColorEyeDropper.Label base.ColorField.Label base.ColorField.Placeholder base.ColorMenuParts.Label
base.ColorSlider.Label base.ColorSwatch.Label base.Combobox.Label base.Combobox.Placeholder
base.Command.Label base.Command.Placeholder base.CommandEntry.Label base.CommandEntry.Description
base.CommandItem.Label base.CommandItem.Description base.DatePicker.Label base.DatePreset.Label
base.Link.Label base.Link.Hint base.List.Label
base.Pagination.Label base.Radio.Label base.Radio.Hint base.RadioGroup.Label base.ResizeHandle.Label
base.Scrollbar.Label base.ScrollbarLook.Label base.SelectEntry.Title base.SelectGroup.Title
base.SelectItem.Title base.Submenu.Label base.SuggestItem.Label base.SuggestList.Label base.Switch.Hint
base.Table.Label base.TimeField.Label base.TimeFieldSegmentEntry.Label base.Toggle.Label base.Toggle.Hint
base.ToggleGroup.Label base.Toolbar.Label base.ToolbarGroup.Label base.Tree.Label base.TreeEntry.Label
base.TreeItem.Label

ui.AccordionItem.Text ui.Avatar.Label ui.AvatarGroup.Label ui.AvatarGroupMember.Label ui.Carousel.Label
ui.Collapsible.Text ui.ColorWell.Label ui.Combobox.Label ui.Combobox.Placeholder ui.Command.Label
ui.Command.Placeholder ui.DatePicker.Label ui.DatePicker.Placeholder ui.DockTabLabel.Text ui.FormField.Title
ui.HoldToConfirm.Hint ui.HoldToConfirm.Text
ui.Image.Label ui.ImageCompare.Label ui.Link.Label ui.Link.Text ui.List.Label ui.MenuLabel.Text
ui.Pagination.Label ui.PlainShell.Title
ui.Radio.Label ui.Radio.Hint ui.Radio.Text ui.RadioGroup.Label ui.Rating.Label ui.ResizeHandle.Label
ui.Scrollbar.Label ui.Select.Placeholder ui.Settings.Title ui.SettingsCategory.Title
ui.SettingsCategory.Description ui.SettingsDialog.Label ui.SettingsGroup.Title ui.SettingsItem.Title
ui.SettingsItem.Description ui.SidebarButton.Text ui.SidebarItem.Text
ui.SidebarSection.Title ui.Skeleton.Label ui.Stepper.Label ui.StepperStep.Label
ui.StepperStep.Description ui.Submenu.Text ui.SuggestList.Label ui.TagInput.Label
ui.TagInput.Placeholder ui.TimeField.Label ui.ToastContent.Title ui.ToastContent.Description
ui.ToggleButton.Label ui.ToggleButton.Text ui.ToggleGroup.Label ui.Toolbar.Label ui.ToolbarGroup.Label
ui.Tree.Label

core.Screen.HeaderBackTitle
base.AvatarGroup.OverflowLabel base.CalendarConfig.NextLabel base.CalendarConfig.PreviousLabel
base.Combobox.ClearButtonLabel base.Command.SearchLabel base.CommandEntry.ShortcutLabel
base.DatePicker.ClearButtonLabel base.DatePicker.TimeFieldLabel
base.NumberInput.IncrementLabel base.NumberInput.DecrementLabel
base.Pagination.NextLabel base.Pagination.PreviousLabel base.ResizablePanelGroup.HandleLabel
base.Select.ClearButtonLabel base.SuggestItem.Detail
ui.Carousel.PreviousLabel ui.Carousel.NextLabel ui.Combobox.Empty ui.Command.SearchLabel
ui.Command.Empty ui.Command.LoadingText ui.DatePicker.TimeLabel ui.DockArea.HandleLabel ui.DockTabLabel.CloseLabel
ui.HoldToConfirm.ConfirmedText ui.ImageCompare.BeforeLabel ui.ImageCompare.AfterLabel ui.NavHeader.BackLabel
ui.NumberInput.IncrementLabel ui.NumberInput.LeadingText ui.NumberInput.DecrementLabel
ui.Pagination.PreviousLabel ui.Pagination.NextLabel ui.ResizablePanelGroup.HandleLabel
ui.ScrollView.TopBarLabel ui.ScrollView.BottomBarLabel
ui.Settings.SearchPlaceholder ui.Settings.EmptyText ui.Settings.SearchLabel ui.Settings.CloseLabel ui.Settings.ResultsTitle
ui.SidebarShell.ResizeLabel ui.SidebarShell.RightResizeLabel ui.SidebarShell.ToggleLabel ui.SidebarShell.RightToggleLabel
ui.Stepper.CompleteLabel ui.Toasts.CloseLabel ui.Tree.ToggleLabel
`)

// isTextProp also covers a part’s words, such as SearchPlaceholder and IncrementLabel, so adding a prefix cannot bypass the reactive-text guard.
func isTextProp(name string) bool {
	for _, suffix := range []string{"Label", "Title", "Description", "Hint", "Placeholder", "Text", "Footer", "Detail", "Empty"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// TestFuncFieldsBindTheirValue checks the type of each XFunc twin and
// requires twins for public text props and the fields in twinned.
func TestFuncFieldsBindTheirValue(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks the component packages from source")
	}
	imp := importer.ForCompiler(token.NewFileSet(), "source", nil)
	bound := map[string]bool{}
	exceptions := map[string]bool{}
	for _, field := range textFuncExceptions {
		exceptions[field] = true
	}
	for _, rel := range []string{"core", "base", "base/textview", "base/editor", "base/diff", "mobile", "desktop", "ui"} {
		pkg, err := imp.Import(modulePath + "/" + rel)
		if err != nil {
			t.Fatal(err)
		}
		checkText := rel == "core" || rel == "base" || rel == "ui"
		for _, name := range pkg.Scope().Names() {
			tn, ok := pkg.Scope().Lookup(name).(*types.TypeName)
			if !ok || !tn.Exported() || tn.IsAlias() {
				continue
			}
			st, ok := tn.Type().Underlying().(*types.Struct)
			if !ok {
				continue
			}
			fields := map[string]types.Type{}
			for i := range st.NumFields() {
				fields[st.Field(i).Name()] = st.Field(i).Type()
			}
			if checkText {
				for text := range fields {
					if !isTextProp(text) {
						continue
					}
					field := pkg.Name() + "." + name + "." + text
					if fields[text] == types.Typ[types.String] && fields[text+"Func"] == nil {
						if !exceptions[field] {
							t.Errorf("text prop %s needs a Func twin or an exception", field)
						}
						delete(exceptions, field)
					}
				}
			}
			for i := range st.NumFields() {
				f := st.Field(i)
				base, ok := strings.CutSuffix(f.Name(), "Func")
				if !f.Exported() || !ok || base == "" {
					continue
				}
				sig, isFunc := f.Type().Underlying().(*types.Signature)
				value, twin := fields[base]
				if !isFunc || sig.Params().Len() != 0 || sig.Results().Len() != 1 || !twin ||
					!types.Identical(sig.Results().At(0).Type(), value) {
					t.Errorf("%s.%s.%s is not a func() of %s's type beside it", pkg.Name(), name, f.Name(), base)
					continue
				}
				bound[pkg.Name()+"."+name+"."+base] = true
			}
		}
	}
	for _, field := range textFuncExceptions {
		if exceptions[field] {
			t.Errorf("text prop exception %s is no longer needed", field)
		}
	}
	for _, field := range twinned {
		if !bound[field] {
			t.Errorf("%s has no %sFunc twin: it changes while its component is shown", field, field[strings.LastIndex(field, ".")+1:])
		}
	}
}

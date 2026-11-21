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
	"core.Tree": true, "core.NewTree": true, "core.ApproxTextEngine": true, "core.NewStore": true,
}

// protocolOnly lists driver types that have no place in the public API.
var protocolOnly = map[string]bool{
	"ShareRequest": true, "WindowAction": true, "WindowCommand": true,
}

// TestPublicAPINamesNoInternalType keeps protocol-only types out of
// public APIs and checks that any other reachable internal type has an
// alias in core or in the public package.
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
				if obj.Pkg().Path() == modulePath+"/internal/driver" && protocolOnly[obj.Name()] {
					t.Errorf("%s reaches the protocol type driver.%s through %s", pkg.Path(), obj.Name(), from)
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

// twinned are the fields that must have an XFunc twin: part styles that live as long as their component, such as a list of rows' padding, which follows the window's layout and would otherwise remount the list as it changes (#294). The other part styles are read once per presentation, such as a popup's or a backdrop's, and may stay fixed.
var twinned = []string{"base.List.ListStyle", "base.Tree.ListStyle"}

// TestFuncFieldsBindTheirValue checks the type of each XFunc twin and requires twins for core nodes' accessibility text and the fields in twinned.
func TestFuncFieldsBindTheirValue(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks the component packages from source")
	}
	imp := importer.ForCompiler(token.NewFileSet(), "source", nil)
	bound := map[string]bool{}
	for _, rel := range []string{"core", "base", "base/textview", "base/editor", "base/diff", "mobile", "desktop", "ui"} {
		pkg, err := imp.Import(modulePath + "/" + rel)
		if err != nil {
			t.Fatal(err)
		}
		var coreNode *types.Interface
		if rel == "core" {
			coreNode = pkg.Scope().Lookup("Node").Type().Underlying().(*types.Interface)
		}
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
			if coreNode != nil && types.Implements(types.NewPointer(tn.Type()), coreNode) {
				for _, text := range []string{"Label", "Hint", "AccessibilityDescription"} {
					if fields[text] == types.Typ[types.String] && fields[text+"Func"] == nil {
						t.Errorf("%s.%s.%s needs a %sFunc twin for changing accessibility text", pkg.Name(), name, text, text)
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
	for _, field := range twinned {
		if !bound[field] {
			t.Errorf("%s has no %sFunc twin: it changes while its component is shown", field, field[strings.LastIndex(field, ".")+1:])
		}
	}
}

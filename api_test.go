package nsgo

import (
	"go/importer"
	"go/token"
	"go/types"
	"slices"
	"sort"
	"strings"
	"testing"
)

// publicPackages are what apps import; everything under internal/ is the
// implementation.
var publicPackages = []string{"", "core", "base", "base/textview", "base/textview/highlight", "icons/lucide", "nsgotest"}

// runtimeAPI is what the engine and nsgotest drive core's tree with. Apps
// never call it, so the internal types it takes need no alias.
var runtimeAPI = map[string]bool{
	"core.Tree": true, "core.NewTree": true, "core.ApproxTextEngine": true, "core.NewStore": true,
}

// TestPublicAPINamesNoInternalType checks that every internal type a public
// package's API reaches has an alias in core or in that package, so apps
// can name what they are given.
func TestPublicAPINamesNoInternalType(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks the public packages from source")
	}
	imp := importer.ForCompiler(token.NewFileSet(), "source", nil)
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
	for _, rel := range publicPackages {
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

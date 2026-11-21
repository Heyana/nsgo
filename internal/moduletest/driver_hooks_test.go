package moduletest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestDriverHooks requires documentation for each exported function variable
// that can connect the driver directly to a backend.
func TestDriverHooks(t *testing.T) {
	dir := filepath.Join("..", "driver")
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var hooks []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.VAR {
				continue
			}
			for _, spec := range group.Specs {
				v := spec.(*ast.ValueSpec)
				_, explicit := v.Type.(*ast.FuncType)
				for i, n := range v.Names {
					if !n.IsExported() {
						continue
					}
					inferred := false
					if i < len(v.Values) {
						_, inferred = v.Values[i].(*ast.FuncLit)
					}
					if explicit || inferred {
						hooks = append(hooks, n.Name)
					}
				}
			}
		}
	}

	doc, err := os.ReadFile(filepath.Join(moduleRoot, "docs", "architecture", "layers.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, section, ok := strings.Cut(string(doc), "\n### Driver hooks\n")
	if !ok {
		t.Fatal("layers.md is missing its Driver hooks section")
	}
	section, _, _ = strings.Cut(section, "\n## ")
	var documented []string
	for line := range strings.SplitSeq(section, "\n") {
		if line, ok := strings.CutPrefix(line, "- `"); ok {
			hook, _, _ := strings.Cut(line, "`")
			documented = append(documented, hook)
		}
	}
	slices.Sort(hooks)
	slices.Sort(documented)
	if len(hooks) == 0 || !slices.Equal(hooks, documented) {
		t.Errorf("driver hooks = %v, documented in layers.md = %v", hooks, documented)
	}
}

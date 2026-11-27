package moduletest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestUISubpackagesNoInternalImports ensures no Go file under ui/*/ (including
// tests, build-tagged files and locales packages) imports any internal package,
// so declared ui subpackage components can be copied directly into apps (#2471).
func TestUISubpackagesNoInternalImports(t *testing.T) {
	uiRoot := filepath.Join(moduleRoot, "ui")
	entries, err := os.ReadDir(uiRoot)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "testdata" {
			continue
		}
		subRoot := filepath.Join(uiRoot, entry.Name())
		err := filepath.WalkDir(subRoot, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
			if err != nil {
				t.Errorf("%s: parse error: %v", path, err)
				return nil
			}
			checked++
			rel, _ := filepath.Rel(moduleRoot, path)
			for _, spec := range f.Imports {
				imp, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Errorf("%s: unquote import %s: %v", rel, spec.Path.Value, err)
					continue
				}
				if hasInternalSegment(imp) {
					t.Errorf("%s imports internal package %q; ui subpackage components must only import exported packages", rel, imp)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked == 0 {
		t.Fatal("no Go files found under ui/*/")
	}
}

func hasInternalSegment(imp string) bool {
	return imp == "internal" ||
		strings.HasPrefix(imp, "internal/") ||
		strings.HasSuffix(imp, "/internal") ||
		strings.Contains(imp, "/internal/")
}

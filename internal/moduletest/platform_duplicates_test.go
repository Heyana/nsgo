package moduletest

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const platformDir = "internal/platform"

type platformFunction struct {
	file, backend, name string
	line                int
}

type platformFunctions map[string][]platformFunction

// TestPlatformFunctionDuplicates reports new copies of the same function
// across backends. The pull request job supplies the base of the comparison.
func TestPlatformFunctionDuplicates(t *testing.T) {
	base := os.Getenv("NSGO_PLATFORM_DUPLICATES_BASE")
	if base == "" {
		t.Skip("needs NSGO_PLATFORM_DUPLICATES_BASE")
	}
	base = strings.TrimSpace(string(platformGit(t, "rev-parse", "--verify", "--end-of-options", base+"^{commit}")))
	fields := strings.Split(string(platformGit(t, "diff", "--name-status", "--no-renames", "-z", base, "--", platformDir)), "\x00")
	changes := map[string]string{}
	for i := 0; i+1 < len(fields); i += 2 {
		if file := fields[i+1]; platformBackend(file) != "" {
			changes[file] = fields[i]
		}
	}
	if len(changes) == 0 {
		return
	}

	before, after := platformFunctions{}, platformFunctions{}
	files := strings.SplitSeq(string(platformGit(t, "ls-files", "-z", "--", platformDir)), "\x00")
	for file := range files {
		backend := platformBackend(file)
		if backend == "" || changes[file] == "D" {
			continue
		}
		source, err := os.ReadFile(filepath.Join(moduleRoot, filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		functions := readPlatformFunctions(t, file, backend, source)
		after.add(functions)
		if changes[file] == "" {
			before.add(functions)
		}
	}
	for file, status := range changes {
		if status != "A" {
			before.add(readPlatformFunctions(t, file, platformBackend(file), platformGit(t, "show", base+":"+file)))
		}
	}

	for _, detail := range newPlatformDuplicates(before, after) {
		t.Error(detail)
	}
}

// newPlatformDuplicates compares declaration pairs. Two backends that
// already shared an exact copy can edit that pair without adding another.
func newPlatformDuplicates(before, after platformFunctions) []string {
	type declaration struct{ dir, name string }
	id := func(f platformFunction) declaration { return declaration{path.Dir(f.file), f.name} }
	previous := map[declaration]map[string]bool{}
	for body, functions := range before {
		for _, f := range functions {
			key := id(f)
			if previous[key] == nil {
				previous[key] = map[string]bool{}
			}
			previous[key][body] = true
		}
	}
	hadDuplicate := func(first, second platformFunction) bool {
		firstBodies, secondBodies := previous[id(first)], previous[id(second)]
		for body := range firstBodies {
			if secondBodies[body] {
				return true
			}
		}
		return false
	}

	var diagnostics []string
	for _, functions := range after {
		for i, first := range functions {
			for _, second := range functions[i+1:] {
				if first.backend != second.backend && !hadDuplicate(first, second) {
					diagnostics = append(diagnostics, fmt.Sprintf("%s:%d duplicates %s:%d", first.file, first.line, second.file, second.line))
				}
			}
		}
	}
	slices.Sort(diagnostics)
	return diagnostics
}

func platformBackend(file string) string {
	if path.Ext(file) != ".go" || strings.HasSuffix(file, "_test.go") {
		return ""
	}
	rest, _ := strings.CutPrefix(file, platformDir+"/")
	backend, _, _ := strings.Cut(rest, "/")
	switch backend {
	case "android", "harmonyos", "ios", "linux", "macos", "miniprogram", "watchos", "web", "windows":
		return backend
	default:
		return ""
	}
}

func readPlatformFunctions(t *testing.T, file, backend string, source []byte) platformFunctions {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	functions := platformFunctions{}
	if ast.IsGenerated(parsed) {
		return functions
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || len(fn.Body.List) < 2 || allowsPlatformDuplicate(fn.Doc) {
			continue
		}
		line := fset.Position(fn.Pos()).Line
		fn.Doc = nil
		var formatted bytes.Buffer
		name := fn.Name.Name
		if fn.Recv != nil {
			if err := printer.Fprint(&formatted, fset, fn.Recv.List[0].Type); err != nil {
				t.Fatal(err)
			}
			name = formatted.String() + "." + name
			formatted.Reset()
		}
		if err := printer.Fprint(&formatted, fset, fn); err != nil {
			t.Fatal(err)
		}
		key := formatted.String()
		functions[key] = append(functions[key], platformFunction{file: file, backend: backend, name: name, line: line})
	}
	return functions
}

func allowsPlatformDuplicate(doc *ast.CommentGroup) bool {
	if doc == nil {
		return false
	}
	for _, comment := range doc.List {
		if reason, ok := strings.CutPrefix(comment.Text, "// nsgo:allow-duplicate "); ok && strings.TrimSpace(reason) != "" {
			return true
		}
	}
	return false
}

func (f platformFunctions) add(other platformFunctions) {
	for body, functions := range other {
		f[body] = append(f[body], functions...)
	}
}

func platformGit(t *testing.T, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = moduleRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return output
}

func TestPlatformDuplicateChanges(t *testing.T) {
	const (
		first   = "internal/platform/linux/scroll.go"
		second  = "internal/platform/windows/scroll.go"
		third   = "internal/platform/web/scroll.go"
		oldCode = "package backend\nfunc (w *window) scroll() { start(); stop() }"
		newCode = "package backend\nfunc (w *window) scroll() { settle(); report() }"
	)
	read := func(file, code string) platformFunctions {
		return readPlatformFunctions(t, file, platformBackend(file), []byte(code))
	}
	before := read(first, oldCode)
	before.add(read(second, oldCode))
	after := read(first, newCode)
	after.add(read("internal/platform/windows/animate.go", newCode))
	if got := newPlatformDuplicates(before, after); len(got) != 0 {
		t.Fatalf("editing and moving an existing pair reported %v", got)
	}
	after.add(read(third, newCode))
	if got := newPlatformDuplicates(before, after); len(got) != 2 {
		t.Fatalf("a third copy made %v, want two new pairs", got)
	}

	differentReceiver := "package backend\nfunc (w *other) scroll() { start(); stop() }"
	before = read(first, oldCode)
	before.add(read(second, differentReceiver))
	after = read(first, newCode)
	after.add(read(second, newCode))
	if got := newPlatformDuplicates(before, after); len(got) != 1 {
		t.Fatalf("a new pair of methods reported %v, want one duplicate", got)
	}
}

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
	file, backend string
	line          int
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

	var diagnostics []string
	for body, functions := range after {
		old := map[string]bool{}
		for _, fn := range before[body] {
			old[fn.backend] = true
		}
		for i, first := range functions {
			for _, second := range functions[i+1:] {
				if first.backend != second.backend && !(old[first.backend] && old[second.backend]) {
					diagnostics = append(diagnostics, fmt.Sprintf("%s:%d duplicates %s:%d", first.file, first.line, second.file, second.line))
				}
			}
		}
	}
	slices.Sort(diagnostics)
	for _, detail := range diagnostics {
		t.Error(detail)
	}
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
		if err := printer.Fprint(&formatted, fset, fn); err != nil {
			t.Fatal(err)
		}
		key := formatted.String()
		functions[key] = append(functions[key], platformFunction{file: file, backend: backend, line: line})
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

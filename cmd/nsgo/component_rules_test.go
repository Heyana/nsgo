package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// componentModuleFixture creates a temporary library module with a valid
// declared component at ui/chip plus any extra or overriding files.
func componentModuleFixture(t *testing.T, extra map[string]string) *project {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/kit\n\ngo 1.26\n",
		"nsgo-module.yaml": `version: 1
components:
  chip: ui/chip
`,
		"ui/chip/chip.go": "package chip\n\n// Value is exported.\nconst Value = 1\n",
	}
	for k, v := range extra {
		files[k] = v
	}
	writeFiles(t, dir, files)
	p, err := loadOrDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// memFile constructs an in-memory componentFile for direct checkComponentRules tests.
func memFile(p string, content []byte) componentFile {
	return componentFile{
		Path: p,
		Size: int64(len(content)),
		Open: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(content)), nil
		},
	}
}

func TestCheckComponentsValidFixture(t *testing.T) {
	if testing.Short() {
		t.Skip("runs real go vet")
	}
	p := componentModuleFixture(t, map[string]string{
		"ui/chip/chip.go": `package chip

import (
	_ "embed"

	"example.com/kit/ui/chip/locales"
)

//go:embed assets/config.json
var configJSON []byte

var Label = locales.Hello
`,
		"ui/chip/chip_linux.go":  "//go:build linux\n\npackage chip\n\nvar linuxOnly = true\n",
		"ui/chip/chip_test.go":   "package chip_test\n\nimport (\n\t\"testing\"\n\t\"example.com/kit/ui/chip\"\n)\n\nfunc TestChip(t *testing.T) { _ = chip.Label }\n",
		"ui/chip/README.md":      "# Chip\n",
		"ui/chip/LICENSE":        "MIT License\n",
		"ui/chip/notes.txt":      "Plain text note.\r\nSecond line.\r\n",
		"ui/chip/assets/a.svg":   "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>\n",
		"ui/chip/assets/b.png":   "\x89PNG\r\n\x1a\n\x00\x00\xff\xfe", // arbitrary binary bytes
		"ui/chip/assets/c.woff2": "wOF2\x00\xff",
		"ui/chip/assets/d.wav":   "RIFF\x00\xff",
		"ui/chip/assets/config.json": `{
  "ok": true
}
`,
		"ui/chip/testdata/sample.yaml": "key: value\n",
		"ui/chip/testdata/nested/data.json": `{"nested": 1}
`,
		"ui/chip/locales/locales.go": "package locales\n\n//go:generate go tool nsgo i18n -source en\n\nconst Hello = \"hi\"\n",
		"ui/chip/locales/en.yaml":    "hello: Hello\n",
		"ui/chip/locales/ar.yaml":    "hello: \"مرحبا\u200F بك\u200E\u061C\u2066RTL\u2069\"\n",
	})
	var err error
	out := captureStderr(t, func() { err = check(t.Context(), p, "", 0) })
	if err != nil {
		t.Fatalf("valid component failed nsgo check: %v\n%s", err, out)
	}
	if !strings.Contains(out, "checked components") {
		t.Errorf("expected checked components step in output:\n%s", out)
	}
	if strings.Contains(out, "warn") {
		t.Errorf("unexpected warning for valid component:\n%s", out)
	}
}

func TestCheckModuleWithoutComponentsUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("runs real go vet")
	}
	t.Run("no nsgo-module.yaml", func(t *testing.T) {
		p := checkFixture(t)
		var err error
		out := captureStderr(t, func() { err = check(t.Context(), p, "", 0) })
		if err != nil {
			t.Fatalf("check without nsgo-module.yaml failed: %v\n%s", err, out)
		}
		if strings.Contains(out, "component") {
			t.Errorf("module without nsgo-module.yaml should not run component check:\n%s", out)
		}
	})
	t.Run("nsgo-module.yaml without components", func(t *testing.T) {
		p := checkFixture(t)
		writeFiles(t, p.Dir, map[string]string{
			"nsgo-module.yaml": "version: 1\ntargets: [macos, linux, windows]\n",
			"AGENTS.md":        "# agent instructions in a non-component module are fine\n",
		})
		var err error
		out := captureStderr(t, func() { err = check(t.Context(), p, "", 0) })
		if err != nil {
			t.Fatalf("check without components failed: %v\n%s", err, out)
		}
		if strings.Contains(out, "checking components") {
			t.Errorf("module without components should not run component check:\n%s", out)
		}
	})
}

func TestCheckComponentRuleErrors(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		wantParts []string
	}{
		{
			name:      "AGENTS.md",
			files:     map[string]string{"ui/chip/AGENTS.md": "# instructions\n"},
			wantParts: []string{"ui/chip/AGENTS.md"},
		},
		{
			name:      "CLAUDE.md",
			files:     map[string]string{"ui/chip/CLAUDE.md": "# instructions\n"},
			wantParts: []string{"ui/chip/CLAUDE.md"},
		},
		{
			name:      "SKILL.md",
			files:     map[string]string{"ui/chip/SKILL.md": "# skill\n"},
			wantParts: []string{"ui/chip/SKILL.md"},
		},
		{
			name:      "README.md in subdirectory",
			files:     map[string]string{"ui/chip/assets/README.md": "# nested readme\n"},
			wantParts: []string{"ui/chip/assets/README.md"},
		},
		{
			name:      "package.json in asset directory",
			files:     map[string]string{"ui/chip/assets/package.json": `{"name":"bad"}`},
			wantParts: []string{"ui/chip/assets/package.json", "package.json"},
		},
		{
			name:      "tsconfig.json case-insensitive in testdata",
			files:     map[string]string{"ui/chip/testdata/TSConfig.JSON": `{}`},
			wantParts: []string{"ui/chip/testdata/TSConfig.JSON", "tsconfig.json"},
		},
		{
			name:      "top-level json",
			files:     map[string]string{"ui/chip/data.json": `{}`},
			wantParts: []string{"ui/chip/data.json", "JSON"},
		},
		{
			name:      "syso file",
			files:     map[string]string{"ui/chip/resource.syso": "\x00\x01\x02"},
			wantParts: []string{"ui/chip/resource.syso", ".syso"},
		},
		{
			name:      "assembly .s file",
			files:     map[string]string{"ui/chip/fast.s": "TEXT ·f(SB),$0\n\tRET\n"},
			wantParts: []string{"ui/chip/fast.s", ".s"},
		},
		{
			name:      "C source file",
			files:     map[string]string{"ui/chip/helper.c": "int x(void) { return 1; }\n"},
			wantParts: []string{"ui/chip/helper.c", ".c"},
		},
		{
			name:      "wasm file",
			files:     map[string]string{"ui/chip/assets/mod.wasm": "\x00asm\x01\x00\x00\x00"},
			wantParts: []string{"ui/chip/assets/mod.wasm", ".wasm"},
		},
		{
			name:      "license prefix with refused extension",
			files:     map[string]string{"ui/chip/LICENSE.syso": "\x00\x01"},
			wantParts: []string{"ui/chip/LICENSE.syso", ".syso"},
		},
		{
			name:      "nsgo-natives.yaml",
			files:     map[string]string{"ui/chip/nsgo-natives.yaml": "version: 1\n"},
			wantParts: []string{"ui/chip/nsgo-natives.yaml", "nsgo-natives.yaml"},
		},
		{
			name:      "nsgo-copy.yaml",
			files:     map[string]string{"ui/chip/nsgo-copy.yaml": "version: 1\n"},
			wantParts: []string{"ui/chip/nsgo-copy.yaml", "nsgo-copy.yaml"},
		},
		{
			name:      "go.work inside component",
			files:     map[string]string{"ui/chip/go.work": "go 1.26\n"},
			wantParts: []string{"ui/chip/go.work", "go.work"},
		},
		{
			name:      "top-level yaml",
			files:     map[string]string{"ui/chip/config.yaml": "a: 1\n"},
			wantParts: []string{"ui/chip/config.yaml", "YAML"},
		},
		{
			name:      "path starting with dot",
			files:     map[string]string{"ui/chip/.secret.txt": "secret\n"},
			wantParts: []string{"ui/chip/.secret.txt", "start with '.' or '_'"},
		},
		{
			name:      "path starting with underscore",
			files:     map[string]string{"ui/chip/_draft.go": "package chip\n"},
			wantParts: []string{"ui/chip/_draft.go", "start with '.' or '_'"},
		},
		{
			name:      "second package in subdirectory",
			files:     map[string]string{"ui/chip/sub/extra.go": "package sub\n"},
			wantParts: []string{"ui/chip/sub/extra.go", "second package"},
		},
		{
			name:      "second package under locales",
			files:     map[string]string{"ui/chip/locales/sub/extra.go": "package sub\n"},
			wantParts: []string{"ui/chip/locales/sub/extra.go", "second package"},
		},
		{
			name: "internal import",
			files: map[string]string{
				"internal/secret/secret.go": "package secret\nconst X = 1\n",
				"ui/chip/chip.go": `package chip

import "example.com/kit/internal/secret"

var _ = secret.X
`,
			},
			wantParts: []string{"ui/chip/chip.go:3:", "internal package"},
		},
		{
			name: "import C",
			files: map[string]string{
				"ui/chip/chip.go": `package chip

import "C"

var _ = 1
`,
			},
			wantParts: []string{"ui/chip/chip.go:3:", `import "C"`},
		},
		{
			name: "tsx import outside tests",
			files: map[string]string{
				"ui/chip/chip.go": `package chip

import _ "github.com/nsgo-dev/nsgo/tsx"

var _ = 1
`,
			},
			wantParts: []string{"ui/chip/chip.go:3:", "github.com/nsgo-dev/nsgo/tsx"},
		},
		{
			name: "foreign go:generate at root",
			files: map[string]string{
				"ui/chip/chip.go": `package chip

//go:generate echo hello
const Value = 1
`,
			},
			wantParts: []string{"ui/chip/chip.go:3:", "//go:generate"},
		},
		{
			name: "foreign go:generate in locales",
			files: map[string]string{
				"ui/chip/locales/locales.go": `package locales

//go:generate sh -c evil
`,
			},
			wantParts: []string{"ui/chip/locales/locales.go:3:", "//go:generate"},
		},
		{
			name: "go:build ignore",
			files: map[string]string{
				"ui/chip/ignored.go": `//go:build ignore

package chip

var ignored = 1
`,
			},
			wantParts: []string{"ui/chip/ignored.go:1:", "//go:build ignore"},
		},
		{
			name: "plus build ignore",
			files: map[string]string{
				"ui/chip/ignored.go": `// +build ignore

package chip

var ignored = 1
`,
			},
			wantParts: []string{"ui/chip/ignored.go:1:", "//go:build ignore"},
		},
		{
			name: "2 MiB file",
			files: map[string]string{
				"ui/chip/assets/big.txt": strings.Repeat("a", 2<<20),
			},
			wantParts: []string{"ui/chip/assets/big.txt", "1 MiB"},
		},
		{
			name: "embed pattern matching refused file",
			files: map[string]string{
				"ui/chip/assets/package.json": `{"name":"x"}`,
				"ui/chip/chip.go": `package chip

import _ "embed"

//go:embed assets
var assetsFS any
`,
			},
			wantParts: []string{
				"ui/chip/assets/package.json",
				"ui/chip/chip.go:5:",
				"//go:embed",
				"assets/package.json",
			},
		},
		{
			name: "embed pattern matching nsgo-copy.yaml",
			files: map[string]string{
				"ui/chip/chip.go": `package chip

import _ "embed"

//go:embed *
var allFS any
`,
			},
			wantParts: []string{"ui/chip/chip.go:5:", "nsgo-copy.yaml"},
		},
		{
			name: "embed pattern escaping package directory",
			files: map[string]string{
				"ui/chip/chip.go": `package chip

import _ "embed"

//go:embed ../outside.txt
var badFS any
`,
			},
			wantParts: []string{"ui/chip/chip.go:5:", "escapes"},
		},
		{
			name: "UTF-8 BOM in Go file",
			files: map[string]string{
				"ui/chip/chip.go": "\xEF\xBB\xBFpackage chip\n",
			},
			wantParts: []string{"ui/chip/chip.go:1:1:", "BOM"},
		},
		{
			name: "invalid UTF-8 in text file",
			files: map[string]string{
				"ui/chip/notes.txt": "line 1\nline 2 \xff bad\n",
			},
			wantParts: []string{"ui/chip/notes.txt:2:8:", "UTF-8"},
		},
		{
			name: "Go parse error behind unmatched build tag",
			files: map[string]string{
				"ui/chip/broken.go": "//go:build customnever\n\npackage chip\n\nfunc broken( {\n",
			},
			wantParts: []string{"ui/chip/broken.go:5:", "parse error"},
		},
		{
			name: "wrong package declaration",
			files: map[string]string{
				"ui/chip/other.go": "//go:build customnever\n\npackage wrong\n",
			},
			wantParts: []string{"ui/chip/other.go:3:", `declares package "wrong"`},
		},
		{
			name: "nested go.mod in component",
			files: map[string]string{
				"ui/chip/go.mod": "module example.com/kit/ui/chip\n\ngo 1.26\n",
			},
			wantParts: []string{"ui/chip", "nested module"},
		},
		{
			name: "main package component refused",
			files: map[string]string{
				"nsgo-module.yaml": "version: 1\ncomponents:\n  app: cmd/main\n",
				"cmd/main/main.go": "package main\n\nfunc main() {}\n",
			},
			wantParts: []string{"cmd/main", "main package"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := componentModuleFixture(t, tc.files)
			var err error
			out := captureStderr(t, func() { err = check(t.Context(), p, "", 0) })
			if err == nil {
				t.Fatalf("expected error for %s, got nil\n%s", tc.name, out)
			}
			msg := err.Error()
			for _, want := range tc.wantParts {
				if !strings.Contains(msg, want) {
					t.Errorf("error missing %q:\n%s", want, msg)
				}
			}
		})
	}
}

func TestComponentRulesCaseCollisionAndLimits(t *testing.T) {
	t.Run("two paths equal but for case", func(t *testing.T) {
		res := checkComponentRules([]componentFile{
			memFile("chip.go", []byte("package chip\n")),
			memFile("Chip.go", []byte("package chip\n")),
		}, componentCheckOptions{ModulePath: "example.com/kit", ComponentDir: "ui/chip"})
		if len(res.Errors) == 0 {
			t.Fatal("expected case collision error")
		}
		var joined strings.Builder
		for _, e := range res.Errors {
			joined.WriteString(formatComponentFinding("ui/chip", e) + "\n")
		}
		if !strings.Contains(joined.String(), "ui/chip/chip.go") || !strings.Contains(joined.String(), "collides case-insensitively") {
			t.Errorf("unexpected error output:\n%s", joined.String())
		}
	})

	t.Run("directory case collision", func(t *testing.T) {
		res := checkComponentRules([]componentFile{
			memFile("chip.go", []byte("package chip\n")),
			memFile("Assets/a.txt", []byte("a\n")),
			memFile("assets/b.txt", []byte("b\n")),
		}, componentCheckOptions{ModulePath: "example.com/kit", ComponentDir: "ui/chip"})
		found := false
		for _, e := range res.Errors {
			if strings.Contains(e.Message, "collides case-insensitively") {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected directory case collision error, got %+v", res.Errors)
		}
	})

	t.Run("more than 256 files", func(t *testing.T) {
		files := []componentFile{memFile("chip.go", []byte("package chip\n"))}
		for i := range maxComponentFiles {
			files = append(files, memFile(filepath.ToSlash(filepath.Join("assets", "f"+string(rune('a'+i/26))+string(rune('a'+i%26))+".txt")), []byte("x")))
		}
		res := checkComponentRules(files, componentCheckOptions{ModulePath: "example.com/kit", ComponentDir: "ui/chip"})
		found := false
		for _, e := range res.Errors {
			if strings.Contains(e.Message, "maximum is 256") {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected file-count limit error, got %+v", res.Errors)
		}
	})

	t.Run("more than 4 MiB total", func(t *testing.T) {
		chunk := bytes.Repeat([]byte("a"), 900<<10) // 900 KiB each, 5 * 900 KiB > 4 MiB
		files := []componentFile{
			memFile("chip.go", []byte("package chip\n")),
			memFile("assets/f1.txt", chunk),
			memFile("assets/f2.txt", chunk),
			memFile("assets/f3.txt", chunk),
			memFile("assets/f4.txt", chunk),
			memFile("assets/f5.txt", chunk),
		}
		res := checkComponentRules(files, componentCheckOptions{ModulePath: "example.com/kit", ComponentDir: "ui/chip"})
		found := false
		for _, e := range res.Errors {
			if strings.Contains(e.Message, "4 MiB") {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected total-size limit error, got %+v", res.Errors)
		}
	})

	t.Run("tsx import allowed in _test.go", func(t *testing.T) {
		res := checkComponentRules([]componentFile{
			memFile("chip.go", []byte("package chip\n")),
			memFile("chip_test.go", []byte("package chip\n\nimport _ \"github.com/nsgo-dev/nsgo/tsx\"\n")),
		}, componentCheckOptions{ModulePath: "example.com/kit", ComponentDir: "ui/chip"})
		if len(res.Errors) != 0 {
			t.Fatalf("expected tsx import in _test.go to be allowed, got %+v", res.Errors)
		}
	})
}

func TestCheckComponentWarningFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("runs real go vet")
	}
	p := componentModuleFixture(t, map[string]string{
		"ui/chip/chip.go": `package chip

import (
	"os/exec"
	"syscall"
	"unsafe"
)

// bad comment ` + "\u202E" + ` reversed
var v` + "\u0430" + `l = unsafe.Sizeof(0) // Cyrillic 'а' (U+0430)

//go:wasmimport env foo
func wasmFoo()

var (
	_ = exec.ErrNotFound
	_ = syscall.EINTR
	_ = v` + "\u0430" + `l
)
`,
	})
	// Use a runner stub for go vet so the test does not fail on //go:wasmimport
	// on non-wasm hosts while still running the full check() pipeline.
	t.Setenv("GOEXPERIMENT", "nojsonv2")
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		if cmd.name == "go" && len(cmd.args) > 0 && (cmd.args[0] == "vet" || cmd.args[0] == "env") {
			return nil, nil
		}
		return nil, os.ErrInvalid
	})

	var err error
	out := captureStderr(t, func() { err = check(ctx, p, "", 0) })
	if err != nil {
		t.Fatalf("warning flags should not fail nsgo check: %v\n%s", err, out)
	}
	wants := []string{
		"warn ui/chip/chip.go:9:16: contains control or format character U+202E (nsgo add -y will refuse the copy)",
		"warn ui/chip/chip.go:10:5: identifier \"v\u0430l\" contains non-ASCII characters (nsgo add -y will refuse the copy)",
		"warn ui/chip/chip.go:6: imports \"unsafe\" (nsgo add -y will refuse the copy)",
		"warn ui/chip/chip.go:5: imports \"syscall\" (nsgo add -y will refuse the copy)",
		"warn ui/chip/chip.go:4: imports \"os/exec\" (nsgo add -y will refuse the copy)",
		"warn ui/chip/chip.go:12: uses directive //go:wasmimport (nsgo add -y will refuse the copy)",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("missing warning %q in:\n%s", w, out)
		}
	}
}

func TestComponentRulesFlagsAndRTLException(t *testing.T) {
	t.Run("RTL catalog exception and boundaries", func(t *testing.T) {
		arYAML := "# comment with \u200F mark\n" +
			"greeting: \"مرحبا\u200F بك\u200E\u061C\u2066اسم\u2069\"\n" +
			"multiline:\n" +
			"  message: |\n" +
			"    سطر\u200F أول\n" +
			"  description: \"desc with \u200F mark\"\n" +
			"override: \"bad\u202Eoverride\"\n"
		enYAML := "greeting: \"hello\u200F\"\n"
		res := checkComponentRules([]componentFile{
			memFile("chip.go", []byte("package chip\n")),
			memFile("locales/locales.go", []byte("package locales\n")),
			memFile("locales/ar.yaml", []byte(arYAML)),
			memFile("locales/en.yaml", []byte(enYAML)),
		}, componentCheckOptions{ModulePath: "example.com/kit", ComponentDir: "ui/chip"})
		if len(res.Errors) != 0 {
			t.Fatalf("unexpected errors: %+v", res.Errors)
		}
		var got []string
		for _, f := range res.Flags {
			got = append(got, formatComponentFinding("", f))
		}
		joined := strings.Join(got, "\n")
		// Expected flags:
		// - ar.yaml line 1 (comment with U+200F)
		// - ar.yaml line 6 (description with U+200F)
		// - ar.yaml line 7 (U+202E override is never exempted)
		// - en.yaml line 1 (non-RTL language with U+200F)
		for _, want := range []string{
			"locales/ar.yaml:1:",
			"locales/ar.yaml:6:",
			"locales/ar.yaml:7:",
			"U+202E",
			"locales/en.yaml:1:",
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("missing expected flag %q in:\n%s", want, joined)
			}
		}
		// Must NOT flag ar.yaml lines 2 or 5 (message values with exempted RTL marks).
		for _, forbidden := range []string{"locales/ar.yaml:2:", "locales/ar.yaml:5:"} {
			if strings.Contains(joined, forbidden) {
				t.Errorf("RTL message mark should be exempted, but got %q in:\n%s", forbidden, joined)
			}
		}
	})

	t.Run("all flagged imports and directives and lone CR", func(t *testing.T) {
		src := "package chip\n\n" +
			"import (\n" +
			"\t_ \"golang.org/x/sys/unix\"\n" +
			"\t_ \"github.com/ebitengine/purego\"\n" +
			"\t_ \"plugin\"\n" +
			")\n\n" +
			"//go:linkname f runtime.f\n" +
			"//go:wasmexport g\n" +
			"//go:cgo_ldflag -lm\n" +
			"//go:debug x=1\n" +
			"// lone CR\r here\n" +
			"// line sep \u2028 and para sep \u2029\n"
		res := checkComponentRules([]componentFile{
			memFile("chip.go", []byte(src)),
		}, componentCheckOptions{ModulePath: "example.com/kit", ComponentDir: "ui/chip"})
		if len(res.Errors) != 0 {
			t.Fatalf("unexpected errors: %+v", res.Errors)
		}
		var lines []string
		for _, f := range res.Flags {
			lines = append(lines, formatComponentFinding("", f))
		}
		joined := strings.Join(lines, "\n")
		for _, want := range []string{
			`chip.go:4: imports "golang.org/x/sys/unix"`,
			`chip.go:5: imports "github.com/ebitengine/purego"`,
			`chip.go:6: imports "plugin"`,
			`chip.go:9: uses directive //go:linkname`,
			`chip.go:10: uses directive //go:wasmexport`,
			`chip.go:11: uses directive //go:cgo_ldflag`,
			`chip.go:12: uses directive //go:debug`,
			`U+000D`,
			`U+2028`,
			`U+2029`,
		} {
			if !strings.Contains(joined, want) {
				t.Errorf("missing expected flag %q in:\n%s", want, joined)
			}
		}
	})
}

func TestCheckComponentNotesNonComponentModuleImports(t *testing.T) {
	if testing.Short() {
		t.Skip("runs real go vet")
	}
	p := componentModuleFixture(t, map[string]string{
		"nsgo-module.yaml": `version: 1
components:
  chip: ui/chip
  badge: ui/badge
`,
		"theme/theme.go":             "package theme\nconst Primary = 1\n",
		"ui/badge/badge.go":          "package badge\nconst Kind = 1\n",
		"ui/chip/locales/locales.go": "package locales\nconst Title = \"Chip\"\n",
		"ui/chip/chip.go": `package chip

import (
	"example.com/kit/theme"
	"example.com/kit/ui/badge"
	"example.com/kit/ui/chip/locales"
)

var Summary = theme.Primary + badge.Kind + len(locales.Title)
`,
	})
	var err error
	out := captureStderr(t, func() { err = check(t.Context(), p, "", 0) })
	if err != nil {
		t.Fatalf("check failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "example.com/kit/theme") || !strings.Contains(out, "will require example.com/kit") {
		t.Errorf("expected note about non-component module import example.com/kit/theme:\n%s", out)
	}
	if strings.Contains(out, "imports example.com/kit/ui/badge") {
		t.Errorf("should not note another declared component (ui/badge):\n%s", out)
	}
	if strings.Contains(out, "imports example.com/kit/ui/chip/locales") {
		t.Errorf("should not note component's own locales package:\n%s", out)
	}
}

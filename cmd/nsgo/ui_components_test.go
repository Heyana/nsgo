package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	modzip "golang.org/x/mod/zip"
)

type testModuleManifest struct {
	Version    int               `yaml:"version"`
	Import     []string          `yaml:"import"`
	Targets    []string          `yaml:"targets"`
	Notes      string            `yaml:"notes"`
	Components map[string]string `yaml:"components"`
}

type testCopyRecord struct {
	Version       int      `yaml:"version"`
	Module        string   `yaml:"module"`
	ModuleVersion string   `yaml:"module_version"`
	Component     string   `yaml:"component"`
	Tests         bool     `yaml:"tests"`
	Files         []string `yaml:"files"`
	Generated     []string `yaml:"generated,omitempty"`
}

// expectedUIComponents are the eight ui subpackages declared in nsgo-module.yaml and listed in cmd/nsgo/registry.yaml (#2471).
var expectedUIComponents = map[string]string{
	"chat":         "ui/chat",
	"code-editor":  "ui/editor",
	"commit-graph": "ui/commitgraph",
	"diff-view":    "ui/diff",
	"file-picker":  "ui/filepicker",
	"flow-diagram": "ui/flow",
	"rich-editor":  "ui/richeditor",
	"terminal":     "ui/terminal",
}

func nsgoRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("locate repo root from %s: %v", wd, err)
	}
	return root
}

func readTestModuleManifest(t *testing.T, data []byte) testModuleManifest {
	t.Helper()
	var m testModuleManifest
	if err := decodeStrictYAML(data, &m); err != nil {
		t.Fatalf("parse nsgo-module.yaml: %v", err)
	}
	if m.Version != 1 {
		t.Fatalf("nsgo-module.yaml version = %d, want 1", m.Version)
	}
	return m
}

// TestRegistryListsUISubpackageComponents checks that root nsgo-module.yaml declares all eight ui subpackages, cmd/nsgo/registry.yaml lists them in canonical form, and nsgo add -list chat shows chat as a component (#2471).
func TestRegistryListsUISubpackageComponents(t *testing.T) {
	root := nsgoRepoRoot(t)
	rawManifest, err := os.ReadFile(filepath.Join(root, "nsgo-module.yaml"))
	if err != nil {
		t.Fatalf("read nsgo-module.yaml: %v", err)
	}
	manifest := readTestModuleManifest(t, rawManifest)
	if !maps.Equal(manifest.Components, expectedUIComponents) {
		t.Fatalf("nsgo-module.yaml components = %v, want %v", manifest.Components, expectedUIComponents)
	}

	idx := builtinRegistry()
	canonical, err := encodeRegistryIndex(idx)
	if err != nil {
		t.Fatalf("encodeRegistryIndex: %v", err)
	}
	if !bytes.Equal(builtinRegistryYAML, canonical) {
		t.Fatalf("cmd/nsgo/registry.yaml is not in canonical form;\ngot:\n%s\nwant:\n%s", builtinRegistryYAML, canonical)
	}

	byName := make(map[string]registryItem, len(idx.Items))
	for _, item := range idx.Items {
		byName[item.Name] = item
	}
	for _, name := range slices.Sorted(maps.Keys(expectedUIComponents)) {
		item, ok := byName[name]
		if !ok {
			t.Errorf("registry.yaml missing component %q", name)
			continue
		}
		if item.Module != nsgoModule {
			t.Errorf("registry item %q module = %q, want %q", name, item.Module, nsgoModule)
		}
		if item.Component != name {
			t.Errorf("registry item %q component = %q, want %q", name, item.Component, name)
		}
		if item.License != "Apache-2.0" {
			t.Errorf("registry item %q license = %q, want Apache-2.0", name, item.License)
		}
		if item.kind() != "component" {
			t.Errorf("registry item %q kind = %q, want component", name, item.kind())
		}
	}

	// nsgo add -list chat must work outside a module and show chat as a component.
	outside := t.TempDir()
	out, err := runAdd(t, outside, addOptions{list: true}, nil, "chat")
	if err != nil {
		t.Fatalf("nsgo add -list chat: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Fatalf("nsgo add -list chat printed %d lines:\n%s", len(lines), out)
	}
	fields := strings.Fields(lines[0])
	if len(fields) < 4 || fields[0] != "chat" || fields[1] != "component" || fields[2] != nsgoModule {
		t.Fatalf("nsgo add -list chat = %q, want chat component %s ...", lines[0], nsgoModule)
	}
}

// TestUISubpackageComponentsCopyIntoScratchModule serves this checkout as a release through a file:// proxy (golang.org/x/mod/zip.CreateFromDir), verifies copied-component rules for all eight declared components, copies each into a scratch module requiring that release through a replace of the checkout, vets each copy for the host, verifies chat's 16 files + LICENSE + NOTICE + nsgo-copy.yaml, and runs the copied chat tests with nsgo test (#2471).
func TestUISubpackageComponentsCopyIntoScratchModule(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}

	root := nsgoRepoRoot(t)
	const releaseVersion = "v0.99.0"
	modVer := module.Version{Path: nsgoModule, Version: releaseVersion}

	// Serve this checkout as a release through a file:// module proxy using x/mod/zip.CreateFromDir.
	proxyDir := t.TempDir()
	verDir := filepath.Join(proxyDir, filepath.FromSlash(nsgoModule), "@v")
	if err := os.MkdirAll(verDir, 0o755); err != nil {
		t.Fatal(err)
	}
	goModBytes, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	goSumBytes, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	zipPath := filepath.Join(verDir, releaseVersion+".zip")
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := modzip.CreateFromDir(zf, modVer, root); err != nil {
		zf.Close()
		t.Fatalf("zip.CreateFromDir(%s): %v", root, err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := modzip.CheckZip(modVer, zipPath); err != nil {
		t.Fatalf("zip.CheckZip: %v", err)
	}
	wantHash, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	if err != nil {
		t.Fatalf("dirhash.HashZip: %v", err)
	}
	writeFiles(t, verDir, map[string]string{
		releaseVersion + ".info": `{"Version":"` + releaseVersion + `","Time":"2026-11-27T00:00:00Z"}`,
		releaseVersion + ".mod":  string(goModBytes),
		"list":                   releaseVersion + "\n",
	})

	// Download the release offline through the file:// proxy into an isolated cache and verify its hash.
	dlCache := t.TempDir()
	dlDir := t.TempDir()
	dlCmd := exec.Command("go", "mod", "download", "-json", nsgoModule+"@"+releaseVersion)
	dlCmd.Dir = dlDir
	dlCmd.Env = append(os.Environ(),
		"GOPROXY=file://"+filepath.ToSlash(proxyDir),
		"GOSUMDB=off",
		"GOMODCACHE="+dlCache,
		"GOWORK=off",
		"GOTOOLCHAIN=local",
		"GOFLAGS=-modcacherw",
	)
	dlOut, err := dlCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod download from file:// proxy: %v\n%s", err, dlOut)
	}
	var downloaded struct {
		Zip string
		Sum string
	}
	if err := json.Unmarshal(dlOut, &downloaded); err != nil {
		t.Fatalf("parse go mod download json: %v\n%s", err, dlOut)
	}
	if downloaded.Sum != wantHash {
		t.Fatalf("downloaded zip sum = %q, want %q", downloaded.Sum, wantHash)
	}
	gotHash, err := dirhash.HashZip(downloaded.Zip, dirhash.Hash1)
	if err != nil || gotHash != wantHash {
		t.Fatalf("downloaded zip hash = %q (%v), want %q", gotHash, err, wantHash)
	}

	// Open the verified release zip and read nsgo-module.yaml and root license/notice files.
	zr, err := zip.OpenReader(downloaded.Zip)
	if err != nil {
		t.Fatalf("open release zip: %v", err)
	}
	defer zr.Close()

	zipPrefix := nsgoModule + "@" + releaseVersion + "/"
	zipFiles := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		rel, ok := strings.CutPrefix(f.Name, zipPrefix)
		if !ok {
			t.Fatalf("unexpected zip entry %q outside %q", f.Name, zipPrefix)
		}
		zipFiles[rel] = f
	}

	manifestData := readZipEntry(t, zipFiles, "nsgo-module.yaml")
	manifest := readTestModuleManifest(t, manifestData)
	if !maps.Equal(manifest.Components, expectedUIComponents) {
		t.Fatalf("release zip nsgo-module.yaml components = %v, want %v", manifest.Components, expectedUIComponents)
	}
	licenseData := readZipEntry(t, zipFiles, "LICENSE")
	noticeData := readZipEntry(t, zipFiles, "NOTICE")

	// Create the scratch module requiring the release through a replace of the checkout,
	// carrying the repository's require/replace entries and go.sum so vetting and testing
	// run completely offline under GOPROXY=off without invoking go mod tidy (which would
	// inspect transitive test-only dependencies absent from an offline module cache).
	const scratchMod = "example.com/scratch"
	scratchDir := t.TempDir()
	scratchGoMod := buildScratchGoMod(t, scratchMod, releaseVersion, root, goModBytes)
	writeFiles(t, scratchDir, map[string]string{
		"go.mod": scratchGoMod,
		"go.sum": string(goSumBytes),
	})

	// Check copy rules and copy every declared component from the release zip into the scratch module.
	for _, compName := range slices.Sorted(maps.Keys(manifest.Components)) {
		compDir := manifest.Components[compName]
		compFiles := collectZipComponentFiles(t, zipFiles, compDir)
		if len(compFiles) == 0 {
			t.Fatalf("component %q (%s) has no files in release zip", compName, compDir)
		}
		verifyCopiedComponentRules(t, compName, compDir, compFiles)

		pkgName := path.Base(compDir)
		dstRel := path.Join("components", pkgName)
		dstDir := filepath.Join(scratchDir, filepath.FromSlash(dstRel))
		oldImport := nsgoModule + "/" + compDir
		newImport := scratchMod + "/" + dstRel

		var recordFiles, recordGenerated []string
		hasLocalesYAML := false
		for _, rel := range slices.Sorted(maps.Keys(compFiles)) {
			data := compFiles[rel]
			if rel == "locales/catalog_gen.go" {
				continue // regenerated below under the copy's import path
			}
			if strings.HasPrefix(rel, "locales/") && strings.HasSuffix(rel, ".yaml") {
				hasLocalesYAML = true
			}
			if strings.HasSuffix(rel, ".go") {
				data = rewriteCopiedGoFile(t, rel, data, oldImport, newImport)
			}
			outPath := filepath.Join(dstDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(outPath, data, 0o644); err != nil {
				t.Fatal(err)
			}
			recordFiles = append(recordFiles, rel)
		}

		// Copy root LICENSE and NOTICE beside the component.
		if err := os.WriteFile(filepath.Join(dstDir, "LICENSE"), licenseData, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dstDir, "NOTICE"), noticeData, 0o644); err != nil {
			t.Fatal(err)
		}
		recordFiles = append(recordFiles, "LICENSE", "NOTICE")
		slices.Sort(recordFiles)

		// Regenerate locales/catalog_gen.go when the component has locales/*.yaml.
		if hasLocalesYAML {
			if err := generateCatalog(t.Context(), filepath.Join(dstDir, "locales"), ""); err != nil {
				t.Fatalf("generateCatalog for copied %s: %v", compName, err)
			}
			recordGenerated = append(recordGenerated, "locales/catalog_gen.go")
		}

		// Write deterministic nsgo-copy.yaml matching version 1 schema.
		recBytes := formatTestCopyRecord(testCopyRecord{
			Version:       1,
			Module:        nsgoModule,
			ModuleVersion: releaseVersion,
			Component:     compName,
			Tests:         true,
			Files:         recordFiles,
			Generated:     recordGenerated,
		})
		if err := os.WriteFile(filepath.Join(dstDir, "nsgo-copy.yaml"), recBytes, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Vet every one of the eight copied components for the host completely offline.
	for _, compName := range slices.Sorted(maps.Keys(manifest.Components)) {
		pkgName := path.Base(manifest.Components[compName])
		vetCmd := exec.Command("go", "vet", "-mod=readonly", "./components/"+pkgName+"/...")
		vetCmd.Dir = scratchDir
		vetCmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off")
		if out, err := vetCmd.CombinedOutput(); err != nil {
			t.Fatalf("go vet copied component %q (./components/%s/...): %v\n%s", compName, pkgName, err, out)
		}
	}

	// Verify chat's copy holds its 16 files, LICENSE, NOTICE and nsgo-copy.yaml.
	chatDir := filepath.Join(scratchDir, "components", "chat")
	gotChatFiles := listDirFiles(t, chatDir)
	wantChatFiles := []string{
		"LICENSE",
		"NOTICE",
		"actions.go",
		"actions_test.go",
		"chat.go",
		"chat_test.go",
		"composer.go",
		"controls.go",
		"conversation.go",
		"effort.go",
		"find.go",
		"locales/catalog_gen.go",
		"locales/en.yaml",
		"locales/locales.go",
		"locales/zh-Hans.yaml",
		"native_test.go",
		"nsgo-copy.yaml",
		"reasoning.go",
		"reasoning_test.go",
	}
	if !slices.Equal(gotChatFiles, wantChatFiles) {
		t.Fatalf("copied chat files =\n%v\nwant:\n%v", gotChatFiles, wantChatFiles)
	}
	var chatRec testCopyRecord
	recData, err := os.ReadFile(filepath.Join(chatDir, "nsgo-copy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := decodeStrictYAML(recData, &chatRec); err != nil {
		t.Fatalf("parse chat nsgo-copy.yaml: %v", err)
	}
	if chatRec.Component != "chat" || chatRec.Module != nsgoModule || chatRec.ModuleVersion != releaseVersion || !chatRec.Tests {
		t.Fatalf("unexpected chat nsgo-copy.yaml metadata: %+v", chatRec)
	}
	if len(chatRec.Files) != 17 || !slices.Equal(chatRec.Generated, []string{"locales/catalog_gen.go"}) {
		t.Fatalf("chat nsgo-copy.yaml files=%d generated=%v, want 17 files and [locales/catalog_gen.go]", len(chatRec.Files), chatRec.Generated)
	}

	// Run the copied chat tests with nsgo test.
	t.Chdir(scratchDir)
	p, err := loadForTest("", nil)
	if err != nil {
		t.Fatalf("loadForTest(scratch): %v", err)
	}
	var testErr error
	out := captureStderr(t, func() {
		testErr = runTests(t.Context(), p, testOptions{
			patterns: []string{"./components/chat"},
			env:      []string{"GOWORK=off", "GOTOOLCHAIN=local", "GOPROXY=off"},
		})
	})
	if testErr != nil {
		t.Fatalf("nsgo test ./components/chat failed: %v\n%s", testErr, out)
	}
}

func buildScratchGoMod(t *testing.T, scratchMod, releaseVersion, repoRoot string, repoGoMod []byte) string {
	t.Helper()
	mf, err := modfile.Parse("go.mod", repoGoMod, nil)
	if err != nil {
		t.Fatalf("parse repo go.mod: %v", err)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\n", scratchMod)
	if mf.Go != nil && mf.Go.Version != "" {
		fmt.Fprintf(&b, "go %s\n\n", mf.Go.Version)
	} else {
		b.WriteString("go 1.27.3\n\n")
	}
	b.WriteString("require (\n")
	fmt.Fprintf(&b, "\t%s %s\n", nsgoModule, releaseVersion)
	for _, req := range mf.Require {
		fmt.Fprintf(&b, "\t%s %s // indirect\n", req.Mod.Path, req.Mod.Version)
	}
	b.WriteString(")\n\n")
	fmt.Fprintf(&b, "replace %s => %s\n", nsgoModule, filepath.ToSlash(repoRoot))
	for _, rep := range mf.Replace {
		if rep.Old.Version != "" {
			fmt.Fprintf(&b, "replace %s %s => %s %s\n", rep.Old.Path, rep.Old.Version, rep.New.Path, rep.New.Version)
		} else if rep.New.Version != "" {
			fmt.Fprintf(&b, "replace %s => %s %s\n", rep.Old.Path, rep.New.Path, rep.New.Version)
		} else {
			fmt.Fprintf(&b, "replace %s => %s\n", rep.Old.Path, rep.New.Path)
		}
	}
	return b.String()
}

func readZipEntry(t *testing.T, files map[string]*zip.File, name string) []byte {
	t.Helper()
	f, ok := files[name]
	if !ok {
		t.Fatalf("release zip missing %q", name)
	}
	rc, err := f.Open()
	if err != nil {
		t.Fatalf("open zip entry %q: %v", name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read zip entry %q: %v", name, err)
	}
	return data
}

func collectZipComponentFiles(t *testing.T, files map[string]*zip.File, compDir string) map[string][]byte {
	t.Helper()
	prefix := compDir + "/"
	out := map[string][]byte{}
	for name, f := range files {
		rel, ok := strings.CutPrefix(name, prefix)
		if !ok || f.FileInfo().IsDir() {
			continue
		}
		out[rel] = readZipEntry(t, files, name)
	}
	return out
}

// verifyCopiedComponentRules enforces the copied-component rules (#2466, #2467) on a component's files from the release zip.
func verifyCopiedComponentRules(t *testing.T, compName, compDir string, files map[string][]byte) {
	t.Helper()
	const (
		maxFiles     = 256
		maxFileBytes = 1 << 20
		maxTotalSize = 4 << 20
	)
	if len(files) > maxFiles {
		t.Errorf("%s: %d files exceeds limit %d", compName, len(files), maxFiles)
	}
	wantPkg := path.Base(compDir)
	seenLower := map[string]string{}
	totalSize := 0
	fset := token.NewFileSet()

	for _, rel := range slices.Sorted(maps.Keys(files)) {
		data := files[rel]
		totalSize += len(data)
		if len(data) > maxFileBytes {
			t.Errorf("%s/%s: size %d exceeds %d", compDir, rel, len(data), maxFileBytes)
		}
		low := strings.ToLower(rel)
		if prev, dup := seenLower[low]; dup {
			t.Errorf("%s: case-insensitive path collision %q vs %q", compDir, rel, prev)
		}
		seenLower[low] = rel

		for _, seg := range strings.Split(rel, "/") {
			if seg == "" || seg[0] == '.' || seg[0] == '_' {
				t.Errorf("%s/%s: invalid path segment %q", compDir, rel, seg)
			}
			for i := range len(seg) {
				c := seg[i]
				if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.') {
					t.Errorf("%s/%s: disallowed character %q in segment %q", compDir, rel, c, seg)
				}
			}
		}

		// File allowlist check.
		dir := path.Dir(rel)
		ext := strings.ToLower(path.Ext(rel))
		switch {
		case ext == ".go" && (dir == "." || dir == "locales"):
		case (ext == ".yaml" || ext == ".yml") && dir == "locales":
		default:
			t.Errorf("%s/%s: unexpected file type/location for copied component", compDir, rel)
		}

		// UTF-8 without BOM and no disallowed control/format characters.
		if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
			t.Errorf("%s/%s: UTF-8 BOM is not allowed", compDir, rel)
		}
		if !utf8.Valid(data) {
			t.Errorf("%s/%s: invalid UTF-8", compDir, rel)
		}
		for _, r := range string(data) {
			if r == '\t' || r == '\n' || r == '\r' {
				continue
			}
			if unicode.IsControl(r) || unicode.In(r, unicode.Cf) || r == '\u2028' || r == '\u2029' {
				t.Errorf("%s/%s: disallowed control or format character U+%04X", compDir, rel, r)
			}
		}

		if ext != ".go" {
			continue
		}
		f, err := parser.ParseFile(fset, rel, data, parser.ParseComments)
		if err != nil {
			t.Errorf("%s/%s: parse error: %v", compDir, rel, err)
			continue
		}
		expectedPkg := wantPkg
		if dir == "locales" {
			expectedPkg = "locales"
		}
		isTest := strings.HasSuffix(rel, "_test.go")
		if f.Name.Name != expectedPkg && !(isTest && f.Name.Name == expectedPkg+"_test") {
			t.Errorf("%s/%s: package %q, want %q", compDir, rel, f.Name.Name, expectedPkg)
		}
		for _, spec := range f.Imports {
			imp, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Errorf("%s/%s: unquote import: %v", compDir, rel, err)
				continue
			}
			if imp == "C" {
				t.Errorf("%s/%s: import \"C\" is forbidden", compDir, rel)
			}
			if imp == "internal" || strings.HasPrefix(imp, "internal/") || strings.HasSuffix(imp, "/internal") || strings.Contains(imp, "/internal/") {
				t.Errorf("%s/%s: internal import %q is forbidden", compDir, rel, imp)
			}
			if !isTest && (imp == nsgoModule+"/tsx" || strings.HasPrefix(imp, nsgoModule+"/tsx/")) {
				t.Errorf("%s/%s: tsx import %q outside tests is forbidden", compDir, rel, imp)
			}
			switch {
			case imp == "unsafe", imp == "syscall", imp == "os/exec", imp == "plugin",
				imp == "golang.org/x/sys", strings.HasPrefix(imp, "golang.org/x/sys/"),
				imp == "github.com/ebitengine/purego", strings.HasPrefix(imp, "github.com/ebitengine/purego/"):
				t.Errorf("%s/%s: flagged import %q", compDir, rel, imp)
			}
		}
		for _, cg := range f.Comments {
			for _, c := range cg.List {
				text := c.Text
				if strings.HasPrefix(text, "//go:embed") {
					t.Errorf("%s/%s: unexpected //go:embed directive: %s", compDir, rel, text)
				}
				if strings.HasPrefix(text, "//go:build ignore") || strings.HasPrefix(text, "// +build ignore") {
					t.Errorf("%s/%s: //go:build ignore is forbidden", compDir, rel)
				}
				if strings.HasPrefix(text, "//go:generate") {
					if !(dir == "locales" && strings.TrimSpace(text) == "//go:generate go tool nsgo i18n") {
						t.Errorf("%s/%s: disallowed //go:generate directive: %s", compDir, rel, text)
					}
				}
				for _, bad := range []string{"//go:linkname", "//go:wasmimport", "//go:wasmexport", "//go:cgo_", "//go:debug"} {
					if strings.HasPrefix(text, bad) {
						t.Errorf("%s/%s: flagged directive %s", compDir, rel, bad)
					}
				}
			}
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				for _, r := range id.Name {
					if r > unicode.MaxASCII {
						t.Errorf("%s/%s: non-ASCII identifier %q", compDir, rel, id.Name)
						break
					}
				}
			}
			return true
		})
	}
	if totalSize > maxTotalSize {
		t.Errorf("%s: total size %d exceeds %d", compName, totalSize, maxTotalSize)
	}
}

func rewriteCopiedGoFile(t *testing.T, filename string, src []byte, oldImport, newImport string) []byte {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	rewritePath := func(p string) (string, bool) {
		if p == oldImport {
			return newImport, true
		}
		if rest, ok := strings.CutPrefix(p, oldImport+"/"); ok {
			return newImport + "/" + rest, true
		}
		return p, false
	}
	for _, spec := range f.Imports {
		imp, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		if next, ok := rewritePath(imp); ok {
			spec.Path.Value = strconv.Quote(next)
		}
	}
	for _, cg := range f.Comments {
		for _, c := range cg.List {
			if strings.Contains(c.Text, oldImport) {
				c.Text = strings.ReplaceAll(c.Text, oldImport, newImport)
			}
		}
	}
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, f); err != nil {
		t.Fatalf("format %s: %v", filename, err)
	}
	return buf.Bytes()
}

func formatTestCopyRecord(r testCopyRecord) []byte {
	var b bytes.Buffer
	b.WriteString("# Written by nsgo add. nsgo diff compares this directory with the copy of this version.\n")
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	_ = enc.Encode(r)
	_ = enc.Close()
	return b.Bytes()
}

func listDirFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

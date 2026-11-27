package main

import (
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	modzip "golang.org/x/mod/zip"
)

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

// TestRegistryListsUISubpackageComponents checks that root nsgo-module.yaml declares all eight ui subpackages, passes nsgo check's component rules, matches cmd/nsgo/registry.yaml in canonical form, and nsgo add -list chat shows chat as a component (#2471).
func TestRegistryListsUISubpackageComponents(t *testing.T) {
	root := checkoutRoot(t)
	manifest, err := readModuleManifest(root, nsgoModule, "")
	if err != nil {
		t.Fatalf("readModuleManifest: %v", err)
	}
	if manifest == nil || !maps.Equal(manifest.Components, expectedUIComponents) {
		t.Fatalf("nsgo-module.yaml components = %v, want %v", manifest, expectedUIComponents)
	}
	captureStderr(t, func() {
		err = checkModuleComponents(t.Context(), &project{Dir: root})
	})
	if err != nil {
		t.Fatalf("checkModuleComponents(repo root): %v", err)
	}
	if err := checkRegistryPolicy(builtinRegistryYAML); err != nil {
		t.Fatalf("checkRegistryPolicy: %v", err)
	}

	idx := builtinRegistry()
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

// TestUISubpackageComponentsCopyIntoScratchModule serves this checkout as a release through a file:// proxy (golang.org/x/mod/zip.CreateFromDir), copies all eight declared ui components into a scratch module with nsgo add -y (which enforces copy rules, rewrites imports, generates chat's catalogs, writes nsgo-copy.yaml, and vets each copy for the host), verifies chat's 16 files + LICENSE + NOTICE + nsgo-copy.yaml, and runs the copied chat tests with nsgo test (#2471).
func TestUISubpackageComponentsCopyIntoScratchModule(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}

	root := checkoutRoot(t)
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
	zipHash, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	if err != nil {
		t.Fatalf("dirhash.HashZip: %v", err)
	}
	modHash, err := dirhash.Hash1([]string{"go.mod"}, func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(string(goModBytes))), nil
	})
	if err != nil {
		t.Fatalf("dirhash.Hash1(go.mod): %v", err)
	}
	writeFiles(t, verDir, map[string]string{
		releaseVersion + ".info": `{"Version":"` + releaseVersion + `","Time":"2026-11-27T00:00:00Z"}`,
		releaseVersion + ".mod":  string(goModBytes),
		"list":                   releaseVersion + "\n",
	})

	// Keep the run 100% offline: serve this checkout from proxyDir and NSGo's
	// already-cached transitive dependencies from the host GOMODCACHE download dir.
	hostCacheOut, _ := exec.Command("go", "env", "GOMODCACHE").Output()
	hostDownload := filepath.Join(strings.TrimSpace(string(hostCacheOut)), "cache", "download")
	offlineAdd(t, proxyDir)
	if info, err := os.Stat(hostDownload); err == nil && info.IsDir() {
		t.Setenv("GOPROXY", "file://"+filepath.ToSlash(proxyDir)+",file://"+filepath.ToSlash(hostDownload))
	}

	// Build scratch module go.mod/go.sum directly without running go mod tidy
	// (which would inspect transitive test-only dependencies absent offline).
	const scratchMod = "example.com/scratch"
	scratchDir := t.TempDir()
	scratchGoMod := buildScratchGoMod(t, scratchMod, releaseVersion, goModBytes)
	scratchGoSum := strings.TrimRight(string(goSumBytes), "\n") + "\n" +
		fmt.Sprintf("%s %s %s\n", nsgoModule, releaseVersion, zipHash) +
		fmt.Sprintf("%s %s/go.mod %s\n", nsgoModule, releaseVersion, modHash)
	writeFiles(t, scratchDir, map[string]string{
		"go.mod": scratchGoMod,
		"go.sum": scratchGoSum,
	})

	// Copy all eight components through nsgo add -y using their built-in registry names.
	compNames := slices.Sorted(maps.Keys(expectedUIComponents))
	plan, err := runAdd(t, scratchDir, addOptions{yes: true}, nil, compNames...)
	if err != nil {
		t.Fatalf("nsgo add -y %s failed: %v\n%s", strings.Join(compNames, " "), err, plan)
	}

	// Verify every copied component has LICENSE, NOTICE and a valid nsgo-copy.yaml.
	for _, compName := range compNames {
		pkgName := path.Base(expectedUIComponents[compName])
		dstDir := filepath.Join(scratchDir, "components", pkgName)
		recData, err := os.ReadFile(filepath.Join(dstDir, copyRecordName))
		if err != nil {
			t.Fatalf("read %s for %s: %v", copyRecordName, compName, err)
		}
		rec, err := parseCopyRecord(recData)
		if err != nil {
			t.Fatalf("parse %s for %s: %v", copyRecordName, compName, err)
		}
		if rec.Component != compName || rec.Module != nsgoModule || rec.ModuleVersion != releaseVersion || !rec.Tests {
			t.Fatalf("unexpected %s metadata for %s: %+v", copyRecordName, compName, rec)
		}
		for _, reqFile := range []string{"LICENSE", "NOTICE"} {
			if _, err := os.Stat(filepath.Join(dstDir, reqFile)); err != nil {
				t.Errorf("copied %s missing %s: %v", compName, reqFile, err)
			}
		}
	}

	// Verify chat's copy holds its exact 16 files, LICENSE, NOTICE and nsgo-copy.yaml.
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
	chatRecData, err := os.ReadFile(filepath.Join(chatDir, copyRecordName))
	if err != nil {
		t.Fatal(err)
	}
	chatRec, err := parseCopyRecord(chatRecData)
	if err != nil {
		t.Fatalf("parse chat %s: %v", copyRecordName, err)
	}
	if len(chatRec.Files) != 17 || !slices.Equal(chatRec.Generated, []string{"locales/catalog_gen.go"}) {
		t.Fatalf("chat %s files=%d generated=%v, want 17 files and [locales/catalog_gen.go]", copyRecordName, len(chatRec.Files), chatRec.Generated)
	}

	// Run the copied chat tests with nsgo test completely offline.
	t.Chdir(scratchDir)
	p, err := loadForTest("", nil)
	if err != nil {
		t.Fatalf("loadForTest(scratch): %v", err)
	}
	var testErr error
	out := captureStderr(t, func() {
		testErr = runTests(t.Context(), p, testOptions{
			patterns: []string{"./components/chat"},
			env:      []string{"GOWORK=off", "GOTOOLCHAIN=local"},
		})
	})
	if testErr != nil {
		t.Fatalf("nsgo test ./components/chat failed: %v\n%s", testErr, out)
	}
}

func buildScratchGoMod(t *testing.T, scratchMod, releaseVersion string, repoGoMod []byte) string {
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
	b.WriteString(")\n")
	for _, rep := range mf.Replace {
		if rep.Old.Version != "" {
			fmt.Fprintf(&b, "\nreplace %s %s => %s %s\n", rep.Old.Path, rep.Old.Version, rep.New.Path, rep.New.Version)
		} else if rep.New.Version != "" {
			fmt.Fprintf(&b, "\nreplace %s => %s %s\n", rep.Old.Path, rep.New.Path, rep.New.Version)
		} else {
			fmt.Fprintf(&b, "\nreplace %s => %s\n", rep.Old.Path, rep.New.Path)
		}
	}
	return b.String()
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

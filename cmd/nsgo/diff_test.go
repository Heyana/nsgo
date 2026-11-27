package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"
)

// addDiffProxyModule builds a valid module zip via x/mod/zip.CreateFromDir and
// serves it from proxyDir as a file:// module proxy.
func addDiffProxyModule(t *testing.T, proxyDir, modPath, version, gomod string, files map[string]string) {
	t.Helper()
	srcDir := t.TempDir()
	all := map[string]string{"go.mod": gomod}
	for k, v := range files {
		all[k] = v
	}
	writeFiles(t, srcDir, all)

	escPath, err := module.EscapePath(modPath)
	if err != nil {
		t.Fatal(err)
	}
	escVer, err := module.EscapeVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	at := filepath.Join(proxyDir, filepath.FromSlash(escPath), "@v")
	if err := os.MkdirAll(at, 0o755); err != nil {
		t.Fatal(err)
	}
	info := `{"Version":"` + version + `","Time":"2026-01-01T00:00:00Z"}`
	writeFiles(t, at, map[string]string{
		escVer + ".info": info,
		escVer + ".mod":  gomod,
	})
	listFile, err := os.OpenFile(filepath.Join(at, "list"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := listFile.WriteString(version + "\n"); err != nil {
		listFile.Close()
		t.Fatal(err)
	}
	if err := listFile.Close(); err != nil {
		t.Fatal(err)
	}

	zf, err := os.Create(filepath.Join(at, escVer+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := modzip.CreateFromDir(zf, module.Version{Path: modPath, Version: version}, srcDir); err != nil {
		zf.Close()
		t.Fatal(err)
	}
	if err := zf.Close(); err != nil {
		t.Fatal(err)
	}
}

// runDiff runs diffCommand in dir and returns stdout, stderr, changed, err.
func runDiff(t *testing.T, dir string, o diffOptions, arg string) (stdout, stderr string, changed bool, err error) {
	t.Helper()
	t.Chdir(dir)
	var outBuf, errBuf bytes.Buffer
	changed, err = diffCommand(t.Context(), o, arg, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), changed, err
}

// kitFixtureProxy creates an offline file:// proxy containing example.com/kit
// at v1.0.0 and v1.1.0 with a component "field" (including locales, tests,
// ancestor LICENSE, and a PNG asset) and a sibling component "badge".
func kitFixtureProxy(t *testing.T) string {
	t.Helper()
	proxy := t.TempDir()
	gomod := "module example.com/kit\n\ngo 1.26\n"
	manifest := "version: 1\ncomponents:\n  badge: ui/badge\n  field: ui/field\n"
	license := "Apache License\nVersion 2.0\n"

	pngV1 := "\x89PNG\r\n\x1a\nv1-pixel-bytes"
	pngV2 := "\x89PNG\r\n\x1a\nv2-pixel-bytes-updated"

	addDiffProxyModule(t, proxy, "example.com/kit", "v1.0.0", gomod, map[string]string{
		"nsgo-module.yaml":                manifest,
		"LICENSE":                         license,
		"ui/badge/badge.go":               "package badge\n\nfunc Label() string { return \"badge\" }\n",
		"ui/field/field.go":               "package field\n\nimport (\n\t\"example.com/kit/ui/badge\"\n\t\"example.com/kit/ui/field/locales\"\n)\n\n// Field uses example.com/kit/ui/field/locales for messages.\nfunc Title() string {\n\treturn badge.Label() + \": \" + locales.Title()\n}\n",
		"ui/field/field_test.go":          "package field\n\nimport \"testing\"\n\nfunc TestTitle(t *testing.T) {\n\tif Title() == \"\" {\n\t\tt.Fatal(\"empty\")\n\t}\n}\n",
		"ui/field/helper.go":              "package field\n\nfunc Helper() string {\n\treturn \"v1\"\n}\n",
		"ui/field/locales/locales.go":     "// Package locales holds translations for example.com/kit/ui/field.\npackage locales\n\n//go:generate go tool nsgo i18n\n\nfunc Title() string { return \"Field\" }\n",
		"ui/field/locales/en.yaml":        "title: Field\n",
		"ui/field/locales/catalog_gen.go": genHeader + "\npackage locales\n\nconst Source = \"en\"\n",
		"ui/field/assets/icon.png":        pngV1,
	})

	addDiffProxyModule(t, proxy, "example.com/kit", "v1.1.0", gomod, map[string]string{
		"nsgo-module.yaml":                manifest,
		"LICENSE":                         license,
		"ui/badge/badge.go":               "package badge\n\nfunc Label() string { return \"badge-v2\" }\n",
		"ui/field/field.go":               "package field\n\nimport (\n\t\"example.com/kit/ui/badge\"\n\t\"example.com/kit/ui/field/locales\"\n)\n\n// Field uses example.com/kit/ui/field/locales for messages.\nfunc Title() string {\n\treturn badge.Label() + \" — \" + locales.Title()\n}\n",
		"ui/field/field_test.go":          "package field\n\nimport \"testing\"\n\nfunc TestTitle(t *testing.T) {\n\tif Title() == \"\" {\n\t\tt.Fatal(\"empty\")\n\t}\n}\n",
		"ui/field/locales/locales.go":     "// Package locales holds translations for example.com/kit/ui/field.\npackage locales\n\n//go:generate go tool nsgo i18n\n\nfunc Title() string { return \"Field\" }\n",
		"ui/field/locales/en.yaml":        "title: Field\n",
		"ui/field/locales/catalog_gen.go": genHeader + "\npackage locales\n\nconst Source = \"en\"\n",
		"ui/field/assets/icon.png":        pngV2,
		"ui/field/extra.go":               "package field\n\nfunc Extra() bool { return true }\n",
	})
	return proxy
}

// writeCleanFieldCopy writes an app module with clean copies of badge (at
// components/badge) and field (at components/field) matching #2467's exact
// output for example.com/kit v1.0.0.
func writeCleanFieldCopy(t *testing.T, badgeRelDir string) string {
	t.Helper()
	if badgeRelDir == "" {
		badgeRelDir = "components/badge"
	}
	dir := t.TempDir()
	badgeImp := "example.com/app/" + badgeRelDir

	badgeRec, err := formatCopyRecord(&copyRecord{
		Version:       1,
		Module:        "example.com/kit",
		ModuleVersion: "v1.0.0",
		Component:     "badge",
		Tests:         true,
		Files:         []string{"LICENSE", "badge.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fieldRec, err := formatCopyRecord(&copyRecord{
		Version:       1,
		Module:        "example.com/kit",
		ModuleVersion: "v1.0.0",
		Component:     "field",
		Tests:         true,
		Files: []string{
			"LICENSE",
			"assets/icon.png",
			"field.go",
			"field_test.go",
			"helper.go",
			"locales/en.yaml",
			"locales/locales.go",
		},
		Generated: []string{"locales/catalog_gen.go"},
	})
	if err != nil {
		t.Fatal(err)
	}

	writeFiles(t, dir, map[string]string{
		"go.mod":                        "module example.com/app\n\ngo 1.26\n",
		badgeRelDir + "/nsgo-copy.yaml": string(badgeRec),
		badgeRelDir + "/LICENSE":        "Apache License\nVersion 2.0\n",
		badgeRelDir + "/badge.go":       "package badge\n\nfunc Label() string { return \"badge\" }\n",

		"components/field/nsgo-copy.yaml":     string(fieldRec),
		"components/field/LICENSE":            "Apache License\nVersion 2.0\n",
		"components/field/field.go":           "package field\n\nimport (\n\t\"" + badgeImp + "\"\n\t\"example.com/app/components/field/locales\"\n)\n\n// Field uses example.com/app/components/field/locales for messages.\nfunc Title() string {\n\treturn badge.Label() + \": \" + locales.Title()\n}\n",
		"components/field/field_test.go":      "package field\n\nimport \"testing\"\n\nfunc TestTitle(t *testing.T) {\n\tif Title() == \"\" {\n\t\tt.Fatal(\"empty\")\n\t}\n}\n",
		"components/field/helper.go":          "package field\n\nfunc Helper() string {\n\treturn \"v1\"\n}\n",
		"components/field/locales/locales.go": "// Package locales holds translations for example.com/app/components/field.\npackage locales\n\n//go:generate go tool nsgo i18n\n\nfunc Title() string { return \"Field\" }\n",
		"components/field/locales/en.yaml":    "title: Field\n",
		// Intentionally different generated catalog to verify generated files are ignored.
		"components/field/locales/catalog_gen.go": genHeader + "\npackage locales\n\nconst Source = \"en\"\n// regenerated locally\n",
		"components/field/assets/icon.png":        "\x89PNG\r\n\x1a\nv1-pixel-bytes",
	})
	return dir
}

func TestDiffGoldenOutputs(t *testing.T) {
	proxy := kitFixtureProxy(t)
	offlineAdd(t, proxy)

	t.Run("unedited copy prints nothing", func(t *testing.T) {
		dir := writeCleanFieldCopy(t, "internal/ui/badge")
		out, errOut, changed, err := runDiff(t, dir, diffOptions{}, "field")
		if err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut)
		}
		if changed || out != "" || errOut != "" {
			t.Fatalf("got changed=%v out=%q err=%q; want empty", changed, out, errOut)
		}
	})

	t.Run("edited file golden output", func(t *testing.T) {
		dir := writeCleanFieldCopy(t, "components/badge")
		edited := "package field\n\nimport (\n\t\"example.com/app/components/badge\"\n\t\"example.com/app/components/field/locales\"\n)\n\n// Field uses example.com/app/components/field/locales for messages.\nfunc Title() string {\n\treturn badge.Label() + \"! \" + locales.Title()\n}\n"
		if err := os.WriteFile(filepath.Join(dir, "components/field/field.go"), []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}

		out, errOut, changed, err := runDiff(t, dir, diffOptions{}, "field")
		if err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut)
		}
		want := "" +
			"diff --git a/components/field/field.go b/components/field/field.go\n" +
			"--- a/components/field/field.go\n" +
			"+++ b/components/field/field.go\n" +
			"@@ -7,5 +7,5 @@\n" +
			" \n" +
			" // Field uses example.com/app/components/field/locales for messages.\n" +
			" func Title() string {\n" +
			"-\treturn badge.Label() + \": \" + locales.Title()\n" +
			"+\treturn badge.Label() + \"! \" + locales.Title()\n" +
			" }\n"
		if !changed || out != want {
			t.Fatalf("changed=%v\ngot:\n%s\nwant:\n%s", changed, out, want)
		}
	})

	t.Run("added, deleted and binary file golden output", func(t *testing.T) {
		dir := writeCleanFieldCopy(t, "components/badge")
		// Add a text file, delete helper.go, and modify binary icon.png.
		writeFiles(t, dir, map[string]string{
			"components/field/custom.go":       "package field\n\nfunc Custom() int { return 42 }\n",
			"components/field/assets/icon.png": "\x89PNG\r\n\x1a\nlocal-png",
		})
		if err := os.Remove(filepath.Join(dir, "components/field/helper.go")); err != nil {
			t.Fatal(err)
		}

		out, errOut, changed, err := runDiff(t, dir, diffOptions{}, "components/field")
		if err != nil {
			t.Fatalf("unexpected error: %v (stderr: %s)", err, errOut)
		}
		want := "" +
			"diff --git a/components/field/assets/icon.png b/components/field/assets/icon.png\n" +
			"Binary files a/components/field/assets/icon.png and b/components/field/assets/icon.png differ\n" +
			"diff --git a/components/field/custom.go b/components/field/custom.go\n" +
			"new file mode 100644\n" +
			"--- /dev/null\n" +
			"+++ b/components/field/custom.go\n" +
			"@@ -0,0 +1,3 @@\n" +
			"+package field\n" +
			"+\n" +
			"+func Custom() int { return 42 }\n" +
			"diff --git a/components/field/helper.go b/components/field/helper.go\n" +
			"deleted file mode 100644\n" +
			"--- a/components/field/helper.go\n" +
			"+++ /dev/null\n" +
			"@@ -1,5 +0,0 @@\n" +
			"-package field\n" +
			"-\n" +
			"-func Helper() string {\n" +
			"-\treturn \"v1\"\n" +
			"-}\n"
		if !changed || out != want {
			t.Fatalf("changed=%v\ngot:\n%s\nwant:\n%s", changed, out, want)
		}
	})

	t.Run("upgrade between two fixture versions golden output", func(t *testing.T) {
		dir := writeCleanFieldCopy(t, "components/badge")
		// Local edits do not interfere with comparing recorded v1.0.0 vs v1.1.0.
		if err := os.WriteFile(filepath.Join(dir, "components/field/custom.go"), []byte("package field\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		outV, errOut, changedV, err := runDiff(t, dir, diffOptions{}, "field@v1.1.0")
		if err != nil {
			t.Fatalf("field@v1.1.0 error: %v (stderr: %s)", err, errOut)
		}
		outU, errOutU, changedU, err := runDiff(t, dir, diffOptions{}, "field@upgrade")
		if err != nil {
			t.Fatalf("field@upgrade error: %v (stderr: %s)", err, errOutU)
		}
		want := "" +
			"diff --git a/components/field/assets/icon.png b/components/field/assets/icon.png\n" +
			"Binary files a/components/field/assets/icon.png and b/components/field/assets/icon.png differ\n" +
			"diff --git a/components/field/extra.go b/components/field/extra.go\n" +
			"new file mode 100644\n" +
			"--- /dev/null\n" +
			"+++ b/components/field/extra.go\n" +
			"@@ -0,0 +1,3 @@\n" +
			"+package field\n" +
			"+\n" +
			"+func Extra() bool { return true }\n" +
			"diff --git a/components/field/field.go b/components/field/field.go\n" +
			"--- a/components/field/field.go\n" +
			"+++ b/components/field/field.go\n" +
			"@@ -7,5 +7,5 @@\n" +
			" \n" +
			" // Field uses example.com/app/components/field/locales for messages.\n" +
			" func Title() string {\n" +
			"-\treturn badge.Label() + \": \" + locales.Title()\n" +
			"+\treturn badge.Label() + \" — \" + locales.Title()\n" +
			" }\n" +
			"diff --git a/components/field/helper.go b/components/field/helper.go\n" +
			"deleted file mode 100644\n" +
			"--- a/components/field/helper.go\n" +
			"+++ /dev/null\n" +
			"@@ -1,5 +0,0 @@\n" +
			"-package field\n" +
			"-\n" +
			"-func Helper() string {\n" +
			"-\treturn \"v1\"\n" +
			"-}\n"
		if !changedV || outV != want {
			t.Fatalf("v1.1.0 changed=%v\ngot:\n%s\nwant:\n%s", changedV, outV, want)
		}
		if !changedU || outU != want {
			t.Fatalf("@upgrade changed=%v\ngot:\n%s\nwant:\n%s", changedU, outU, want)
		}
	})
}

func TestDiffGitApplyReverseRestoresRecordedVersion(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	proxy := kitFixtureProxy(t)
	offlineAdd(t, proxy)
	dir := writeCleanFieldCopy(t, "components/badge")

	origBytes, err := os.ReadFile(filepath.Join(dir, "components/field/field.go"))
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.ReplaceAll(string(origBytes), `": "`, `" -> "`) + "\nfunc Added() {}\n"
	if err := os.WriteFile(filepath.Join(dir, "components/field/field.go"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also add and delete files to verify git apply -R restores everything cleanly.
	writeFiles(t, dir, map[string]string{
		"components/field/notes.txt": "local note without newline",
	})
	if err := os.Remove(filepath.Join(dir, "components/field/helper.go")); err != nil {
		t.Fatal(err)
	}

	patch, errOut, changed, err := runDiff(t, dir, diffOptions{}, "field")
	if err != nil || !changed {
		t.Fatalf("diff failed: changed=%v err=%v stderr=%s", changed, err, errOut)
	}

	cmd := exec.Command("git", "apply", "-R", "-")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(patch)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git apply -R failed: %v\noutput:\n%s\npatch:\n%s", err, out, patch)
	}

	restored, err := os.ReadFile(filepath.Join(dir, "components/field/field.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != string(origBytes) {
		t.Fatalf("restored file differs:\ngot:\n%s\nwant:\n%s", restored, origBytes)
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/notes.txt")); !os.IsNotExist(err) {
		t.Fatalf("added file notes.txt still exists after git apply -R")
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/helper.go")); err != nil {
		t.Fatalf("deleted file helper.go was not restored by git apply -R: %v", err)
	}

	// Subsequent diff must be completely empty.
	afterOut, _, afterChanged, err := runDiff(t, dir, diffOptions{}, "field")
	if err != nil || afterChanged || afterOut != "" {
		t.Fatalf("diff after git apply -R: changed=%v err=%v out=%q", afterChanged, err, afterOut)
	}
}

func TestDiffStatAndExitCode(t *testing.T) {
	proxy := kitFixtureProxy(t)
	offlineAdd(t, proxy)
	dir := writeCleanFieldCopy(t, "components/badge")

	// Unedited copy with -exit-code via CLI subprocess exits 0 and prints nothing.
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH")
	}
	env := []string{
		"GOPROXY=file://" + filepath.ToSlash(proxy),
		"GOSUMDB=off",
		"GOTOOLCHAIN=local",
		"GOMODCACHE=" + os.Getenv("GOMODCACHE"),
		"GOPRIVATE=",
		"GONOPROXY=",
		"GONOSUMDB=",
	}
	out, code := runNsgo(t, dir, filepath.Dir(gobin), env, "diff", "-exit-code", "field")
	if code != 0 || out != "" {
		t.Fatalf("unedited -exit-code: code=%d out=%q; want 0 and empty", code, out)
	}

	// Edit field.go and icon.png.
	orig, err := os.ReadFile(filepath.Join(dir, "components/field/field.go"))
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(orig), `": "`, `":: "`, 1) + "\nfunc Extra() {}\n"
	writeFiles(t, dir, map[string]string{
		"components/field/field.go":        edited,
		"components/field/assets/icon.png": "\x89PNG\r\n\x1a\nlonger-png-bytes",
	})

	statOut, errOut, changed, err := runDiff(t, dir, diffOptions{stat: true}, "field")
	if err != nil || !changed {
		t.Fatalf("-stat failed: changed=%v err=%v stderr=%s", changed, err, errOut)
	}
	wantStat := "" +
		" components/field/assets/icon.png | Bin 22 -> 24 bytes\n" +
		" components/field/field.go        | 4 +++-\n" +
		" 2 files changed, 3 insertions(+), 1 deletion(-)\n"
	if statOut != wantStat {
		t.Fatalf("-stat output mismatch:\ngot:\n%s\nwant:\n%s", statOut, wantStat)
	}

	out, code = runNsgo(t, dir, filepath.Dir(gobin), env, "diff", "-stat", "-exit-code", "field")
	if code != 1 || out != wantStat {
		t.Fatalf("edited -stat -exit-code: code=%d out=%q; want 1 and %q", code, out, wantStat)
	}
}

func TestDiffOfflineFromModuleCache(t *testing.T) {
	proxy := kitFixtureProxy(t)
	offlineAdd(t, proxy)
	dir := writeCleanFieldCopy(t, "components/badge")

	// Fill GOMODCACHE while proxy is enabled, then disable proxy completely.
	dl := exec.Command("go", "mod", "download", "example.com/kit@v1.0.0")
	dl.Dir = dir
	if out, err := dl.CombinedOutput(); err != nil {
		t.Fatalf("go mod download: %v\n%s", err, out)
	}
	t.Setenv("GOPROXY", "off")

	// Finding the copy by its record name "field" works 100% offline.
	out, errOut, changed, err := runDiff(t, dir, diffOptions{}, "field")
	if err != nil || changed || out != "" || errOut != "" {
		t.Fatalf("offline unedited diff: changed=%v err=%v out=%q stderr=%q", changed, err, out, errOut)
	}

	// Edit a file and confirm diff still works offline.
	writeFiles(t, dir, map[string]string{
		"components/field/helper.go": "package field\n\nfunc Helper() string {\n\treturn \"offline-edit\"\n}\n",
	})
	out, errOut, changed, err = runDiff(t, dir, diffOptions{}, "field")
	if err != nil || !changed || !strings.Contains(out, "+	return \"offline-edit\"") {
		t.Fatalf("offline edited diff: changed=%v err=%v out=%q stderr=%q", changed, err, out, errOut)
	}
}

func TestDiffAmbiguousComponentNameAsksForDirectory(t *testing.T) {
	proxy := t.TempDir()
	gomodA := "module example.com/a\n\ngo 1.26\n"
	gomodB := "module example.com/b\n\ngo 1.26\n"
	manifest := "version: 1\ncomponents:\n  chat: ui/chat\n"
	addDiffProxyModule(t, proxy, "example.com/a", "v1.0.0", gomodA, map[string]string{
		"nsgo-module.yaml": manifest,
		"ui/chat/chat.go":  "package chat\n\nfunc Source() string { return \"a\" }\n",
	})
	addDiffProxyModule(t, proxy, "example.com/b", "v1.0.0", gomodB, map[string]string{
		"nsgo-module.yaml": manifest,
		"ui/chat/chat.go":  "package chat\n\nfunc Source() string { return \"b\" }\n",
	})
	offlineAdd(t, proxy)

	recA, _ := formatCopyRecord(&copyRecord{
		Version: 1, Module: "example.com/a", ModuleVersion: "v1.0.0", Component: "chat", Tests: true, Files: []string{"chat.go"},
	})
	recB, _ := formatCopyRecord(&copyRecord{
		Version: 1, Module: "example.com/b", ModuleVersion: "v1.0.0", Component: "chat", Tests: true, Files: []string{"chat.go"},
	})
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":                           "module example.com/app\n\ngo 1.26\n",
		"components/a/chat/nsgo-copy.yaml": string(recA),
		"components/a/chat/chat.go":        "package chat\n\nfunc Source() string { return \"a\" }\n",
		"components/b/chat/nsgo-copy.yaml": string(recB),
		"components/b/chat/chat.go":        "package chat\n\nfunc Source() string { return \"b-edited\" }\n",
	})

	_, _, _, err := runDiff(t, dir, diffOptions{}, "chat")
	if err == nil || !strings.Contains(err.Error(), "components/a/chat") || !strings.Contains(err.Error(), "components/b/chat") || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("expected ambiguity error naming both directories, got: %v", err)
	}

	// Passing directory or module#component disambiguates cleanly.
	out, _, changed, err := runDiff(t, dir, diffOptions{}, "components/b/chat")
	if err != nil || !changed || !strings.Contains(out, "b-edited") {
		t.Fatalf("dir diff failed: changed=%v err=%v out=%q", changed, err, out)
	}
	outA, _, changedA, err := runDiff(t, dir, diffOptions{}, "example.com/a#chat")
	if err != nil || changedA || outA != "" {
		t.Fatalf("module#component diff failed: changed=%v err=%v out=%q", changedA, err, outA)
	}
}

func TestDiffCRLFCountsAsUnedited(t *testing.T) {
	proxy := kitFixtureProxy(t)
	offlineAdd(t, proxy)
	dir := writeCleanFieldCopy(t, "components/badge")

	// Convert every text file in components/field to CRLF endings.
	textPaths := []string{
		"components/field/LICENSE",
		"components/field/field.go",
		"components/field/field_test.go",
		"components/field/helper.go",
		"components/field/locales/locales.go",
		"components/field/locales/en.yaml",
	}
	for _, rel := range textPaths {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		crlf := strings.ReplaceAll(string(raw), "\n", "\r\n")
		if err := os.WriteFile(p, []byte(crlf), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	out, errOut, changed, err := runDiff(t, dir, diffOptions{}, "field")
	if err != nil || changed || out != "" || errOut != "" {
		t.Fatalf("CRLF copy should be unedited: changed=%v err=%v out=%q stderr=%q", changed, err, out, errOut)
	}

	// List mode also reports unedited.
	listOut, _, listChanged, err := runDiff(t, dir, diffOptions{}, "")
	if err != nil || listChanged {
		t.Fatalf("list mode: changed=%v err=%v out=%q", listChanged, err, listOut)
	}
	if !strings.Contains(listOut, "components/field  v1.0.0  unedited\n") {
		t.Fatalf("list output:\n%s", listOut)
	}
}

func TestDiffRecordSafetyAndSymlinks(t *testing.T) {
	proxy := kitFixtureProxy(t)
	offlineAdd(t, proxy)

	t.Run("record listing ../form/form.go is refused with line number", func(t *testing.T) {
		dir := writeCleanFieldCopy(t, "components/badge")
		badRec := "" +
			"version: 1\n" +
			"module: example.com/kit\n" +
			"module_version: v1.0.0\n" +
			"component: field\n" +
			"tests: true\n" +
			"files:\n" +
			"  - field.go\n" +
			"  - ../form/form.go\n"
		if err := os.WriteFile(filepath.Join(dir, "components/field/nsgo-copy.yaml"), []byte(badRec), 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, _, err := runDiff(t, dir, diffOptions{}, "components/field")
		if err == nil {
			t.Fatal("expected error for ../form/form.go")
		}
		if !strings.Contains(err.Error(), "line 8:") || !strings.Contains(err.Error(), "../form/form.go") {
			t.Fatalf("expected error naming line 8 and ../form/form.go, got: %v", err)
		}
	})

	t.Run("recorded symlink is refused", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks require elevated privileges on Windows")
		}
		dir := writeCleanFieldCopy(t, "components/badge")
		secret := filepath.Join(dir, "secret.txt")
		if err := os.WriteFile(secret, []byte("secret"), 0o644); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(dir, "components/field/helper.go")
		if err := os.Remove(target); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(secret, target); err != nil {
			t.Fatal(err)
		}
		_, _, _, err := runDiff(t, dir, diffOptions{}, "field")
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected symlink refusal, got: %v", err)
		}
	})

	t.Run("non-copied directory returns clear error", func(t *testing.T) {
		dir := writeCleanFieldCopy(t, "components/badge")
		writeFiles(t, dir, map[string]string{"components/plain/plain.go": "package plain\n"})
		_, _, _, err := runDiff(t, dir, diffOptions{}, "components/plain")
		if err == nil || !strings.Contains(err.Error(), "not a copied component") {
			t.Fatalf("expected non-copied component error, got: %v", err)
		}
	})
}

func TestDiffUnavailableRecordedVersion(t *testing.T) {
	// Proxy does NOT serve v1.0.0 at all, and GOMODCACHE is empty.
	proxy := t.TempDir()
	offlineAdd(t, proxy)
	dir := writeCleanFieldCopy(t, "components/badge")

	// Diff against recorded version counts every non-generated file as edited and states why.
	out, errOut, changed, err := runDiff(t, dir, diffOptions{}, "field")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true when recorded version is unavailable")
	}
	if !strings.Contains(errOut, "example.com/kit@v1.0.0") || !strings.Contains(errOut, "unavailable") {
		t.Fatalf("expected unavailable reason on stderr, got: %q", errOut)
	}
	for _, wantFile := range []string{
		"diff --git a/components/field/LICENSE b/components/field/LICENSE",
		"diff --git a/components/field/field.go b/components/field/field.go",
		"diff --git a/components/field/helper.go b/components/field/helper.go",
		"diff --git a/components/field/locales/en.yaml b/components/field/locales/en.yaml",
	} {
		if !strings.Contains(out, wantFile) {
			t.Fatalf("expected %q in output:\n%s", wantFile, out)
		}
	}
	if strings.Contains(out, "catalog_gen.go") || strings.Contains(out, "nsgo-copy.yaml") {
		t.Fatalf("generated file or record should not appear in diff:\n%s", out)
	}

	// List mode also marks the copy edited and gives the reason.
	listOut, _, listChanged, err := runDiff(t, dir, diffOptions{}, "")
	if err != nil || !listChanged {
		t.Fatalf("list mode: changed=%v err=%v", listChanged, err)
	}
	if !strings.Contains(listOut, "components/field  v1.0.0  edited (unavailable:") {
		t.Fatalf("list output missing unavailable reason:\n%s", listOut)
	}
}

func TestDiffListAllCopies(t *testing.T) {
	proxy := kitFixtureProxy(t)
	offlineAdd(t, proxy)
	dir := writeCleanFieldCopy(t, "components/badge")

	// Edit badge only; leave field unedited.
	writeFiles(t, dir, map[string]string{
		"components/badge/badge.go": "package badge\n\nfunc Label() string { return \"edited\" }\n",
	})

	out, errOut, changed, err := runDiff(t, dir, diffOptions{}, "")
	if err != nil || !changed || errOut != "" {
		t.Fatalf("list mode: changed=%v err=%v stderr=%q", changed, err, errOut)
	}
	want := "" +
		"components/badge  v1.0.0  edited\n" +
		"components/field  v1.0.0  unedited\n"
	if out != want {
		t.Fatalf("list output mismatch:\ngot:\n%s\nwant:\n%s", out, want)
	}
}

func TestDiffBuiltinWinsOverExtensionAndDelegation(t *testing.T) {
	// Built-in diff wins over nsgo-diff on PATH.
	bin := demoOnPath(t, "diff")
	out, code := runNsgo(t, t.TempDir(), bin, []string{"NSGO_TEST_EXTENSION=1"}, "diff", "a", "b")
	if code != 1 || strings.Contains(out, "demo [") || !strings.Contains(out, "takes one path") {
		t.Fatalf("exit %d, output %s; want built-in diff error", code, out)
	}

	// Module pinning NSGo v0.3.4 is refused before delegation.
	path, env := goEnv(t, nsgoProxy(t, "v0.9.0"))
	out, code = runNsgo(t, pinnedModule(t, "v0.3.4", ""), path, env, "diff", "field")
	want := "nsgo diff needs NSGo v0.3.5 or later; this module pins v0.3.4: go get github.com/nsgo-dev/nsgo@v0.3.5\n"
	if code != 1 || !strings.Contains(out, want) || strings.Contains(out, "fake nsgo") {
		t.Fatalf("exit %d, output %s; want %q and exit 1", code, out, want)
	}
}

func TestDiffEdgeCases(t *testing.T) {
	proxy := kitFixtureProxy(t)
	offlineAdd(t, proxy)

	t.Run("tests false omits tests from reconstructed base", func(t *testing.T) {
		dir := writeCleanFieldCopy(t, "components/badge")
		if err := os.Remove(filepath.Join(dir, "components/field/field_test.go")); err != nil {
			t.Fatal(err)
		}
		rec, err := formatCopyRecord(&copyRecord{
			Version:       1,
			Module:        "example.com/kit",
			ModuleVersion: "v1.0.0",
			Component:     "field",
			Tests:         false,
			Files: []string{
				"LICENSE",
				"assets/icon.png",
				"field.go",
				"helper.go",
				"locales/en.yaml",
				"locales/locales.go",
			},
			Generated: []string{"locales/catalog_gen.go"},
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "components/field/nsgo-copy.yaml"), rec, 0o644); err != nil {
			t.Fatal(err)
		}
		out, errOut, changed, err := runDiff(t, dir, diffOptions{}, "field")
		if err != nil || changed || out != "" || errOut != "" {
			t.Fatalf("tests:false copy should be unedited: changed=%v err=%v out=%q stderr=%q", changed, err, out, errOut)
		}
	})

	t.Run("unformatted Go whitespace counts as unedited", func(t *testing.T) {
		dir := writeCleanFieldCopy(t, "components/badge")
		unformatted := "package field\n\nfunc Helper() string {\nreturn   \"v1\"\n}\n"
		if err := os.WriteFile(filepath.Join(dir, "components/field/helper.go"), []byte(unformatted), 0o644); err != nil {
			t.Fatal(err)
		}
		out, errOut, changed, err := runDiff(t, dir, diffOptions{}, "field")
		if err != nil || changed || out != "" || errOut != "" {
			t.Fatalf("whitespace-only Go file should be unedited: changed=%v err=%v out=%q stderr=%q", changed, err, out, errOut)
		}
	})

	t.Run("go.sum mismatch is refused as an integrity error", func(t *testing.T) {
		dir := writeCleanFieldCopy(t, "components/badge")
		writeFiles(t, dir, map[string]string{
			"go.sum": "example.com/kit v1.0.0 h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n",
		})
		_, _, _, err := runDiff(t, dir, diffOptions{}, "field")
		if err == nil || !strings.Contains(err.Error(), "go.sum mismatch") {
			t.Fatalf("expected go.sum mismatch error, got: %v", err)
		}
	})

	t.Run("multi-hunk diff has three lines of context and reverses cleanly", func(t *testing.T) {
		oldLines := make([]string, 20)
		newLines := make([]string, 20)
		for i := range 20 {
			line := "line " + string(rune('a'+i))
			oldLines[i] = line
			newLines[i] = line
		}
		newLines[2] = "line C"
		newLines[16] = "line Q"
		oldText := strings.Join(oldLines, "\n") + "\n"
		newText := strings.Join(newLines, "\n") + "\n"

		d := buildTextFileDiff("components/field/multi.txt", diffModified, []byte(oldText), []byte(newText))
		if d == nil || len(d.Hunks) != 2 {
			t.Fatalf("expected 2 hunks, got %+v", d)
		}
		if d.Hunks[0].oldStart != 1 || d.Hunks[0].oldCount != 6 || d.Hunks[1].oldStart != 14 || d.Hunks[1].oldCount != 7 {
			t.Fatalf("unexpected hunk ranges: %+v", d.Hunks)
		}
		if _, err := exec.LookPath("git"); err == nil {
			tmp := t.TempDir()
			writeFiles(t, tmp, map[string]string{"components/field/multi.txt": newText})
			patch := formatUnifiedDiff([]fileDiff{*d})
			cmd := exec.Command("git", "apply", "-R", "-")
			cmd.Dir = tmp
			cmd.Stdin = strings.NewReader(patch)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git apply -R failed: %v\n%s\npatch:\n%s", err, out, patch)
			}
			got, err := os.ReadFile(filepath.Join(tmp, "components/field/multi.txt"))
			if err != nil || string(got) != oldText {
				t.Fatalf("restored multi-hunk mismatch: err=%v got=%q want=%q", err, string(got), oldText)
			}
		}
	})
}

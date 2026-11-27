package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func setUpdateProxy(t *testing.T, proxy string) {
	t.Helper()
	hostCacheOut, _ := exec.Command("go", "env", "GOMODCACHE").Output()
	hostDownload := filepath.Join(strings.TrimSpace(string(hostCacheOut)), "cache", "download")
	offlineAdd(t, proxy)
	if info, err := os.Stat(hostDownload); err == nil && info.IsDir() {
		t.Setenv("GOPROXY", "file://"+filepath.ToSlash(proxy)+",file://"+filepath.ToSlash(hostDownload))
	}
}

// updateFixtureEnv publishes a file:// module proxy with v1.0.0, v1.1.0 and
// v1.2.0 of example.com/upkit (components: field, form -> imports field +
// example.com/flags) so #2469 update scenarios run fully offline.
func updateFixtureEnv(t *testing.T) (proxy, nsgoCheckout string) {
	t.Helper()
	nsgoCheckout = checkoutRoot(t)
	proxy = t.TempDir()
	mit := readFixtureLicense(t, "License/MIT/pristine.txt")

	addCopyZipProxyModule(t, proxy, "example.com/flags", "v1.0.0",
		"module example.com/flags\n\ngo 1.26\n",
		map[string][]byte{
			"LICENSE":  mit,
			"flags.go": []byte("package flags\n\nfunc Code() string { return \"v1\" }\n"),
		})
	addCopyZipProxyModule(t, proxy, "example.com/flags", "v1.1.0",
		"module example.com/flags\n\ngo 1.26\n",
		map[string][]byte{
			"LICENSE":  mit,
			"flags.go": []byte("package flags\n\nfunc Code() string { return \"v1.1\" }\n"),
		})

	v100Mod := "module example.com/upkit\n\ngo 1.26\n\nrequire (\n\texample.com/flags v1.0.0\n\tgithub.com/nsgo-dev/nsgo v0.0.0\n)\n"
	v100Files := map[string][]byte{
		"LICENSE":          mit,
		"nsgo-module.yaml": []byte("version: 1\ncomponents:\n  field: ui/field\n  form: ui/form\n"),
		"ui/field/field.go": []byte(`package field

import (
	"example.com/upkit/ui/field/locales"
)

// Label returns the field label.
func Label() string { return locales.Label(nil) }

// Version returns the component version string.
func Version() string { return "1.0.0" }
`),
		"ui/field/extra.go": []byte(`package field

// Helper is left alone in v1.1.0 so app edits stay intact.
func Helper() string { return "base-helper" }
`),
		"ui/field/drop_unedited.go": []byte(`package field

func LegacyUnedited() string { return "old" }
`),
		"ui/field/drop_edited.go": []byte(`package field

func LegacyEditable() string { return "old-editable" }
`),
		"ui/field/locales/locales.go": []byte(`// Package locales holds translations for example.com/upkit/ui/field.
//go:generate go tool nsgo i18n
package locales
`),
		"ui/field/locales/en.yaml": []byte("label: \"Field v1\"\n"),
		"ui/form/form.go": []byte(`package form

import (
	"example.com/flags"
	"example.com/upkit/ui/field"
)

func Summary() string {
	return field.Label() + ":" + field.Version() + ":" + flags.Code()
}
`),
	}
	addCopyZipProxyModule(t, proxy, "example.com/upkit", "v1.0.0", v100Mod, v100Files)

	v110Mod := "module example.com/upkit\n\ngo 1.26\n\nrequire (\n\texample.com/flags v1.1.0\n\tgithub.com/nsgo-dev/nsgo v0.0.0\n)\n"
	v110Files := map[string][]byte{
		"LICENSE":          mit,
		"nsgo-module.yaml": []byte("version: 1\ncomponents:\n  field: ui/field\n  form: ui/form\n"),
		"ui/field/field.go": []byte(`package field

import (
	"example.com/upkit/ui/field/locales"
)

// Label returns the field label.
func Label() string { return locales.Label(nil) }

// Version returns the component version string.
func Version() string { return "1.1.0" }
`),
		"ui/field/extra.go": []byte(`package field

// Helper is left alone in v1.1.0 so app edits stay intact.
func Helper() string { return "base-helper" }
`),
		// drop_unedited.go and drop_edited.go are dropped in v1.1.0.
		"ui/field/added.go": []byte(`package field

// Added is introduced in v1.1.0.
func Added() string { return "from-1.1.0" }
`),
		"ui/field/locales/locales.go": []byte(`// Package locales holds translations for example.com/upkit/ui/field.
//go:generate go tool nsgo i18n
package locales
`),
		"ui/field/locales/en.yaml": []byte("label: \"Field v1.1\"\n"),
		"ui/form/form.go": []byte(`package form

import (
	"example.com/flags"
	"example.com/upkit/ui/field"
)

func Summary() string {
	return field.Label() + ":" + field.Version() + ":" + field.Added() + ":" + flags.Code()
}
`),
	}
	addCopyZipProxyModule(t, proxy, "example.com/upkit", "v1.1.0", v110Mod, v110Files)

	// v1.2.0 drops the "field" component declaration from nsgo-module.yaml.
	v120Mod := "module example.com/upkit\n\ngo 1.26\n\nrequire (\n\texample.com/flags v1.1.0\n\tgithub.com/nsgo-dev/nsgo v0.0.0\n)\n"
	v120Files := map[string][]byte{
		"LICENSE":          mit,
		"nsgo-module.yaml": []byte("version: 1\ncomponents:\n  form: ui/form\n"),
		"ui/form/form.go": []byte(`package form

import "example.com/flags"

func Summary() string { return "v1.2:" + flags.Code() }
`),
	}
	addCopyZipProxyModule(t, proxy, "example.com/upkit", "v1.2.0", v120Mod, v120Files)

	setUpdateProxy(t, proxy)
	return proxy, nsgoCheckout
}

func makeUpdateApp(t *testing.T, nsgoCheckout, modPath string) string {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":    "module " + modPath + "\n\ngo 1.27.3\n\nrequire github.com/nsgo-dev/nsgo v0.0.0\n\nreplace github.com/nsgo-dev/nsgo => " + filepath.ToSlash(nsgoCheckout) + "\n",
		"nsgo.yaml": "name: UpdateApp\nid: com.example.updateapp\nbuild:\n  targets:\n    - " + hostVetTargetLabel() + "\n",
		"main.go":   "package main\n\nimport _ \"github.com/nsgo-dev/nsgo/i18n\"\n\nfunc main() {}\n",
	})
	goIn(t, dir, "mod", "tidy")
	return dir
}

func TestUpdateComponentFileByFileTransitiveAndCatalog(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go vet and nsgo check")
	}
	_, nsgoCheckout := updateFixtureEnv(t)
	dir := makeUpdateApp(t, nsgoCheckout, "example.com/upapp")

	// Copy form@v1.0.0, which pulls in field@v1.0.0 too.
	plan, err := runAdd(t, dir, addOptions{yes: true}, nil, "example.com/upkit@v1.0.0#form")
	if err != nil {
		t.Fatalf("initial copy form@v1.0.0 failed: %v\n%s", err, plan)
	}

	// Edit field/extra.go (author leaves it alone in v1.1.0) and add an app-only file.
	appExtra := "package field\n\n// App custom edit.\nfunc Helper() string { return \"app-edited-helper\" }\n"
	if err := os.WriteFile(filepath.Join(dir, "components/field/extra.go"), []byte(appExtra), 0o644); err != nil {
		t.Fatal(err)
	}
	appCustom := "package field\n\nfunc AppOnly() string { return \"custom\" }\n"
	if err := os.WriteFile(filepath.Join(dir, "components/field/app_custom.go"), []byte(appCustom), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. -dry-run shows per-file plan for both form and transitive field and writes nothing.
	dryPlan, err := runAdd(t, dir, addOptions{dryRun: true}, nil, "form@v1.1.0")
	if err != nil {
		t.Fatalf("dry-run update form@v1.1.0 failed: %v\n%s", err, dryPlan)
	}
	for _, want := range []string{
		"updating copied component from v1.0.0 to v1.1.0",
		"~ field.go",
		"+ added.go",
		"- drop_unedited.go",
		"= extra.go (app edits kept)",
		"~ locales/en.yaml",
		"~ locales/catalog_gen.go, written by this nsgo",
		"~ form.go",
	} {
		if !strings.Contains(dryPlan, want) {
			t.Errorf("dry-run plan missing %q:\n%s", want, dryPlan)
		}
	}
	recBefore, err := readCopyRecord(filepath.Join(dir, "components/field"))
	if err != nil || recBefore.ModuleVersion != "v1.0.0" {
		t.Fatalf("dry-run modified field record: %+v, err=%v", recBefore, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/added.go")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote added.go")
	}

	// 2. Apply the update by bare component name (resolved offline from nsgo-copy.yaml).
	upPlan, err := runAdd(t, dir, addOptions{yes: true}, nil, "form@v1.1.0")
	if err != nil {
		t.Fatalf("update form@v1.1.0 failed: %v\n%s", err, upPlan)
	}

	// Verify field.go took v1.1.0, extra.go kept app edits, app_custom.go kept,
	// added.go added, drop_unedited.go and drop_edited.go (unedited here) deleted.
	fieldGo, err := os.ReadFile(filepath.Join(dir, "components/field/field.go"))
	if err != nil || !strings.Contains(string(fieldGo), `"1.1.0"`) {
		t.Errorf("field.go did not take v1.1.0: %q, err=%v", fieldGo, err)
	}
	extraGo, err := os.ReadFile(filepath.Join(dir, "components/field/extra.go"))
	if err != nil || string(extraGo) != appExtra {
		t.Errorf("extra.go did not keep app edits: got %q, want %q", extraGo, appExtra)
	}
	customGo, err := os.ReadFile(filepath.Join(dir, "components/field/app_custom.go"))
	if err != nil || string(customGo) != appCustom {
		t.Errorf("app_custom.go was not preserved: %q, err=%v", customGo, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/added.go")); err != nil {
		t.Errorf("added.go was not added: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/drop_unedited.go")); !os.IsNotExist(err) {
		t.Errorf("drop_unedited.go was not deleted")
	}

	// Verify generated catalog reflects the changed message in v1.1.0.
	catGen, err := os.ReadFile(filepath.Join(dir, "components/field/locales/catalog_gen.go"))
	if err != nil || !strings.Contains(string(catGen), `"Field v1.1"`) {
		t.Errorf("locales/catalog_gen.go did not follow changed message:\n%s\nerr=%v", catGen, err)
	}

	// Verify both form and field records now name v1.1.0 and go.mod raised flags to v1.1.0.
	fieldRec, err := readCopyRecord(filepath.Join(dir, "components/field"))
	if err != nil || fieldRec.ModuleVersion != "v1.1.0" {
		t.Fatalf("field record = %+v, err=%v; want v1.1.0", fieldRec, err)
	}
	formRec, err := readCopyRecord(filepath.Join(dir, "components/form"))
	if err != nil || formRec.ModuleVersion != "v1.1.0" {
		t.Fatalf("form record = %+v, err=%v; want v1.1.0", formRec, err)
	}
	if slices.Contains(fieldRec.Files, "drop_unedited.go") || !slices.Contains(fieldRec.Files, "added.go") {
		t.Errorf("field record files not updated to v1.1.0: %v", fieldRec.Files)
	}
	gomod, _ := moduleFiles(t, dir)
	if !strings.Contains(string(gomod), "example.com/flags v1.1.0") {
		t.Errorf("go.mod did not raise example.com/flags to v1.1.0:\n%s", gomod)
	}

	// Verify `nsgo diff field` after the update shows ONLY the app's own edits
	// (extra.go and app_custom.go) — zero diff on all unedited files.
	t.Chdir(dir)
	var diffOut, diffErr bytes.Buffer
	hasDiff, err := diffCommand(t.Context(), diffOptions{}, "field", &diffOut, &diffErr)
	if err != nil {
		t.Fatalf("nsgo diff field after update failed: %v (stderr=%q)", err, diffErr.String())
	}
	if !hasDiff {
		t.Fatalf("nsgo diff field reported no diffs; want app edits in extra.go and app_custom.go")
	}
	diffText := diffOut.String()
	for _, want := range []string{"components/field/extra.go", "components/field/app_custom.go", "app-edited-helper"} {
		if !strings.Contains(diffText, want) {
			t.Errorf("nsgo diff field missing %q:\n%s", want, diffText)
		}
	}
	for _, unwanted := range []string{"components/field/field.go", "components/field/added.go", "components/field/locales/en.yaml", "components/field/LICENSE"} {
		if strings.Contains(diffText, unwanted) {
			t.Errorf("nsgo diff field unexpectedly included unedited %q:\n%s", unwanted, diffText)
		}
	}

	// Verify nsgo check passes on the updated app.
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := check(t.Context(), p, "", 0); err != nil {
		t.Fatalf("nsgo check after update failed: %v", err)
	}

	// 3. Updating again to the same version (v1.1.0) is a clean no-op.
	again, err := runAdd(t, dir, addOptions{yes: true}, nil, "form@v1.1.0")
	if err != nil {
		t.Fatalf("same-version add failed: %v\n%s", err, again)
	}
	if !strings.Contains(again, "form (example.com/upkit v1.1.0) is already copied at components/form") {
		t.Errorf("same-version output unexpected:\n%s", again)
	}
}

// updateLiteFixtureEnv publishes a pure-Go (no NSGo/core import) file:// proxy
// with v1.0.0, v1.1.0 and v1.2.0 of example.com/upkit so file-by-file update
// tests that do not test catalog generation run in under a second each.
func updateLiteFixtureEnv(t *testing.T) (proxy string) {
	t.Helper()
	proxy = t.TempDir()
	mit := readFixtureLicense(t, "License/MIT/pristine.txt")

	addCopyZipProxyModule(t, proxy, "example.com/flags", "v1.0.0",
		"module example.com/flags\n\ngo 1.26\n",
		map[string][]byte{
			"LICENSE":  mit,
			"flags.go": []byte("package flags\n\nfunc Code() string { return \"v1\" }\n"),
		})
	addCopyZipProxyModule(t, proxy, "example.com/flags", "v1.1.0",
		"module example.com/flags\n\ngo 1.26\n",
		map[string][]byte{
			"LICENSE":  mit,
			"flags.go": []byte("package flags\n\nfunc Code() string { return \"v1.1\" }\n"),
		})

	v100Mod := "module example.com/upkit\n\ngo 1.26\n\nrequire example.com/flags v1.0.0\n"
	addCopyZipProxyModule(t, proxy, "example.com/upkit", "v1.0.0", v100Mod, map[string][]byte{
		"LICENSE":                   mit,
		"nsgo-module.yaml":          []byte("version: 1\ncomponents:\n  field: ui/field\n  form: ui/form\n"),
		"ui/field/field.go":         []byte("package field\n\n// Version returns the component version string.\nfunc Version() string { return \"1.0.0\" }\n"),
		"ui/field/extra.go":         []byte("package field\n\n// Helper is left alone in v1.1.0.\nfunc Helper() string { return \"base-helper\" }\n"),
		"ui/field/drop_unedited.go": []byte("package field\n\nfunc LegacyUnedited() string { return \"old\" }\n"),
		"ui/field/drop_edited.go":   []byte("package field\n\nfunc LegacyEditable() string { return \"old-editable\" }\n"),
		"ui/form/form.go":           []byte("package form\n\nimport (\n\t\"example.com/flags\"\n\t\"example.com/upkit/ui/field\"\n)\n\nfunc Summary() string { return field.Version() + \":\" + flags.Code() }\n"),
	})

	v110Mod := "module example.com/upkit\n\ngo 1.26\n\nrequire example.com/flags v1.1.0\n"
	addCopyZipProxyModule(t, proxy, "example.com/upkit", "v1.1.0", v110Mod, map[string][]byte{
		"LICENSE":           mit,
		"nsgo-module.yaml":  []byte("version: 1\ncomponents:\n  field: ui/field\n  form: ui/form\n"),
		"ui/field/field.go": []byte("package field\n\n// Version returns the component version string.\nfunc Version() string { return \"1.1.0\" }\n"),
		"ui/field/extra.go": []byte("package field\n\n// Helper is left alone in v1.1.0.\nfunc Helper() string { return \"base-helper\" }\n"),
		"ui/field/added.go": []byte("package field\n\n// Added is introduced in v1.1.0.\nfunc Added() string { return \"from-1.1.0\" }\n"),
		"ui/form/form.go":   []byte("package form\n\nimport (\n\t\"example.com/flags\"\n\t\"example.com/upkit/ui/field\"\n)\n\nfunc Summary() string { return field.Version() + \":\" + field.Added() + \":\" + flags.Code() }\n"),
	})

	v120Mod := "module example.com/upkit\n\ngo 1.26\n\nrequire example.com/flags v1.1.0\n"
	addCopyZipProxyModule(t, proxy, "example.com/upkit", "v1.2.0", v120Mod, map[string][]byte{
		"LICENSE":          mit,
		"nsgo-module.yaml": []byte("version: 1\ncomponents:\n  form: ui/form\n"),
		"ui/form/form.go":  []byte("package form\n\nimport \"example.com/flags\"\n\nfunc Summary() string { return \"v1.2:\" + flags.Code() }\n"),
	})

	offlineAdd(t, proxy)
	return proxy
}

func makeUpdateLiteApp(t *testing.T, modPath string) string {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module " + modPath + "\n\ngo 1.27.3\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})
	return dir
}

func TestUpdateComponentBothChangedRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go vet")
	}
	updateLiteFixtureEnv(t)
	dir := makeUpdateLiteApp(t, "example.com/conflictapp")

	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "example.com/upkit@v1.0.0#field"); err != nil {
		t.Fatalf("initial copy failed: %v", err)
	}

	// Edit field.go differently from v1.1.0.
	editedField := "package field\n\n// Version returns the app's custom version string.\nfunc Version() string { return \"app-custom-1.0\" }\n"
	if err := os.WriteFile(filepath.Join(dir, "components/field/field.go"), []byte(editedField), 0o644); err != nil {
		t.Fatal(err)
	}

	// Must be refused under -y AND interactively, naming field.go and nsgo diff, writing nothing.
	yesAsk := func(context.Context) (bool, error) { return true, nil }
	for _, opts := range []addOptions{{yes: true}, {}} {
		plan, err := runAdd(t, dir, opts, yesAsk, "field@v1.1.0")
		if err == nil {
			t.Fatalf("expected conflict refusal (opts=%+v), got nil\n%s", opts, plan)
		}
		if !strings.Contains(err.Error(), "components/field/field.go") || !strings.Contains(err.Error(), "nsgo diff field") {
			t.Errorf("conflict error missing file name or nsgo diff hint: %v", err)
		}
	}

	// Verify nothing was written on disk.
	gotField, _ := os.ReadFile(filepath.Join(dir, "components/field/field.go"))
	if string(gotField) != editedField {
		t.Errorf("field.go was modified despite conflict")
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/added.go")); !os.IsNotExist(err) {
		t.Errorf("added.go was written despite conflict")
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/drop_unedited.go")); err != nil {
		t.Errorf("drop_unedited.go was deleted despite conflict")
	}
	rec, err := readCopyRecord(filepath.Join(dir, "components/field"))
	if err != nil || rec.ModuleVersion != "v1.0.0" {
		t.Errorf("record changed despite conflict: %+v, err=%v", rec, err)
	}

	// Now make the app's field.go match v1.1.0's exact change -> both made it the same -> succeeds!
	sameAsV110 := "package field\n\n// Version returns the component version string.\nfunc Version() string { return \"1.1.0\" }\n"
	if err := os.WriteFile(filepath.Join(dir, "components/field/field.go"), []byte(sameAsV110), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := runAdd(t, dir, addOptions{yes: true}, nil, "field@v1.1.0")
	if err != nil {
		t.Fatalf("identical change by both sides should succeed: %v\n%s", err, plan)
	}
	if !strings.Contains(plan, "app and author made the same change") {
		t.Errorf("plan did not report identical change:\n%s", plan)
	}
	rec, err = readCopyRecord(filepath.Join(dir, "components/field"))
	if err != nil || rec.ModuleVersion != "v1.1.0" {
		t.Errorf("record after identical change = %+v, err=%v; want v1.1.0", rec, err)
	}
}

func TestUpdateComponentDroppedEditedFlaggedAndRefusedUnderYes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go vet")
	}
	updateLiteFixtureEnv(t)
	dir := makeUpdateLiteApp(t, "example.com/dropapp")

	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "example.com/upkit@v1.0.0#field"); err != nil {
		t.Fatalf("initial copy failed: %v", err)
	}

	// Edit drop_edited.go, which v1.1.0 drops, while leaving drop_unedited.go alone.
	editedDropped := "package field\n\nfunc LegacyEditable() string { return \"app-edited\" }\n"
	if err := os.WriteFile(filepath.Join(dir, "components/field/drop_edited.go"), []byte(editedDropped), 0o644); err != nil {
		t.Fatal(err)
	}

	// Under -y: refused, naming drop_edited.go, and nothing written.
	plan, err := runAdd(t, dir, addOptions{yes: true}, nil, "field@v1.1.0")
	if err == nil {
		t.Fatalf("expected -y refusal for edited dropped file\n%s", plan)
	}
	if !strings.Contains(err.Error(), "components/field/drop_edited.go") {
		t.Errorf("error did not name drop_edited.go: %v", err)
	}
	if !strings.Contains(plan, "- drop_edited.go (edited by app; flagged)") {
		t.Errorf("plan did not flag drop_edited.go:\n%s", plan)
	}
	// Nothing written yet.
	if got, err := os.ReadFile(filepath.Join(dir, "components/field/drop_edited.go")); err != nil || string(got) != editedDropped {
		t.Errorf("drop_edited.go was changed despite -y refusal")
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/drop_unedited.go")); err != nil {
		t.Errorf("drop_unedited.go was deleted despite -y refusal")
	}
	rec, _ := readCopyRecord(filepath.Join(dir, "components/field"))
	if rec.ModuleVersion != "v1.0.0" {
		t.Errorf("record version changed to %s despite -y refusal", rec.ModuleVersion)
	}

	// Confirming interactively succeeds and deletes both drop_unedited.go and drop_edited.go.
	yesAsk := func(context.Context) (bool, error) { return true, nil }
	plan, err = runAdd(t, dir, addOptions{}, yesAsk, "field@v1.1.0")
	if err != nil {
		t.Fatalf("interactive confirmation failed: %v\n%s", err, plan)
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/drop_unedited.go")); !os.IsNotExist(err) {
		t.Errorf("drop_unedited.go was not deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/drop_edited.go")); !os.IsNotExist(err) {
		t.Errorf("drop_edited.go was not deleted after interactive confirmation")
	}
	rec, _ = readCopyRecord(filepath.Join(dir, "components/field"))
	if rec.ModuleVersion != "v1.1.0" {
		t.Errorf("record version = %s, want v1.1.0", rec.ModuleVersion)
	}
}

func TestUpdateComponentAddedCaseCollisionRefused(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go vet")
	}
	updateLiteFixtureEnv(t)
	dir := makeUpdateLiteApp(t, "example.com/caseapp")

	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "example.com/upkit@v1.0.0#field"); err != nil {
		t.Fatalf("initial copy failed: %v", err)
	}

	// App adds ADDED.go (different case from v1.1.0's added.go).
	appCollision := "package field\n\nfunc AppAdded() string { return \"mine\" }\n"
	if err := os.WriteFile(filepath.Join(dir, "components/field/ADDED.go"), []byte(appCollision), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := runAdd(t, dir, addOptions{yes: true}, nil, "field@v1.1.0")
	if err == nil {
		t.Fatalf("expected case-insensitive add collision refusal\n%s", plan)
	}
	if !strings.Contains(err.Error(), "added.go") || !strings.Contains(err.Error(), "ADDED.go") {
		t.Errorf("collision error should name both added.go and ADDED.go: %v", err)
	}
	// Verify nothing was written and ADDED.go is untouched.
	got, err := os.ReadFile(filepath.Join(dir, "components/field/ADDED.go"))
	if err != nil || string(got) != appCollision {
		t.Errorf("ADDED.go was altered despite refusal: %q, err=%v", got, err)
	}
	rec, _ := readCopyRecord(filepath.Join(dir, "components/field"))
	if rec.ModuleVersion != "v1.0.0" {
		t.Errorf("record version = %s, want v1.0.0", rec.ModuleVersion)
	}
}

func TestUpdateComponentCRLFPreserved(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go vet and nsgo check")
	}
	_, nsgoCheckout := updateFixtureEnv(t)
	dir := makeUpdateApp(t, nsgoCheckout, "example.com/crlfapp")

	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "example.com/upkit@v1.0.0#field"); err != nil {
		t.Fatalf("initial copy failed: %v", err)
	}

	// Convert field.go and locales/en.yaml on disk to CRLF (simulating core.autocrlf).
	for _, rel := range []string{"components/field/field.go", "components/field/locales/en.yaml"} {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		raw, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		crlf := bytes.ReplaceAll(bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n")), []byte("\n"), []byte("\r\n"))
		if err := os.WriteFile(full, crlf, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	plan, err := runAdd(t, dir, addOptions{yes: true}, nil, "field@v1.1.0")
	if err != nil {
		t.Fatalf("CRLF update failed: %v\n%s", err, plan)
	}
	if !strings.Contains(plan, "~ field.go (CRLF)") {
		t.Errorf("plan did not note CRLF preservation:\n%s", plan)
	}

	for _, checkFile := range []struct {
		rel  string
		want string
	}{
		{"components/field/field.go", `"1.1.0"`},
		{"components/field/locales/en.yaml", `"Field v1.1"`},
	} {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(checkFile.rel)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(got), checkFile.want) {
			t.Errorf("%s did not take v1.1.0 content:\n%s", checkFile.rel, got)
		}
		if !bytes.Contains(got, []byte("\r\n")) {
			t.Errorf("%s lost CRLF line endings", checkFile.rel)
		}
		// Ensure no bare LF without preceding CR.
		if bytes.Contains(bytes.ReplaceAll(got, []byte("\r\n"), nil), []byte("\n")) {
			t.Errorf("%s has mixed LF/CRLF line endings", checkFile.rel)
		}
	}

	// nsgo check still passes with CRLF files.
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := check(t.Context(), p, "", 0); err != nil {
		t.Fatalf("nsgo check failed after CRLF update: %v", err)
	}
}

func TestUpdateComponentDowngradeUpgradeAndRemovedDeclaration(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go vet")
	}
	updateLiteFixtureEnv(t)
	dir := makeUpdateLiteApp(t, "example.com/downapp")

	// Start at v1.1.0.
	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "example.com/upkit@v1.1.0#field"); err != nil {
		t.Fatalf("initial copy v1.1.0 failed: %v", err)
	}
	// Edit extra.go (identical between v1.0.0 and v1.1.0).
	appExtra := "package field\n\nfunc Helper() string { return \"keep-across-downgrade\" }\n"
	if err := os.WriteFile(filepath.Join(dir, "components/field/extra.go"), []byte(appExtra), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Downgrade to v1.0.0: plan calls it a downgrade and preserves app edits.
	downPlan, err := runAdd(t, dir, addOptions{yes: true}, nil, "field@v1.0.0")
	if err != nil {
		t.Fatalf("downgrade to v1.0.0 failed: %v\n%s", err, downPlan)
	}
	if !strings.Contains(downPlan, "downgrade of copied component from v1.1.0 to v1.0.0") {
		t.Errorf("plan did not call it a downgrade:\n%s", downPlan)
	}
	rec, _ := readCopyRecord(filepath.Join(dir, "components/field"))
	if rec.ModuleVersion != "v1.0.0" {
		t.Errorf("record after downgrade = %s, want v1.0.0", rec.ModuleVersion)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "components/field/extra.go")); string(got) != appExtra {
		t.Errorf("app edits lost during downgrade: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/added.go")); !os.IsNotExist(err) {
		t.Errorf("added.go should be deleted when downgrading to v1.0.0")
	}

	// 2. Version whose module no longer declares field (v1.2.0) is refused, naming v1.2.0.
	_, err = runAdd(t, dir, addOptions{yes: true}, nil, "field@v1.2.0")
	if err == nil || !strings.Contains(err.Error(), "v1.2.0") || !strings.Contains(err.Error(), "declares no component field") {
		t.Errorf("missing component refusal = %v, want v1.2.0 + declares no component field", err)
	}

	// 3. @upgrade moves form from v1.0.0 to newest release (v1.2.0).
	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "example.com/upkit@v1.0.0#form"); err != nil {
		t.Fatalf("copy form@v1.0.0 failed: %v", err)
	}
	upPlan, err := runAdd(t, dir, addOptions{yes: true}, nil, "form@upgrade")
	if err != nil {
		t.Fatalf("form@upgrade failed: %v\n%s", err, upPlan)
	}
	formRec, _ := readCopyRecord(filepath.Join(dir, "components/form"))
	if formRec.ModuleVersion != "v1.2.0" {
		t.Errorf("form after @upgrade = %s, want v1.2.0", formRec.ModuleVersion)
	}
}

func TestUpdateComponentAmbiguousNameAndDirResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go vet")
	}
	proxy := t.TempDir()
	mit := readFixtureLicense(t, "License/MIT/pristine.txt")
	for _, mod := range []string{"example.com/alpha", "example.com/beta"} {
		for _, ver := range []string{"v1.0.0", "v1.1.0"} {
			addCopyZipProxyModule(t, proxy, mod, ver, "module "+mod+"\n\ngo 1.26\n", map[string][]byte{
				"LICENSE":           mit,
				"nsgo-module.yaml":  []byte("version: 1\ncomponents:\n  badge: ui/badge\n"),
				"ui/badge/badge.go": []byte("package badge\n\nconst Source = \"" + mod + "@" + ver + "\"\n"),
			})
		}
	}
	setUpdateProxy(t, proxy)

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module example.com/twoapp\n\ngo 1.27.3\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})

	if _, err := runAdd(t, dir, addOptions{yes: true, dir: "components/alpha/badge"}, nil, "example.com/alpha@v1.0.0#badge"); err != nil {
		t.Fatalf("copy alpha badge: %v", err)
	}
	if _, err := runAdd(t, dir, addOptions{yes: true, dir: "components/beta/badge"}, nil, "example.com/beta@v1.0.0#badge"); err != nil {
		t.Fatalf("copy beta badge: %v", err)
	}

	// Ambiguous bare name asks for the directory, naming both copies.
	_, err := runAdd(t, dir, addOptions{yes: true}, nil, "badge@v1.1.0")
	if err == nil || !strings.Contains(err.Error(), "components/alpha/badge") || !strings.Contains(err.Error(), "components/beta/badge") {
		t.Fatalf("ambiguous name error = %v, want both copy directories", err)
	}

	// Directory@version updates only the named copy.
	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "components/alpha/badge@v1.1.0"); err != nil {
		t.Fatalf("dir@version update failed: %v", err)
	}
	alphaRec, _ := readCopyRecord(filepath.Join(dir, "components/alpha/badge"))
	betaRec, _ := readCopyRecord(filepath.Join(dir, "components/beta/badge"))
	if alphaRec.ModuleVersion != "v1.1.0" || betaRec.ModuleVersion != "v1.0.0" {
		t.Errorf("records after dir update: alpha=%s beta=%s; want v1.1.0 and v1.0.0", alphaRec.ModuleVersion, betaRec.ModuleVersion)
	}
}

func TestUpdateComponentRollbackSymlinkAndUnavailableBase(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go vet")
	}
	proxy := updateLiteFixtureEnv(t)
	dir := makeUpdateLiteApp(t, "example.com/rollapp")

	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "example.com/upkit@v1.0.0#form"); err != nil {
		t.Fatalf("initial copy failed: %v", err)
	}

	// Snapshot pre-update bytes.
	origField, _ := os.ReadFile(filepath.Join(dir, "components/field/field.go"))
	origRec, _ := os.ReadFile(filepath.Join(dir, "components/field/nsgo-copy.yaml"))
	origMod, _ := os.ReadFile(filepath.Join(dir, "go.mod"))

	// 1. Mid-apply failure rolls back every modified/added/deleted file and go.mod.
	origRename := copyUpdateApplyRename
	calls := 0
	copyUpdateApplyRename = func(root *os.Root, oldName, newName string) error {
		calls++
		if calls == 2 {
			return errors.New("injected rename failure")
		}
		return origRename(root, oldName, newName)
	}
	_, err := runAdd(t, dir, addOptions{yes: true}, nil, "form@v1.1.0")
	copyUpdateApplyRename = origRename
	if err == nil || !strings.Contains(err.Error(), "injected rename failure") {
		t.Fatalf("expected injected rename failure, got: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "components/field/field.go")); !bytes.Equal(got, origField) {
		t.Errorf("field.go was not rolled back")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "components/field/nsgo-copy.yaml")); !bytes.Equal(got, origRec) {
		t.Errorf("nsgo-copy.yaml was not rolled back")
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "go.mod")); !bytes.Equal(got, origMod) {
		t.Errorf("go.mod was not rolled back")
	}
	if _, err := os.Stat(filepath.Join(dir, "components/field/drop_unedited.go")); err != nil {
		t.Errorf("drop_unedited.go was not restored on rollback: %v", err)
	}

	// 2. Recorded symlink is refused before any write.
	if runtime.GOOS != "windows" {
		symDir := makeUpdateLiteApp(t, "example.com/symapp")
		if _, err := runAdd(t, symDir, addOptions{yes: true}, nil, "example.com/upkit@v1.0.0#field"); err != nil {
			t.Fatalf("initial copy for symlink test failed: %v", err)
		}
		outside := filepath.Join(symDir, "outside.go")
		if err := os.WriteFile(outside, []byte("package outside\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(symDir, "components/field/extra.go")
		_ = os.Remove(target)
		if err := os.Symlink(outside, target); err != nil {
			t.Fatal(err)
		}
		if _, err := runAdd(t, symDir, addOptions{yes: true}, nil, "field@v1.1.0"); err == nil || !strings.Contains(err.Error(), "symlinks are not allowed") {
			t.Errorf("symlink refusal = %v, want symlinks are not allowed", err)
		}
	}

	// 3. Unavailable recorded version counts every file as edited and explains why.
	unavailDir := makeUpdateLiteApp(t, "example.com/unavailapp")
	if _, err := runAdd(t, unavailDir, addOptions{yes: true}, nil, "example.com/upkit@v1.0.0#field"); err != nil {
		t.Fatalf("initial copy for unavailable test failed: %v", err)
	}
	// Remove v1.0.0 from the proxy and use a fresh empty GOMODCACHE so v1.0.0 cannot be fetched.
	vDir := filepath.Join(proxy, "example.com/upkit/@v")
	for _, suffix := range []string{".zip", ".info", ".mod"} {
		_ = os.Remove(filepath.Join(vDir, "v1.0.0"+suffix))
	}
	t.Setenv("GOMODCACHE", t.TempDir())
	plan, err := runAdd(t, unavailDir, addOptions{yes: true}, nil, "field@v1.1.0")
	if err == nil {
		t.Fatalf("expected refusal when recorded version is unavailable\n%s", plan)
	}
	if !strings.Contains(plan, "recorded version v1.0.0 unavailable") {
		t.Errorf("plan did not explain why recorded version was unavailable:\n%s", plan)
	}
}

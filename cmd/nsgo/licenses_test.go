package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/licenseclassifier/v2/assets"
)

func TestLicenseNoticesFollowBuildTags(t *testing.T) {
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	dir := t.TempDir()
	app := filepath.Join(dir, "app")
	dep := filepath.Join(dir, "dep")
	files := map[string]string{
		filepath.Join(app, "go.mod"):        "module go.example/app\ngo 1.27.3\nrequire go.example/dep v0.0.0\nreplace go.example/dep => ../dep\n",
		filepath.Join(app, "main.go"):       "package main\nfunc main() {}\n",
		filepath.Join(app, "extra.go"):      "//go:build extras\n\npackage main\nimport _ \"go.example/dep/sub\"\n",
		filepath.Join(dep, "go.mod"):        "module go.example/dep\ngo 1.27.3\n",
		filepath.Join(dep, "sub", "dep.go"): "package sub\n",
		filepath.Join(dep, "LICENSE"):       "A test license specific to this dependency.\n",
	}
	for name, data := range files {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p := &project{Dir: app}
	if _, err := p.licenseNoticesFile(t.Context(), "", nil, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.thirdPartyNotices, "go.example/dep") {
		t.Fatal("included a dependency excluded by the build tags")
	}

	tags := []string{"extras"}
	if _, err := p.licenseNoticesFile(t.Context(), "", nil, tags, nil, ""); err == nil || !strings.Contains(err.Error(), "go.example/dep has unrecognized license text in LICENSE") {
		t.Fatalf("unrecognized license = %v", err)
	}
	p.Licenses.Allow = []string{"go.example/dep"}
	if _, err := p.licenseNoticesFile(t.Context(), "", nil, tags, nil, ""); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"go.example/dep v0.0.0 (local replacement)", files[filepath.Join(dep, "LICENSE")]} {
		if !strings.Contains(p.thirdPartyNotices, want) {
			t.Errorf("notice text lacks %q", want)
		}
	}
	if strings.Contains(p.thirdPartyNotices, dir) {
		t.Fatal("notice text reveals the local replacement path")
	}
	p.Mobile.ExcludeResources = []string{noticesName}
	var resource string
	if err := walkMobileResources(p, func(name string, data []byte) error {
		if name == noticesName {
			resource = string(data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if resource != p.thirdPartyNotices {
		t.Fatal("the package's notice resource differs from the generated notices")
	}
}

func TestLicenseNoticesCopyleftPolicy(t *testing.T) {
	data, err := assets.ReadLicenseFile("License/MPL-2.0/pristine.txt")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "LICENSE"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	const name = "go.example/module"
	sources := map[string]*licenseSource{
		name: {name: name, label: name, root: dir, dirs: map[string]bool{dir: true}},
	}
	p := &project{Licenses: licensePolicy{Copyleft: "error"}}
	if _, err := p.renderLicenseNotices(t.Context(), sources, nil); err == nil || !strings.Contains(err.Error(), "copyleft license text (MPL-2.0)") {
		t.Fatalf("copyleft license = %v", err)
	}
	p.Licenses.Copyleft = "warn"
	if text, err := p.renderLicenseNotices(t.Context(), sources, nil); err != nil || !strings.Contains(text, string(data)) {
		t.Fatalf("copyleft warning did not preserve the text, err = %v", err)
	}
}

func TestLicenseNoticesWorkspaceModules(t *testing.T) {
	t.Setenv("GOWORK", "")
	t.Setenv("GOFLAGS", "")
	copyleftText, err := assets.ReadLicenseFile("License/MPL-2.0/pristine.txt")
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	app := filepath.Join(dir, "app")
	lib := filepath.Join(dir, "lib")
	dep := filepath.Join(dir, "dep")
	files := map[string]string{
		filepath.Join(dir, "go.work"):           "go 1.27.3\n\nuse (\n\t./app\n\t./lib\n)\n\nreplace go.example/dep => ./dep\n",
		filepath.Join(app, "go.mod"):            "module go.example/app\n\ngo 1.27.3\n\nrequire go.example/dep v0.0.0\n",
		filepath.Join(app, "main.go"):           "package main\n\nimport _ \"go.example/lib/sub\"\n\nfunc main() {}\n",
		filepath.Join(app, "dep.go"):            "//go:build withdep\n\npackage main\n\nimport _ \"go.example/dep/sub\"\n",
		filepath.Join(lib, "go.mod"):            "module go.example/lib\n\ngo 1.27.3\n",
		filepath.Join(lib, "sub", "sub.go"):     "package sub\n",
		filepath.Join(lib, "unused", "u.go"):    "package unused\n",
		filepath.Join(lib, "unused", "LICENSE"): "Unused package license text.\n",
		filepath.Join(dep, "go.mod"):            "module go.example/dep\n\ngo 1.27.3\n",
		filepath.Join(dep, "sub", "dep.go"):     "package sub\n",
	}
	for name, data := range files {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// A sibling workspace module without a license file is first-party:
	// no error, and omitted from the notices when it has no license documents.
	p := &project{Dir: app, Licenses: licensePolicy{Unknown: "error", Copyleft: "error"}}
	if _, err := p.licenseNoticesFile(t.Context(), "", nil, nil, nil, ""); err != nil {
		t.Fatalf("workspace sibling without license failed: %v", err)
	}
	if strings.Contains(p.thirdPartyNotices, "go.example/lib") {
		t.Fatalf("workspace sibling without license documents should be omitted:\n%s", p.thirdPartyNotices)
	}

	// A nested license document in an imported package of a workspace module is
	// included in notices without triggering unknown or copyleft policy errors.
	nestedLicense := filepath.Join(lib, "sub", "LICENSE")
	if err := os.WriteFile(nestedLicense, copyleftText, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.licenseNoticesFile(t.Context(), "", nil, nil, nil, ""); err != nil {
		t.Fatalf("workspace sibling with nested copyleft license failed: %v", err)
	}
	for _, want := range []string{"go.example/lib", "sub/LICENSE", string(copyleftText)} {
		if !strings.Contains(p.thirdPartyNotices, want) {
			t.Errorf("notices lack %q", want)
		}
	}
	for _, unwanted := range []string{"License identification incomplete.", "unused/LICENSE"} {
		if strings.Contains(p.thirdPartyNotices, unwanted) {
			t.Errorf("notices should not contain %q", unwanted)
		}
	}

	// A root license with unrecognized text in a workspace module is also
	// included without failing unknown-license checks.
	if err := os.Remove(nestedLicense); err != nil {
		t.Fatal(err)
	}
	const customLicense = "Proprietary workspace module license.\n"
	if err := os.WriteFile(filepath.Join(lib, "LICENSE"), []byte(customLicense), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.licenseNoticesFile(t.Context(), "", nil, nil, nil, ""); err != nil {
		t.Fatalf("workspace sibling with custom license failed: %v", err)
	}
	for _, want := range []string{"go.example/lib", customLicense} {
		if !strings.Contains(p.thirdPartyNotices, want) {
			t.Errorf("notices lack %q", want)
		}
	}
	if strings.Contains(p.thirdPartyNotices, "License identification incomplete.") {
		t.Error("workspace sibling should not be marked incomplete")
	}

	// A module replaced by go.work (not listed in use) stays third-party and is
	// still subject to unknown-license checks.
	tags := []string{"withdep"}
	if _, err := p.licenseNoticesFile(t.Context(), "", nil, tags, nil, ""); err == nil || !strings.Contains(err.Error(), "go.example/dep has no recognized license text") {
		t.Fatalf("replaced third-party module in go.work = %v", err)
	}
	p.Licenses.Allow = []string{"go.example/dep"}
	if _, err := p.licenseNoticesFile(t.Context(), "", nil, tags, nil, ""); err != nil {
		t.Fatalf("allowed replaced third-party module failed: %v", err)
	}
}

func TestAndroidReviewNotices(t *testing.T) {
	var data bytes.Buffer
	z := zip.NewWriter(&data)
	f, err := z.Create("META-INF/nsgo-play-review-NOTICE.txt")
	if err != nil {
		t.Fatal(err)
	}
	const notice = "Play Core Software Development Kit Terms of Service\nSDK license text\n"
	if _, err := f.Write([]byte(notice)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := androidReviewNotices(data.Bytes())
	if err != nil || !strings.Contains(got, notice) {
		t.Fatalf("SDK notices lost from packaged runtime: %q, %v", got, err)
	}
}

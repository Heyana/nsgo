package main

import (
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
	if _, err := p.licenseNoticesFile(t.Context(), nil, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(p.thirdPartyNotices, "go.example/dep") {
		t.Fatal("included a dependency excluded by the build tags")
	}

	tags := []string{"extras"}
	if _, err := p.licenseNoticesFile(t.Context(), nil, tags, nil, ""); err == nil || !strings.Contains(err.Error(), "go.example/dep has unrecognized license text in LICENSE") {
		t.Fatalf("unrecognized license = %v", err)
	}
	p.Licenses.Allow = []string{"go.example/dep"}
	if _, err := p.licenseNoticesFile(t.Context(), nil, tags, nil, ""); err != nil {
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

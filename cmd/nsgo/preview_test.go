package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPreviewHostOnlyUsesTaggedFixtures(t *testing.T) {
	app := t.TempDir()
	component := filepath.Join(app, "components")
	host := filepath.Join(component, ".nsgo-preview-test")
	if err := os.MkdirAll(host, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{
		"go.mod":                 "module example.test/app\n\ngo 1.27\n",
		"nsgo.yaml":              "name: Demo\ntheme: palette.yml\n",
		"palette.yml":            "light:\n  accent: '#336699'\n",
		"components/ordinary.go": "package components\nfunc PreviewOrdinary() {}\n",
		"components/fixture.go":  "//go:build nsgopreview\n\npackage components\nfunc PreviewTwo() {}\nfunc PreviewOne() {}\n",
	} {
		if err := os.WriteFile(filepath.Join(app, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if found := previewProjectDir(component, app); found != app {
		t.Fatalf("app config = %q, want %q", found, app)
	}
	ps := &previewSession{source: component, projectDir: app, host: host}
	p, err := ps.project()
	if err != nil {
		t.Fatal(err)
	}
	if p.Dir != host || p.resourceRoot() != filepath.Join(app, "resources") || p.Theme != filepath.Join(app, "palette.yml") {
		t.Fatalf("host sources = %q, resource root = %q, theme = %q", p.Dir, p.resourceRoot(), p.Theme)
	}
	if !slices.Equal(p.Build.Tags, []string{previewTag}) {
		t.Fatalf("preview tags = %v", p.Build.Tags)
	}
	p.Dev.Target = "web"
	if err := ps.writeHost(context.Background(), p, "zh-Hans"); err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(filepath.Join(host, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	code := string(generated)
	for _, literal := range []string{"components.PreviewOne", "components.PreviewTwo", "\"example.test/app/components\"", "#336699", `initialLocale = "zh-Hans"`} {
		if !strings.Contains(code, literal) {
			t.Errorf("generated host omitted %q", literal)
		}
	}
	if strings.Contains(code, "PreviewOrdinary") || strings.Index(code, "components.PreviewOne") > strings.Index(code, "components.PreviewTwo") {
		t.Errorf("fixtures were not filtered and sorted")
	}

	file := filepath.Join(component, "fixture.go")
	if err := os.WriteFile(file, []byte("//go:build nsgopreview\n\npackage components\nfunc PreviewReplacement() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ps.writeHost(context.Background(), p, ""); err != nil {
		t.Fatal(err)
	}
	generated, err = os.ReadFile(filepath.Join(host, "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(generated), "components.PreviewOne") || !strings.Contains(string(generated), "components.PreviewReplacement") {
		t.Error("host generation kept the old fixture")
	}
}

package project_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/project"
)

func TestLoadQuickLookDefaultsFromFileTypes(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `name: Scenra
id: dev.example.scenra
file_types:
  - name: Scenra Scene
    extensions: [scenra]
  - name: Scenra Library
    extensions: [scenralib]
    id: dev.example.scenra.library
quicklook:
  main: ./cmd/quicklook
sign:
  quicklook_profile: certs/QuickLook.provisionprofile
`)
	cfg, err := project.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.QuickLook == nil {
		t.Fatal("QuickLook is nil")
	}
	if cfg.QuickLook.Main != "./cmd/quicklook" {
		t.Errorf("QuickLook.Main = %q, want ./cmd/quicklook", cfg.QuickLook.Main)
	}
	if cfg.QuickLook.ID != "dev.example.scenra.quicklook" {
		t.Errorf("QuickLook.ID = %q, want dev.example.scenra.quicklook", cfg.QuickLook.ID)
	}
	wantTypes := []string{"dev.example.scenra.scenra", "dev.example.scenra.library"}
	if !slices.Equal(cfg.QuickLook.Types, wantTypes) {
		t.Errorf("QuickLook.Types = %v, want %v", cfg.QuickLook.Types, wantTypes)
	}
	wantProfile := filepath.Join(dir, "certs/QuickLook.provisionprofile")
	if cfg.Sign.QuickLookProfile != wantProfile {
		t.Errorf("Sign.QuickLookProfile = %q, want %q", cfg.Sign.QuickLookProfile, wantProfile)
	}
}

func TestLoadQuickLookExplicitIDAndTypes(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `name: Scenra
id: dev.example.scenra
quicklook:
  main: ./preview
  id: dev.example.scenra.previewer
  types:
    - dev.example.scenra.scenra
    - public.json
`)
	cfg, err := project.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.QuickLook.ID != "dev.example.scenra.previewer" {
		t.Errorf("QuickLook.ID = %q", cfg.QuickLook.ID)
	}
	if want := []string{"dev.example.scenra.scenra", "public.json"}; !slices.Equal(cfg.QuickLook.Types, want) {
		t.Errorf("QuickLook.Types = %v, want %v", cfg.QuickLook.Types, want)
	}
}

func TestLoadQuickLookValidationErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "missing main",
			yaml: "name: App\nid: dev.example.app\nquicklook:\n  types: [dev.example.app.doc]\n",
			want: "quicklook.main",
		},
		{
			name: "wildcard main",
			yaml: "name: App\nid: dev.example.app\nquicklook:\n  main: ./...\n  types: [dev.example.app.doc]\n",
			want: "quicklook.main",
		},
		{
			name: "invalid id outside app id",
			yaml: "name: App\nid: dev.example.app\nquicklook:\n  main: ./ql\n  id: com.other.quicklook\n  types: [dev.example.app.doc]\n",
			want: "quicklook.id",
		},
		{
			name: "collides with widgets",
			yaml: "name: App\nid: dev.example.app\nwidgets:\n  - kind: stats\nquicklook:\n  main: ./ql\n  id: dev.example.app.widgets\n  types: [dev.example.app.doc]\n",
			want: "collides with the widget extension",
		},
		{
			name: "no types and no file_types",
			yaml: "name: App\nid: dev.example.app\nquicklook:\n  main: ./ql\n",
			want: "quicklook needs types or file_types",
		},
		{
			name: "invalid uti",
			yaml: "name: App\nid: dev.example.app\nquicklook:\n  main: ./ql\n  types: [notauti]\n",
			want: "not a uniform type identifier",
		},
		{
			name: "duplicate uti",
			yaml: "name: App\nid: dev.example.app\nquicklook:\n  main: ./ql\n  types: [dev.example.app.doc, dev.example.app.doc]\n",
			want: "listed twice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeYAML(t, dir, tc.yaml)
			_, err := project.Load(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

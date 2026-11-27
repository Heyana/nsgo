package main

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestParseModuleManifest(t *testing.T) {
	maxNotes := strings.Repeat("a", maxModuleNotesBytes)
	overNotes := strings.Repeat("a", maxModuleNotesBytes+1)
	maxName := strings.Repeat("a", 64)
	overName := strings.Repeat("a", 65)

	tests := []struct {
		name         string
		yaml         string
		modPath      string
		requiredNSGo string
		currentNSGo  string
		wantErr      string
		check        func(t *testing.T, m *moduleManifest)
	}{
		{
			name: "minimal version 1",
			yaml: "version: 1\n",
			check: func(t *testing.T, m *moduleManifest) {
				if m.Version != 1 || m.hasImport || len(m.expandedTargets) != 0 || m.Notes != "" || len(m.Components) != 0 {
					t.Errorf("unexpected manifest: %+v", m)
				}
				if !m.supportsTarget("android_arm64") {
					t.Error("manifest without targets should support any target")
				}
			},
		},
		{
			name: "full valid manifest",
			yaml: `version: 1
import: [., player, player/ui]
targets: [macos, windows_x64, linux_x64, ios, ios_simulator]
notes: |
  FFmpeg's libraries are LGPL 2.1: offer their source beside your downloads.
  Credit FFmpeg and allow replacing them.
components:
  chat: ui/chat
  file-picker: ui/filepicker
  ` + maxName + `: ui/max
  a: ui/a
`,
			check: func(t *testing.T, m *moduleManifest) {
				wantImports := []string{"example.com/lib", "example.com/lib/player", "example.com/lib/player/ui"}
				if !slices.Equal(m.importPaths("example.com/lib"), wantImports) {
					t.Errorf("importPaths = %v, want %v", m.importPaths("example.com/lib"), wantImports)
				}
				if !m.supportsTarget("macos_arm64") || !m.supportsTarget("macos_x64") || !m.supportsTarget("macos_universal") || !m.supportsTarget("ios") {
					t.Errorf("expected macOS/iOS support: %v", m.expandedTargets)
				}
				if m.supportsTarget("android_arm64") || m.supportsTarget("web") {
					t.Errorf("did not expect android_arm64 or web support: %v", m.expandedTargets)
				}
				wantMissing := []string{"android_arm64", "web"}
				if got := m.missingTargets([]string{"macos_arm64", "android_arm64", "ios", "web"}); !slices.Equal(got, wantMissing) {
					t.Errorf("missingTargets = %v, want %v", got, wantMissing)
				}
				lines := m.formattedNotes()
				if len(lines) != 2 || lines[0] != "FFmpeg's libraries are LGPL 2.1: offer their source beside your downloads." {
					t.Errorf("formattedNotes = %q", lines)
				}
				if got := m.componentNames(); len(got) != 4 || got[0] != "a" || got[1] != maxName {
					t.Errorf("componentNames = %v", got)
				}
			},
		},
		{
			name: "explicit empty import suppresses fallback",
			yaml: "version: 1\nimport: []\n",
			check: func(t *testing.T, m *moduleManifest) {
				if !m.hasImport || len(m.Import) != 0 {
					t.Errorf("got hasImport=%v Import=%v", m.hasImport, m.Import)
				}
			},
		},
		{
			name: "macos_arm64 alone lacks macos_universal",
			yaml: "version: 1\ntargets: [macos_arm64]\n",
			check: func(t *testing.T, m *moduleManifest) {
				if !m.supportsTarget("macos_arm64") || m.supportsTarget("macos_universal") {
					t.Errorf("macos_arm64 alone should not cover macos_universal")
				}
			},
		},
		{
			name: "target groups all tui miniprogram",
			yaml: "version: 1\ntargets: [all, miniprogram]\n",
			check: func(t *testing.T, m *moduleManifest) {
				if !m.supportsTarget("web") || !m.supportsTarget("miniprogram_wechat") || !m.supportsTarget("tui_macos_arm64") {
					t.Errorf("expandedTargets = %v", m.expandedTargets)
				}
			},
		},
		{
			name: "notes at exact 4 KiB limit and CRLF",
			yaml: "version: 1\nnotes: \"" + maxNotes + "\"\n",
			check: func(t *testing.T, m *moduleManifest) {
				if len(m.Notes) != maxModuleNotesBytes {
					t.Errorf("notes len = %d", len(m.Notes))
				}
			},
		},
		{
			name: "notes with CRLF normalized to LF",
			yaml: "version: 1\nnotes: \"line 1\\r\\nline 2\"\n",
			check: func(t *testing.T, m *moduleManifest) {
				if m.Notes != "line 1\nline 2" {
					t.Errorf("notes = %q", m.Notes)
				}
			},
		},
		{
			name:         "known fields with newer NSGo still succeed",
			yaml:         "version: 1\nimport: [player]\n",
			modPath:      "github.com/acme/media",
			requiredNSGo: "v0.5.0",
			currentNSGo:  "v0.4.0",
		},
		{
			name:    "empty file",
			yaml:    "   \n# comment only\n",
			wantErr: "empty YAML",
		},
		{
			name:    "exceeds 1 MiB",
			yaml:    "version: 1\n#" + strings.Repeat("x", maxModuleManifestBytes),
			wantErr: "exceeds 1 MiB",
		},
		{
			name:    "multiple YAML documents",
			yaml:    "version: 1\n---\nversion: 1\n",
			wantErr: "multiple YAML documents",
		},
		{
			name:    "non-mapping root",
			yaml:    "- one\n- two\n",
			wantErr: "must be a YAML mapping",
		},
		{
			name:    "YAML anchor refused",
			yaml:    "version: 1\nimport: &imp [player]\n",
			wantErr: "anchors are not allowed",
		},
		{
			name:    "YAML alias refused",
			yaml:    "version: 1\nimport: &a [player]\ntargets: *a\n",
			wantErr: "anchors are not allowed",
		},
		{
			name:    "YAML merge key refused",
			yaml:    "version: 1\ncomponents:\n  <<: { chat: ui/chat }\n",
			wantErr: "merge keys are not allowed",
		},
		{
			name:    "duplicate key refused",
			yaml:    "version: 1\nversion: 1\n",
			wantErr: "duplicate key",
		},
		{
			name:    "missing version",
			yaml:    "import: [player]\n",
			wantErr: "version: 1 is required",
		},
		{
			name:         "unsupported version with same NSGo",
			yaml:         "version: 2\n",
			modPath:      "github.com/acme/lib",
			requiredNSGo: "v0.4.0",
			currentNSGo:  "v0.4.0",
			wantErr:      "version is 2",
		},
		{
			name:         "unsupported version with newer NSGo names required version",
			yaml:         "version: 2\n",
			modPath:      "github.com/acme/lib",
			requiredNSGo: "v0.5.0",
			currentNSGo:  "v0.4.0",
			wantErr:      "github.com/acme/lib needs NSGo v0.5.0: go get github.com/nsgo-dev/nsgo@v0.5.0",
		},
		{
			name:         "unknown field with same NSGo",
			yaml:         "version: 1\nskills: [foo]\n",
			modPath:      "github.com/acme/lib",
			requiredNSGo: "v0.4.0",
			currentNSGo:  "v0.4.0",
			wantErr:      `unknown field "skills"`,
		},
		{
			name:         "unknown field with older NSGo requirement",
			yaml:         "version: 1\nskills: [foo]\n",
			modPath:      "github.com/acme/lib",
			requiredNSGo: "v0.3.0",
			currentNSGo:  "v0.4.0",
			wantErr:      `unknown field "skills"`,
		},
		{
			name:         "unknown field with no NSGo requirement",
			yaml:         "version: 1\nskills: [foo]\n",
			modPath:      "github.com/acme/lib",
			requiredNSGo: "",
			currentNSGo:  "v0.4.0",
			wantErr:      `unknown field "skills"`,
		},
		{
			name:         "unknown field with newer NSGo fails naming required version",
			yaml:         "version: 1\nskills: [foo]\n",
			modPath:      "github.com/acme/lib",
			requiredNSGo: "v0.5.0",
			currentNSGo:  "v0.4.0",
			wantErr:      "github.com/acme/lib needs NSGo v0.5.0: go get github.com/nsgo-dev/nsgo@v0.5.0",
		},
		{
			name:    "import empty entry",
			yaml:    "version: 1\nimport: [\"\"]\n",
			wantErr: "clean relative directory",
		},
		{
			name:    "import parent traversal",
			yaml:    "version: 1\nimport: [../other]\n",
			wantErr: "clean relative directory",
		},
		{
			name:    "import absolute path",
			yaml:    "version: 1\nimport: [/etc]\n",
			wantErr: "clean relative directory",
		},
		{
			name:    "import uncleaned dot prefix",
			yaml:    "version: 1\nimport: [./player]\n",
			wantErr: "clean relative directory",
		},
		{
			name:    "import backslash",
			yaml:    "version: 1\nimport: [\"a\\\\b\"]\n",
			wantErr: "clean relative directory",
		},
		{
			name:    "import duplicate",
			yaml:    "version: 1\nimport: [player, player]\n",
			wantErr: `import repeats "player"`,
		},
		{
			name:    "targets empty list",
			yaml:    "version: 1\ntargets: []\n",
			wantErr: "targets must not be empty",
		},
		{
			name:    "targets unknown target",
			yaml:    "version: 1\ntargets: [macos, plan9]\n",
			wantErr: `unknown target "plan9"`,
		},
		{
			name:    "notes exceed 4 KiB",
			yaml:    "version: 1\nnotes: \"" + overNotes + "\"\n",
			wantErr: "notes exceed 4 KiB",
		},
		{
			name:    "notes with ESC control char",
			yaml:    "version: 1\nnotes: \"hello\\e[31mred\"\n",
			wantErr: "disallowed character",
		},
		{
			name:    "notes with NUL control char",
			yaml:    "version: 1\nnotes: \"hello\\0world\"\n",
			wantErr: "disallowed character",
		},
		{
			name:    "notes with TAB control char",
			yaml:    "version: 1\nnotes: \"hello\\tworld\"\n",
			wantErr: "disallowed character",
		},
		{
			name:    "notes with bare CR",
			yaml:    "version: 1\nnotes: \"safe\\rfake\"\n",
			wantErr: "disallowed character",
		},
		{
			name:    "notes with bidi override format char",
			yaml:    "version: 1\nnotes: \"hello \\u202e world\"\n",
			wantErr: "disallowed character",
		},
		{
			name:    "notes with zero-width space format char",
			yaml:    "version: 1\nnotes: \"hello\\u200bworld\"\n",
			wantErr: "disallowed character",
		},
		{
			name:    "notes with BOM format char",
			yaml:    "version: 1\nnotes: \"\\ufeffhello\"\n",
			wantErr: "disallowed character",
		},
		{
			name:    "component name empty",
			yaml:    "version: 1\ncomponents:\n  \"\": ui/chat\n",
			wantErr: "component name",
		},
		{
			name:    "component name leading hyphen",
			yaml:    "version: 1\ncomponents:\n  -chat: ui/chat\n",
			wantErr: "component name",
		},
		{
			name:    "component name trailing hyphen",
			yaml:    "version: 1\ncomponents:\n  chat-: ui/chat\n",
			wantErr: "component name",
		},
		{
			name:    "component name uppercase",
			yaml:    "version: 1\ncomponents:\n  Chat: ui/chat\n",
			wantErr: "component name",
		},
		{
			name:    "component name dot",
			yaml:    "version: 1\ncomponents:\n  chat.v1: ui/chat\n",
			wantErr: "component name",
		},
		{
			name:    "component name underscore",
			yaml:    "version: 1\ncomponents:\n  file_picker: ui/filepicker\n",
			wantErr: "component name",
		},
		{
			name:    "component name over 64 chars",
			yaml:    "version: 1\ncomponents:\n  " + overName + ": ui/chat\n",
			wantErr: "component name",
		},
		{
			name:    "component dir root dot refused",
			yaml:    "version: 1\ncomponents:\n  root: .\n",
			wantErr: "below the module root",
		},
		{
			name:    "component dir parent traversal refused",
			yaml:    "version: 1\ncomponents:\n  bad: ../chat\n",
			wantErr: "below the module root",
		},
		{
			name:    "component dir absolute refused",
			yaml:    "version: 1\ncomponents:\n  bad: /ui/chat\n",
			wantErr: "below the module root",
		},
		{
			name:    "component dir backslash refused",
			yaml:    "version: 1\ncomponents:\n  bad: \"ui\\\\chat\"\n",
			wantErr: "below the module root",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := parseModuleManifest([]byte(tc.yaml), tc.modPath, tc.requiredNSGo, tc.currentNSGo)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("parseModuleManifest error = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.check != nil {
				tc.check(t, m)
			}
		})
	}
}

func TestReadModuleManifestThroughRoot(t *testing.T) {
	t.Run("missing file returns nil", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"go.mod": "module example.com/lib\n\ngo 1.26\n"})
		m, err := readModuleManifest(dir, "example.com/lib", "v0.4.0")
		if err != nil || m != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", m, err)
		}
	})

	t.Run("reads go.mod requirement for unknown field check", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"go.mod":           "module example.com/lib\n\ngo 1.26\n\nrequire github.com/nsgo-dev/nsgo v0.6.0\n",
			"nsgo-module.yaml": "version: 1\nfuture_key: true\n",
		})
		_, err := readModuleManifest(dir, "example.com/lib", "v0.4.0")
		want := "example.com/lib needs NSGo v0.6.0: go get github.com/nsgo-dev/nsgo@v0.6.0"
		if err == nil || err.Error() != want {
			t.Fatalf("got %v, want %q", err, want)
		}
	})

	t.Run("directory named nsgo-module.yaml refused", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, moduleManifestName), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := readModuleManifest(dir, "example.com/lib", "v0.4.0"); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("got %v, want regular file error", err)
		}
	})

	t.Run("symlink escaping module root refused", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need extra privileges on Windows")
		}
		outside := t.TempDir()
		secret := filepath.Join(outside, "nsgo-module.yaml")
		if err := os.WriteFile(secret, []byte("version: 1\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		if err := os.Symlink(secret, filepath.Join(dir, moduleManifestName)); err != nil {
			t.Fatal(err)
		}
		if _, err := readModuleManifest(dir, "example.com/lib", "v0.4.0"); err == nil {
			t.Fatal("symlink escaping module root was allowed")
		}
	})
}

func FuzzParseModuleManifest(f *testing.F) {
	seeds := []string{
		"version: 1\n",
		"version: 1\nimport: [., player]\ntargets: [macos, ios]\nnotes: hello\ncomponents:\n  chat: ui/chat\n",
		"version: 1\nimport: &a [player]\ntargets: *a\n",
		"version: 1\ncomponents:\n  <<: { a: b }\n",
		"version: 1\n---\nversion: 1\n",
		"version: 1\nnotes: \"bad\\u202echar\"\n",
		"version: 1\nunknown: 1\n",
		"",
		"- list\n",
	}
	for _, s := range seeds {
		f.Add([]byte(s), "v0.5.0", "v0.4.0")
	}
	f.Fuzz(func(t *testing.T, data []byte, req, cur string) {
		m, err := parseModuleManifest(data, "example.com/fuzz", req, cur)
		if err != nil {
			return
		}
		if m.Version != moduleManifestVersion {
			t.Fatalf("accepted version %d", m.Version)
		}
		if len(m.Notes) > maxModuleNotesBytes {
			t.Fatalf("accepted notes of length %d", len(m.Notes))
		}
		for _, dir := range m.Import {
			if !validManifestRelPath(dir, true) {
				t.Fatalf("accepted invalid import dir %q", dir)
			}
		}
		for name, dir := range m.Components {
			if !validComponentName(name) {
				t.Fatalf("accepted invalid component name %q", name)
			}
			if !validManifestRelPath(dir, false) {
				t.Fatalf("accepted invalid component dir %q", dir)
			}
		}
	})
}

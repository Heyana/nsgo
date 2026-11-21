package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"text/template"
)

func TestGamePackageMetadata(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"nsgo.yaml": "name: Game\ngame: true\ngamepad: true\nlinux:\n  categories: [ActionGame]\n"})
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Game || !p.Gamepad {
		t.Fatalf("game = %v, gamepad = %v", p.Game, p.Gamepad)
	}
	if got := p.linuxCategories(); !slices.Equal(got, []string{"ActionGame", "Game"}) {
		t.Errorf("Linux categories = %v", got)
	}
	for _, target := range []struct {
		name  string
		plist *template.Template
		mode  bool
	}{
		{"macOS", macPlist, true},
		{"iOS", iosPlist, true},
		{"tvOS", tvosPlist, false},
	} {
		t.Run(target.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Info.plist")
			if err := writePlist(path, p, target.plist, p.gamePlist(target.mode)); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			for _, key := range []string{"GCSupportsControllerUserInteraction", "GCSupportedGameControllers", "ProfileName", "ExtendedGamepad"} {
				if !strings.Contains(got, key) {
					t.Errorf("Info.plist lacks %s", key)
				}
			}
			for _, key := range []string{"GCSupportsGameMode", "LSSupportsGameMode"} {
				if strings.Contains(got, key) != target.mode {
					t.Errorf("Info.plist has unexpected presence of %s", key)
				}
			}
			if target.name == "macOS" && !strings.Contains(got, "<key>LSApplicationCategoryType</key><string>public.app-category.games</string>") {
				t.Errorf("macOS Info.plist lacks the Games category")
			}
		})
	}
	p.MacOS.Category = "public.app-category.action-games"
	p.Apple.Info = map[string]any{"LSSupportsGameMode": false}
	if p.MacOSCategory() != p.MacOS.Category || strings.Contains(p.gamePlist(true), "LSSupportsGameMode") {
		t.Fatal("explicit Apple settings did not take precedence")
	}
	p = &project{}
	if p.MacOSCategory() != "" || p.gamePlist(true) != "" || len(p.linuxCategories()) != 0 {
		t.Errorf("an unclassified app has game metadata")
	}
}

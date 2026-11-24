package main

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSteamBuildChannel(t *testing.T) {
	p, _ := updatingProject(t, "")
	p.Build.Installers = new(true)
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	flags := bindBuildFlags(fs)
	if err := fs.Parse([]string{"-channel", "steam"}); err != nil {
		t.Fatal(err)
	}
	flags.apply(p)
	if p.installers("macos_arm64") || p.updatesOn() || p.updates() || p.sparklePlist("macos_arm64") != "" {
		t.Fatal("Steam build enables installers or app updates")
	}
	for _, tc := range []struct {
		goos, arch, target string
		wantTag            bool
	}{
		{"darwin", "arm64", "macos_universal", true},
		{"windows", "amd64", "windows_x64", true},
		{"linux", "amd64", "linux_x64", true},
		{"android", "arm64", "android_arm64", false},
	} {
		env := []string{"GOOS=" + tc.goos, "GOARCH=" + tc.arch, "GOFLAGS="}
		tags, err := goTags(t.Context(), p, env)
		if err != nil {
			t.Fatal(err)
		}
		if slices.Contains(tags, steamBuildTag) != tc.wantTag {
			t.Errorf("%s tags = %v, Steam tag wanted: %v", tc.target, tags, tc.wantTag)
		}
		if got := p.checkSteamTargets([]string{tc.target}) == nil; got != tc.wantTag {
			t.Errorf("accept target %s = %v, want %v", tc.target, got, tc.wantTag)
		}
		// A Steam build links only its channel, which updater.CapabilityOf reports; no feed, key or schedule.
		if flags := p.updateLdflags(env); len(flags) != 1 || !strings.Contains(flags[0], "updateChannel=steam") {
			t.Errorf("%s links update flags %v, want only the steam channel", tc.target, flags)
		}
	}
	p.Build.embed = true
	if err := p.checkSteamTargets([]string{"macos_arm64"}); err == nil {
		t.Error("Steam accepted an embedded build")
	}
}

func TestSteamDevConfigAndDepotResources(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFiles(t, src, map[string]string{
		"nsgo.yaml":                      "name: app\nsteam: {app_id: 480}\n",
		"resources/steam_appid.txt":      "480",
		"resources/data/STEAM_APPID.TXT": "123",
		"resources/data/example.txt":     "game data",
	})
	p, err := load(src)
	if err != nil {
		t.Fatal(err)
	}
	if p.Steam.AppID != 480 || p.steamDevelopment() {
		t.Fatalf("App ID = %d, Steam development = %v", p.Steam.AppID, p.steamDevelopment())
	}
	p.prepareDev(devOptions{})
	if !p.steamDevelopment() || !p.steamBuild("macos_arm64") || p.steamBuild("android_arm64") {
		t.Errorf("Steam App ID should select native development on desktop")
	}
	p.Dev.Target = "web"
	if p.steamDevelopment() {
		t.Error("web dev enabled the Steam App ID")
	}
	p.Dev.Client = false
	p.Updates.Channel = channelSteam
	if err := copyResources(p, dst); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{steamAppIDFile, "data/STEAM_APPID.TXT"} {
		if _, err := os.Stat(filepath.Join(dst, name)); !os.IsNotExist(err) {
			t.Errorf("Steam depot file %s: stat returned %v", name, err)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dst, "data", "example.txt")); err != nil || string(data) != "game data" {
		t.Errorf("depot resource = %q, %v", data, err)
	}
}

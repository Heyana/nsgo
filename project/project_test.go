package project_test

import (
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/project"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

func writeYAML(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func testPublicKey(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return pkg.EncodePublicKey(pub)
}

func TestLoadMissingFile(t *testing.T) {
	_, err := project.Load(t.TempDir())
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load(empty dir) error = %v, want os.ErrNotExist", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "name: My Game\n")

	cfg, err := project.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Dir != dir {
		t.Errorf("Dir = %q, want %q", cfg.Dir, dir)
	}
	if cfg.Name != "My Game" {
		t.Errorf("Name = %q, want %q", cfg.Name, "My Game")
	}
	if cfg.ID != "dev.nsgo.MyGame" || !cfg.DefaultID() {
		t.Errorf("ID = %q (DefaultID=%v), want dev.nsgo.MyGame (true)", cfg.ID, cfg.DefaultID())
	}
	if cfg.Version != "0.0.0" || cfg.BundleVersion() != "0.0.0" {
		t.Errorf("Version = %q BundleVersion = %q, want 0.0.0", cfg.Version, cfg.BundleVersion())
	}
	if cfg.Dev.Target != "native" {
		t.Errorf("Dev.Target = %q, want native", cfg.Dev.Target)
	}
	if want := filepath.Join(dir, "dist"); cfg.Build.Output != want {
		t.Errorf("Build.Output = %q, want %q", cfg.Build.Output, want)
	}
	if want := filepath.Join(dir, "locales"); cfg.I18n.Dir != want {
		t.Errorf("I18n.Dir = %q, want %q", cfg.I18n.Dir, want)
	}
	if cfg.I18n.Source != "en" {
		t.Errorf("I18n.Source = %q, want en", cfg.I18n.Source)
	}
	if cfg.Channel() != project.ChannelNone {
		t.Errorf("Channel() = %q, want %q", cfg.Channel(), project.ChannelNone)
	}
	if !cfg.HotReload() {
		t.Error("HotReload() = false, want true by default")
	}
	if cfg.OpenOutput() {
		t.Error("OpenOutput() = true, want false by default")
	}
	if !cfg.UpdatesAutomatic() {
		t.Error("UpdatesAutomatic() = false, want true by default")
	}
	if cfg.TagPrefix() != "v" {
		t.Errorf("TagPrefix() = %q, want v", cfg.TagPrefix())
	}
	if cfg.AllTargets() {
		t.Error("AllTargets() = true, want false")
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	if want := []string{"macos_" + arch}; !slices.Equal(cfg.Build.Targets, want) {
		t.Errorf("Build.Targets = %v, want %v", cfg.Build.Targets, want)
	}
}

func TestLoadExplicitFieldsAndPaths(t *testing.T) {
	dir := t.TempDir()
	absOut := filepath.Join(t.TempDir(), "out")
	if err := os.MkdirAll(filepath.Join(dir, "packs", "levels"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeYAML(t, dir, `name: Orbit
id: com.example.orbit
version: 1.2.3
build_number: 42
game: true
gamepad: true
appearance: dark
build:
  output: `+absOut+`
  targets: [windows, web]
  tags: [demo, timetzdata]
  skip_installers: [macos]
  open: true
dev:
  target: web
  hot_reload: false
release:
  github: acme/orbit
  tag_prefix: ""
steam:
  app_id: 480
itch:
  project: acme/orbit
  channels:
    web: html5
store:
  dir: metadata/store
linux:
  share: packaging/share
  categories: [ActionGame]
mobile:
  asset_packs:
    - name: levels
      dir: packs/levels
      delivery: install-time
apple:
  info:
    NSCameraUsageDescription: Scans QR codes.
    LSMinimumSystemVersion: "14.0"
macos:
  entitlements:
    com.apple.security.network.client: true
`)

	cfg, err := project.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultID() {
		t.Error("DefaultID() = true for explicit id")
	}
	if cfg.BundleVersion() != "42" {
		t.Errorf("BundleVersion() = %q, want 42", cfg.BundleVersion())
	}
	if cfg.Build.Output != absOut {
		t.Errorf("Build.Output = %q, want %q", cfg.Build.Output, absOut)
	}
	if want := []string{"windows_arm64", "windows_x64", "web"}; !slices.Equal(cfg.Build.Targets, want) {
		t.Errorf("Build.Targets = %v, want %v", cfg.Build.Targets, want)
	}
	if cfg.HotReload() || !cfg.OpenOutput() {
		t.Errorf("HotReload=%v OpenOutput=%v, want false true", cfg.HotReload(), cfg.OpenOutput())
	}
	if cfg.TagPrefix() != "" {
		t.Errorf("TagPrefix() = %q, want empty string", cfg.TagPrefix())
	}
	if cfg.Steam.AppID != 480 {
		t.Errorf("Steam.AppID = %d, want 480", cfg.Steam.AppID)
	}
	if cfg.Itch.Project != "acme/orbit" || cfg.Itch.Channels["web"] != "html5" {
		t.Errorf("Itch = %+v", cfg.Itch)
	}
	if want := filepath.Join(dir, "metadata", "store"); cfg.Store.Dir != want {
		t.Errorf("Store.Dir = %q, want %q", cfg.Store.Dir, want)
	}
	if want := filepath.Join(dir, "packaging", "share"); cfg.Linux.Share != want {
		t.Errorf("Linux.Share = %q, want %q", cfg.Linux.Share, want)
	}
	if want := filepath.Join(dir, "packs", "levels"); cfg.Mobile.AssetPacks[0].Dir != want {
		t.Errorf("AssetPacks[0].Dir = %q, want %q", cfg.Mobile.AssetPacks[0].Dir, want)
	}
	if cfg.MacOSCategory() != "public.app-category.games" {
		t.Errorf("MacOSCategory() = %q, want public.app-category.games", cfg.MacOSCategory())
	}
	if !slices.Contains(cfg.LinuxCategories(), "Game") {
		t.Errorf("LinuxCategories() = %v, want Game included", cfg.LinuxCategories())
	}
}

func TestLoadStrictUnknownKeys(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{"top-level typo", "name: App\nverison: 1.0.0\n", "verison"},
		{"nested typo", "name: App\nupdates:\n  chanel: steam\n", "chanel"},
		{"android typo", "name: App\nandroid:\n  permisions: [CAMERA]\n", "permisions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeYAML(t, dir, tc.yaml)
			_, err := project.Load(dir)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Load error = %v, want error mentioning %q", err, tc.want)
			}
		})
	}
}

func TestChannelAndUpdatesValidation(t *testing.T) {
	pubKey := testPublicKey(t)

	for _, tc := range []struct {
		name        string
		yaml        string
		wantChannel string
		wantFeed    string
		wantErr     string
	}{
		{
			name:        "default without key is none",
			yaml:        "name: App\n",
			wantChannel: project.ChannelNone,
		},
		{
			name:        "default with key is direct",
			yaml:        "name: App\nupdates:\n  key: " + pubKey + "\n",
			wantChannel: project.ChannelDirect,
		},
		{
			name:        "direct with feed normalizes trailing slash",
			yaml:        "name: App\nupdates:\n  key: " + pubKey + "\n  feed: https://releases.example.com/app\n",
			wantChannel: project.ChannelDirect,
			wantFeed:    "https://releases.example.com/app/",
		},
		{
			name:        "explicit store channel",
			yaml:        "name: App\nupdates:\n  channel: store\n",
			wantChannel: project.ChannelStore,
		},
		{
			name:        "explicit steam channel",
			yaml:        "name: App\nupdates:\n  channel: steam\n",
			wantChannel: project.ChannelSteam,
		},
		{
			name:        "explicit none overrides key",
			yaml:        "name: App\nupdates:\n  key: " + pubKey + "\n  channel: none\n",
			wantChannel: project.ChannelNone,
		},
		{
			name:    "invalid channel",
			yaml:    "name: App\nupdates:\n  channel: beta\n",
			wantErr: `channel "beta"`,
		},
		{
			name:    "feed without key and empty channel",
			yaml:    "name: App\nupdates:\n  feed: https://example.com/\n",
			wantErr: "key holds no public key",
		},
		{
			name:    "invalid key",
			yaml:    "name: App\nupdates:\n  key: not-a-key\n",
			wantErr: "updates: key:",
		},
		{
			name:    "non-https feed",
			yaml:    "name: App\nupdates:\n  key: " + pubKey + "\n  feed: http://example.com/\n",
			wantErr: "not an https URL",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeYAML(t, dir, tc.yaml)
			cfg, err := project.Load(dir)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Load error = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := cfg.Channel(); got != tc.wantChannel {
				t.Errorf("Channel() = %q, want %q", got, tc.wantChannel)
			}
			if tc.wantFeed != "" && cfg.Updates.Feed != tc.wantFeed {
				t.Errorf("Updates.Feed = %q, want %q", cfg.Updates.Feed, tc.wantFeed)
			}
		})
	}
}

// TestLoadDoesNotInvokeGit verifies Load performs no Git or network calls even
// when updates.key is set and updates.feed is omitted.
func TestLoadDoesNotInvokeGit(t *testing.T) {
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "git-called")
	fakeGit := filepath.Join(binDir, "git")
	script := "#!/bin/sh\necho called > " + marker + "\nexit 1\n"
	if err := os.WriteFile(fakeGit, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	dir := t.TempDir()
	writeYAML(t, dir, "name: App\nupdates:\n  key: "+testPublicKey(t)+"\n")

	cfg, err := project.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Channel() != project.ChannelDirect {
		t.Fatalf("Channel() = %q, want %q", cfg.Channel(), project.ChannelDirect)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("Load invoked git, but must make no Git or network calls")
	}
}

func TestLoadValidationErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{"missing name", "version: 1.0.0\n", "name is required"},
		{"both quotes in name", "name: Bob's \"App\"\n", "single and a double quote"},
		{"bad appearance", "name: App\nappearance: sepia\n", "appearance"},
		{"removed window", "name: App\nwindow: {width: 400}\n", "window is set in Go"},
		{"removed dev fps", "name: App\ndev: {fps: true}\n", "fps is gone"},
		{"removed build fps", "name: App\nbuild: {fps: true}\n", "fps is gone"},
		{"bad github repo", "name: App\nrelease: {github: not-a-repo}\n", "not owner/name"},
		{"bad skip installers", "name: App\nbuild: {skip_installers: [ios]}\n", "unknown desktop system"},
		{"bad build tag", "name: App\nbuild: {tags: ['bad tag']}\n", "build.tags"},
		{"bad target", "name: App\nbuild: {targets: [amiga]}\n", "unknown target"},
		{"bad i18n coverage", "name: App\ni18n: {coverage: strict}\n", "i18n.coverage"},
		{"bad url scheme", "name: App\nurl_schemes: [https]\n", "belongs to the system"},
		{"bad app link", "name: App\napp_links: ['https://example.com']\n", "app_links[0]"},
		{"bad cloud container", "name: App\ncloud: {containers: [example.container]}\n", `must start with "iCloud."`},
		{"bad privacy category", "name: App\napple: {privacy: {accessed_apis: {DiskSpace: [E174.1]}}}\n", "NSPrivacyAccessedAPICategory"},
		{"bad apple minimum", "name: App\napple: {info: {MinimumOSVersion: '12.0'}}\n", "below the runtime minimum"},
		{"bad license rule", "name: App\nlicenses: {unknown: ignore}\n", "licenses.unknown"},
		{"bad launch screen color", "name: App\nlaunch_screen: {background: red}\n", "not a #RRGGBB color"},
		{"bad appimage update", "name: App\nlinux: {appimage_update: ftp://example.com}\n", "linux.appimage_update"},
	} {
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

func TestLoadAllTargetsAndLaunchColor(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `name: App
build:
  targets: [all]
launch_screen:
  background: "#14A7CD"
  dark:
    background: "#000000"
`)
	cfg, err := project.Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.AllTargets() {
		t.Error("AllTargets() = false, want true")
	}
	if !slices.Contains(cfg.Build.Targets, "web") || !slices.Contains(cfg.Build.Targets, "macos_arm64") {
		t.Errorf("expanded all targets missing expected entries: %v", cfg.Build.Targets)
	}
	if cfg.LaunchScreen == nil || cfg.LaunchScreen.Color() != 0xff14a7cd || cfg.LaunchScreen.Dark.Color() != 0xff000000 {
		t.Errorf("LaunchScreen colors = %#x / %#x", cfg.LaunchScreen.Color(), cfg.LaunchScreen.Dark.Color())
	}
}

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Android's PackageParser takes segments of letters, digits and underscores that start with a letter; Apple's CFBundleIdentifier takes letters, digits, hyphens and dots.
var (
	androidPackage = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)
	appleBundleID  = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
)

func TestGeneratedID(t *testing.T) {
	for name, want := range map[string]string{
		"formula": "dev.nsgo.formula",
		"Chat":    "dev.nsgo.Chat",
		"My App":  "dev.nsgo.MyApp",
		"my-lib":  "dev.nsgo.mylib",
		"ui_kit":  "dev.nsgo.uikit",
		"v1.2":    "dev.nsgo.v12",
		"2d":      "dev.nsgo.app2d",
		"001":     "dev.nsgo.app001",
		"café":    "dev.nsgo.caf",
		"/":       "dev.nsgo.app",
		"テスト":     "dev.nsgo.app",
	} {
		got := generatedID(name)
		if got != want {
			t.Errorf("generatedID(%q) = %q, want %q", name, got, want)
		}
		if !androidPackage.MatchString(got) || !appleBundleID.MatchString(got) {
			t.Errorf("generatedID(%q) = %q, not an Android package and an Apple bundle ID", name, got)
		}
	}
}

func TestHotReload(t *testing.T) {
	for _, tt := range []struct {
		yaml string
		want bool
	}{
		{"", true},
		{"  hot_reload: true\n", true},
		{"  hot_reload: false\n", false},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "nsgo.yaml")
		if err := os.WriteFile(path, []byte("name: Test\ndev:\n"+tt.yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.hotReload(); got != tt.want {
			t.Errorf("dev.hot_reload %q: got %v, want %v", tt.yaml, got, tt.want)
		}
	}
}

func TestAndroidPermissions(t *testing.T) {
	dir := t.TempDir()
	yaml := "name: Test\nandroid:\n  permissions: [REQUEST_INSTALL_PACKAGES, com.example.READ]\n"
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Android.Permissions; len(got) != 2 || got[0] != "android.permission.REQUEST_INSTALL_PACKAGES" || got[1] != "com.example.READ" {
		t.Fatalf("permissions = %v", got)
	}
}

func TestLoadRefusesUnknownConfigKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: Test\nandroid:\n  permisions: [RECORD_AUDIO]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := load(dir); err == nil || !strings.Contains(err.Error(), "permisions") {
		t.Fatalf("load unknown key = %v, want an error naming permisions", err)
	}
}

func TestLinuxPackageDependencies(t *testing.T) {
	dir := t.TempDir()
	write := func(linux string) {
		if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: Test\nlinux:\n"+linux), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("  depends:\n    deb: [gstreamer1.0-plugins-good, 'libsoup-3.0-0 | libsoup3']\n    arch: [gst-plugins-good]\n  recommends:\n    deb: [gstreamer1.0-libav]\n    arch: ['gst-libav: H.264 and AAC video']\n")
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Linux.Depends.Deb; len(got) != 2 || got[0] != "gstreamer1.0-plugins-good" || got[1] != "libsoup-3.0-0 | libsoup3" {
		t.Errorf("linux.depends.deb = %q", got)
	}
	if got := p.Linux.Depends.Arch; len(got) != 1 || got[0] != "gst-plugins-good" {
		t.Errorf("linux.depends.arch = %q", got)
	}
	if got := p.Linux.Recommends.Deb; len(got) != 1 || got[0] != "gstreamer1.0-libav" {
		t.Errorf("linux.recommends.deb = %q", got)
	}
	if got := p.Linux.Recommends.Arch; len(got) != 1 || got[0] != "gst-libav: H.264 and AAC video" {
		t.Errorf("linux.recommends.arch = %q", got)
	}
	for _, tc := range []struct{ linux, want string }{
		{"  depends: { deb: [a, ''] }\n", "nsgo.yaml: linux.depends.deb entry 2 is empty"},
		{"  depends: { arch: ['  '] }\n", "nsgo.yaml: linux.depends.arch entry 1 is empty"},
		{"  recommends: { deb: [\"\"] }\n", "nsgo.yaml: linux.recommends.deb entry 1 is empty"},
		{"  recommends: { arch: [\"a\\nDescription: b\"] }\n", "nsgo.yaml: linux.recommends.arch entry 1 contains a line break"},
		{"  depends: { deb: [\"a\\r\"] }\n", "nsgo.yaml: linux.depends.deb entry 1 contains a line break"},
	} {
		write(tc.linux)
		if _, err := load(dir); err == nil || err.Error() != tc.want {
			t.Errorf("%q: load error = %v, want %q", tc.linux, err, tc.want)
		}
	}
}

// linux.categories takes what desktop-file-validate accepts and asks for a main category, without which menus file the app under Other.
func TestLinuxCategories(t *testing.T) {
	dir := t.TempDir()
	write := func(categories string) {
		if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: Test\nlinux:\n  categories: "+categories+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, ok := range []string{"[Office, Dictionary]", "[AudioVideo, Audio, Player]", "[Utility, X-GNOME-Utilities, X-a_b.c]", "[]"} {
		write(ok)
		if _, err := load(dir); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, tc := range []struct{ categories, want string }{
		{"[Office, Bogus]", `nsgo.yaml: linux.categories entry 2, "Bogus", is not a freedesktop category; an extension starts with X- and holds letters, digits, '-', '_' and '.'`},
		{"[Office, '']", `nsgo.yaml: linux.categories entry 2, "", is not a freedesktop category; an extension starts with X- and holds letters, digits, '-', '_' and '.'`},
		{"['Office;Game']", `nsgo.yaml: linux.categories entry 1, "Office;Game", is not a freedesktop category; an extension starts with X- and holds letters, digits, '-', '_' and '.'`},
		{"[Office, 'X-a;b']", `nsgo.yaml: linux.categories entry 2, "X-a;b", is not a freedesktop category; an extension starts with X- and holds letters, digits, '-', '_' and '.'`},
		{"[Office, \"X-a\\nName=b\"]", `nsgo.yaml: linux.categories entry 2, "X-a\nName=b", is not a freedesktop category; an extension starts with X- and holds letters, digits, '-', '_' and '.'`},
		{"[Office, X-]", `nsgo.yaml: linux.categories entry 2, "X-", is not a freedesktop category; an extension starts with X- and holds letters, digits, '-', '_' and '.'`},
		{"[Utility, TrayIcon]", `nsgo.yaml: linux.categories entry 2, "TrayIcon", is reserved for desktop components`},
		{"[Office, Office]", `nsgo.yaml: linux.categories names "Office" twice`},
		{"[Audio, Player]", "nsgo.yaml: linux.categories names Audio without AudioVideo, which it belongs to"},
		{"[Dictionary, X-Words]", "nsgo.yaml: linux.categories names no main category, so menus would file the app under Other; add one of AudioVideo, Audio, Video, Development, Education, Game, Graphics, Network, Office, Science, Settings, System, Utility"},
	} {
		write(tc.categories)
		if _, err := load(dir); err == nil || err.Error() != tc.want {
			t.Errorf("%s: load error = %v, want %q", tc.categories, err, tc.want)
		}
	}
}

func TestBuildOpen(t *testing.T) {
	for _, tt := range []struct {
		yaml string
		want bool
	}{
		{"", false},
		{"open: true\n", true},
		{"open: false\n", false},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "nsgo.yaml")
		if err := os.WriteFile(path, []byte("name: Test\nbuild:\n  "+tt.yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.openOutput(); got != tt.want {
			t.Errorf("build.open %q: got %v, want %v", tt.yaml, got, tt.want)
		}
	}
}

func TestBuildTags(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: Test\nbuild:\n  tags: [demo_lite, timetzdata]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Build.Tags, []string{"demo_lite", "timetzdata"}) {
		t.Fatalf("build.tags = %v", p.Build.Tags)
	}
}

func TestWindowsCommandLine(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: Test\nwindows:\n  command_line: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Windows.CommandLine {
		t.Fatal("windows.command_line was not loaded")
	}
}

func TestIcons(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	load := func(yaml string) (*project, error) {
		if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: Test\n"+yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		return load(dir)
	}

	p, err := load("")
	if err != nil {
		t.Fatal(err)
	}
	if p.Icon.Apple != "" || p.Icon.PNG != "" || p.Icon.TVOS != "" || p.Icon.WatchOS != "" {
		t.Errorf("without icons: got %+v", p.Icon)
	}

	write("assets/AppIcon.icns")
	write("assets/AppIcon.icon/icon.json")
	write("assets/Icon.png")
	write("assets/AppIconTV.brandassets/Contents.json")
	write("assets/AppIconWatch.icon/icon.json")
	p, err = load("")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "assets", "AppIcon.icon"); p.Icon.Apple != want {
		t.Errorf("default Apple icon: got %s, want %s", p.Icon.Apple, want)
	}
	if want := filepath.Join(dir, "assets", "Icon.png"); p.Icon.PNG != want {
		t.Errorf("default PNG icon: got %s, want %s", p.Icon.PNG, want)
	}
	if want := filepath.Join(dir, "assets", "AppIconTV.brandassets"); p.Icon.TVOS != want {
		t.Errorf("default tvOS icon: got %s, want %s", p.Icon.TVOS, want)
	}
	if want := filepath.Join(dir, "assets", "AppIconWatch.icon"); p.Icon.WatchOS != want {
		t.Errorf("default watchOS icon: got %s, want %s", p.Icon.WatchOS, want)
	}

	write("art/App.icns")
	write("art/app.png")
	write("art/tv.png")
	write("art/watch.icon/icon.json")
	p, err = load("icon:\n  apple: art/App.icns\n  png: art/app.png\n  tvos: art/tv.png\n  watchos: art/watch.icon\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "art", "App.icns"); p.Icon.Apple != want {
		t.Errorf("icon.apple: got %s, want %s", p.Icon.Apple, want)
	}
	if want := filepath.Join(dir, "art", "tv.png"); p.Icon.TVOS != want {
		t.Errorf("icon.tvos: got %s, want %s", p.Icon.TVOS, want)
	}
	if want := filepath.Join(dir, "art", "watch.icon"); p.Icon.WatchOS != want {
		t.Errorf("icon.watchos: got %s, want %s", p.Icon.WatchOS, want)
	}

	for _, yaml := range []string{
		"icon:\n  png: art/missing.png\n",
		"icon:\n  apple: art/app.png\n",
		"icon:\n  tvos: art/App.icns\n",
		"icon:\n  tvos: art/missing.png\n",
		"icon:\n  watchos: art/App.icns\n",
		"icon:\n  watchos: art/missing.png\n",
	} {
		if _, err := load(yaml); err == nil {
			t.Errorf("%q loaded", yaml)
		}
	}
	write("art/NotAssets.brandassets")
	if _, err := load("icon:\n  tvos: art/NotAssets.brandassets\n"); err == nil {
		t.Error("a file loaded as icon.tvos")
	}

	// nsgo dev takes the -dev defaults and dev.icon over icon.
	write("assets/AppIcon-dev.icon/icon.json")
	write("assets/AppIconTV-dev.brandassets/Contents.json")
	write("assets/AppIconWatch-dev.icon/icon.json")
	p, err = load("dev:\n  icon:\n    png: art/app.png\n")
	if err != nil {
		t.Fatal(err)
	}
	p.useDevIcons()
	if want := filepath.Join(dir, "assets", "AppIcon-dev.icon"); p.Icon.Apple != want {
		t.Errorf("default dev Apple icon: got %s, want %s", p.Icon.Apple, want)
	}
	if want := filepath.Join(dir, "art", "app.png"); p.Icon.PNG != want {
		t.Errorf("dev.icon.png: got %s, want %s", p.Icon.PNG, want)
	}
	if want := filepath.Join(dir, "assets", "AppIconTV-dev.brandassets"); p.Icon.TVOS != want {
		t.Errorf("default dev tvOS icon: got %s, want %s", p.Icon.TVOS, want)
	}
	if want := filepath.Join(dir, "assets", "AppIconWatch-dev.icon"); p.Icon.WatchOS != want {
		t.Errorf("default dev watchOS icon: got %s, want %s", p.Icon.WatchOS, want)
	}
	p, err = load("dev:\n  icon:\n    tvos: art/tv.png\n    watchos: art/watch.icon\n")
	if err != nil {
		t.Fatal(err)
	}
	p.useDevIcons()
	if want := filepath.Join(dir, "art", "tv.png"); p.Icon.TVOS != want {
		t.Errorf("dev.icon.tvos: got %s, want %s", p.Icon.TVOS, want)
	}
	if want := filepath.Join(dir, "art", "watch.icon"); p.Icon.WatchOS != want {
		t.Errorf("dev.icon.watchos: got %s, want %s", p.Icon.WatchOS, want)
	}
	if _, err := load("dev:\n  icon:\n    apple: art/app.png\n"); err == nil {
		t.Error("a PNG loaded as dev.icon.apple")
	}
	write("art/foreground.png")
	write("art/mono.png")
	p, err = load("icon:\n  android:\n    foreground: art/foreground.png\n    background: '#123456'\n    monochrome: art/mono.png\n")
	if err != nil {
		t.Fatal(err)
	}
	if p.Icon.Android == nil || p.Icon.Android.Foreground != filepath.Join(dir, "art", "foreground.png") || p.Icon.Android.Background != "#123456" || p.Icon.Android.Monochrome != filepath.Join(dir, "art", "mono.png") {
		t.Fatalf("android icon = %+v", p.Icon.Android)
	}
	if _, err := load("icon:\n  android:\n    foreground: art/foreground.png\n"); err == nil || !strings.Contains(err.Error(), "needs foreground and background") {
		t.Errorf("an Android icon without a background loaded: %v", err)
	}
	if _, err := load("icon:\n  android:\n    foreground: art/foreground.png\n    background: '#12345'\n"); err == nil {
		t.Error("an Android icon with a five-digit color loaded")
	}
}

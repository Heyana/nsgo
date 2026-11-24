package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadAndOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: App\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}

	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	f := bindDevFlags(fs)
	if err := fs.Parse([]string{"-name", "Other"}); err != nil {
		t.Fatal(err)
	}
	f.apply(p)

	if p.Name != "Other" || p.ID != "dev.nsgo.Other" {
		t.Errorf("app: got %q, %q", p.Name, p.ID)
	}
}

func TestLoadAndOverrideSanitizeGeneratedID(t *testing.T) {
	for name, want := range map[string]string{
		"My App": "dev.nsgo.MyApp",
		"my-app": "dev.nsgo.myapp",
		"ui_kit": "dev.nsgo.uikit",
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if p.ID != want || !androidPackage.MatchString(p.ID) || !appleBundleID.MatchString(p.ID) {
			t.Errorf("load of name %q: ID %q, want %q", name, p.ID, want)
		}

		if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: App\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if p, err = load(dir); err != nil {
			t.Fatal(err)
		}
		fs := flag.NewFlagSet("dev", flag.ContinueOnError)
		f := bindDevFlags(fs)
		if err := fs.Parse([]string{"-name", name}); err != nil {
			t.Fatal(err)
		}
		f.apply(p)
		if p.ID != want {
			t.Errorf("-name %q: ID %q, want %q", name, p.ID, want)
		}
	}

	// An ID nsgo.yaml gives stays as written.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: my-app\nid: com.example.my-app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	f := bindDevFlags(fs)
	if err := fs.Parse([]string{"-name", "ui_kit"}); err != nil {
		t.Fatal(err)
	}
	f.apply(p)
	if p.ID != "com.example.my-app" {
		t.Errorf("explicit id: got %q, want com.example.my-app", p.ID)
	}
}

func TestLoadBuildLdflags(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: App\nbuild:\n  ldflags:\n    - -X main.releaseDate=2026-11-17\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Build.LDFlags, []string{"-X main.releaseDate=2026-11-17"}) {
		t.Fatalf("build.ldflags = %q", p.Build.LDFlags)
	}
}

// TestAppLdflagsReachTheBinary: the app's linker flags stamp its variables in the binary go build links, values with spaces included, and a -X of the tool's for the same variable wins.
func TestAppLdflagsReachTheBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go build")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module stamp\n\ngo 1.26\n",
		"main.go": "package main\n\nimport \"fmt\"\n\nvar releaseDate, note, owner string\n\nfunc main() { fmt.Print(releaseDate + \"|\" + note + \"|\" + owner) }\n",
	})
	p := &project{Dir: dir, Name: "Stamp", ID: "com.example.stamp", Version: "1.0.0"}
	p.Build.LDFlags = []string{"-X main.releaseDate=2026-11-17", "-X 'main.note=two words' -X main.owner=app"}
	bin := filepath.Join(dir, "app")
	cmd, done, err := goBuildCommand(t.Context(), p, bin, nil, buildExtra{}, "-X main.owner=tool")
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.CombinedOutput()
	done()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	got, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatal(err)
	}
	if want := "2026-11-17|two words|tool"; string(got) != want {
		t.Errorf("the binary printed %q, want %q", got, want)
	}
}

func TestLoadRefusesWindow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: App\nwindow: {width: 420, height: 560}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := load(dir); err == nil || !strings.Contains(err.Error(), "nsgo.WindowSize") {
		t.Errorf("a window in nsgo.yaml: %v", err)
	}
}

func TestDevFlagsApply(t *testing.T) {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	f := bindDevFlags(fs)
	args := []string{"-target", "web", "-hot-reload", "-name", "Other"}
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	p := &project{
		Name:    "App",
		ID:      "com.example.app",
		Version: "1.0.0",
	}
	p.Dev.Target = "native"
	off := false
	p.Dev.HotReload = &off

	f.apply(p)

	if p.Dev.Target != "web" {
		t.Errorf("dev.target: got %q, want web", p.Dev.Target)
	}
	if !p.hotReload() {
		t.Error("dev.hot_reload: got false, want true")
	}
	if p.Name != "Other" {
		t.Errorf("name: got %q, want Other", p.Name)
	}
	if p.ID != "com.example.app" {
		t.Errorf("explicit id changed to %q", p.ID)
	}
	if p.Version != "1.0.0" {
		t.Errorf("version: got %q, want 1.0.0", p.Version)
	}
}

func TestDevFlagsKeepValues(t *testing.T) {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	f := bindDevFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	hot := true
	p := &project{Name: "App", ID: "com.example.app", Version: "1.0.0"}
	p.Dev.Target = "web"
	p.Dev.HotReload = &hot

	f.apply(p)

	if p.Dev.Target != "web" || !p.hotReload() {
		t.Errorf("dev: got target %q, hot reload %v", p.Dev.Target, p.hotReload())
	}
	if p.Name != "App" || p.ID != "com.example.app" || p.Version != "1.0.0" {
		t.Errorf("app: got %q, %q, %q", p.Name, p.ID, p.Version)
	}
}

func TestDevFlagsDisableHotReload(t *testing.T) {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	f := bindDevFlags(fs)
	if err := fs.Parse([]string{"-hot-reload=false"}); err != nil {
		t.Fatal(err)
	}
	hot := true
	p := &project{}
	p.Dev.HotReload = &hot

	f.apply(p)

	if p.hotReload() {
		t.Error("dev.hot_reload: got true, want false")
	}
}

func TestNameFollowsGeneratedID(t *testing.T) {
	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	f := bindDevFlags(fs)
	if err := fs.Parse([]string{"-name", "Other"}); err != nil {
		t.Fatal(err)
	}
	p := &project{Name: "App", ID: "dev.nsgo.App", idDefault: true}

	f.apply(p)

	if p.ID != "dev.nsgo.Other" {
		t.Errorf("id: got %q, want dev.nsgo.Other", p.ID)
	}
}

func TestBuildFlagsApply(t *testing.T) {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	f := bindBuildFlags(fs)
	args := []string{"-target", "web,ios", "-output", "out", "-open=false", "-version", "2.0.0", "-jobs", "3"}
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	p := &project{Dir: t.TempDir()}
	p.Build.Output = p.resolve("dist")
	p.Build.Targets = []string{"macos_arm64"}

	f.apply(p)

	if got := p.Build.Targets; len(got) != 2 || got[0] != "web" || got[1] != "ios" {
		t.Errorf("build.targets: got %v, want [web ios]", got)
	}
	if want := filepath.Join(p.Dir, "out"); p.Build.Output != want {
		t.Errorf("build.output: got %q, want %q", p.Build.Output, want)
	}
	if p.openOutput() {
		t.Error("build.open: got true, want false")
	}
	if p.Version != "2.0.0" {
		t.Errorf("version: got %q, want 2.0.0", p.Version)
	}
	if p.Build.Jobs != 3 {
		t.Errorf("build.jobs: got %d, want 3", p.Build.Jobs)
	}
}

func TestBuildInstallerSelection(t *testing.T) {
	desktop := []string{"macos_arm64", "macos_x64", "macos_universal", "windows_x64", "linux_x64"}
	withoutMac := []string{"windows_x64", "linux_x64"}
	for _, tc := range []struct {
		name, config string
		args, want   []string
	}{
		{"default", "", nil, desktop},
		{"skip_macos", "skip_installers: [macos]", nil, withoutMac},
		{"replace_skips", "skip_installers: [windows]", []string{"-skip-installers=macos,linux"}, []string{"windows_x64"}},
		{"clear_skips", "skip_installers: [macos]", []string{"-skip-installers="}, desktop},
		{"global_config", "installers: false", nil, nil},
		{"global_flag", "skip_installers: [macos]", []string{"-installers=false"}, nil},
		{"enable_unskipped", "installers: false, skip_installers: [macos]", []string{"-installers"}, withoutMac},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{"nsgo.yaml": "name: Studio\nbuild: {" + tc.config + "}\n"})
			p, err := load(dir)
			if err != nil {
				t.Fatal(err)
			}
			fs := flag.NewFlagSet("build", flag.ContinueOnError)
			flags := bindBuildFlags(fs)
			if err := fs.Parse(tc.args); err != nil {
				t.Fatal(err)
			}
			flags.apply(p)
			for _, target := range desktop {
				if got, want := p.installers(target), slices.Contains(tc.want, target); got != want {
					t.Errorf("%s installer enabled = %t, want %t", target, got, want)
				}
			}
		})
	}
}

func TestBuildRejectsUnknownInstallerSystem(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"nsgo.yaml": "name: Studio\nbuild: {skip_installers: [macso]}\n"})
	if _, err := load(dir); err == nil || !strings.Contains(err.Error(), `build.skip_installers: unknown desktop system "macso"`) {
		t.Errorf("load with misspelled installer system: %v", err)
	}
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	bindBuildFlags(fs)
	if err := fs.Set("skip-installers", "macso"); err == nil || !strings.Contains(err.Error(), `"macso"`) {
		t.Errorf("flag with misspelled installer system: %v", err)
	}
}

func TestBuildSigningProfileFlags(t *testing.T) {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	f := bindBuildFlags(fs)
	if err := fs.Parse([]string{
		"-profile", "profiles/app.mobileprovision",
		"-widgets-profile", "profiles/widgets.mobileprovision",
		"-share-profile", "profiles/share.mobileprovision",
		"-clip-profile", "profiles/clip.mobileprovision",
		"-macos-profile", "profiles/macos.provisionprofile",
		"-installer", "Mac Installer Distribution: Example (TEAM123)",
	}); err != nil {
		t.Fatal(err)
	}
	p := &project{Dir: t.TempDir(), Sign: signing{
		Profile:        "from-yaml.mobileprovision",
		WidgetsProfile: "from-yaml-widgets.mobileprovision",
		ShareProfile:   "from-yaml-share.mobileprovision",
		ClipProfile:    "from-yaml-clip.mobileprovision",
		MacOSProfile:   "from-yaml-macos.provisionprofile",
		Installer:      "from-yaml-installer",
	}}

	f.apply(p)

	if got, want := p.Sign.Profile, filepath.Join(p.Dir, "profiles/app.mobileprovision"); got != want {
		t.Errorf("sign.profile: got %q, want %q", got, want)
	}
	if got, want := p.Sign.WidgetsProfile, filepath.Join(p.Dir, "profiles/widgets.mobileprovision"); got != want {
		t.Errorf("sign.widgets_profile: got %q, want %q", got, want)
	}
	if got, want := p.Sign.ShareProfile, filepath.Join(p.Dir, "profiles/share.mobileprovision"); got != want {
		t.Errorf("sign.share_profile: got %q, want %q", got, want)
	}
	if got, want := p.Sign.ClipProfile, filepath.Join(p.Dir, "profiles/clip.mobileprovision"); got != want {
		t.Errorf("sign.clip_profile: got %q, want %q", got, want)
	}
	if got, want := p.Sign.MacOSProfile, filepath.Join(p.Dir, "profiles/macos.provisionprofile"); got != want {
		t.Errorf("sign.macos_profile: got %q, want %q", got, want)
	}
	if got, want := p.Sign.Installer, "Mac Installer Distribution: Example (TEAM123)"; got != want {
		t.Errorf("sign.installer: got %q, want %q", got, want)
	}
}

func TestBuildFlagsLdflags(t *testing.T) {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	f := bindBuildFlags(fs)
	if err := fs.Parse([]string{"-ldflags", "-X main.releaseDate=2026-11-17", "-ldflags=-X main.channel=stable"}); err != nil {
		t.Fatal(err)
	}
	p := &project{}
	f.apply(p)
	if !slices.Equal(p.Build.LDFlags, []string{"-X main.releaseDate=2026-11-17", "-X main.channel=stable"}) {
		t.Fatalf("build.ldflags = %q", p.Build.LDFlags)
	}
}

func TestBuildFlagsKeepValues(t *testing.T) {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	f := bindBuildFlags(fs)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	open := false
	p := &project{Dir: t.TempDir()}
	p.Build.Output = p.resolve("dist")
	p.Build.Targets = []string{"macos_arm64"}
	p.Build.Open = &open

	f.apply(p)

	if p.Build.Output != filepath.Join(p.Dir, "dist") {
		t.Errorf("build.output: got %q", p.Build.Output)
	}
	if len(p.Build.Targets) != 1 || p.Build.Targets[0] != "macos_arm64" {
		t.Errorf("build.targets: got %v", p.Build.Targets)
	}
	if p.openOutput() {
		t.Error("build.open: got true, want false")
	}
}

func TestBuildFlagsOpen(t *testing.T) {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	f := bindBuildFlags(fs)
	if err := fs.Parse([]string{"-open"}); err != nil {
		t.Fatal(err)
	}
	open := false
	p := &project{}
	p.Build.Open = &open

	f.apply(p)

	if !p.openOutput() {
		t.Error("build.open: got false, want true")
	}
}

func TestBuildFlagsAbsoluteOutput(t *testing.T) {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	f := bindBuildFlags(fs)
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	if err := fs.Parse([]string{"-output", out}); err != nil {
		t.Fatal(err)
	}
	p := &project{Dir: t.TempDir()}

	f.apply(p)

	if p.Build.Output != out {
		t.Errorf("build.output: got %q, want %q", p.Build.Output, out)
	}
}

func TestAppFlags(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: App\ndev: { fps: true }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := load(dir); err == nil || !strings.Contains(err.Error(), "--nsgo-fps") {
		t.Errorf("dev.fps: %v", err)
	}

	fs := flag.NewFlagSet("dev", flag.ContinueOnError)
	f := bindDevFlags(fs)
	if err := fs.Parse([]string{"-fps", "-lang", "zh-Hans"}); err != nil {
		t.Fatal(err)
	}
	opts := devOptions{fps: f.fps, lang: f.lang}
	if got := opts.appFlags(); !slices.Equal(got, []string{"--nsgo-fps", "--nsgo-lang=zh-Hans"}) {
		t.Errorf("appFlags = %q", got)
	}
	if got := androidExtras(opts.appFlags()); !slices.Equal(got, []string{"--ez", "nsgo-fps", "true", "--es", "nsgo-lang", "zh-Hans"}) {
		t.Errorf("androidExtras = %q", got)
	}
	if got := webQuery(opts.appFlags()); got != "?nsgo-fps&nsgo-lang=zh-Hans" {
		t.Errorf("webQuery = %q", got)
	}
	if got := (devOptions{}).appFlags(); got != nil {
		t.Errorf("appFlags without -fps or -lang = %q", got)
	}
	p := &project{Name: "App", ID: "com.example.app"}
	if strings.Contains(p.ldflags(), "fps") {
		t.Errorf("ldflags = %q", p.ldflags())
	}
}

func TestLayoutFlag(t *testing.T) {
	for args, want := range map[string]string{"-layout": "bounds", "-layout=bounds,grid=4": "bounds,grid=4", "-layout=grid": "grid", "-layout=bounds, grid": "bounds,grid"} {
		fs := flag.NewFlagSet("dev", flag.ContinueOnError)
		f := bindDevFlags(fs)
		if err := fs.Parse([]string{args, "app"}); err != nil || string(f.layout) != want || fs.Arg(0) != "app" {
			t.Errorf("%s: layout %q, args %q, %v", args, f.layout, fs.Args(), err)
		}
	}
	for _, bad := range []string{"rulers", "grid=2", "grid=x", "grid=NaN", "grid=nan", "grid=1e30", "bounds,", "bounds=1"} {
		var l layoutFlag
		if l.Set(bad) == nil {
			t.Errorf("-layout=%s was taken", bad)
		}
	}
	p := &project{Name: "App", ID: "com.example.app"}
	p.prepareDev(devOptions{layout: "bounds,grid=4"})
	if flags := p.ldflags(); !strings.Contains(flags, " -X github.com/nsgo-dev/nsgo/internal/dev.devLayout=bounds,grid=4") {
		t.Errorf("ldflags = %q, want -layout's value for the dev client", flags)
	}
	p.prepareDev(devOptions{})
	if flags := p.ldflags(); strings.Contains(flags, "devLayout") {
		t.Errorf("ldflags without -layout = %q", flags)
	}
}

func TestIdentityLdflags(t *testing.T) {
	p := &project{Name: "App", ID: "com.example.app", Version: "1.2.0", BuildNumber: 12}
	flags := p.ldflags()
	for _, want := range []string{"core.appName=App'", "core.appID=com.example.app'", "core.appVersion=1.2.0'", "core.appBuild=12'"} {
		if !strings.Contains(flags, want) {
			t.Errorf("ldflags %s lack %s", flags, want)
		}
	}
	if strings.Contains(flags, "appRelease") {
		t.Errorf("a development build's ldflags %s say release", flags)
	}
	p.Release = true
	if !strings.Contains(p.ldflags(), "core.appRelease=1") {
		t.Errorf("a release's ldflags %s say development", p.ldflags())
	}
	// The linker keeps a variable's last -X, so the app's own flags come before every one of the tool's.
	p.Build.LDFlags = []string{"-X main.releaseDate=2026-11-17", "-X 'main.note=two words'"}
	p.Appearance = "dark"
	flags = p.ldflags("-X main.tool=1")
	if !strings.HasPrefix(flags, "-X main.releaseDate=2026-11-17 -X 'main.note=two words' -X ") {
		t.Errorf("ldflags %s do not start with the app's own", flags)
	}
	p.Build.LDFlags, p.Appearance = nil, ""
	if flags := p.ldflags(); strings.HasPrefix(flags, " ") {
		t.Errorf("ldflags without the app's own %q start with a space", flags)
	}
	// go build splits -ldflags at spaces outside quotes and takes no escapes inside them, so a name with a single quote goes in double quotes.
	p = &project{Name: "Bob's Notes", ID: "com.example.notes", Version: "1.0.0"}
	if want := `-X "github.com/nsgo-dev/nsgo/core.appName=Bob's Notes"`; !strings.Contains(p.ldflags(), want) {
		t.Errorf("ldflags %s lack %s", p.ldflags(), want)
	}
}

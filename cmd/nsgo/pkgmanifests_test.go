package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestCaskDualArch(t *testing.T) {
	p, _ := updatingProject(t, "")
	p.Name = "My App"
	p.ID = "dev.example.myapp"
	p.Linux.Summary = "Fast notes for daily work."
	if err := p.cleanReleaseDir(); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, p.releaseDir(), map[string]string{
		"my-app-1.2.0-macos-arm64.dmg": "arm64-dmg-bytes",
		"my-app-1.2.0-macos-x64.dmg":   "x64-dmg-bytes",
	})
	if err := p.writePackageManifests(t.Context(), []string{"macos_arm64", "macos_x64"}); err != nil {
		t.Fatal(err)
	}
	caskPath := filepath.Join(p.releaseDir(), "homebrew", "my-app.rb")
	data, err := os.ReadFile(caskPath)
	if err != nil {
		t.Fatal(err)
	}
	want := `cask "my-app" do
  arch arm: "arm64", intel: "x64"

  version "1.2.0"
  sha256 arm:   "` + sha256Hex("arm64-dmg-bytes") + `",
         intel: "` + sha256Hex("x64-dmg-bytes") + `"

  url "https://github.com/example/hello/releases/download/v#{version}/my-app-#{version}-macos-#{arch}.dmg"
  name "My App"
  desc "Fast notes for daily work"
  homepage "https://github.com/example/hello"

  auto_updates true
  depends_on :macos

  app "My App.app"

  zap trash: [
    "~/Library/Application Support/dev.example.myapp",
    "~/Library/Caches/dev.example.myapp",
    "~/Library/Logs/dev.example.myapp",
    "~/Library/Preferences/dev.example.myapp.plist",
  ]
end
`
	if string(data) != want {
		t.Fatalf("cask mismatch:\ngot:\n%s\nwant:\n%s", data, want)
	}
	if _, err := os.Stat(filepath.Join(p.releaseDir(), "winget")); err == nil {
		t.Error("macOS-only release wrote winget directory")
	}
	checkRubySyntax(t, caskPath)
}

func TestCaskSingleArchAndUniversal(t *testing.T) {
	cases := []struct {
		name      string
		targets   []string
		files     map[string]string
		wantSub   []string
		unwantSub []string
	}{
		{
			name:    "universal",
			targets: []string{"macos_universal"},
			files:   map[string]string{"hello-1.2.0-macos-universal.dmg": "uni"},
			wantSub: []string{
				`sha256 "` + sha256Hex("uni") + `"`,
				`url "https://github.com/example/hello/releases/download/v#{version}/hello-#{version}-macos-universal.dmg"`,
				"  auto_updates true\n  depends_on :macos\n",
			},
			unwantSub: []string{"arch arm:", "depends_on arch:"},
		},
		{
			name:    "arm64_only",
			targets: []string{"macos_arm64"},
			files:   map[string]string{"hello-1.2.0-macos-arm64.dmg": "arm"},
			wantSub: []string{
				`sha256 "` + sha256Hex("arm") + `"`,
				`url "https://github.com/example/hello/releases/download/v#{version}/hello-#{version}-macos-arm64.dmg"`,
				"  auto_updates true\n  depends_on arch: :arm64\n  depends_on :macos\n",
			},
			unwantSub: []string{"arch arm:"},
		},
		{
			name:    "x64_only",
			targets: []string{"macos_x64"},
			files:   map[string]string{"hello-1.2.0-macos-x64.dmg": "x64"},
			wantSub: []string{
				`sha256 "` + sha256Hex("x64") + `"`,
				`url "https://github.com/example/hello/releases/download/v#{version}/hello-#{version}-macos-x64.dmg"`,
				"  auto_updates true\n  depends_on arch: :x86_64\n  depends_on :macos\n",
			},
			unwantSub: []string{"arch arm:"},
		},
		{
			name:    "universal_preferred_when_targeted",
			targets: []string{"macos_universal"},
			files: map[string]string{
				"hello-1.2.0-macos-universal.dmg": "uni",
				"hello-1.2.0-macos-arm64.dmg":     "arm",
				"hello-1.2.0-macos-x64.dmg":       "x64",
			},
			wantSub:   []string{`sha256 "` + sha256Hex("uni") + `"`, `-macos-universal.dmg"`},
			unwantSub: []string{"arch arm:"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := updatingProject(t, "")
			if err := p.cleanReleaseDir(); err != nil {
				t.Fatal(err)
			}
			writeFiles(t, p.releaseDir(), tc.files)
			if err := p.writePackageManifests(t.Context(), tc.targets); err != nil {
				t.Fatal(err)
			}
			caskPath := filepath.Join(p.releaseDir(), "homebrew", "hello.rb")
			data, err := os.ReadFile(caskPath)
			if err != nil {
				t.Fatal(err)
			}
			text := string(data)
			for _, w := range tc.wantSub {
				if !strings.Contains(text, w) {
					t.Errorf("cask lacks %q:\n%s", w, text)
				}
			}
			for _, u := range tc.unwantSub {
				if strings.Contains(text, u) {
					t.Errorf("cask unexpectedly contains %q:\n%s", u, text)
				}
			}
			checkRubySyntax(t, caskPath)
		})
	}
}

func TestCaskEscapingAndCustomFeed(t *testing.T) {
	p, _ := updatingProject(t, ", feed: 'https://downloads.example.com/app'")
	p.Releases.GitHub = ""
	p.Updates.repo = ""
	p.Name = `Bob's #{App}\Tool`
	p.ID = `dev.example.bobs`
	p.Version = "1.2.0 beta"
	p.Linux.Summary = `An app with "quotes" and #{interp}\slash.`
	if err := p.cleanReleaseDir(); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, p.releaseDir(), map[string]string{
		p.releaseName("macos_universal", ".dmg"): "uni-bytes",
	})
	if err := p.writePackageManifests(t.Context(), []string{"macos_universal"}); err != nil {
		t.Fatal(err)
	}
	caskPath := filepath.Join(p.releaseDir(), "homebrew", p.packageSlug()+".rb")
	data, err := os.ReadFile(caskPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		`url "https://downloads.example.com/app/` + p.packageSlug() + `-1.2.0%20beta-macos-universal.dmg"`,
		`name "Bob's \#{App}\\Tool"`,
		`desc "App with \"quotes\" and \#{interp}\\slash"`,
		`homepage "https://downloads.example.com/app/"`,
		`app "Bob's \#{App}\\Tool.app"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("cask lacks %s:\n%s", want, text)
		}
	}
	checkRubySyntax(t, caskPath)
}

func TestCaskDescSanitization(t *testing.T) {
	for _, tc := range []struct {
		summary, token, appName, want string
	}{
		{"", "my-app", "My App", "Desktop application"},
		{"a fast command line viewer.", "viewer", "Viewer", "Fast command-line viewer"},
		{"The quick notes tool.", "notes", "Notes", "Quick notes tool"},
		{"My App does everything", "my-app", "My App", "Desktop application"},
		{"Fast editor for macOS", "editor", "Editor", "Desktop application"},
		{"🚀 Rocket launcher.", "rocket", "Rocket", "Desktop application"},
		{"🚀 Fast launcher.", "foo", "Foo", "Fast launcher"},
		{"Supports etc.", "foo", "Foo", "Supports etc."},
		{"iOS companion tool", "foo", "Foo", "iOS companion tool"},
	} {
		if got := homebrewDesc(tc.summary, tc.token, tc.appName); got != tc.want {
			t.Errorf("homebrewDesc(%q, %q, %q) = %q, want %q", tc.summary, tc.token, tc.appName, got, tc.want)
		}
	}
}

func TestWingetManifestMultiArch(t *testing.T) {
	p, _ := updatingProject(t, "")
	p.Name = "My App"
	p.ID = "dev.example.myapp"
	p.Windows.Publisher = "Example Ltd"
	p.Copyright = "© 2026 Example Ltd"
	p.Linux.Summary = "Fast notes for daily work"
	p.Linux.License = "MIT"
	if err := p.cleanReleaseDir(); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, p.releaseDir(), map[string]string{
		"my-app-1.2.0-windows-x64-setup.exe":   "x64-setup",
		"my-app-1.2.0-windows-arm64-setup.exe": "arm64-setup",
	})
	if err := p.writePackageManifests(t.Context(), []string{"windows_x64", "windows_arm64"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(p.releaseDir(), "homebrew")); err == nil {
		t.Error("Windows-only release wrote homebrew directory")
	}
	wingetDir := filepath.Join(p.releaseDir(), "winget")
	verBytes, err := os.ReadFile(filepath.Join(wingetDir, "dev.example.myapp.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	instBytes, err := os.ReadFile(filepath.Join(wingetDir, "dev.example.myapp.installer.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	locBytes, err := os.ReadFile(filepath.Join(wingetDir, "dev.example.myapp.locale.en-US.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify schema comments.
	if !strings.HasPrefix(string(verBytes), "# yaml-language-server: $schema=https://aka.ms/winget-manifest.version.1.9.0.schema.json\n\n") {
		t.Errorf("version manifest header:\n%s", verBytes)
	}
	if !strings.HasPrefix(string(instBytes), "# yaml-language-server: $schema=https://aka.ms/winget-manifest.installer.1.9.0.schema.json\n\n") {
		t.Errorf("installer manifest header:\n%s", instBytes)
	}
	if !strings.HasPrefix(string(locBytes), "# yaml-language-server: $schema=https://aka.ms/winget-manifest.defaultLocale.1.9.0.schema.json\n\n") {
		t.Errorf("locale manifest header:\n%s", locBytes)
	}

	// Strict round-trip decode (KnownFields refuses unknown keys).
	var ver wingetVersionManifest
	mustDecodeWingetYAML(t, verBytes, &ver)
	if ver.PackageIdentifier != "dev.example.myapp" || ver.PackageVersion != "1.2.0" ||
		ver.DefaultLocale != "en-US" || ver.ManifestType != "version" || ver.ManifestVersion != "1.9.0" {
		t.Fatalf("version manifest = %+v", ver)
	}

	var inst wingetInstallerManifest
	mustDecodeWingetYAML(t, instBytes, &inst)
	if inst.PackageIdentifier != "dev.example.myapp" || inst.PackageVersion != "1.2.0" ||
		inst.InstallerType != "exe" || inst.Scope != "user" || inst.UpgradeBehavior != "install" ||
		inst.ManifestType != "installer" || inst.ManifestVersion != "1.9.0" {
		t.Fatalf("installer manifest = %+v", inst)
	}
	if inst.InstallerSwitches == nil || inst.InstallerSwitches.Silent != "/S" || inst.InstallerSwitches.SilentWithProgress != "/S" || inst.InstallerSwitches.InstallLocation != "/D=<INSTALLPATH>" {
		t.Fatalf("installer switches = %+v", inst.InstallerSwitches)
	}
	if len(inst.AppsAndFeaturesEntries) != 1 || inst.AppsAndFeaturesEntries[0].DisplayName != "My App" ||
		inst.AppsAndFeaturesEntries[0].Publisher != "Example Ltd" || inst.AppsAndFeaturesEntries[0].DisplayVersion != "1.2.0" {
		t.Fatalf("apps and features = %+v", inst.AppsAndFeaturesEntries)
	}
	if len(inst.Installers) != 2 {
		t.Fatalf("installers count = %d, want 2", len(inst.Installers))
	}
	wantX64 := wingetInstallerEntry{
		Architecture:    "x64",
		InstallerUrl:    "https://github.com/example/hello/releases/download/v1.2.0/my-app-1.2.0-windows-x64-setup.exe",
		InstallerSha256: strings.ToUpper(sha256Hex("x64-setup")),
	}
	wantARM64 := wingetInstallerEntry{
		Architecture:    "arm64",
		InstallerUrl:    "https://github.com/example/hello/releases/download/v1.2.0/my-app-1.2.0-windows-arm64-setup.exe",
		InstallerSha256: strings.ToUpper(sha256Hex("arm64-setup")),
	}
	if inst.Installers[0] != wantX64 || inst.Installers[1] != wantARM64 {
		t.Fatalf("installers = %+v, want [%+v, %+v]", inst.Installers, wantX64, wantARM64)
	}

	var loc wingetDefaultLocaleManifest
	mustDecodeWingetYAML(t, locBytes, &loc)
	if loc.PackageIdentifier != "dev.example.myapp" || loc.PackageVersion != "1.2.0" ||
		loc.PackageLocale != "en-US" || loc.Publisher != "Example Ltd" ||
		loc.PublisherUrl != "https://github.com/example/hello" || loc.PackageName != "My App" ||
		loc.PackageUrl != "https://github.com/example/hello" || loc.License != "MIT" ||
		loc.Copyright != "© 2026 Example Ltd" || loc.ShortDescription != "Fast notes for daily work" ||
		loc.ReleaseNotesUrl != "https://github.com/example/hello/releases/tag/v1.2.0" ||
		loc.ManifestType != "defaultLocale" || loc.ManifestVersion != "1.9.0" {
		t.Fatalf("locale manifest = %+v", loc)
	}

	validateWingetSchema(t, ver, inst, loc)
	if runtime.GOOS == "windows" {
		if winget, err := exec.LookPath("winget"); err == nil {
			out, err := exec.Command(winget, "validate", "--manifest", wingetDir).CombinedOutput()
			if err != nil {
				t.Fatalf("winget validate failed: %v\n%s", err, out)
			}
		}
	}
}

func TestWingetManifestEscapingAndNumericVersion(t *testing.T) {
	p, _ := updatingProject(t, ", feed: 'https://downloads.example.com/app'")
	p.Releases.GitHub = ""
	p.Updates.repo = ""
	p.Name = `App: "Quoted" #1`
	p.ID = `singleid`
	p.Version = "1.0"
	p.Windows.Publisher = "ACME: Corp & Sons"
	p.Linux.Summary = "Line one\nLine two: *special*"
	if err := p.cleanReleaseDir(); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, p.releaseDir(), map[string]string{
		p.releaseName("windows_x64", "-setup.exe"): "exe",
	})
	if err := p.writePackageManifests(t.Context(), []string{"windows_x64"}); err != nil {
		t.Fatal(err)
	}
	pkgID := "ACMECorp&Sons.singleid"
	wingetDir := filepath.Join(p.releaseDir(), "winget")
	verBytes, err := os.ReadFile(filepath.Join(wingetDir, pkgID+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	instBytes, err := os.ReadFile(filepath.Join(wingetDir, pkgID+".installer.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	locBytes, err := os.ReadFile(filepath.Join(wingetDir, pkgID+".locale.en-US.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	// Numeric-looking version "1.0" must round-trip as a YAML string, not float 1.
	var raw map[string]any
	if err := yaml.Unmarshal(verBytes, &raw); err != nil {
		t.Fatal(err)
	}
	if v, ok := raw["PackageVersion"].(string); !ok || v != "1.0" {
		t.Fatalf("PackageVersion decoded as %T (%v), want string \"1.0\"", raw["PackageVersion"], raw["PackageVersion"])
	}

	var ver wingetVersionManifest
	var inst wingetInstallerManifest
	var loc wingetDefaultLocaleManifest
	mustDecodeWingetYAML(t, verBytes, &ver)
	mustDecodeWingetYAML(t, instBytes, &inst)
	mustDecodeWingetYAML(t, locBytes, &loc)
	if loc.PackageName != `App: "Quoted" #1` || loc.Publisher != "ACME: Corp & Sons" ||
		loc.ShortDescription != "Line one Line two: *special*" || loc.License != "Proprietary" ||
		loc.ReleaseNotesUrl != "" {
		t.Fatalf("locale manifest = %+v", loc)
	}
	validateWingetSchema(t, ver, inst, loc)
}

func TestManifestReleaseIntegrationAndDeterminism(t *testing.T) {
	p, _ := updatingProject(t, "")
	p.Name = "Studio"
	p.ID = "dev.example.studio"
	p.Windows.Publisher = "Example Studio"
	p.Linux.Summary = "Creative desktop studio"
	if err := p.cleanReleaseDir(); err != nil {
		t.Fatal(err)
	}

	// Simulate separate builds for the same version adding artifacts to dist/releases.
	writeFiles(t, p.releaseDir(), map[string]string{
		"studio-1.2.0-macos-arm64.dmg":       "mac-arm",
		"studio-1.2.0-windows-x64-setup.exe": "win-x64",
	})
	if err := p.writePackageManifests(t.Context(), []string{"macos_arm64", "windows_x64"}); err != nil {
		t.Fatal(err)
	}
	caskPath := filepath.Join(p.releaseDir(), "homebrew", "studio.rb")
	instPath := filepath.Join(p.releaseDir(), "winget", "dev.example.studio.installer.yaml")
	firstCask, err := os.ReadFile(caskPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(firstCask), "depends_on arch: :arm64") {
		t.Fatalf("first cask should be arm64-only:\n%s", firstCask)
	}

	// Second build adds the other architecture for each OS.
	writeFiles(t, p.releaseDir(), map[string]string{
		"studio-1.2.0-macos-x64.dmg":           "mac-x64",
		"studio-1.2.0-windows-arm64-setup.exe": "win-arm64",
	})
	if err := p.writePackageManifests(t.Context(), []string{"macos_x64", "windows_arm64"}); err != nil {
		t.Fatal(err)
	}
	secondCask, err := os.ReadFile(caskPath)
	if err != nil {
		t.Fatal(err)
	}
	secondInst, err := os.ReadFile(instPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(secondCask), `arch arm: "arm64", intel: "x64"`) {
		t.Fatalf("merged cask should be dual-arch:\n%s", secondCask)
	}
	var inst wingetInstallerManifest
	mustDecodeWingetYAML(t, secondInst, &inst)
	if len(inst.Installers) != 2 || inst.Installers[0].Architecture != "x64" || inst.Installers[1].Architecture != "arm64" {
		t.Fatalf("merged winget installers = %+v", inst.Installers)
	}

	// Re-running must produce byte-for-byte identical files.
	if err := p.writePackageManifests(t.Context(), []string{"macos_arm64", "macos_x64", "windows_x64", "windows_arm64"}); err != nil {
		t.Fatal(err)
	}
	thirdCask, _ := os.ReadFile(caskPath)
	thirdInst, _ := os.ReadFile(instPath)
	if !bytes.Equal(secondCask, thirdCask) {
		t.Error("Homebrew cask output is not deterministic")
	}
	if !bytes.Equal(secondInst, thirdInst) {
		t.Error("winget installer manifest output is not deterministic")
	}
}

func TestManifestSkippedWhenInapplicable(t *testing.T) {
	checkNoneWritten := func(t *testing.T, p *project, targets []string) {
		t.Helper()
		if err := p.writePackageManifests(t.Context(), targets); err != nil {
			t.Fatal(err)
		}
		for _, sub := range []string{"homebrew", "winget"} {
			if _, err := os.Stat(filepath.Join(p.releaseDir(), sub)); err == nil {
				t.Errorf("unexpected %s manifest directory written", sub)
			}
		}
	}

	// Non-direct channels skip manifests.
	for _, ch := range []string{channelStore, channelSteam, channelNone} {
		t.Run("channel_"+ch, func(t *testing.T) {
			p, _ := updatingProject(t, "")
			p.Updates.Channel = ch
			if err := p.cleanReleaseDir(); err != nil {
				t.Fatal(err)
			}
			writeFiles(t, p.releaseDir(), map[string]string{
				"hello-1.2.0-macos-arm64.dmg":       "dmg",
				"hello-1.2.0-windows-x64-setup.exe": "exe",
			})
			checkNoneWritten(t, p, []string{"macos_arm64", "windows_x64"})
		})
	}

	// -installers=false skips manifests.
	t.Run("no_installers", func(t *testing.T) {
		p, _ := updatingProject(t, "")
		no := false
		p.Build.Installers = &no
		if err := p.cleanReleaseDir(); err != nil {
			t.Fatal(err)
		}
		writeFiles(t, p.releaseDir(), map[string]string{
			"hello-1.2.0-macos-arm64.dmg":       "dmg",
			"hello-1.2.0-windows-x64-setup.exe": "exe",
		})
		checkNoneWritten(t, p, []string{"macos_arm64", "windows_x64"})
	})

	t.Run("skip_macos", func(t *testing.T) {
		p, _ := updatingProject(t, "")
		p.Build.SkipInstallers = []string{"macos"}
		if err := p.cleanReleaseDir(); err != nil {
			t.Fatal(err)
		}
		writeFiles(t, p.releaseDir(), map[string]string{
			p.releaseName("macos_arm64", ".dmg"):       "dmg from an earlier build",
			p.releaseName("windows_x64", "-setup.exe"): "exe",
		})
		if err := p.writePackageManifests(t.Context(), []string{"macos_arm64", "windows_x64"}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(p.releaseDir(), "homebrew")); !os.IsNotExist(err) {
			t.Errorf("Homebrew cask directory: want none, got %v", err)
		}
		if _, err := os.Stat(filepath.Join(p.releaseDir(), "winget", p.ID+".installer.yaml")); err != nil {
			t.Errorf("winget installer manifest: %v", err)
		}
	})

	// Missing release URL skips manifests without error.
	t.Run("no_release_url", func(t *testing.T) {
		p, _ := updatingProject(t, "")
		p.Releases.GitHub = ""
		p.Updates.repo = ""
		p.Updates.Feed = ""
		if err := p.cleanReleaseDir(); err != nil {
			t.Fatal(err)
		}
		writeFiles(t, p.releaseDir(), map[string]string{
			"hello-1.2.0-macos-arm64.dmg":       "dmg",
			"hello-1.2.0-windows-x64-setup.exe": "exe",
		})
		ctx := withProcessRunner(t, func(_ context.Context, _ processCommand) ([]byte, error) {
			return nil, os.ErrNotExist
		})
		if err := p.writePackageManifests(ctx, []string{"macos_arm64", "windows_x64"}); err != nil {
			t.Fatal(err)
		}
		for _, sub := range []string{"homebrew", "winget"} {
			if _, err := os.Stat(filepath.Join(p.releaseDir(), sub)); err == nil {
				t.Errorf("unexpected %s manifest written without release URL", sub)
			}
		}
	})

	// Missing built installers (e.g. installers disabled) writes nothing.
	t.Run("no_built_files", func(t *testing.T) {
		p, _ := updatingProject(t, "")
		if err := p.cleanReleaseDir(); err != nil {
			t.Fatal(err)
		}
		checkNoneWritten(t, p, []string{"macos_arm64", "windows_x64"})
	})
}

func TestManifestGitRemoteLookup(t *testing.T) {
	p, _ := updatingProject(t, "")
	p.Releases.GitHub = ""
	p.Updates.repo = ""
	if err := p.cleanReleaseDir(); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, p.releaseDir(), map[string]string{
		"hello-1.2.0-macos-universal.dmg": "uni",
	})
	ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
		want := []string{"-C", p.Dir, "remote", "get-url", "origin"}
		if cmd.name != "git" || !slices.Equal(cmd.args, want) {
			t.Fatalf("unexpected command %s %q", cmd.name, cmd.args)
		}
		return []byte("https://github.com/acme/hello.git\n"), nil
	})
	if err := p.writePackageManifests(ctx, []string{"macos_universal"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(p.releaseDir(), "homebrew", "hello.rb"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `homepage "https://github.com/acme/hello"`) ||
		!strings.Contains(string(data), `url "https://github.com/acme/hello/releases/download/v#{version}/hello-#{version}-macos-universal.dmg"`) {
		t.Fatalf("git remote not used in cask:\n%s", data)
	}
}

// TestCaskBrewAudit runs Homebrew's own style and strict cask audit
// checks when NSGO_BREW_AUDIT=1 and brew is installed.
func TestCaskBrewAudit(t *testing.T) {
	if os.Getenv("NSGO_BREW_AUDIT") != "1" {
		t.Skip("set NSGO_BREW_AUDIT=1 to run Homebrew style and strict cask audits")
	}
	brew, err := exec.LookPath("brew")
	if err != nil {
		t.Skip("brew not installed")
	}
	for _, tc := range []struct {
		name    string
		targets []string
		files   map[string]string
		feed    string
	}{
		{
			name:    "dual_arch",
			targets: []string{"macos_arm64", "macos_x64"},
			files: map[string]string{
				"nsgo-demo-1.2.0-macos-arm64.dmg": "arm",
				"nsgo-demo-1.2.0-macos-x64.dmg":   "x64",
			},
		},
		{
			name:    "universal",
			targets: []string{"macos_universal"},
			files:   map[string]string{"nsgo-demo-1.2.0-macos-universal.dmg": "uni"},
		},
		{
			name:    "arm_only",
			targets: []string{"macos_arm64"},
			files:   map[string]string{"nsgo-demo-1.2.0-macos-arm64.dmg": "arm"},
		},
		{
			name:    "x64_only",
			targets: []string{"macos_x64"},
			files:   map[string]string{"nsgo-demo-1.2.0-macos-x64.dmg": "x64"},
		},
		{
			name:    "custom_feed",
			targets: []string{"macos_universal"},
			files:   map[string]string{"nsgo-demo-1.2.0-macos-universal.dmg": "uni"},
			feed:    ", feed: 'https://downloads.example.com/nsgo-demo'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := updatingProject(t, tc.feed)
			p.Name = "NSGo Demo"
			p.ID = "dev.example.nsgodemo"
			p.Linux.Summary = "Fast notes for daily work."
			if err := p.cleanReleaseDir(); err != nil {
				t.Fatal(err)
			}
			writeFiles(t, p.releaseDir(), tc.files)
			if err := p.writePackageManifests(t.Context(), tc.targets); err != nil {
				t.Fatal(err)
			}
			src := filepath.Join(p.releaseDir(), "homebrew", "nsgo-demo.rb")
			// Homebrew's RuboCop cask cops match files under Casks/<token>.rb.
			casksDir := filepath.Join(t.TempDir(), "Casks")
			if err := os.MkdirAll(casksDir, 0o755); err != nil {
				t.Fatal(err)
			}
			caskPath := filepath.Join(casksDir, "nsgo-demo.rb")
			raw, err := os.ReadFile(src)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(caskPath, raw, 0o644); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(brew, "style", caskPath).CombinedOutput(); err != nil {
				t.Fatalf("brew style failed: %v\n%s", err, out)
			}
			rb := `require "cask/cask_loader"
require "cask/audit"
cask = Cask::CaskLoader::FromPathLoader.new(Pathname(ARGV[0])).load(config: nil)
audit = Cask::Audit.new(cask, strict: true, online: false, download: false, signing: false)
audit.run!
abort(audit.summary) unless audit.success?`
			if out, err := exec.Command(brew, "ruby", "-e", rb, "--", caskPath).CombinedOutput(); err != nil {
				t.Fatalf("Cask::Audit failed: %v\n%s", err, out)
			}
		})
	}
}

func checkRubySyntax(t *testing.T, path string) {
	t.Helper()
	ruby, err := exec.LookPath("ruby")
	if err != nil {
		return
	}
	if out, err := exec.Command(ruby, "-c", path).CombinedOutput(); err != nil {
		t.Fatalf("ruby -c %s failed: %v\n%s", path, err, out)
	}
}

func mustDecodeWingetYAML(t *testing.T, data []byte, dst any) {
	t.Helper()
	if err := decodeStrictYAML(data, dst); err != nil {
		t.Fatalf("strict YAML decode failed: %v\n%s", err, data)
	}
}

// validateWingetSchema checks the generated structs against the official
// winget-manifest v1.9.0 JSON schema patterns, enums and length bounds.
func validateWingetSchema(t *testing.T, ver wingetVersionManifest, inst wingetInstallerManifest, loc wingetDefaultLocaleManifest) {
	t.Helper()
	pkgIDRE := regexp.MustCompile(`^[^\.\s\\/:\*\?"<>\|\x01-\x1f]{1,32}(\.[^\.\s\\/:\*\?"<>\|\x01-\x1f]{1,32}){1,7}$`)
	pkgVerRE := regexp.MustCompile(`^[^\\/:\*\?"<>\|\x01-\x1f]+$`)
	shaRE := regexp.MustCompile(`^[A-Fa-f0-9]{64}$`)
	urlRE := regexp.MustCompile(`^https?://`)

	if !pkgIDRE.MatchString(ver.PackageIdentifier) || len(ver.PackageIdentifier) > 128 {
		t.Errorf("invalid PackageIdentifier %q", ver.PackageIdentifier)
	}
	if !pkgVerRE.MatchString(ver.PackageVersion) || len(ver.PackageVersion) > 128 {
		t.Errorf("invalid PackageVersion %q", ver.PackageVersion)
	}
	if inst.PackageIdentifier != ver.PackageIdentifier || loc.PackageIdentifier != ver.PackageIdentifier {
		t.Errorf("PackageIdentifier mismatch across manifests")
	}
	if inst.PackageVersion != ver.PackageVersion || loc.PackageVersion != ver.PackageVersion {
		t.Errorf("PackageVersion mismatch across manifests")
	}
	if loc.PackageLocale != ver.DefaultLocale {
		t.Errorf("PackageLocale %q != DefaultLocale %q", loc.PackageLocale, ver.DefaultLocale)
	}
	if len(inst.Installers) == 0 {
		t.Errorf("Installers must not be empty")
	}
	for _, entry := range inst.Installers {
		if !slices.Contains([]string{"x86", "x64", "arm", "arm64", "neutral"}, entry.Architecture) {
			t.Errorf("invalid Architecture %q", entry.Architecture)
		}
		if !urlRE.MatchString(entry.InstallerUrl) || len(entry.InstallerUrl) > 2048 {
			t.Errorf("invalid InstallerUrl %q", entry.InstallerUrl)
		}
		if !shaRE.MatchString(entry.InstallerSha256) {
			t.Errorf("invalid InstallerSha256 %q", entry.InstallerSha256)
		}
	}
	if len(loc.Publisher) < 2 || len(loc.Publisher) > 256 {
		t.Errorf("Publisher length out of bounds: %q", loc.Publisher)
	}
	if len(loc.PackageName) < 2 || len(loc.PackageName) > 256 {
		t.Errorf("PackageName length out of bounds: %q", loc.PackageName)
	}
	if len(loc.License) < 3 || len(loc.License) > 512 {
		t.Errorf("License length out of bounds: %q", loc.License)
	}
	if len(loc.ShortDescription) < 3 || len(loc.ShortDescription) > 256 {
		t.Errorf("ShortDescription length out of bounds: %q", loc.ShortDescription)
	}
}

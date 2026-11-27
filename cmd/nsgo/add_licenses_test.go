package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// An offline module with nsgo-natives.yaml entries for two targets lists each target's native libraries, their classified licenses and their download hosts, reading import packages from nsgo-module.yaml when present (#2463).
func TestAddShowsTargetsNativeLibrariesAndHosts(t *testing.T) {
	proxy := libraryProxy(t)
	mit := licenseFixtureText(t, "MIT")
	lgpl := licenseFixtureText(t, "LGPL-2.1")
	sha := strings.Repeat("a", 64)
	natives := fmt.Sprintf(`version: 2
libraries:
  macos_arm64:
    - name: libcodec.dylib
      url: https://mac-cdn.example.com/releases/libcodec.dylib
      sha256: %s
      licenses: [notices/COPYING.LGPLv2.1]
  linux_x64:
    - name: libcodec.so.1
      url: https://linux-dl.example.org/files/libcodec.so.1
      sha256: %s
      licenses: [notices/COPYING.LGPLv2.1]
  ios:
    - name: Codec.framework
      url: https://apple.example.net/Codec.framework
      sha256: %s
      licenses: [notices/MIT.txt]
`, sha, sha, sha)
	addProxyModule(t, proxy, "example.com/media", "v1.0.0", "module example.com/media\n\ngo 1.26\n", map[string]string{
		"nsgo-module.yaml":                "version: 1\nimport: [player]\n",
		"LICENSE":                         mit,
		"player/player.go":                "package player\n",
		"player/" + nativeManifestName:    natives,
		"player/notices/COPYING.LGPLv2.1": lgpl,
		"player/notices/MIT.txt":          mit,
	})
	offlineAdd(t, proxy)

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.26\n",
		"nsgo.yaml": `name: MediaApp
version: 1.0.0
build:
  targets: [macos_arm64, linux_x64, ios]
`,
	})
	plan, err := planAdd(t, dir, "example.com/media")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, plan)
	}
	for _, want := range []string{
		"  licenses   example.com/media: MIT\n",
		"example.com/media native library libcodec.dylib: LGPL-2.1 (copyleft; licenses.copyleft: warn)",
		"example.com/media native library libcodec.so.1: LGPL-2.1 (copyleft; licenses.copyleft: warn)",
		"  targets    macos_arm64: libcodec.dylib (LGPL-2.1, from mac-cdn.example.com)\n",
		"             linux_x64: libcodec.so.1 (LGPL-2.1, from linux-dl.example.org)\n",
		"             ios: Codec.framework (MIT, from apple.example.net)\n",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan lacks %q:\n%s", want, plan)
		}
	}
}

// Under licenses.copyleft: error, a copyleft native library is refused under -dry-run and -y with the module and licenses.allow line to add, leaving go.mod and go.sum untouched; listing the module in licenses.allow lets the plan proceed (#2463).
func TestAddRefusesCopyleftNativeLibraryAndAllowsExplicit(t *testing.T) {
	proxy := libraryProxy(t)
	mit := licenseFixtureText(t, "MIT")
	lgpl := licenseFixtureText(t, "LGPL-2.1")
	sha := strings.Repeat("b", 64)
	natives := fmt.Sprintf(`version: 1
libraries:
  macos_arm64:
    - name: libaudio.dylib
      url: https://cdn.example.com/libaudio.dylib
      sha256: %s
      licenses: [notices/COPYING.LGPLv2.1]
  linux_x64: []
`, sha)
	addProxyModule(t, proxy, "example.com/audio", "v1.0.0", "module example.com/audio\n\ngo 1.26\n", map[string]string{
		"audio.go":                 "package audio\n",
		"LICENSE":                  mit,
		nativeManifestName:         natives,
		"notices/COPYING.LGPLv2.1": lgpl,
	})
	offlineAdd(t, proxy)

	dir := t.TempDir()
	const gomod = "module example.com/app\n\ngo 1.26\n"
	writeFiles(t, dir, map[string]string{
		"go.mod": gomod,
		"nsgo.yaml": `name: AudioApp
version: 1.0.0
build:
  targets: [macos_arm64, linux_x64]
licenses:
  copyleft: error
`,
	})
	oldMod, oldSum := moduleFiles(t, dir)

	for _, o := range []addOptions{{dryRun: true}, {yes: true}} {
		plan, err := runAdd(t, dir, o, nil, "example.com/audio")
		if err == nil {
			t.Fatalf("%+v: expected refusal, got plan:\n%s", o, plan)
		}
		for _, want := range []string{
			"example.com/audio",
			"libaudio.dylib",
			"LGPL-2.1",
			"licenses.allow: [example.com/audio]",
			"licenses.copyleft: warn",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%+v: error %q lacks %q", o, err.Error(), want)
			}
		}
		if !strings.Contains(plan, "macos_arm64: libaudio.dylib (LGPL-2.1, from cdn.example.com)") {
			t.Errorf("%+v: plan lacks native library detail:\n%s", o, plan)
		}
		if gotMod, gotSum := moduleFiles(t, dir); !bytes.Equal(gotMod, oldMod) || !bytes.Equal(gotSum, oldSum) {
			t.Fatalf("%+v: refused add modified go.mod or go.sum:\n%s\n%s", o, gotMod, gotSum)
		}
	}

	// Listing the module in licenses.allow lets the plan go on and apply cleanly.
	writeFiles(t, dir, map[string]string{
		"nsgo.yaml": `name: AudioApp
version: 1.0.0
build:
  targets: [macos_arm64, linux_x64]
licenses:
  copyleft: error
  allow: [example.com/audio]
`,
	})
	plan, err := runAdd(t, dir, addOptions{yes: true}, nil, "example.com/audio")
	if err != nil {
		t.Fatalf("allowed module refused: %v\n%s", err, plan)
	}
	if !strings.Contains(plan, "allowed by licenses.allow") {
		t.Errorf("plan lacks allowed note:\n%s", plan)
	}
	if gotMod, _ := moduleFiles(t, dir); !strings.Contains(string(gotMod), "example.com/audio v1.0.0") {
		t.Errorf("go.mod lacks allowed module:\n%s", gotMod)
	}

	// A copyleft Go module is also refused under licenses.copyleft: error and admitted under warn.
	mpl := licenseFixtureText(t, "MPL-2.0")
	addProxyModule(t, proxy, "example.com/mplmod", "v1.0.0", "module example.com/mplmod\n\ngo 1.26\n", map[string]string{
		"mpl.go":  "package mplmod\n",
		"LICENSE": mpl,
	})
	plan, err = planAdd(t, dir, "example.com/mplmod")
	if err == nil || !strings.Contains(err.Error(), "example.com/mplmod has copyleft license text (MPL-2.0): add licenses.allow: [example.com/mplmod] or set licenses.copyleft: warn in nsgo.yaml") {
		t.Fatalf("copyleft module error = %v\nplan:\n%s", err, plan)
	}
	writeFiles(t, dir, map[string]string{
		"nsgo.yaml": `name: AudioApp
version: 1.0.0
build:
  targets: [macos_arm64, linux_x64]
licenses:
  copyleft: warn
`,
	})
	plan, err = planAdd(t, dir, "example.com/mplmod")
	if err != nil || !strings.Contains(plan, "example.com/mplmod: MPL-2.0 (copyleft; licenses.copyleft: warn)") {
		t.Fatalf("copyleft warn: err=%v\nplan:\n%s", err, plan)
	}
}

// A module with no recognizable license text is refused under the default policy without touching go.mod/go.sum, and shown and admitted under licenses.unknown: warn (#2463).
func TestAddUnknownLicensePolicy(t *testing.T) {
	proxy := libraryProxy(t)
	addProxyModule(t, proxy, "example.com/nolicense", "v1.0.0", "module example.com/nolicense\n\ngo 1.26\n", map[string]string{
		"nolicense.go": "package nolicense\n",
	})
	addProxyModule(t, proxy, "example.com/customlic", "v1.0.0", "module example.com/customlic\n\ngo 1.26\n", map[string]string{
		"customlic.go": "package customlic\n",
		"LICENSE":      "Proprietary terms nobody recognizes.\n",
	})
	offlineAdd(t, proxy)

	dir := t.TempDir()
	const gomod = "module example.com/app\n\ngo 1.26\n"
	writeFiles(t, dir, map[string]string{"go.mod": gomod})
	oldMod, oldSum := moduleFiles(t, dir)

	for _, c := range []struct {
		mod, wantErr string
	}{
		{"example.com/nolicense", "example.com/nolicense has no recognized license text: add licenses.allow: [example.com/nolicense] or set licenses.unknown: warn in nsgo.yaml"},
		{"example.com/customlic", "example.com/customlic has unrecognized license text in LICENSE: add licenses.allow: [example.com/customlic] or set licenses.unknown: warn in nsgo.yaml"},
	} {
		plan, err := runAdd(t, dir, addOptions{yes: true}, nil, c.mod)
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("%s: got err %v, want %q\nplan:\n%s", c.mod, err, c.wantErr, plan)
		}
		if gotMod, gotSum := moduleFiles(t, dir); !bytes.Equal(gotMod, oldMod) || !bytes.Equal(gotSum, oldSum) {
			t.Fatalf("%s: refused add modified go.mod or go.sum", c.mod)
		}
	}

	writeFiles(t, dir, map[string]string{
		"nsgo.yaml": `name: WarnApp
version: 1.0.0
build:
  targets: [linux_x64]
licenses:
  unknown: warn
`,
	})
	plan, err := planAdd(t, dir, "example.com/nolicense", "example.com/customlic")
	if err != nil {
		t.Fatalf("licenses.unknown: warn failed: %v\n%s", err, plan)
	}
	for _, want := range []string{
		"  licenses   example.com/nolicense: unknown (licenses.unknown: warn)\n",
		"  licenses   example.com/customlic: unknown (unrecognized in LICENSE) (licenses.unknown: warn)\n",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan lacks %q:\n%s", want, plan)
		}
	}
}

// A package excluded by build constraints on one target is reported for that target while other targets still list cleanly (#2463).
func TestAddReportsBuildConstraintExcludedPackages(t *testing.T) {
	proxy := libraryProxy(t)
	mit := licenseFixtureText(t, "MIT")
	sha := strings.Repeat("c", 64)
	addProxyModule(t, proxy, "example.com/linuxonly", "v1.0.0", "module example.com/linuxonly\n\ngo 1.26\n", map[string]string{
		"linux.go":         "//go:build linux\n\npackage linuxonly\n",
		"LICENSE":          mit,
		nativeManifestName: fmt.Sprintf("version: 1\nlibraries:\n  linux_x64:\n    - name: liblinux.so.1\n      url: https://pkg.example.com/liblinux.so.1\n      sha256: %s\n      licenses: [LICENSE]\n", sha),
	})
	offlineAdd(t, proxy)

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.26\n",
		"nsgo.yaml": `name: MultiTarget
version: 1.0.0
build:
  targets: [macos_arm64, linux_x64]
`,
	})
	plan, err := planAdd(t, dir, "example.com/linuxonly")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, plan)
	}
	for _, want := range []string{
		"  licenses   example.com/linuxonly: MIT\n",
		"macos_arm64: example.com/linuxonly does not build (build constraints exclude all Go files)",
		"linux_x64: liblinux.so.1 (MIT, from pkg.example.com)",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan lacks %q:\n%s", want, plan)
		}
	}
}

// A registry item whose declared license differs from what the classifier detected is noted as the registry saying otherwise (#2463).
func TestAddShowsRegistryLicenseMismatch(t *testing.T) {
	proxy := libraryProxy(t)
	apache := licenseFixtureText(t, "Apache-2.0")
	lgpl := licenseFixtureText(t, "LGPL-2.1")
	addProxyModule(t, proxy, "example.com/apache", "v1.0.0", "module example.com/apache\n\ngo 1.26\n", map[string]string{
		"apache.go": "package apache\n",
		"LICENSE":   apache,
	})
	addProxyModule(t, proxy, "example.com/lgplmod", "v1.0.0", "module example.com/lgplmod\n\ngo 1.26\n", map[string]string{
		"lgpl.go": "package lgplmod\n",
		"LICENSE": lgpl,
	})
	offlineAdd(t, proxy)

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.26\n",
		"nsgo.yaml": `name: RegistryCheck
version: 1.0.0
build:
  targets: [linux_x64]
`,
	})
	t.Chdir(dir)
	m, err := findAddModule(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	p, err := m.plan(t.Context(), []addRequest{
		{path: "example.com/apache", version: "upgrade", registryLicense: "MIT"},
		{path: "example.com/lgplmod", version: "upgrade", registryLicense: "LGPL-2.1-or-later"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := p.write(&out); err != nil {
		t.Fatal(err)
	}
	plan := out.String()
	if !strings.Contains(plan, "example.com/apache: Apache-2.0 (registry says MIT)") {
		t.Errorf("plan lacks registry mismatch note:\n%s", plan)
	}
	if strings.Contains(plan, "registry says LGPL-2.1-or-later") {
		t.Errorf("plan falsely reported mismatch for matching LGPL-2.1-or-later:\n%s", plan)
	}
}

func TestLicenseMatchesRegistry(t *testing.T) {
	for _, c := range []struct {
		detected []string
		registry string
		want     bool
	}{
		{[]string{"MIT"}, "MIT", true},
		{[]string{"Apache-2.0"}, "MIT", false},
		{[]string{"LGPL-2.1"}, "LGPL-2.1-or-later", true},
		{[]string{"GPL-3.0"}, "GPL-3.0-only", true},
		{[]string{"Apache-2.0", "MIT"}, "MIT OR Apache-2.0", true},
		{nil, "MIT", false},
	} {
		if got := licenseMatchesRegistry(c.detected, c.registry); got != c.want {
			t.Errorf("licenseMatchesRegistry(%v, %q) = %v, want %v", c.detected, c.registry, got, c.want)
		}
	}
}

// In a module under a go.work, target listing sees the planned go.mod through -overlay, and in a module with a vendor directory it lists with -mod=readonly without go mod vendor (#2463).
func TestAddInspectsWorkspaceAndVendor(t *testing.T) {
	proxy := libraryProxy(t)
	mit := licenseFixtureText(t, "MIT")
	sha := strings.Repeat("d", 64)
	addProxyModule(t, proxy, "example.com/worknative", "v1.0.0", "module example.com/worknative\n\ngo 1.26\n", map[string]string{
		"worknative.go":    "package worknative\n",
		"LICENSE":          mit,
		nativeManifestName: fmt.Sprintf("version: 1\nlibraries:\n  linux_x64:\n    - name: libwork.so.1\n      url: https://work.example.com/libwork.so.1\n      sha256: %s\n      licenses: [LICENSE]\n", sha),
	})
	offlineAdd(t, proxy)

	t.Run("workspace", func(t *testing.T) {
		root := t.TempDir()
		writeFiles(t, root, map[string]string{
			"go.work":        "go 1.27.3\n\nuse (\n\t./app\n\t./sibling\n)\n",
			"app/go.mod":     "module example.com/app\n\ngo 1.26\n",
			"app/nsgo.yaml":  "name: WorkApp\nversion: 1.0.0\nbuild:\n  targets: [linux_x64]\n",
			"sibling/go.mod": "module example.com/sibling\n\ngo 1.26\n",
			"sibling/sib.go": "package sibling\n",
		})
		t.Setenv("GOWORK", "")
		dir := filepath.Join(root, "app")
		plan, err := planAdd(t, dir, "example.com/worknative")
		if err != nil {
			t.Fatalf("workspace plan: %v\n%s", err, plan)
		}
		for _, want := range []string{
			"  licenses   example.com/worknative: MIT\n",
			"  targets    linux_x64: libwork.so.1 (MIT, from work.example.com)\n",
			"  workspace  go.mod is planned for this module alone",
		} {
			if !strings.Contains(plan, want) {
				t.Errorf("workspace plan lacks %q:\n%s", want, plan)
			}
		}
	})

	t.Run("vendor", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{
			"go.mod":             "module example.com/app\n\ngo 1.26\n",
			"nsgo.yaml":          "name: VendorApp\nversion: 1.0.0\nbuild:\n  targets: [linux_x64]\n",
			"vendor/modules.txt": "## explicit; go 1.26\n",
		})
		plan, err := planAdd(t, dir, "example.com/worknative")
		if err != nil {
			t.Fatalf("vendor plan: %v\n%s", err, plan)
		}
		for _, want := range []string{
			"  licenses   example.com/worknative: MIT\n",
			"  targets    linux_x64: libwork.so.1 (MIT, from work.example.com)\n",
			"  next       go mod vendor\n",
		} {
			if !strings.Contains(plan, want) {
				t.Errorf("vendor plan lacks %q:\n%s", want, plan)
			}
		}
	})
}

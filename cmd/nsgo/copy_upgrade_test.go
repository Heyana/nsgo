package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// upgradeProxy creates an offline file:// module proxy with:
//   - example.com/kit v1.0.0: exports Greet() and Legacy()
//   - example.com/kit v1.0.1: exports Greet() and Legacy() (non-breaking move)
//   - example.com/kit v1.1.0: exports Greet() only (removes Legacy(), breaking copies that call it)
//   - example.com/consumer v1.0.0: requires example.com/kit v1.1.0 (transitive upgrade)
//   - example.com/unrelated v1.0.0: independent module
func upgradeProxy(t *testing.T) string {
	t.Helper()
	proxy := t.TempDir()
	mit := licenseFixtureText(t, "MIT")
	addProxyModule(t, proxy, "example.com/kit", "v1.0.0",
		"module example.com/kit\n\ngo 1.26\n",
		map[string]string{
			"kit.go":  "package kit\n\nfunc Greet() string { return \"hi\" }\nfunc Legacy() string { return \"old\" }\n",
			"LICENSE": mit,
		})
	addProxyModule(t, proxy, "example.com/kit", "v1.0.1",
		"module example.com/kit\n\ngo 1.26\n",
		map[string]string{
			"kit.go":  "package kit\n\nfunc Greet() string { return \"hi\" }\nfunc Legacy() string { return \"old\" }\n",
			"LICENSE": mit,
		})
	addProxyModule(t, proxy, "example.com/kit", "v1.1.0",
		"module example.com/kit\n\ngo 1.26\n",
		map[string]string{
			"kit.go":  "package kit\n\nfunc Greet() string { return \"hi\" }\n",
			"LICENSE": mit,
		})
	addProxyModule(t, proxy, "example.com/consumer", "v1.0.0",
		"module example.com/consumer\n\ngo 1.26\n\nrequire example.com/kit v1.1.0\n",
		map[string]string{
			"consumer.go": "package consumer\n\nimport \"example.com/kit\"\n\nfunc Call() string { return kit.Greet() }\n",
			"LICENSE":     mit,
		})
	addProxyModule(t, proxy, "example.com/unrelated", "v1.0.0",
		"module example.com/unrelated\n\ngo 1.26\n",
		map[string]string{
			"unrelated.go": "package unrelated\n\nfunc Value() int { return 1 }\n",
			"LICENSE":      mit,
		})
	return proxy
}

// copyUpgradeApp creates an offline app module with a copied component at
// components/widget (from example.com/kit v1.0.0) that imports example.com/kit
// and calls kit.Legacy(), imported by main.
func copyUpgradeApp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.26\n\nrequire example.com/kit v1.0.0\n",
		"main.go": `package main

import (
	"fmt"

	"example.com/app/components/widget"
)

func main() {
	fmt.Println(widget.Render())
}
`,
		"components/widget/nsgo-copy.yaml": `version: 1
module: example.com/kit
module_version: v1.0.0
component: widget
tests: false
files:
  - widget.go
`,
		"components/widget/widget.go": `package widget

import "example.com/kit"

func Render() string {
	return kit.Legacy()
}
`,
	})
	goIn(t, dir, "mod", "tidy")
	return dir
}

// Raising example.com/kit to v1.1.0 (which removes kit.Legacy()) names the
// broken copied component, its first error, and diff/add commands in nsgo add's
// plan, refuses -y without writing go.mod/go.sum, and allows interactive
// confirmation (#2470).
func TestAddPlanNamesBrokenCopyAndRefusesYes(t *testing.T) {
	offlineAdd(t, upgradeProxy(t))
	cmd := nsgoCommand()

	for _, tc := range []struct {
		name string
		arg  string
	}{
		{name: "direct raise", arg: "example.com/kit@v1.1.0"},
		{name: "transitive raise", arg: "example.com/consumer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := copyUpgradeApp(t)
			oldMod, oldSum := moduleFiles(t, dir)

			plan, err := planAdd(t, dir, tc.arg)
			if err != nil {
				t.Fatalf("-dry-run failed: %v\nplan:\n%s", err, plan)
			}
			for _, want := range []string{
				"copies",
				"components/widget is widget from example.com/kit v1.0.0:",
				"undefined: kit.Legacy",
				cmd + " diff widget@v1.1.0",
				cmd + " add widget@v1.1.0",
			} {
				if !strings.Contains(plan, want) {
					t.Errorf("plan missing %q:\n%s", want, plan)
				}
			}
			if gm, gs := moduleFiles(t, dir); !bytes.Equal(gm, oldMod) || !bytes.Equal(gs, oldSum) {
				t.Fatalf("-dry-run modified go.mod or go.sum")
			}

			// -y must refuse and leave go.mod / go.sum untouched.
			yesPlan, err := runAdd(t, dir, addOptions{yes: true}, nil, tc.arg)
			if err == nil || !strings.Contains(err.Error(), "refused under -y") || !strings.Contains(err.Error(), "components/widget") {
				t.Fatalf("-y error = %v, want refused under -y naming components/widget\nplan:\n%s", err, yesPlan)
			}
			if !strings.Contains(yesPlan, "undefined: kit.Legacy") {
				t.Errorf("-y plan did not print the error:\n%s", yesPlan)
			}
			if gm, gs := moduleFiles(t, dir); !bytes.Equal(gm, oldMod) || !bytes.Equal(gs, oldSum) {
				t.Fatalf("refused -y modified go.mod or go.sum")
			}

			// Without -y and without a terminal, refuse without suggesting -y.
			if _, err := runAdd(t, dir, addOptions{}, nil, tc.arg); err == nil || strings.Contains(err.Error(), "add -y") {
				t.Fatalf("non-interactive error = %v, want refusal not suggesting -y", err)
			}

			// Interactive confirmation applies the plan after showing it.
			asked := false
			confirm := func(context.Context) (bool, error) { asked = true; return true, nil }
			if _, err := runAdd(t, dir, addOptions{}, confirm, tc.arg); err != nil {
				t.Fatalf("interactive confirmation failed: %v", err)
			}
			if !asked {
				t.Fatal("interactive confirmation did not ask")
			}
			if gm, _ := moduleFiles(t, dir); !strings.Contains(string(gm), "example.com/kit v1.1.0") {
				t.Errorf("go.mod was not updated after interactive confirmation:\n%s", gm)
			}
		})
	}
}

// A non-breaking module raise (v1.0.0 -> v1.0.1) or an unrelated add prints no
// broken copy in the plan and succeeds under -y (#2470).
func TestAddPlanAllowsNonBreakingUpgrade(t *testing.T) {
	offlineAdd(t, upgradeProxy(t))

	for _, arg := range []string{"example.com/kit@v1.0.1", "example.com/unrelated"} {
		t.Run(arg, func(t *testing.T) {
			dir := copyUpgradeApp(t)
			plan, err := runAdd(t, dir, addOptions{yes: true}, nil, arg)
			if err != nil {
				t.Fatalf("add -y %s failed: %v\nplan:\n%s", arg, err, plan)
			}
			if strings.Contains(plan, "copies") || strings.Contains(plan, "components/widget") {
				t.Errorf("plan unexpectedly reported healthy copy:\n%s", plan)
			}
		})
	}
}

// After `go get example.com/kit@v1.1.0`, nsgo check warns of the copy and fails
// on the vet error naming the copy and both commands, and nsgo build fails the
// same way with the exact same line (#2470).
func TestCheckAndBuildWarnAndFailOnBrokenCopy(t *testing.T) {
	offlineAdd(t, upgradeProxy(t))
	dir := copyUpgradeApp(t)
	goIn(t, dir, "get", "example.com/kit@v1.1.0")

	cmd := nsgoCommand()
	wantLine := "components/widget is widget from example.com/kit v1.0.0; this module requires v1.1.0: " +
		cmd + " diff widget@v1.1.0, " + cmd + " add widget@v1.1.0"

	p, err := loadOrDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var checkErr error
	stderr := captureStderr(t, func() { checkErr = check(t.Context(), p, "", 0) })
	if !strings.Contains(stderr, "warn "+wantLine) {
		t.Errorf("nsgo check stderr missing warning %q:\n%s", wantLine, stderr)
	}
	if checkErr == nil {
		t.Fatalf("nsgo check unexpectedly passed on broken copy\nstderr:\n%s", stderr)
	}
	if !strings.Contains(checkErr.Error(), "undefined: kit.Legacy") {
		t.Errorf("nsgo check error missing vet error:\n%v", checkErr)
	}
	if !strings.Contains(checkErr.Error(), wantLine) {
		t.Errorf("nsgo check error missing copy line %q:\n%v", wantLine, checkErr)
	}

	// nsgo build (via goBuildWith, which both nsgo build and nsgo dev use)
	// fails with the exact same copy line after go build's output.
	buildErr := goBuildWith(t.Context(), p, filepath.Join(t.TempDir(), "app"), []string{"CGO_ENABLED=0"}, buildExtra{})
	if buildErr == nil {
		t.Fatal("goBuildWith unexpectedly succeeded on broken copy")
	}
	if !strings.Contains(buildErr.Error(), "undefined: kit.Legacy") {
		t.Errorf("goBuildWith error missing compiler error:\n%v", buildErr)
	}
	if !strings.Contains(buildErr.Error(), wantLine) {
		t.Errorf("goBuildWith error missing copy line %q:\n%v", wantLine, buildErr)
	}
}

// A copy whose module moved without breaking it gets the warning alone, and
// nsgo check exits 0 (#2470).
func TestCheckWarnsWithoutFailingWhenModuleMovedWithoutBreak(t *testing.T) {
	offlineAdd(t, upgradeProxy(t))
	dir := copyUpgradeApp(t)
	goIn(t, dir, "get", "example.com/kit@v1.0.1")

	cmd := nsgoCommand()
	wantLine := "components/widget is widget from example.com/kit v1.0.0; this module requires v1.0.1: " +
		cmd + " diff widget@v1.0.1, " + cmd + " add widget@v1.0.1"

	p, err := loadOrDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var checkErr error
	stderr := captureStderr(t, func() { checkErr = check(t.Context(), p, "", 0) })
	if checkErr != nil {
		t.Fatalf("nsgo check failed on non-breaking move: %v\nstderr:\n%s", checkErr, stderr)
	}
	if !strings.Contains(stderr, "warn "+wantLine) {
		t.Errorf("nsgo check stderr missing warning %q:\n%s", wantLine, stderr)
	}
}

// An up-to-date copy is not reported by nsgo check (#2470).
func TestCheckQuietWhenCopyUpToDate(t *testing.T) {
	offlineAdd(t, upgradeProxy(t))
	dir := copyUpgradeApp(t)

	p, err := loadOrDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var checkErr error
	stderr := captureStderr(t, func() { checkErr = check(t.Context(), p, "", 0) })
	if checkErr != nil {
		t.Fatalf("nsgo check failed on up-to-date copy: %v\nstderr:\n%s", checkErr, stderr)
	}
	if strings.Contains(stderr, "components/widget") || strings.Contains(stderr, "warn ") {
		t.Errorf("nsgo check reported up-to-date copy:\n%s", stderr)
	}
}

// nsgo check strictly validates nsgo-copy.yaml records: path traversal,
// disallowed extensions, unknown fields, anchors, unsupported version, size
// limit, and symlinks are all refused (#2470, #2467).
func TestCheckRejectsInvalidCopyRecordAndSymlinks(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		setup   func(t *testing.T, copyDir string)
		wantErr string
	}{
		{
			name:    "path traversal",
			yaml:    "version: 1\nmodule: example.com/kit\nmodule_version: v1.0.0\ncomponent: widget\ntests: false\nfiles:\n  - ../escape.go\n",
			wantErr: "escapes",
		},
		{
			name:    "disallowed file extension",
			yaml:    "version: 1\nmodule: example.com/kit\nmodule_version: v1.0.0\ncomponent: widget\ntests: false\nfiles:\n  - native.c\n",
			wantErr: ".c files are not allowed",
		},
		{
			name:    "unknown YAML field",
			yaml:    "version: 1\nmodule: example.com/kit\nmodule_version: v1.0.0\ncomponent: widget\ntests: false\nextra: nope\nfiles:\n  - widget.go\n",
			wantErr: "extra",
		},
		{
			name:    "unsupported version",
			yaml:    "version: 2\nmodule: example.com/kit\nmodule_version: v1.0.0\ncomponent: widget\ntests: false\nfiles:\n  - widget.go\n",
			wantErr: "reads version 1",
		},
		{
			name:    "YAML anchor",
			yaml:    "version: 1\nmodule: &m example.com/kit\nmodule_version: v1.0.0\ncomponent: widget\ntests: false\nfiles:\n  - widget.go\n",
			wantErr: "anchors are not allowed",
		},
		{
			name:    "exceeds 64 KiB",
			yaml:    "version: 1\nmodule: example.com/kit\nmodule_version: v1.0.0\ncomponent: widget\ntests: false\nfiles:\n  - widget.go\n#" + strings.Repeat("a", maxCopyRecordBytes),
			wantErr: "exceeds 64 KiB",
		},
		{
			name: "symlinked nsgo-copy.yaml",
			setup: func(t *testing.T, copyDir string) {
				if runtime.GOOS == "windows" {
					t.Skip("symlinks need privileges on Windows")
				}
				target := filepath.Join(t.TempDir(), "real.yaml")
				if err := os.WriteFile(target, []byte("version: 1\nmodule: example.com/kit\nmodule_version: v1.0.0\ncomponent: widget\ntests: false\nfiles:\n  - widget.go\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(copyDir, copyRecordName)); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "symlinks are not allowed",
		},
		{
			name: "symlinked file in files list",
			yaml: "version: 1\nmodule: example.com/kit\nmodule_version: v1.0.0\ncomponent: widget\ntests: false\nfiles:\n  - widget.go\n",
			setup: func(t *testing.T, copyDir string) {
				if runtime.GOOS == "windows" {
					t.Skip("symlinks need privileges on Windows")
				}
				target := filepath.Join(t.TempDir(), "real.go")
				if err := os.WriteFile(target, []byte("package widget\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Join(copyDir, "widget.go")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "symlinks are not allowed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{
				"go.mod":  "module example.com/app\n\ngo 1.26\n",
				"main.go": "package main\n\nfunc main() {}\n",
			})
			copyDir := filepath.Join(dir, "components", "widget")
			if err := os.MkdirAll(copyDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if tc.yaml != "" {
				if err := os.WriteFile(filepath.Join(copyDir, copyRecordName), []byte(tc.yaml), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.setup != nil {
				tc.setup(t, copyDir)
			}
			err := checkCopies(dir)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("checkCopies() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

// Duplicate component names disambiguate via relative directory, and a copy
// broken by another module (whose own source module did not move) suggests
// @upgrade (#2470).
func TestCopyFailureAnnotationDisambiguationAndExternalBreak(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":                             "module example.com/app\n\ngo 1.26\n\nrequire example.com/kit v1.1.0\n",
		"components/a/widget/nsgo-copy.yaml": "version: 1\nmodule: example.com/kit\nmodule_version: v1.0.0\ncomponent: widget\ntests: false\nfiles:\n  - widget.go\n",
		"components/b/widget/nsgo-copy.yaml": "version: 1\nmodule: example.com/other\nmodule_version: v1.0.0\ncomponent: widget\ntests: false\nfiles:\n  - widget.go\n",
	})
	cmd := nsgoCommand()

	// Both copies share the component name "widget", so diagnostics name their directories.
	out := "# example.com/app/components/a/widget [example.com/app/components/a/widget.test]\n" +
		"components/a/widget/widget.go:5:2: undefined: kit.Legacy\n" +
		"# example.com/app/components/b/widget/locales\n" +
		"components/b/widget/locales/catalog_gen.go:3:1: undefined: i18n.Old\n"
	got := annotateCopyFailure(dir, out)

	wantA := "components/a/widget is widget from example.com/kit v1.0.0; this module requires v1.1.0: " +
		cmd + " diff components/a/widget@v1.1.0, " + cmd + " add components/a/widget@v1.1.0"
	wantB := "components/b/widget is widget from example.com/other v1.0.0: " +
		cmd + " diff components/b/widget@upgrade, " + cmd + " add components/b/widget@upgrade"

	if !strings.Contains(got, wantA) {
		t.Errorf("missing disambiguated mismatch line %q:\n%s", wantA, got)
	}
	if !strings.Contains(got, wantB) {
		t.Errorf("missing external-break line %q:\n%s", wantB, got)
	}
}

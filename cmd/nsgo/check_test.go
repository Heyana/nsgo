package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func checkFixture(t *testing.T) *project {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module checkfixture\n\ngo 1.26\n",
		"main.go": "package fixture\nvar Value = 1\n",
	})
	p, err := loadOrDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCheckTargetCompilesTheRepositoryTestHosts(t *testing.T) {
	p := checkFixture(t)
	writeFiles(t, p.Dir, map[string]string{
		"go.mod": "module " + nsgoModule + "\n\ngo 1.26\n",
		"internal/platform/harmonyos/testhost.go": `//go:build js && wasm && nsgoharmony && nsgotest

package harmonyos

type surface interface { ChooseDockMenu(uint32) error }
type testSurface struct{}
var _ surface = (*testSurface)(nil)
`,
	})
	err := checkTargets(context.Background(), p, "harmonyos", 0)
	if err == nil || !strings.Contains(err.Error(), "ChooseDockMenu") {
		t.Fatalf("a broken test host passed the target check: %v", err)
	}
	file := filepath.Join(p.Dir, "internal/platform/harmonyos/testhost.go")
	source, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append(source, []byte("\nfunc (*testSurface) ChooseDockMenu(uint32) error { return nil }\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkTargets(context.Background(), p, "harmonyos", 0); err != nil {
		t.Fatalf("a complete test host failed the target check: %v", err)
	}
}

// missingXcode also exercises a Mac whose selected Xcode has no mobile SDKs. The go command remains real, so the tests verify cross-target vetting.
func missingXcode(t *testing.T) {
	checkXcrun(t, "echo 'Xcode SDK unavailable' >&2\nexit 1\n")
}

func checkXcrun(t *testing.T, body string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the unavailable Xcode fixture uses a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xcrun"), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCheckAllWithMissingWatchSDK(t *testing.T) {
	if testing.Short() || runtime.GOOS != "darwin" {
		t.Skip("cross-vets with real iOS and tvOS SDKs")
	}
	for _, name := range []string{"ios", "ios_simulator", "tvos", "tvos_simulator"} {
		if _, err := vetEnv(t.Context(), name); err != nil {
			t.Skipf("needs the %s SDK: %v", name, err)
		}
	}
	checkXcrun(t, "case \"$2\" in\n  watchos|watchsimulator) exit 1;;\nesac\nexec /usr/bin/xcrun \"$@\"\n")
	p := checkFixture(t)
	var err error
	out := captureStderr(t, func() { err = check(t.Context(), p, "all", 0) })
	if err != nil {
		t.Fatalf("check with only watchOS missing: %v\n%s", err, out)
	}
	for _, name := range []string{"ios", "ios_simulator", "tvos", "tvos_simulator", "android_arm64", "web"} {
		if !strings.Contains(out, "vetted for "+name) {
			t.Errorf("did not vet available %s:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "skipped watchos, watchos_simulator: the watchOS SDK needs Xcode") || strings.Count(out, "skipped ") != 1 {
		t.Errorf("missing the unavailable SDK summary:\n%s", out)
	}
}

func TestCheckAllWithoutXcode(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-vets a real package")
	}
	missingXcode(t)
	p := checkFixture(t)
	var err error
	out := captureStderr(t, func() { err = check(t.Context(), p, "all", 0) })
	if err != nil {
		t.Fatalf("check all without Xcode: %v\n%s", err, out)
	}
	for _, name := range []string{"macos_arm64", "macos_x64", "windows_arm64", "windows_x64", "linux_arm64", "linux_x64", "android_arm64", "web"} {
		if !strings.Contains(out, "vetted for "+name) {
			t.Errorf("did not vet %s:\n%s", name, out)
		}
	}
	reason := "they need macOS with Xcode"
	if runtime.GOOS == "darwin" {
		reason = "the iOS SDK needs Xcode: xcrun --sdk iphoneos --show-sdk-path"
	}
	if !strings.Contains(out, "skipped ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator: "+reason) || strings.Count(out, "skipped ") != 1 {
		t.Errorf("missing one summary of unavailable Apple targets and why:\n%s", out)
	}
}

func TestCheckExplicitAppleWithoutXcode(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-vets a real package")
	}
	missingXcode(t)
	p := checkFixture(t)
	for _, target := range []string{"ios", "ios_simulator", "tvos", "tvos_simulator", "watchos", "watchos_simulator", "ios,tvos", "all,ios", "ios,all"} {
		t.Run(target, func(t *testing.T) {
			var err error
			out := captureStderr(t, func() { err = check(t.Context(), p, target, 0) })
			if err == nil || !strings.Contains(err.Error(), "Xcode") {
				t.Fatalf("explicit Apple target: %v\n%s", err, out)
			}
			if strings.Contains(target, "all") && !strings.Contains(out, "vetted for web") {
				t.Errorf("an explicit unavailable target prevented later checks:\n%s", out)
			}
			if target == "ios,tvos" && (!strings.Contains(err.Error(), "iOS") || !strings.Contains(err.Error(), "tvOS")) {
				t.Errorf("lost an explicit target's error: %v", err)
			}
		})
	}
}

func TestCheckTargetsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	// Cancellation must fail before any SDK lookup, project access or skip.
	if err := checkTargets(ctx, nil, "all", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled check: %v", err)
	}
}

func TestCheckTargetFailuresContinue(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-vets a real package")
	}
	p := checkFixture(t)
	writeFiles(t, p.Dir, map[string]string{
		"bad_windows.go": "package fixture\nvar _ = missingWindowsValue\n",
	})
	// One at a time, the failure comes first; two at once, the targets run beside each other.
	for _, jobs := range []int{0, 2} {
		var err error
		out := captureStderr(t, func() { err = check(t.Context(), p, "windows_arm64,web", jobs) })
		if err == nil || !strings.Contains(err.Error(), "missingWindowsValue") || !strings.Contains(err.Error(), "windows_arm64") {
			t.Fatalf("-jobs %d: target vet error was lost: %v\n%s", jobs, err, out)
		}
		if !strings.Contains(out, "vetted for web") {
			t.Errorf("-jobs %d: a failed vet prevented the following target:\n%s", jobs, out)
		}
	}
}

func TestCheckTargetEmptyPackageSets(t *testing.T) {
	// Use the runner for vet; the JSON policy needs no package scan when
	// the user already picked an experiment.
	t.Setenv("GOEXPERIMENT", "nojsonv2")
	const noPackages = "go: warning: \"./...\" matched no packages\nno packages to vet\n"
	tests := []struct {
		name, targets        string
		linux, web           string
		wantError, neverSays string
		wantLines            []string
		noXcode              bool
	}{
		{
			name: "mixed", targets: "linux,web", web: noPackages,
			wantLines: []string{"vetted for linux_arm64", "vetted for linux_x64", "skipped web: no packages match ./... under the build constraints"},
			neverSays: "vetted for web",
		},
		{
			name: "all empty", targets: "web,linux", linux: noPackages, web: noPackages,
			wantError: "nothing was vetted",
			wantLines: []string{"skipped web, linux_arm64, linux_x64: no packages match ./... under the build constraints"},
			neverSays: "vetted for ",
		},
		{
			name: "additional diagnostic", targets: "web,linux_arm64",
			web:       noPackages + "broken.go:4:9: undefined: missing\n",
			wantError: "web: go vet failed\n" + noPackages + "broken.go:4:9: undefined: missing",
			wantLines: []string{"vetted for linux_arm64"}, neverSays: "skipped web",
		},
		{
			name: "missing SDKs and empty web", targets: "web,all", web: noPackages, noXcode: true,
			wantLines: []string{
				"skipped web: no packages match ./... under the build constraints",
				"skipped ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator:",
				"Xcode",
			},
			neverSays: "vetted for web",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := checkFixture(t)
			ctx := withProcessRunner(t, func(_ context.Context, cmd processCommand) ([]byte, error) {
				switch {
				case cmd.name == "xcrun" && tc.noXcode:
					return nil, errors.New("Xcode SDK unavailable")
				case cmd.name == "go" && len(cmd.args) == 2 && cmd.args[0] == "env" && cmd.args[1] == "GOFLAGS":
					return nil, nil
				case cmd.name == "go" && len(cmd.args) >= 2 && cmd.args[0] == "vet" && cmd.args[len(cmd.args)-1] == "./...":
					var diagnostic string
					switch envTarget(cmd.env) {
					case "linux_arm64", "linux_x64":
						diagnostic = tc.linux
					case "js_wasm":
						diagnostic = tc.web
					}
					if diagnostic != "" {
						return []byte(diagnostic), errors.New("exit status 1")
					}
					return nil, nil
				default:
					return nil, fmt.Errorf("unexpected command %s %q", cmd.name, cmd.args)
				}
			})
			var err error
			out := captureStderr(t, func() { err = checkTargets(ctx, p, tc.targets, 0) })
			if tc.wantError == "" && err != nil || tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("check returned %v, wanted error %q:\n%s", err, tc.wantError, out)
			}
			rest := out
			for _, want := range tc.wantLines {
				_, after, ok := strings.Cut(rest, want)
				if !ok {
					t.Fatalf("missing or out of order %q:\n%s", want, out)
				}
				rest = after
			}
			if strings.Contains(out, tc.neverSays) {
				t.Errorf("unexpected %q:\n%s", tc.neverSays, out)
			}
		})
	}
}

func TestCheckTargetSkipsRealBuildConstraints(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-vets a real package")
	}
	p := checkFixture(t)
	writeFiles(t, p.Dir, map[string]string{
		"main.go": "//go:build !js && !wasip1\n\npackage fixture\n\nvar Value = 1\n",
	})
	var err error
	out := captureStderr(t, func() { err = check(t.Context(), p, "linux,web", 0) })
	if err != nil || !strings.Contains(out, "vetted for linux_arm64") || !strings.Contains(out, "skipped web: no packages match ./...") {
		t.Fatalf("check did not vet the Linux-only package and skip web: %v\n%s", err, out)
	}
}

// A bare OS name vets that OS, as nsgo test -target spells it (#589).
func TestCheckBareOSTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-vets a real package")
	}
	p := checkFixture(t)
	for _, target := range []string{"windows", "linux"} {
		var err error
		out := captureStderr(t, func() { err = check(t.Context(), p, target, 0) })
		if err != nil {
			t.Fatalf("check %q: %v\n%s", target, err, out)
		}
		for _, name := range []string{target + "_arm64", target + "_x64"} {
			if !strings.Contains(out, "vetted for "+name) {
				t.Errorf("check %q did not vet %s:\n%s", target, name, out)
			}
		}
	}
}

// An explicit target vets that target alone: the host's vet, here failing, is the plain check's (#652).
func TestCheckTargetSkipsTheHostVet(t *testing.T) {
	if testing.Short() {
		t.Skip("vets a real package")
	}
	p := checkFixture(t)
	writeFiles(t, p.Dir, map[string]string{
		"bad_host.go": "//go:build !js\n\npackage fixture\n\nvar _ = missingHostValue\n",
	})
	var err error
	out := captureStderr(t, func() { err = check(t.Context(), p, "", 0) })
	if err == nil || !strings.Contains(err.Error(), "missingHostValue") {
		t.Fatalf("the plain check passed a broken host build: %v\n%s", err, out)
	}
	out = captureStderr(t, func() { err = check(t.Context(), p, "web", 0) })
	if err != nil {
		t.Fatalf("check -target web vetted the host: %v\n%s", err, out)
	}
	if !strings.Contains(out, "vetted for web") || strings.Contains(out, "✓ vetted\n") {
		t.Errorf("check -target web did not vet web alone:\n%s", out)
	}
}

// fakeGo puts a go command first on PATH that answers go env with nothing and records each go vet's arguments in the returned log, and how many vets ran at its start in the returned counts, each vet taking a while.
func fakeGo(t *testing.T) (log, counts string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake go command is a shell script")
	}
	dir := t.TempDir()
	log, counts = filepath.Join(dir, "vet.log"), filepath.Join(dir, "counts.log")
	running := filepath.Join(dir, "running")
	script := `#!/bin/sh
if [ "$1" = list ]; then
echo example.com/checkfixture
exit 0
fi
[ "$1" = vet ] || exit 0
echo "$*" >> '` + log + `'
mkdir -p '` + running + `'
touch '` + running + `'/$$
ls '` + running + `' | wc -l >> '` + counts + `'
sleep 0.5
rm '` + running + `'/$$
`
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log, counts
}

// -jobs bounds both how many targets are vetted at once and each go vet's compile workers; without it they go one at a time with go's default (#652).
func TestCheckTargetsJobs(t *testing.T) {
	p := checkFixture(t)
	for _, tc := range []struct {
		jobs, most int
		flag       string
	}{{0, 1, ""}, {2, 2, "-p 2"}, {3, 3, "-p 3"}} {
		log, counts := fakeGo(t)
		if err := checkTargets(t.Context(), p, "linux_x64,linux_arm64,windows_x64,windows_arm64,web", tc.jobs); err != nil {
			t.Fatalf("-jobs %d: %v", tc.jobs, err)
		}
		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		vets := strings.Split(strings.TrimSpace(string(data)), "\n")
		if len(vets) != 5 {
			t.Fatalf("-jobs %d ran %d vets, not one a target:\n%s", tc.jobs, len(vets), data)
		}
		for _, vet := range vets {
			if tc.flag == "" && strings.Contains(vet, " -p ") || tc.flag != "" && !strings.Contains(vet, " "+tc.flag+" ") {
				t.Errorf("-jobs %d ran go %s", tc.jobs, vet)
			}
		}
		data, err = os.ReadFile(counts)
		if err != nil {
			t.Fatal(err)
		}
		most := 0
		for _, field := range strings.Fields(string(data)) {
			n, err := strconv.Atoi(field)
			if err != nil {
				t.Fatal(err)
			}
			most = max(most, n)
		}
		// A loaded machine may start them apart, but never more at once than -jobs.
		if most > tc.most || tc.jobs == 0 && most != 1 {
			t.Errorf("-jobs %d ran up to %d vets at once", tc.jobs, most)
		}
	}
}

func TestCheckRejectsNegativeJobs(t *testing.T) {
	if err := check(t.Context(), checkFixture(t), "web", -1); err == nil || !strings.Contains(err.Error(), "-jobs") {
		t.Fatalf("check -jobs -1: %v", err)
	}
}

// A second check of a module waits for the first, saying which process it waits for, and goes on once it ends (#652).
func TestCheckLockWaitsForTheModulesOtherCheck(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("checks take no lock here")
	}
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": "module lockfixture\n\ngo 1.26\n", "app/main.go": "package main\n"})
	unlock, err := lockCheck(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	// Cancelling a waiting check ends its wait.
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	if _, err := lockCheck(ctx, filepath.Join(root, "app")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("a check of the same module's app did not wait: %v", err)
	}
	released := make(chan time.Time, 1)
	time.AfterFunc(500*time.Millisecond, func() {
		released <- time.Now()
		unlock()
	})
	var second func()
	out := captureStderr(t, func() { second, err = lockCheck(t.Context(), filepath.Join(root, "app")) })
	returned := time.Now()
	if err != nil {
		t.Fatal(err)
	}
	defer second()
	if at := <-released; returned.Before(at) {
		t.Error("the second check went on before the first ended")
	}
	if want := fmt.Sprintf("(pid %d)", os.Getpid()); !strings.Contains(out, want) || strings.Count(out, "waiting") != 1 {
		t.Errorf("want one note naming %s:\n%s", want, out)
	}
}

// A lock file left by a check that ended without unlocking, as a killed one does, holds no later check.
func TestCheckLockOutlivesItsHolder(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": "module lockfixture\n\ngo 1.26\n", ".nsgo/check.lock": "99999999\n"})
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	unlock, err := lockCheck(ctx, root)
	if err != nil {
		t.Fatalf("a stale lock file held the check: %v", err)
	}
	unlock()
}

// A target no package builds for is skipped with a reason rather than failed, and a run that vet nothing still fails (#2267).
func TestCheckTargetSkipsATargetWithNoPackages(t *testing.T) {
	p := checkFixture(t)
	writeFiles(t, p.Dir, map[string]string{
		"go.mod":  "module checkfixture\n\ngo 1.26\n",
		"main.go": "//go:build !js && !wasip1\n\npackage fixture\n\nvar Value = 1\n",
	})
	var err error
	out := captureStderr(t, func() { err = checkTargets(context.Background(), p, "linux,web", 0) })
	if err != nil {
		t.Fatalf("checkTargets(linux,web) = %v, want the web target skipped", err)
	}
	if !strings.Contains(out, "skipped web: no packages build for it") {
		t.Fatalf("output = %q, want web reported as skipped", out)
	}
	out = captureStderr(t, func() { err = checkTargets(context.Background(), p, "web", 0) })
	if err == nil || !strings.Contains(err.Error(), "nothing was vetted: no packages build for web") {
		t.Fatalf("checkTargets(web) = %v, want a failure that nothing was vetted", err)
	}
	if !strings.Contains(out, "skipped web: no packages build for it") {
		t.Fatalf("output = %q, want web reported as skipped", out)
	}
}

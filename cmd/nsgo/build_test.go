package main

import (
	"context"
	"debug/pe"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestExpandAllTargets(t *testing.T) {
	got, err := expandTargets([]string{"all"})
	if err != nil {
		t.Fatal(err)
	}
	want := goAndXcodeTargets()
	if !slices.Equal(got, want) {
		t.Fatalf("expandTargets(all) = %v, want %v", got, want)
	}
	if slices.Contains(got, "android_x64") {
		t.Error("all includes android_x64, which needs an NDK")
	}
	for _, mp := range miniProgramTargets() {
		if slices.Contains(got, mp) {
			t.Errorf("all includes %s, which only some apps fit", mp)
		}
	}
}

func TestRejectDesktopCgo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/cgo\n\ngo 1.27.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\n/* int answer(void) { return 42; } */\nimport \"C\"\n\nfunc main() { _ = C.answer() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := rejectDesktopCgo(t.Context(), &project{Dir: dir}, []string{"GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0"}, nil, nil, "")
	if err == nil || !strings.Contains(err.Error(), "desktop builds disable cgo") || !strings.Contains(err.Error(), "main.go") {
		t.Fatalf("rejectDesktopCgo = %v, want a cgo policy error naming main.go", err)
	}
}

func TestAndroidTargetNotice(t *testing.T) {
	for _, c := range []struct {
		name    string
		target  int
		targets []string
		want    string
	}{
		{"default", 0, []string{"android_arm64"}, ""},
		{"current", androidPlayTargetSDK, []string{"android_arm64"}, ""},
		{"old", 35, []string{"android_arm64"}, "android.target_sdk 35 is below Google Play's required targetSdkVersion 36; set android.target_sdk to 36 or higher, or omit it to use NSGo's default"},
		{"universal", 35, []string{"android_universal"}, "android.target_sdk 35 is below Google Play's required targetSdkVersion 36; set android.target_sdk to 36 or higher, or omit it to use NSGo's default"},
		{"other target", 35, []string{"macos_arm64"}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := project{}
			p.Android.TargetSDK = c.target
			if got := androidTargetNotice(&p, c.targets); got != c.want {
				t.Fatalf("androidTargetNotice = %q, want %q", got, c.want)
			}
		})
	}
}

func TestBuildStoreRejectsOutdatedAndroidTarget(t *testing.T) {
	p := project{}
	p.Updates.Channel = channelStore
	p.Build.Targets = []string{"android"}
	p.Android.TargetSDK = 35
	if err := buildAll(context.Background(), &p); err == nil ||
		!strings.Contains(err.Error(), "Google Play's required targetSdkVersion 36") ||
		!strings.Contains(err.Error(), "set android.target_sdk to 36 or higher") {
		t.Fatalf("buildAll = %v, want the Play requirement and how to fix it", err)
	}
}

func TestBuildStoreRejectsOutdatedUniversalAndroidTarget(t *testing.T) {
	p := project{}
	p.Updates.Channel = channelStore
	p.Build.Targets = []string{"android_arm64", "android_x64"}
	p.Android.TargetSDK = 35
	if err := buildAll(context.Background(), &p); err == nil ||
		!strings.Contains(err.Error(), "Google Play's required targetSdkVersion 36") {
		t.Fatalf("buildAll(universal android) = %v, want the Play requirement", err)
	}
}

func TestExpandTargetsDropsDuplicates(t *testing.T) {
	got, err := expandTargets([]string{"all", "web", "android_x64"})
	if err != nil {
		t.Fatal(err)
	}
	if want := len(goAndXcodeTargets()) + 1; len(got) != want {
		t.Fatalf("expandTargets = %v, want %d targets: all, web and android_x64", got, want)
	}
	if got[0] != "macos_arm64" || got[len(got)-1] != "android_x64" {
		t.Errorf("order: got %v, want macos_arm64 first and android_x64 last", got)
	}
	seen := make(map[string]bool, len(got))
	for _, target := range got {
		if seen[target] {
			t.Errorf("duplicate target %q", target)
		}
		seen[target] = true
	}
}

func TestExpandTargetsRejectsUnknown(t *testing.T) {
	if _, err := expandTargets([]string{"all", "plan9"}); err == nil || !strings.Contains(err.Error(), "all") {
		t.Fatalf("expandTargets = %v, want an error naming all", err)
	}
}

// A bare OS name expands to its architectures, as nsgo test -target spells
// them, so check and build take the same names as test (#589).
func TestExpandTargetsBareOS(t *testing.T) {
	for _, tc := range []struct {
		name string
		want []string
	}{
		{"windows", []string{"windows_arm64", "windows_x64"}},
		{"linux", []string{"linux_arm64", "linux_x64"}},
		{"macos", []string{"macos_arm64", "macos_x64"}},
		{"macos_universal", []string{"macos_universal"}},
		// android_x64 stays out, as in "all": it needs an NDK.
		{"android", []string{"android_arm64"}},
	} {
		got, err := expandTargets([]string{tc.name})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("expandTargets(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestExpandTargetsBareOSDropsDuplicates(t *testing.T) {
	got, err := expandTargets([]string{"windows", "windows_x64", "web"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"windows_arm64", "windows_x64", "web"}; !slices.Equal(got, want) {
		t.Fatalf("expandTargets = %v, want %v", got, want)
	}
}

func TestUniversalStoreTargets(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in, want []string
	}{
		{"both", []string{"macos_x64", "ios", "android_arm64", "windows_arm64", "macos_arm64", "android_x64"},
			[]string{"macos_universal", "ios", "android_universal", "windows_arm64"}},
		{"single", []string{"macos_x64", "android_x64", "windows_arm64", "windows_x64"},
			[]string{"macos_x64", "android_x64", "windows_arm64", "windows_x64"}},
		{"explicit", []string{"macos_arm64", "macos_universal", "linux_arm64", "android_arm64"},
			[]string{"macos_universal", "linux_arm64", "android_arm64"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := universalStoreTargets(tc.in); !slices.Equal(got, tc.want) {
				t.Fatalf("store targets %v = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestLoadExpandsAllTargets(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: Test\nbuild:\n  targets: [all]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Build.Targets, goAndXcodeTargets()) {
		t.Errorf("build.targets: got %v, want every Go and Xcode target", p.Build.Targets)
	}
}

func TestSmallJSON(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command")
	}
	const v1 = "package main\n\nimport \"encoding/json\"\n\nfunc main() { json.Marshal(1) }\n"
	const v2 = "package main\n\nimport \"encoding/json/v2\"\n\nfunc main() { json.Marshal(1) }\n"
	for _, c := range []struct {
		name, src string
		// goenv is the GOEXPERIMENT go env -w set, which smallJSON counts though the process's GOEXPERIMENT is empty.
		goenv string
		want  string
	}{
		{"v1", v1, "", "GOEXPERIMENT=nojsonv2"},
		{"v2", v2, "", ""},
		{"goenv jsonv2", v1, "jsonv2", ""},
		{"goenv other", v1, "fieldtrack", "GOEXPERIMENT=fieldtrack,nojsonv2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("GOEXPERIMENT", "")
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module app\n\ngo 1.26\n"), 0o644)
			os.WriteFile(filepath.Join(dir, "main.go"), []byte(c.src), 0o644)
			// Go 1.26 builds encoding/json/v2 only under the jsonv2 experiment, which an app importing it sets, here with go env -w; from Go 1.27 it builds by default and smallJSON finds the import.
			goenv := c.goenv
			if c.src == v2 && !jsonv2On(t, dir) {
				goenv = "jsonv2"
			}
			setGoEnv(t, goenv)
			env, err := smallJSON(t.Context(), dir, []string{"GOOS=js", "GOARCH=wasm"}, ".")
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if e := env[len(env)-1]; strings.HasPrefix(e, "GOEXPERIMENT=") {
				got = e
			}
			if got != c.want {
				t.Fatalf("env = %v, want GOEXPERIMENT %q", env, c.want)
			}
		})
	}
	setGoEnv(t, "")
	t.Setenv("GOEXPERIMENT", "jsonv2")
	if env, err := smallJSON(t.Context(), t.TempDir(), nil, "."); err != nil || len(env) != 0 {
		t.Fatalf("with GOEXPERIMENT set, env = %v, %v", env, err)
	}
}

func TestVerificationUsesPackagedJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("vets and tests a real Go module")
	}
	t.Setenv("GOEXPERIMENT", "")
	t.Setenv("GOFLAGS", "")
	t.Setenv("GOWORK", "off")
	setGoEnv(t, "")

	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":    "module example.com/jsonfixture\n\ngo 1.27\n",
		"nsgo.yaml": "name: JSON fixture\n",
		"main.go": `package main
import _ "example.com/jsonfixture/decode"
func main() {}
`,
		"decode/decode.go": `package decode
import "encoding/json"
func cause(err *json.UnmarshalTypeError) error { return err.Err }
`,
		"decode/decode_test.go": `package decode
import ("encoding/json"; "errors"; "testing")
func TestCause(t *testing.T) {
	want := errors.New("bad value")
	if got := cause(&json.UnmarshalTypeError{Err: want}); got != want {
		t.Fatalf("cause = %v, want %v", got, want)
	}
}
`,
		"decode/v2_test.go": `//go:build usejsonv2

package decode
import _ "encoding/json/v2"
`,
	})
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	const diagnostic = ".Err undefined"
	for _, target := range []string{"", "web"} {
		if err := check(ctx, p, target, 0); err == nil || !strings.Contains(err.Error(), diagnostic) {
			t.Errorf("check %q should reject a JSON v2-only API by default: %v", target, err)
		}
	}
	var testErr error
	out := captureStderr(t, func() { testErr = runTests(ctx, p, testOptions{}) })
	if testErr == nil || !strings.Contains(out, diagnostic) {
		t.Errorf("headless test should reject a JSON v2-only API by default: %v\n%s", testErr, out)
	}
	if err := runTests(ctx, p, testOptions{args: []string{"-tags=usejsonv2"}}); err != nil {
		t.Errorf("headless test did not honor a tagged test's JSON v2 import: %v", err)
	}

	t.Setenv("GOEXPERIMENT", "jsonv2")
	if err := check(ctx, p, "web", 0); err != nil {
		t.Errorf("check ignored explicit GOEXPERIMENT=jsonv2: %v", err)
	}
	if err := runTests(ctx, p, testOptions{}); err != nil {
		t.Errorf("headless test ignored explicit GOEXPERIMENT=jsonv2: %v", err)
	}
	if err := check(ctx, p, "miniprogram_wechat", 0); err == nil || !strings.Contains(err.Error(), diagnostic) {
		t.Errorf("check should reject JSON v2-only APIs in mini programs: %v", err)
	}
}

func TestAndroidDNSTag(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command")
	}
	const plain = "package main\n\nfunc main() {}\n"
	const withNet = "package main\n\nimport \"net\"\n\nfunc main() { _, _ = net.LookupHost(\"example.com\") }\n"
	for _, c := range []struct {
		name string
		src  string
		env  []string
		tags []string
		want bool
	}{
		{"plain android", plain, []string{"GOOS=android", "GOARCH=arm64", "CGO_ENABLED=0"}, nil, false},
		{"net android", withNet, []string{"GOOS=android", "GOARCH=arm64", "CGO_ENABLED=0"}, nil, true},
		{"net non-android", withNet, []string{"GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0"}, nil, false},
		{"nsgodev skips list", plain, []string{"GOOS=android", "GOARCH=arm64", "CGO_ENABLED=0"}, []string{"nsgodev"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module app\n\ngo 1.26\n"), 0o644)
			os.WriteFile(filepath.Join(dir, "main.go"), []byte(c.src), 0o644)
			got, err := androidDNSTag(t.Context(), &project{Dir: dir}, c.env, c.tags)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(got, "nsgo_android_dns") != c.want {
				t.Fatalf("tags = %v, want nsgo_android_dns=%v", got, c.want)
			}
		})
	}
}

// setGoEnv points the go command at a go env file of its own, which sets GOEXPERIMENT to experiment, as go env -w does, unless it is empty.
func setGoEnv(t *testing.T, experiment string) {
	file := filepath.Join(t.TempDir(), "env")
	data := ""
	if experiment != "" {
		data = "GOEXPERIMENT=" + experiment + "\n"
	}
	if err := os.WriteFile(file, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOENV", file)
}

// jsonv2On reports whether the go command that builds the module in dir has the jsonv2 experiment on with no GOEXPERIMENT set.
func jsonv2On(t *testing.T, dir string) bool {
	setGoEnv(t, "")
	cmd := exec.CommandContext(t.Context(), "go", "list", "-f", `{{join context.ToolTags " "}}`, "encoding/json")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	return slices.Contains(strings.Fields(string(out)), "goexperiment.jsonv2")
}

func TestBuildJobs(t *testing.T) {
	p := &project{}
	for _, c := range []struct{ jobs, targets, want int }{
		{3, 5, 3},
		{8, 2, 2},
		{1, 4, 1},
	} {
		p.Build.Jobs = c.jobs
		// No more targets than CPUs build at once.
		want := min(c.want, runtime.NumCPU())
		if got, err := p.buildJobs(c.targets); err != nil || got != want {
			t.Errorf("build.jobs %d for %d targets: got %d, %v; want %d", c.jobs, c.targets, got, err, want)
		}
	}
	p.Build.Jobs = 0
	if got, _ := p.buildJobs(100); got != max(1, runtime.NumCPU()/2) {
		t.Errorf("by default: got %d, want half of %d CPUs", got, runtime.NumCPU())
	}
	p.Build.Jobs = -1
	if _, err := p.buildJobs(2); err == nil {
		t.Error("build.jobs -1: got no error")
	}
	p.Build.Jobs = runtime.NumCPU() * 2
	if got, _ := p.buildJobs(100); got != runtime.NumCPU() {
		t.Errorf("build.jobs %d: got %d, want the %d CPUs", p.Build.Jobs, got, runtime.NumCPU())
	}
}

func TestCompileWorkersStayWithinTheCPUs(t *testing.T) {
	if got := compileWorkers(1); got != runtime.NumCPU() {
		t.Errorf("one target: got -p %d, want every one of %d CPUs", got, runtime.NumCPU())
	}
	for _, jobs := range []int{2, 3, max(1, runtime.NumCPU()/2), runtime.NumCPU()} {
		if got := compileWorkers(jobs); got < 1 || jobs*got > max(jobs, runtime.NumCPU()) {
			t.Errorf("%d targets at once: got -p %d, want 1 or more and at most %d in all", jobs, got, runtime.NumCPU())
		}
	}
}

func TestGoBuildCommandUsesBuildParallel(t *testing.T) {
	p := checkFixture(t)
	p.buildParallel = 3
	cmd, done, err := goBuildCommand(t.Context(), p, filepath.Join(t.TempDir(), "app"), nil, buildExtra{})
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	for i, arg := range cmd.Args {
		if arg == "-p" {
			if i+1 >= len(cmd.Args) || cmd.Args[i+1] != "3" {
				t.Fatalf("go build args %q, want -p 3", cmd.Args)
			}
			return
		}
	}
	t.Fatalf("go build args %q lack -p 3", cmd.Args)
}

// The companion is a console program, which the shell waits for and whose streams and exit code it keeps; the app stays a GUI one.
func TestBuildWindowsCommand(t *testing.T) {
	dir := t.TempDir()
	p := &project{Name: "tool"}
	if err := buildWindowsCommand(t.Context(), p, dir, "amd64", nil); err != nil {
		t.Fatal(err)
	}
	f, err := pe.Open(filepath.Join(dir, "tool.com"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	header, ok := f.OptionalHeader.(*pe.OptionalHeader64)
	if !ok {
		t.Fatalf("tool.com has no 64-bit optional header: %T", f.OptionalHeader)
	}
	if header.Subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_CUI {
		t.Fatalf("tool.com's subsystem is %d, want the console's, %d", header.Subsystem, pe.IMAGE_SUBSYSTEM_WINDOWS_CUI)
	}
}

// Without windows.command_line, and for the Microsoft Store, Windows builds write no companion.
func TestWindowsCommandLineBuilds(t *testing.T) {
	for _, c := range []struct {
		commandLine bool
		channel     string
		want        bool
	}{
		{false, "", false},
		{false, channelDirect, false},
		{true, "", true},
		{true, channelDirect, true},
		{true, channelNone, true},
		{true, channelStore, false},
	} {
		p := &project{}
		p.Windows.CommandLine = c.commandLine
		p.Updates.Channel = c.channel
		if got := p.windowsCommandLine(); got != c.want {
			t.Errorf("command_line %v on channel %q: companion %v, want %v", c.commandLine, c.channel, got, c.want)
		}
	}
}

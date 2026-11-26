package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// init makes a copy of the test binary named nsgo-demo act as an extension, before the testing package parses the arguments nsgo hands on unchanged.
func init() {
	if os.Getenv("NSGO_EXTENSION") != "1" || os.Getenv("NSGO_TEST_EXTENSION") != "1" {
		return
	}
	fmt.Printf("demo %q version=%q\n", os.Args[1:], os.Getenv("NSGO_VERSION"))
	code, _ := strconv.Atoi(os.Getenv("NSGO_TEST_EXTENSION_EXIT"))
	os.Exit(code)
}

// runNsgo runs main in a subprocess in dir with args, PATH set to path and env added, and returns its output and exit code.
func runNsgo(t *testing.T, dir, path string, env []string, args ...string) (string, int) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, append([]string{"-test.run=^TestCLIProcess$", "--"}, args...)...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); !strings.EqualFold(k, "PATH") && !slices.Contains([]string{"NSGO_EXTENSION", "NSGO_DELEGATED", "NSGO_DELEGATED_FROM", "NSGO_NO_DELEGATE"}, k) {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "PATH="+path, "NSGO_CLI_PROCESS=1", "GOWORK=off")
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatal(err)
	}
	return string(out), cmd.ProcessState.ExitCode()
}

// demoOnPath puts a copy of the test binary named nsgo-<name> in a new directory and returns the directory.
func demoOnPath(t *testing.T, name string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	dst := filepath.Join(bin, "nsgo-"+name)
	if runtime.GOOS == "windows" {
		dst += ".exe"
	}
	if os.Link(exe, dst) != nil {
		src, err := os.Open(exe)
		if err != nil {
			t.Fatal(err)
		}
		defer src.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o755)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(out, src); err != nil {
			t.Fatal(err)
		}
		if err := out.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

func TestExtensionOnPath(t *testing.T) {
	bin := demoOnPath(t, "demo")
	out, code := runNsgo(t, t.TempDir(), bin, []string{"NSGO_TEST_EXTENSION=1", "NSGO_TEST_EXTENSION_EXIT=4"}, "demo", "a", "-b", "--", "c d")
	if code != 4 || !strings.Contains(out, `demo ["a" "-b" "--" "c d"] version=`) {
		t.Fatalf("exit %d, output %s; want the demo's arguments unchanged and exit 4", code, out)
	}
}

func TestBuiltinCommandsWinOverExtensions(t *testing.T) {
	bin := demoOnPath(t, "doctor")
	out, code := runNsgo(t, t.TempDir(), bin, []string{"NSGO_TEST_EXTENSION=1"}, "doctor", "extra")
	if code != 1 || strings.Contains(out, "demo [") || !strings.Contains(out, "takes no positional arguments") {
		t.Fatalf("exit %d, output %s; want the built-in doctor's error", code, out)
	}
	out, _ = runNsgo(t, t.TempDir(), bin, nil, "help")
	if strings.Contains(out, "nsgo doctor  ") || strings.Contains(out, "extensions:") {
		t.Fatalf("the usage lists an extension named as a built-in:\n%s", out)
	}
}

func TestNoExtensionPrintsUsage(t *testing.T) {
	for _, name := range []string{"nosuch", "../demo", "help"} {
		out, code := runNsgo(t, t.TempDir(), t.TempDir(), nil, name)
		if code != 2 || !strings.Contains(out, "usage:") || strings.Contains(out, "extensions:") {
			t.Fatalf("%s: exit %d, output %s; want the usage without extensions and exit 2", name, code, out)
		}
	}
}

func TestUsageListsExtensions(t *testing.T) {
	bin := demoOnPath(t, "demo")
	out, code := runNsgo(t, t.TempDir(), bin, nil, "nosuch")
	if want := "  nsgo demo  " + filepath.Join(bin, "nsgo-demo"); code != 2 || !strings.Contains(out, "extensions:\n") || !strings.Contains(out, want) {
		t.Fatalf("exit %d, output %s; want the usage listing %q", code, out, want)
	}
}

func TestExtensionModuleTool(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH")
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod": "module example.com/demo\n\ngo 1.26\n\ntool (\n\texample.com/demo/cmd/demo\n\texample.com/demo/cmd/vet\n)\n",
		"cmd/demo/main.go": `package main

import ("fmt"; "os")

func main() {
	fmt.Printf("tool %q extension=%s\n", os.Args[1:], os.Getenv("NSGO_EXTENSION"))
	os.Exit(3)
}
`,
		"cmd/vet/main.go": "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"module vet\") }\n",
		"sub/.keep":       "",
	})
	// A program on PATH of the same name is the module tool's to hide.
	path := demoOnPath(t, "demo") + string(filepath.ListSeparator) + filepath.Dir(gobin)
	out, code := runNsgo(t, filepath.Join(dir, "sub"), path, []string{"NSGO_TEST_EXTENSION=1"}, "demo", "a", "b")
	if code != 3 || !strings.Contains(out, `tool ["a" "b"] extension=1`) {
		t.Fatalf("exit %d, output %s; want the module's tool with its arguments and exit 3", code, out)
	}
	// go tool vet alone runs Go's vet; nsgo vet runs the module's.
	if out, code := runNsgo(t, dir, path, nil, "vet"); code != 0 || !strings.Contains(out, "module vet") {
		t.Fatalf("exit %d, output %s; want the module's vet", code, out)
	}
	out, _ = runNsgo(t, dir, path, nil, "help")
	if !strings.Contains(out, "  nsgo demo  go tool example.com/demo/cmd/demo\n") {
		t.Fatalf("the usage lists no module tool:\n%s", out)
	}
}

func TestExtensionGameModuleTool(t *testing.T) {
	path, env := goEnv(t, "off")
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"game/go.mod": "module github.com/nsgo-dev/game/packages/game\n\ngo 1.26\n",
		"game/cmd/game/main.go": `package main

import ("fmt"; "os")

func main() {
	fmt.Printf("module game %q extension=%s\n", os.Args[1:], os.Getenv("NSGO_EXTENSION"))
	os.Exit(3)
}
`,
		"app/go.mod": "module example.com/app\n\ngo 1.26\n\nrequire github.com/nsgo-dev/game/packages/game v0.0.0\n\nreplace github.com/nsgo-dev/game/packages/game => ../game\n\ntool github.com/nsgo-dev/game/packages/game/cmd/game\n",
	})
	appDir := filepath.Join(root, "app")
	out, code := runNsgo(t, appDir, path, env, "game", "check", "-v", ".")
	if want := `module game ["check" "-v" "."] extension=1`; code != 3 || !strings.Contains(out, want) {
		t.Fatalf("exit %d, output %s; want %q and exit 3", code, out, want)
	}
	out, _ = runNsgo(t, appDir, path, env, "help")
	if want := "  nsgo game  go tool github.com/nsgo-dev/game/packages/game/cmd/game\n"; !strings.Contains(out, want) {
		t.Fatalf("the usage does not list game as a module tool extension:\n%s", out)
	}
	// Outside a module, game resolves as an ordinary extension: nsgo-game on PATH, else usage.
	bin := demoOnPath(t, "game")
	out, code = runNsgo(t, t.TempDir(), bin, []string{"NSGO_TEST_EXTENSION=1", "NSGO_TEST_EXTENSION_EXIT=4"}, "game", "new")
	if code != 4 || !strings.Contains(out, `demo ["new"] version=`) {
		t.Fatalf("PATH nsgo-game: exit %d, output %s; want demo output and exit 4", code, out)
	}
	out, code = runNsgo(t, t.TempDir(), t.TempDir(), nil, "game")
	if code != 2 || !strings.Contains(out, "usage:") {
		t.Fatalf("missing game extension: exit %d, output %s; want usage and exit 2", code, out)
	}
}

func TestToolCommandName(t *testing.T) {
	for path, want := range map[string]string{
		"github.com/nsgo-dev/game/packages/game/cmd/game": "game",
		"example.com/game/v2":                             "game",
		"example.com/game/v1":                             "v1",
		"example.com/game/v0":                             "v0",
		"example.com/v2x":                                 "v2x",
		"v2":                                              "v2",
		"demo":                                            "demo",
	} {
		if got := toolCommandName(path); got != want {
			t.Errorf("toolCommandName(%q) = %q, want %q", path, got, want)
		}
	}
}

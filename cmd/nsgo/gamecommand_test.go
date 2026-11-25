package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGameRunsTheModulesTool(t *testing.T) {
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
	out, code := runNsgo(t, filepath.Join(root, "app"), path, env, "game", "check", "-v", ".")
	if want := `module game ["check" "-v" "."] extension=1`; code != 3 || !strings.Contains(out, want) {
		t.Fatalf("exit %d, output %s; want %q and exit 3", code, out, want)
	}
}

func TestGameCommandOutsideAModule(t *testing.T) {
	t.Chdir(t.TempDir())
	for version, want := range map[string]string{"": "latest", "v1.2.3": "v1.2.3"} {
		t.Setenv("NSGO_GAME_VERSION", version)
		cmd, err := gameCommand(t.Context(), []string{"new", "-template", "starter", "my game"})
		if err != nil {
			t.Fatal(err)
		}
		if args := []string{"go", "run", gameTool + "@" + want, "new", "-template", "starter", "my game"}; !slices.Equal(cmd.Args, args) {
			t.Errorf("NSGO_GAME_VERSION=%q: %q, want %q", version, cmd.Args, args)
		}
		if !slices.Contains(cmd.Env, "NSGO_EXTENSION=1") || cmd.Stdin != os.Stdin || cmd.Stdout != os.Stdout || cmd.Stderr != os.Stderr {
			t.Errorf("NSGO_GAME_VERSION=%q: the command lacks nsgo's streams or NSGO_EXTENSION=1", version)
		}
	}
}

package main

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
)

// Extensions are commands nsgo does not know itself, as git, kubectl and cargo run theirs: nsgo X runs the tool X the working directory's module declares in go.mod, with go tool, so the module pins its version, else the program nsgo-X on PATH, which also works outside a module. Built-in commands always win, and a name that is not a command's name runs nothing.

// builtinCommands are the commands main's switch runs, and help, which prints the usage; an extension never takes their names.
var builtinCommands = []string{"dev", "mcp", "init", "build", "test", "check", "i18n", "add", "game", "doctor", "keygen", "certify", "publish", "verify", "revoke", "version", "help"}

// extension is a command found for a name: the module tool Tool, run with go tool from the working directory, or the program Path.
type extension struct {
	Name string
	Tool string
	Path string
}

// command is how ext runs with args.
func (ext extension) command(args []string) *exec.Cmd {
	if ext.Tool != "" {
		return goToolCommand(ext.Tool, args)
	}
	return extensionCommand(ext.Path, args...)
}

// goToolCommand runs the module's tool at the package path tool with args. The whole path is one go tool resolves only among the module's tools, never to one of Go's own of the same name, such as vet or doc.
func goToolCommand(tool string, args []string) *exec.Cmd {
	return extensionCommand("go", append([]string{"tool", tool}, args...)...)
}

// extensionCommand runs name with args as nsgo runs a command of another program's: the arguments unchanged, nsgo's own streams, and NSGO_EXTENSION=1 and NSGO_VERSION, the tool's version, which is empty for a build that records none, in its environment.
func extensionCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "NSGO_EXTENSION=1", "NSGO_VERSION="+toolVersion())
	return cmd
}

// source says where ext comes from, as the usage lists it.
func (ext extension) source() string {
	if ext.Tool != "" {
		return "go tool " + ext.Tool
	}
	return ext.Path
}

// runExtension runs the extension named name with args and returns its exit code, or false when there is none.
func runExtension(ctx context.Context, name string, args []string) (int, bool, error) {
	ext, ok, err := findExtension(ctx, name)
	if err != nil || !ok {
		return 0, false, err
	}
	code, err := runChild(ext.command(args))
	if err != nil {
		return 0, true, fmt.Errorf("%s: %w", name, err)
	}
	return code, true, nil
}

// runChild runs cmd to its end and returns its exit status, 1 when a signal ended it.
func runChild(cmd *exec.Cmd) (int, error) {
	// Catching an interrupt and SIGTERM, the signals every target nsgo vets for defines, keeps them from stopping nsgo, from before the child starts, so nsgo waits for the child's own exit; like go tool, it hands them on.
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer func() {
		signal.Stop(signals)
		close(signals)
	}()
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go func() {
		for s := range signals {
			cmd.Process.Signal(s)
		}
	}()
	err := cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		// A program a signal ended has no exit code.
		return max(exit.ExitCode(), 1), nil
	}
	return 0, err
}

// findExtension finds the extension named name: the working directory's module's tool of that command name, else nsgo-<name> on PATH.
func findExtension(ctx context.Context, name string) (extension, bool, error) {
	if !validCommandName(name) || slices.Contains(builtinCommands, name) {
		return extension{}, false, nil
	}
	tools, err := moduleTools(ctx, ".")
	if err != nil {
		return extension{}, false, err
	}
	var matches []string
	for _, tool := range tools {
		if tool == name || toolCommandName(tool) == name {
			matches = append(matches, tool)
		}
	}
	switch len(matches) {
	case 0:
	case 1:
		return extension{Name: name, Tool: matches[0]}, true, nil
	default:
		return extension{}, false, fmt.Errorf("%s names several of the module's tools, %s; run one with go tool and its path", name, strings.Join(matches, " and "))
	}
	if p, err := exec.LookPath("nsgo-" + name); err == nil {
		return extension{Name: name, Path: p}, true, nil
	}
	return extension{}, false, nil
}

// listExtensions are the extensions the usage lists, by name: the module's tools whose command names are valid, unique and not a built-in's, then the nsgo- programs on PATH that no tool's name hides.
func listExtensions(ctx context.Context) []extension {
	var found []extension
	named := map[string]bool{}
	tools, _ := moduleTools(ctx, ".")
	for _, tool := range tools {
		name := toolCommandName(tool)
		if !validCommandName(name) || slices.Contains(builtinCommands, name) {
			continue
		}
		if named[name] {
			// Ambiguous: go tool runs these only by their whole paths, and nsgo runs neither.
			found = slices.DeleteFunc(found, func(ext extension) bool { return ext.Name == name })
			continue
		}
		named[name] = true
		found = append(found, extension{Name: name, Tool: tool})
	}
	for _, ext := range pathExtensions() {
		if !named[ext.Name] {
			named[ext.Name] = true
			found = append(found, ext)
		}
	}
	slices.SortFunc(found, func(a, b extension) int { return cmp.Compare(a.Name, b.Name) })
	return found
}

// pathExtensions are the programs named nsgo-<name> on PATH that exec.LookPath finds for their names, so each name's first.
func pathExtensions() []extension {
	var found []extension
	seen := map[string]bool{}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		entries, err := os.ReadDir(cmp.Or(dir, "."))
		if err != nil {
			continue
		}
		for _, e := range entries {
			name, ok := strings.CutPrefix(e.Name(), "nsgo-")
			if !ok {
				continue
			}
			if runtime.GOOS == "windows" {
				name = strings.TrimSuffix(name, filepath.Ext(name))
			}
			if seen[name] || !validCommandName(name) || slices.Contains(builtinCommands, name) {
				continue
			}
			if p, err := exec.LookPath("nsgo-" + name); err == nil {
				seen[name] = true
				found = append(found, extension{Name: name, Path: p})
			}
		}
	}
	return found
}

// moduleTools are the package paths of the tools the go.mod of dir's module declares; a directory outside a module has none.
func moduleTools(ctx context.Context, dir string) ([]string, error) {
	mod, err := moduleFile(ctx, dir)
	if mod == nil || err != nil {
		return nil, err
	}
	tools := make([]string, len(mod.Tool))
	for i, t := range mod.Tool {
		tools[i] = t.Path
	}
	return tools, nil
}

// goModFile is what go mod edit -json prints of a go.mod that nsgo reads: the module's path, its requirements, replacements and tools.
type goModFile struct {
	Module  struct{ Path string }
	Require []struct{ Path, Version string }
	Replace []struct {
		Old, New struct{ Path, Version string }
	}
	Tool []struct{ Path string }
}

// moduleFile reads the go.mod of dir's module, the first found walking up from dir, as go mod edit reads it. It is nil outside a module, and for a go.mod that declares no tools, which says nothing nsgo reads.
func moduleFile(ctx context.Context, dir string) (*goModFile, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	for {
		gomod := filepath.Join(dir, "go.mod")
		data, err := os.ReadFile(gomod)
		if err == nil {
			if !bytes.Contains(data, []byte("tool")) {
				return nil, nil
			}
			return readGoMod(ctx, gomod)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, nil
		}
		dir = parent
	}
}

// readGoMod reads gomod with go mod edit -json.
func readGoMod(ctx context.Context, gomod string) (*goModFile, error) {
	out, err := execute(ctx, processCommand{name: "go", args: []string{"mod", "edit", "-json", gomod}})
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			err = fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(exit.Stderr)))
		}
		return nil, fmt.Errorf("reading %s: %w", rel(gomod), err)
	}
	var mod goModFile
	if err := json.Unmarshal(out, &mod); err != nil {
		return nil, fmt.Errorf("reading %s: %w", rel(gomod), err)
	}
	return &mod, nil
}

// toolCommandName is the command name go tool knows the tool at path by, as go install names its executable: the path's last element, or the one before a major version's, such as game for example.com/game/v2.
func toolCommandName(p string) string {
	dir, elem := path.Split(p)
	if dir != "" && isMajorVersion(elem) {
		_, elem = path.Split(path.Dir(p))
	}
	return elem
}

// isMajorVersion reports whether elem is a module path's major version suffix from v2 on, as v2 or v17.
func isMajorVersion(elem string) bool {
	if len(elem) < 2 || elem[0] != 'v' || elem[1] == '0' || elem == "v1" {
		return false
	}
	for _, r := range elem[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

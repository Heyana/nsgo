package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Commands are command-line programs an app ships beside its desktop
// executable, such as a CLI working on the app's data: nsgo.yaml's
// commands, each a Go main package built for the target as the app is. A
// macOS app holds them in Contents/MacOS, signed before the bundle as its
// libraries are; Windows and Linux packages hold them beside the
// executable, where the installers take every file. The Linux packages
// link each into /usr/bin, Homebrew's cask links each with a binary
// stanza, and the Windows installer puts its folder on the user's Path
// with windows.path. Store packages leave them out: the Mac App Store's
// sandbox, the MSIX and the Flatpak each run only the app's own
// executable.

// command is one of nsgo.yaml's commands: Name is its file's name, with
// .exe on Windows, and Package the Go main package built for it with the
// app's GOOS and GOARCH and CGO_ENABLED=0. LDFlags are the package's own
// linker flags, each as go build's -ldflags takes it, after the app's
// build.ldflags (commandLdflags).
type command struct {
	Name    string   `yaml:"name"`
	Package string   `yaml:"package"`
	LDFlags []string `yaml:"ldflags"`
}

// validateCommands checks commands' names, which become file names in
// every desktop package and links in /usr/bin, and their packages.
func (p *project) validateCommands() error {
	for i, c := range p.Commands {
		if !validCommandName(c.Name) {
			return fmt.Errorf("commands entry %d: name %q must be letters, digits, '-' and '_', starting with a letter or a digit", i+1, c.Name)
		}
		base := strings.ToUpper(c.Name)
		if slices.Contains([]string{"CON", "PRN", "AUX", "NUL"}, base) || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return fmt.Errorf("commands: %s is a reserved Windows file name", c.Name)
		}
		// The app's executable, its Linux command, the Windows uninstaller and the AppImage's entry point are files of the same names.
		for _, taken := range []string{p.Name, p.packageSlug(), "Uninstall", "AppRun"} {
			if strings.EqualFold(c.Name, taken) {
				return fmt.Errorf("commands: %s is the name of the app's own %s; name the command another way", c.Name, takenFile(p, taken))
			}
		}
		for _, other := range p.Commands[:i] {
			// macOS and Windows file systems ignore case.
			if strings.EqualFold(c.Name, other.Name) {
				return fmt.Errorf("commands: %s and %s name the same file", other.Name, c.Name)
			}
		}
		if c.Package == "" || strings.HasPrefix(c.Package, "-") || strings.Contains(c.Package, "...") || strings.ContainsAny(c.Package, " \t\r\n") {
			return fmt.Errorf("commands: %s's package %q is not one Go package's path", c.Name, c.Package)
		}
	}
	return nil
}

// validCommandName reports whether name is letters, digits, '-' and '_' and starts with a letter or a digit, so it is a file name on every system, without an extension any package treats as its own.
func validCommandName(name string) bool {
	for i, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || i > 0 && (r == '-' || r == '_')) {
			return false
		}
	}
	return name != ""
}

// takenFile says what of the app's is named taken.
func takenFile(p *project, taken string) string {
	switch taken {
	case p.Name:
		return "executable"
	case p.packageSlug():
		return "command in /usr/bin"
	case "Uninstall":
		return "Windows uninstaller"
	}
	return "AppImage entry point"
}

// commandsIncluded reports whether the target's packages hold the app's commands: every build but a store's.
func (p *project) commandsIncluded() bool {
	return len(p.Commands) > 0 && p.channel() != channelStore
}

// commandFile is the file name of the command name on target's system, which target may name alone, as windows.
func commandFile(name, target string) string {
	if system, _, _ := strings.Cut(target, "_"); system == "windows" {
		return name + ".exe"
	}
	return name
}

// commandFiles are the paths in dir of the commands a package for target holds.
func (p *project) commandFiles(dir, target string) []string {
	if !p.commandsIncluded() {
		return nil
	}
	var paths []string
	for _, c := range p.Commands {
		path := filepath.Join(dir, commandFile(c.Name, target))
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			paths = append(paths, path)
		}
	}
	return paths
}

// addCommands builds the app's commands for target, a desktop architecture's, into dir, where nothing of the package may have their names.
func (p *project) addCommands(ctx context.Context, target, dir string) error {
	if len(p.Commands) == 0 {
		return nil
	}
	if !p.commandsIncluded() {
		detailIn(ctx, "the store's package leaves out the app's commands, which only the app's own executable runs in")
		return nil
	}
	stage(ctx, "adding the commands")
	for _, c := range p.Commands {
		if err := ctx.Err(); err != nil {
			return err
		}
		dst := filepath.Join(dir, commandFile(c.Name, target))
		if _, err := os.Lstat(dst); err == nil {
			return fmt.Errorf("commands: %s collides with %s, which the package holds already", c.Name, rel(dst))
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := p.buildCommand(ctx, c, target, dst); err != nil {
			return err
		}
		// go build -o writes a package archive for a package that is not main.
		if data, err := os.ReadFile(dst); err != nil || bytes.HasPrefix(data, []byte("!<arch>\n")) {
			return fmt.Errorf("commands: %s's package %s built no executable; is it a main package?", c.Name, c.Package)
		}
	}
	return nil
}

// buildCommand builds c's Go package for target into out, as the app is built: with CGO_ENABLED=0 and the app's build tags, stripped and without file paths for a release.
func (p *project) buildCommand(ctx context.Context, c command, target, out string) error {
	system, arch, _ := strings.Cut(target, "_")
	goos := system
	if goos == "macos" {
		goos = "darwin"
	}
	env := []string{"GOOS=" + goos, "GOARCH=" + goArch(arch), "CGO_ENABLED=0"}
	tags, err := goTags(ctx, p, env)
	if err != nil {
		return err
	}
	args := append([]string{"build"}, tagsArgs(tags)...)
	var ldflags []string
	if p.Release {
		args = append(args, "-trimpath", "-buildvcs=false")
		ldflags = append(ldflags, "-s", "-w")
	}
	if system == "macos" {
		minimum, err := p.appleMinimum(nil)
		if err != nil {
			return err
		}
		ldflags = append(ldflags, "-macos="+minimum)
	}
	args = append(args, "-ldflags", p.commandLdflags(c, ldflags...))
	if err := goTool(ctx, p.Dir, env, append(args, "-o", out, "--", c.Package)...); err != nil {
		return fmt.Errorf("commands: %s's package %s: %w", c.Name, c.Package, err)
	}
	return nil
}

// commandLdflags is the -ldflags value c's package links with: the app's own linker flags, build.ldflags and -ldflags, then c's, so its -X values win over the app's, then the identity the app takes in core.App, which a command reads as the app does and neither replaces, and extra, the tool's.
func (p *project) commandLdflags(c command, extra ...string) string {
	flags := append(append([]string{}, p.Build.LDFlags...), c.LDFlags...)
	flags = append(flags, p.identityLdflags())
	if p.Release {
		flags = append(flags, "-X "+corePath+".appRelease=1")
	}
	return strings.Join(append(flags, extra...), " ")
}

// signMacCommands signs the commands in a macOS app's Contents/MacOS, which the app's signature seals, with codesign's arguments for each from args, and <id>.<name> as each one's identifier in place of the one the linker gave it, a hash of its contents. They take no entitlements: those are the app's, which a command run from a terminal is not.
func (p *project) signMacCommands(ctx context.Context, app string, args func(path string) []string) error {
	for _, path := range p.commandFiles(filepath.Join(app, "Contents", "MacOS"), "macos") {
		a := args(path)
		a = append(a[:len(a)-1:len(a)-1], "--identifier", p.ID+"."+filepath.Base(path), path)
		if err := run(ctx, "codesign", a...); err != nil {
			return err
		}
	}
	return nil
}

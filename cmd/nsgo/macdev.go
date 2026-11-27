package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// macDevBundle tracks the last full macOS .app bundle built during a dev
// session so Go-only edits can replace Contents/MacOS/<name> and re-sign in
// place instead of wiping and rebuilding the whole bundle.
type macDevBundle struct {
	dir, name, id, arch string
	bundleInputs        string
	nativeInputs        string
}

// build updates the existing .app bundle in place when only Go code changed,
// falling back to a full buildMacApp rebuild when bundle inputs, native
// libraries, commands or bundle files changed.
func (b *macDevBundle) build(ctx context.Context, p *project, dir, arch string) (inPlace bool, err error) {
	bundleInputs := macBundleInputs(p)
	app := filepath.Join(dir, p.Name+".app")
	if len(p.Commands) == 0 && p.QuickLook == nil &&
		b.dir == dir && b.name == p.Name && b.id == p.ID && b.arch == arch &&
		b.bundleInputs == bundleInputs &&
		macAppBundleReady(app, p.Name) {
		nativeInputs, nativeErr := macNativeInputs(ctx, p, arch)
		if nativeErr != nil || nativeInputs == b.nativeInputs {
			return true, updateMacAppBinary(ctx, p, app, arch)
		}
	}
	os.RemoveAll(dir)
	builtApp, err := buildMacApp(ctx, p, dir, arch)
	if err != nil {
		*b = macDevBundle{}
		return false, err
	}
	if len(p.Widgets) > 0 {
		// The app starts from its binary, which registers no extension
		// with the system: register its widgets.
		if err := run(ctx, "pluginkit", "-a", macWidgetsAppex(builtApp)); err != nil {
			*b = macDevBundle{}
			return false, err
		}
	}
	if p.QuickLook != nil {
		if err := run(ctx, "pluginkit", "-a", macQuickLookAppex(builtApp)); err != nil {
			*b = macDevBundle{}
			return false, err
		}
	}
	nativeInputs, _ := macNativeInputs(ctx, p, arch)
	*b = macDevBundle{
		dir:          dir,
		name:         p.Name,
		id:           p.ID,
		arch:         arch,
		bundleInputs: bundleInputs,
		nativeInputs: nativeInputs,
	}
	return false, nil
}

// macAppBundleReady reports whether app already holds an Info.plist and main
// executable suitable for an in-place binary swap.
func macAppBundleReady(app, name string) bool {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." {
		return false
	}
	for _, rel := range []string{
		filepath.Join("Contents", "Info.plist"),
		filepath.Join("Contents", "MacOS", name),
	} {
		fi, err := os.Lstat(filepath.Join(app, rel))
		if err != nil || !fi.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// updateMacAppBinary compiles the Go executable to a temporary file inside
// Contents/MacOS, atomically renames it onto Contents/MacOS/<name> (giving it
// a new inode so a still-running previous process keeps executing safely), and
// re-signs the bundle ad hoc.
func updateMacAppBinary(ctx context.Context, p *project, app, arch string) error {
	minimum, err := p.appleMinimum(nil)
	if err != nil {
		return err
	}
	macosDir := filepath.Join(app, "Contents", "MacOS")
	bin := filepath.Join(macosDir, p.Name)
	tmp, err := os.CreateTemp(macosDir, ".nsgo-bin-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	if err := goBuildWith(ctx, p, tmpPath, []string{"GOOS=darwin", "GOARCH=" + arch, "CGO_ENABLED=0"}, buildExtra{target: "macos_" + goArchName(arch)}, "-macos="+minimum); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, bin); err != nil {
		return err
	}
	stage(ctx, "signing")
	return run(ctx, "codesign", "--force", "--sign", "-", app)
}

// macBundleInputs fingerprints the non-Go inputs that affect a macOS .app
// bundle outside Contents/MacOS/<name>: nsgo.yaml, resources, icons, file-type
// icons, localization catalogs and project metadata.
func macBundleInputs(p *project) string {
	var b strings.Builder
	fmt.Fprintf(&b, "meta %q %q %q %t %t %d\n", p.Name, p.ID, p.Version, p.Release, p.devDebug, len(p.Commands))
	if data, err := os.ReadFile(filepath.Join(p.Dir, "nsgo.yaml")); err == nil {
		fmt.Fprintf(&b, "nsgo.yaml %x\n", sha256.Sum256(data))
	}
	paths := []string{filepath.Join(p.Dir, resourcesDir)}
	for _, name := range []string{"AppIcon.icon", "AppIcon-dev.icon", "Icon.icns", "Icon-dev.icns", "Icon.png", "Icon-dev.png"} {
		paths = append(paths, filepath.Join(p.Dir, assetsDir, name))
	}
	for _, path := range []string{p.Icon.Apple, p.Icon.PNG} {
		if path != "" {
			paths = append(paths, path)
		}
	}
	for _, ft := range p.FileTypes {
		if ft.Icon != "" {
			paths = append(paths, p.resolve(ft.Icon))
		}
	}
	if p.I18n.Dir != "" {
		paths = append(paths, p.I18n.Dir)
	}
	for _, root := range paths {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if fi, err := os.Stat(path); err == nil {
				fmt.Fprintf(&b, "%q %o %d %d\n", path, fi.Mode(), fi.Size(), fi.ModTime().UnixNano())
			}
			return nil
		})
	}
	return b.String()
}

// macNativeInputs fingerprints the native libraries linked into a macOS build
// (libghostty-vt and any package's nsgo-natives.yaml) so adding or editing a
// native dependency triggers a full bundle rebuild.
func macNativeInputs(ctx context.Context, p *project, arch string) (string, error) {
	env := []string{"GOOS=darwin", "GOARCH=" + arch, "CGO_ENABLED=0"}
	tags, err := goTags(ctx, p, env)
	if err != nil {
		return "", err
	}
	args := append([]string{"list", "-deps"}, tagsArgs(tags)...)
	args = append(args, "-f", "{{.ImportPath}}\t{{.Dir}}", ".")
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list: %w\n%s", err, stderr.Bytes())
	}
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		imp, dir, ok := strings.Cut(line, "\t")
		if !ok || dir == "" {
			continue
		}
		if imp == ghosttyPackage {
			lib := filepath.Join(dir, "lib", "darwin-"+arch, "libghostty-vt.dylib")
			if fi, err := os.Stat(lib); err == nil {
				fmt.Fprintf(&b, "ghostty %q %d %d\n", lib, fi.Size(), fi.ModTime().UnixNano())
			} else {
				fmt.Fprintf(&b, "ghostty %q missing\n", lib)
			}
		}
		manifest := filepath.Join(dir, nativeManifestName)
		if data, err := os.ReadFile(manifest); err == nil {
			fmt.Fprintf(&b, "natives %q %x\n", imp, sha256.Sum256(data))
			if m, err := readNativeManifest(dir); err == nil && m != nil {
				for _, lib := range m.Libraries["macos_"+goArchName(arch)] {
					if lib.Path != "" {
						if fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(lib.Path))); err == nil {
							fmt.Fprintf(&b, "native-file %q %d %d\n", lib.Path, fi.Size(), fi.ModTime().UnixNano())
						}
					}
				}
			}
		}
	}
	return b.String(), nil
}

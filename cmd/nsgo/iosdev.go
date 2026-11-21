package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// iosSimulatorInputs fingerprints the non-Go inputs to the app's bundle.
// Watch the default icon paths too, since adding one changes what load picks.
func iosSimulatorInputs(p *project) string {
	var b strings.Builder
	if data, err := os.ReadFile(filepath.Join(p.Dir, "nsgo.yaml")); err == nil {
		fmt.Fprintf(&b, "nsgo.yaml %x\n", sha256.Sum256(data))
	}
	paths := []string{filepath.Join(p.Dir, resourcesDir)}
	for _, name := range []string{"AppIcon.icon", "AppIcon-dev.icon", "Icon.png", "Icon-dev.png"} {
		paths = append(paths, filepath.Join(p.Dir, assetsDir, name))
	}
	for _, path := range []string{p.Icon.Apple, p.Icon.PNG} {
		if path != "" && filepath.Ext(path) != ".icns" {
			paths = append(paths, path)
		}
	}
	for _, root := range paths {
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			// Bundling reads symlinked resource files through to their targets.
			if fi, err := os.Stat(path); err == nil {
				fmt.Fprintf(&b, "%q %o %d %d\n", path, fi.Mode(), fi.Size(), fi.ModTime().UnixNano())
			}
			return nil
		})
	}
	return b.String()
}

// iosSimulatorInstall remembers the last full install within this dev session.
type iosSimulatorInstall struct {
	udid, id, inputs string
}

func (s *iosSimulatorInstall) update(ctx context.Context, p *project, udid, bundle, inputs string) error {
	if s.udid == udid && s.id == p.ID && s.inputs == inputs {
		replaced, err := replaceSimulatorExecutable(ctx, p, udid, bundle)
		if replaced {
			event("updated the simulator app's executable")
			return nil
		}
		if err != nil {
			detailIn(ctx, "installing the full app after the executable update failed: %v", err)
		}
	}
	event("installing the simulator app")
	if err := run(ctx, "xcrun", "simctl", "install", udid, bundle); err != nil {
		return err
	}
	*s = iosSimulatorInstall{udid: udid, id: p.ID, inputs: inputs}
	return nil
}

// replaceSimulatorExecutable uses the executable that buildIOSApp already
// signed. Its signature seals Info.plist and CodeResources, which in turn
// seals assets and nested code. When either differs, the bundle needs an
// install. A new inode lets the simulator launch the new signed Mach-O.
func replaceSimulatorExecutable(ctx context.Context, p *project, udid, bundle string) (bool, error) {
	installed, err := output(ctx, "xcrun", "simctl", "get_app_container", udid, p.ID, "app")
	if err != nil || !filepath.IsAbs(installed) || filepath.Ext(installed) != ".app" {
		return false, nil
	}
	for _, name := range []string{"Info.plist", "_CodeSignature/CodeResources"} {
		candidate, err := os.ReadFile(filepath.Join(bundle, name))
		if err != nil {
			return false, err
		}
		current, err := os.ReadFile(filepath.Join(installed, name))
		if err != nil || !bytes.Equal(candidate, current) {
			return false, nil
		}
	}
	if p.Name != filepath.Base(p.Name) || p.Name == "." || p.Name == ".." {
		return false, nil
	}
	dst := filepath.Join(installed, p.Name)
	if info, err := os.Lstat(dst); err != nil || !info.Mode().IsRegular() {
		return false, nil
	}
	src, err := os.Open(filepath.Join(bundle, p.Name))
	if err != nil {
		return false, err
	}
	defer src.Close()
	tmp, err := os.CreateTemp(installed, ".nsgo-executable-")
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, src); err != nil {
		return false, err
	}
	if err := tmp.Chmod(0o755); err != nil {
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return false, err
	}
	return true, nil
}

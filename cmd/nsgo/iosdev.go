package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// errIOSSimulatorFullBuild signals that a staged iOS simulator bundle cannot
// be reused for the new Go build and must be packaged from scratch.
var errIOSSimulatorFullBuild = errors.New("full iOS simulator bundle build required")

// iosSimulatorInputs fingerprints the non-Go inputs to the app's bundle.
// Watch the default icon paths too, since adding one changes what load picks.
func iosSimulatorInputs(p *project) string {
	var b strings.Builder
	fmt.Fprintf(&b, "app %q %q %q %q\n", p.Name, p.ID, p.Version, p.BundleVersion())
	if data, err := os.ReadFile(filepath.Join(p.Dir, "nsgo.yaml")); err == nil {
		fmt.Fprintf(&b, "nsgo.yaml %x\n", sha256.Sum256(data))
	}
	paths := []string{p.resourceRoot()}
	for _, name := range []string{"AppIcon.icon", "AppIcon-dev.icon", "Icon.png", "Icon-dev.png"} {
		paths = append(paths, filepath.Join(p.Dir, assetsDir, name))
	}
	for _, path := range []string{p.Icon.Apple, p.Icon.PNG} {
		if path != "" && filepath.Ext(path) != ".icns" {
			paths = append(paths, path)
		}
	}
	if p.LaunchScreen != nil {
		for _, path := range []string{p.LaunchScreen.Image, p.LaunchScreen.Dark.Image} {
			if path != "" {
				paths = append(paths, path)
			}
		}
	}
	for _, pack := range p.Mobile.AssetPacks {
		if pack.Dir != "" {
			paths = append(paths, pack.Dir)
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
	walkCatalogInputs(&b, p.I18n.Dir)
	return b.String()
}

// walkCatalogInputs fingerprints YAML translation catalogs in dir, whose
// appName and localized strings affect packaged manifests and InfoPlist.strings.
func walkCatalogInputs(b *strings.Builder, dir string) {
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(dir, name)
		if data, err := os.ReadFile(path); err == nil {
			fmt.Fprintf(b, "catalog %q %x\n", path, sha256.Sum256(data))
		}
	}
}

// iosSimulatorFrameworkInputs fingerprints Go-import-driven bundle contents
// (GhosttyVT, package frameworks and notices) so adding or changing a native
// framework dependency triggers a full bundle rebuild.
func iosSimulatorFrameworkInputs(p *project, hasGhostty bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ghostty=%v\n", hasGhostty)
	if p.thirdPartyNotices != "" {
		fmt.Fprintf(&b, "notices=%x\n", sha256.Sum256([]byte(p.thirdPartyNotices)))
	}
	libs := slices.Clone(p.nativeLibraries["ios_simulator"])
	slices.SortFunc(libs, func(a, b nativeLibrary) int {
		if a.Name != b.Name {
			return strings.Compare(a.Name, b.Name)
		}
		return strings.Compare(a.owner, b.owner)
	})
	for _, lib := range libs {
		fmt.Fprintf(&b, "lib %q %q %q %q %q\n", lib.owner, lib.Name, lib.SHA256, lib.URL, lib.Path)
		if lib.Path != "" && lib.packageDir != "" {
			if fi, err := os.Stat(filepath.Join(lib.packageDir, lib.Path)); err == nil {
				fmt.Fprintf(&b, "stat %d %d\n", fi.Size(), fi.ModTime().UnixNano())
			}
		}
	}
	return b.String()
}

// iosSimulatorInstall remembers the last staged bundle and full install within
// this dev session.
type iosSimulatorInstall struct {
	udid, id, inputs string

	stagedBundle     string
	stagedName       string
	stagedID         string
	stagedInputs     string
	stagedFrameworks string

	// Test hooks; nil uses the real build and signing commands.
	buildFull func(ctx context.Context, p *project, dir string) (string, error)
	buildBin  func(ctx context.Context, p *project, out string) (string, error)
	sign      func(ctx context.Context, p *project, app string) error
}

// build produces the iOS simulator .app bundle in dir. On a Go-only rebuild
// whose non-Go packaging inputs and Go-linked frameworks have not changed, it
// rebuilds and re-signs only the main executable in the staged bundle instead
// of repackaging resources, icons, extensions and frameworks.
func (s *iosSimulatorInstall) build(ctx context.Context, p *project, dir, inputs string) error {
	bundle := filepath.Join(dir, p.Name+".app")
	if s.canReuseStaged(p, bundle, inputs) {
		err := s.rebuildExecutable(ctx, p, dir, bundle)
		if err == nil {
			return nil
		}
		if !errors.Is(err, errIOSSimulatorFullBuild) {
			return err
		}
	}
	s.stagedBundle = ""
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	full := s.buildFull
	if full == nil {
		full = func(ctx context.Context, p *project, dir string) (string, error) {
			return buildIOSApp(ctx, p, iOS, dir, true)
		}
	}
	built, err := full(ctx, p, dir)
	if err != nil {
		return err
	}
	if p.AppClip == nil {
		_, ghosttyErr := os.Stat(filepath.Join(built, "Frameworks", ghosttyFramework))
		s.stagedBundle = built
		s.stagedName = p.Name
		s.stagedID = p.ID
		s.stagedInputs = inputs
		s.stagedFrameworks = iosSimulatorFrameworkInputs(p, ghosttyErr == nil)
	}
	return nil
}

func (s *iosSimulatorInstall) canReuseStaged(p *project, bundle, inputs string) bool {
	if p.AppClip != nil || s.stagedBundle != bundle || s.stagedName != p.Name || s.stagedID != p.ID || s.stagedInputs != inputs {
		return false
	}
	if p.Name == "" || p.Name != filepath.Base(p.Name) || p.Name == "." || p.Name == ".." {
		return false
	}
	for _, name := range []string{"Info.plist", "_CodeSignature/CodeResources"} {
		if _, err := os.Stat(filepath.Join(bundle, name)); err != nil {
			return false
		}
	}
	return true
}

func (s *iosSimulatorInstall) rebuildExecutable(ctx context.Context, p *project, dir, bundle string) error {
	tmp, err := os.CreateTemp(dir, ".nsgo-executable-")
	if err != nil {
		return errIOSSimulatorFullBuild
	}
	tmpPath := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpPath)

	buildBin := s.buildBin
	if buildBin == nil {
		buildBin = buildIOSSimulatorBinary
	}
	frameworks, err := buildBin(ctx, p, tmpPath)
	if err != nil {
		return err
	}
	if frameworks != s.stagedFrameworks {
		return errIOSSimulatorFullBuild
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, filepath.Join(bundle, p.Name)); err != nil {
		s.stagedBundle = ""
		return err
	}
	sign := s.sign
	if sign == nil {
		sign = func(ctx context.Context, p *project, app string) error {
			return signIOSSimulatorBundle(ctx, p, iOS, app, p.iosAssetPacks())
		}
	}
	if err := sign(ctx, p, bundle); err != nil {
		s.stagedBundle = ""
		return err
	}
	return nil
}

func buildIOSSimulatorBinary(ctx context.Context, p *project, out string) (string, error) {
	if _, err := p.appleMinimum(iOS); err != nil {
		return "", err
	}
	env, err := iosProjectEnv(ctx, p, iOS, true)
	if err != nil {
		return "", err
	}
	if err := goBuildWith(ctx, p, out, env, buildExtra{target: "ios_simulator"}, "-extldflags=-Wl,-w"); err != nil {
		return "", err
	}
	ghosttyDir, err := linkedDir(ctx, p, env, ghosttyPackage)
	if err != nil {
		return "", err
	}
	return iosSimulatorFrameworkInputs(p, ghosttyDir != ""), nil
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
	s.udid, s.id, s.inputs = udid, p.ID, inputs
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

package main

import (
	"bytes"
	"context"
	"debug/macho"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// Native libraries the app's packages ship: base/terminal's
// libghostty-vt, which its library package holds by GOOS-GOARCH and the
// tool puts into the apps that link it, signed with them.

// ghosttyPackage is the package holding libghostty-vt's builds.
const ghosttyPackage = "github.com/nsgo-dev/nsgo/base/terminal/internal/library"

// ghosttyFramework is libghostty-vt as an iOS app's framework, where the
// terminal looks for it.
const ghosttyFramework = "GhosttyVT.framework"

// linkedDir returns the directory of package pkg when the app's build for env, with the tool's tags extra, links it, else "".
func linkedDir(ctx context.Context, p *project, env []string, pkg string, extra ...string) (string, error) {
	args := []string{"list", "-deps", "-f", fmt.Sprintf(`{{if eq .ImportPath %q}}{{.Dir}}{{end}}`, pkg)}
	if p.testBinary {
		args = append(args, "-test")
		extra = append(extra[:len(extra):len(extra)], "nsgotest")
	}
	tags, err := goTags(ctx, p, env, extra...)
	if err != nil {
		return "", err
	}
	args = append(args, tagsArgs(tags)...)
	cmd := exec.CommandContext(ctx, "go", append(args, ".")...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go list: %w\n%s", err, stderr.Bytes())
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line, nil
		}
	}
	return "", nil
}

// addMacLibraries puts the native libraries the app links for arch into
// its Contents/Frameworks, signed ad hoc as the app is; the app's own
// signature then seals them.
func addMacLibraries(ctx context.Context, p *project, app, arch string) error {
	dir, err := linkedDir(ctx, p, []string{"GOOS=darwin", "GOARCH=" + arch, "CGO_ENABLED=0"}, ghosttyPackage)
	if err != nil {
		return err
	}
	frameworks := filepath.Join(app, "Contents", "Frameworks")
	if dir != "" {
		if err := copyLibrary(filepath.Join(dir, "lib", "darwin-"+arch, "libghostty-vt.dylib"), filepath.Join(frameworks, "libghostty-vt.dylib")); err != nil {
			return err
		}
	}
	if err := p.addNativeLibraries(ctx, "macos_"+goArchName(arch), frameworks); err != nil {
		return err
	}
	libs, err := filepath.Glob(filepath.Join(frameworks, "*.dylib"))
	if err != nil {
		return err
	}
	for _, lib := range libs {
		if err := run(ctx, "codesign", "--force", "--sign", "-", lib); err != nil {
			return err
		}
	}
	return nil
}

// addIOSFrameworks puts the native libraries the app links for sys into its
// Frameworks as frameworks: signed ad hoc for the simulator, left for the
// app's signing on devices (signFrameworks).
func addIOSFrameworks(ctx context.Context, p *project, sys *appleOS, app string, env []string, simulator bool) error {
	dir, err := linkedDir(ctx, p, env, ghosttyPackage)
	if err != nil || dir == "" {
		return err
	}
	build := sys.target + "-arm64"
	if simulator {
		build = sys.target + "simulator-arm64"
	}
	lib := filepath.Join(dir, "lib", build, "libghostty-vt.dylib")
	if _, err := os.Stat(lib); err != nil && sys != iOS {
		// tools/libghostty builds no tvOS library yet; the terminal then
		// reports it missing.
		detailIn(ctx, "no libghostty-vt for %s: the terminal will not start", sys.name)
		return nil
	}
	fw := filepath.Join(app, "Frameworks", ghosttyFramework)
	binary := filepath.Join(fw, "GhosttyVT")
	if err := copyLibrary(lib, binary); err != nil {
		return err
	}
	if err := run(ctx, "install_name_tool", "-id", "@rpath/"+ghosttyFramework+"/GhosttyVT", binary); err != nil {
		return err
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key><string>GhosttyVT</string>
	<key>CFBundleIdentifier</key><string>` + p.ID + `.ghosttyvt</string>
	<key>CFBundleName</key><string>GhosttyVT</string>
	<key>CFBundlePackageType</key><string>FMWK</string>
	<key>CFBundleShortVersionString</key><string>1.0</string>
	<key>CFBundleVersion</key><string>1</string>
	<key>CFBundleSupportedPlatforms</key><array><string>` + sys.platformName(simulator) + `</string></array>
	<key>MinimumOSVersion</key><string>` + sys.minimum + `</string>
</dict>
</plist>
`
	if err := os.WriteFile(filepath.Join(fw, "Info.plist"), []byte(plist), 0o644); err != nil {
		return err
	}
	if simulator {
		return run(ctx, "codesign", "--force", "--sign", "-", fw)
	}
	return nil
}

// signFrameworks signs the frameworks of an iOS app with identity, before
// the app is, which seals them.
func signFrameworks(ctx context.Context, app, identity string) error {
	fws, _ := filepath.Glob(filepath.Join(app, "Frameworks", "*.framework"))
	for _, fw := range fws {
		if err := run(ctx, "codesign", "--force", "--sign", identity, fw); err != nil {
			return err
		}
	}
	return nil
}

// copyLibrary copies a native library to dst, making its directory.
func copyLibrary(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}

// addDesktopLibraries places terminal and package-declared libraries beside the executable before signing and installer assembly.
func addDesktopLibraries(ctx context.Context, p *project, dir, system, arch string) error {
	env := []string{"GOOS=" + system, "GOARCH=" + arch, "CGO_ENABLED=0"}
	source, err := linkedDir(ctx, p, env, ghosttyPackage)
	if err != nil {
		return err
	}
	target := system + "_" + goArchName(arch)
	if source != "" {
		name := "libghostty-vt.so"
		if system == "windows" {
			name = "ghostty-vt.dll"
		}
		source = filepath.Join(source, "lib", system+"-"+arch, name)
		if err := validateNativeBinary(source, target); err != nil {
			return fmt.Errorf("libghostty-vt: %w", err)
		}
		if err := copyLibrary(source, filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return p.addNativeLibraries(ctx, target, dir)
}

// addTUILibraries puts the libraries the packages of a terminal target's build declare for its desktop beside the executable, as a console program has no bundle. On a Mac it signs macOS's ad hoc, as addMacLibraries does; elsewhere, with no codesign, they ship as published, and an arm64 one must be signed already, as Apple silicon loads no unsigned code.
func addTUILibraries(ctx context.Context, p *project, dir, target string) error {
	native := nativeTarget(target)
	if err := p.addNativeLibraries(ctx, native, dir); err != nil {
		return err
	}
	if !strings.HasPrefix(native, "macos_") {
		return nil
	}
	signed := map[string]bool{}
	for _, library := range p.nativeLibraries[native] {
		if signed[library.Name] {
			continue
		}
		signed[library.Name] = true
		path := filepath.Join(dir, library.Name)
		if macHost {
			if err := run(ctx, "codesign", "--force", "--sign", "-", path); err != nil {
				return err
			}
		} else if native == "macos_arm64" && !armSigned(path) {
			return fmt.Errorf("native library %s from %s has no code signature, which Apple silicon requires: build %s on a Mac, which signs it ad hoc", library.Name, library.owner, target)
		}
	}
	return nil
}

// armSigned reports whether every arm64 slice of the Mach-O file at path carries a code signature.
func armSigned(path string) bool {
	var files []*macho.File
	if fat, err := macho.OpenFat(path); err == nil {
		defer fat.Close()
		for _, arch := range fat.Arches {
			files = append(files, arch.File)
		}
	} else if thin, err := macho.Open(path); err == nil {
		defer thin.Close()
		files = append(files, thin)
	}
	arm := false
	for _, f := range files {
		if f.Cpu != macho.CpuArm64 {
			continue
		}
		arm = true
		if !slices.ContainsFunc(f.Loads, func(load macho.Load) bool {
			raw := load.Raw()
			return len(raw) >= 8 && f.ByteOrder.Uint32(raw) == machoCodeSignature
		}) {
			return false
		}
	}
	return arm
}

// addIOSEmbeddedLibraries supplies the host with a companion XCFramework whenever its app links the terminal, for static and dynamic embedding alike.
func addIOSEmbeddedLibraries(ctx context.Context, p *project, dir, tmp, minimum string) error {
	args := []string{"-create-xcframework"}
	for _, sl := range iosSlices {
		var libraries []string
		for _, arch := range sl.archs {
			env := []string{"GOOS=" + sl.goos, "GOARCH=" + arch, "CGO_ENABLED=1"}
			tags := []string{"nsgoembed"}
			if sl.catalyst {
				tags = append(tags, "ios")
			}
			source, err := linkedDir(ctx, p, env, ghosttyPackage, tags...)
			if err != nil {
				return err
			}
			if source == "" {
				continue
			}
			platform := "ios"
			if sl.catalyst {
				platform = "ioscatalyst"
			} else if sl.sdk == "iphonesimulator" {
				platform = "iossimulator"
			}
			libraries = append(libraries, filepath.Join(source, "lib", platform+"-"+arch, "libghostty-vt.dylib"))
		}
		if len(libraries) == 0 {
			continue
		}
		fw, err := writeFrameworkBundle(filepath.Join(tmp, "ghostty", sl.sdk), "GhosttyVT", p.ID+".ghosttyvt", "", frameworkMinimum(minimum, sl.catalyst), sl.catalyst)
		if err != nil {
			return err
		}
		binary, install := filepath.Join(fw, "GhosttyVT"), "@rpath/"+ghosttyFramework+"/GhosttyVT"
		if sl.catalyst {
			binary = filepath.Join(fw, "Versions/A/GhosttyVT")
			install = "@rpath/" + ghosttyFramework + "/Versions/A/GhosttyVT"
		}
		if err := run(ctx, "lipo", append([]string{"-create", "-output", binary}, libraries...)...); err != nil {
			return err
		}
		if err := run(ctx, "install_name_tool", "-id", install, binary); err != nil {
			return err
		}
		if err := run(ctx, "codesign", "--force", "--sign", "-", fw); err != nil {
			return err
		}
		args = append(args, "-framework", fw)
	}
	if len(args) > 1 && embedModule(p) == "GhosttyVT" {
		return fmt.Errorf("the app framework name GhosttyVT conflicts with the terminal companion: choose another app name")
	}
	framework := filepath.Join(dir, "GhosttyVT.xcframework")
	if err := os.RemoveAll(framework); err != nil {
		return err
	}
	if len(args) == 1 {
		return nil
	}
	return run(ctx, "xcodebuild", append(args, "-output", framework)...)
}

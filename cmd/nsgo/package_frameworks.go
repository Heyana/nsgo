package main

import (
	"context"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The frameworks that nsgo-natives.yaml version 2 declares for iOS and tvOS, which NSGo checks against the app and writes into its bundle as it writes libghostty-vt's (addIOSFrameworks).

// Mach-O's load commands that a framework is checked by (<mach-o/loader.h>).
const (
	machoLoadDylib       = 0xc
	machoIDDylib         = 0xd
	machoLazyLoadDylib   = 0x20
	machoBuildVersion    = 0x32
	machoReexportDylib   = 0x8000001f
	machoLoadUpwardDylib = 0x80000023
)

// machoPlatforms names the platforms of LC_BUILD_VERSION, as a refusal says them.
var machoPlatforms = map[uint32]string{
	1: "macOS", 2: "iOS devices", 3: "tvOS devices", 4: "watchOS devices", 5: "bridgeOS", 6: "Mac Catalyst",
	7: "the iOS simulator", 8: "the tvOS simulator", 9: "the watchOS simulator", 10: "DriverKit",
	11: "visionOS devices", 12: "the visionOS simulator",
}

// frameworkPlatforms are the platforms of nsgo-natives.yaml's Apple targets.
var frameworkPlatforms = map[string]uint32{"ios": 2, "ios_simulator": 7, "tvos": 3, "tvos_simulator": 8}

func machoPlatform(platform uint32) string {
	if name, ok := machoPlatforms[platform]; ok {
		return name
	}
	return fmt.Sprintf("platform %d", platform)
}

// machoVersion is a version as Mach-O encodes it, xxxx.yy.zz in nibbles.
func machoVersion(v uint32) [3]uint32 {
	return [3]uint32{v >> 16, v >> 8 & 0xff, v & 0xff}
}

// addPackageFrameworks writes the frameworks the app's packages declare for sys's devices or simulator into the bundle's Frameworks, each checked first (checkFramework) and given an Info.plist of its own like GhosttyVT's: signed ad hoc for the simulator, left for the app's signing on devices (signFrameworks).
func addPackageFrameworks(ctx context.Context, p *project, sys *appleOS, app string, simulator bool) error {
	target := sys.target
	if simulator {
		target += "_simulator"
	}
	frameworks := p.nativeLibraries[target]
	if len(frameworks) == 0 {
		return nil
	}
	minimum, err := p.appleMinimum(sys)
	if err != nil {
		return err
	}
	declared := make([]string, len(frameworks))
	for i, library := range frameworks {
		declared[i] = library.Name
	}
	done := map[string]bool{}
	for _, library := range frameworks {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Packages that declare one name declare one file (addPackageNatives).
		if done[library.Name] {
			continue
		}
		done[library.Name] = true
		if err := addPackageFramework(ctx, p, library, sys, simulator, app, minimum, declared); err != nil {
			return fmt.Errorf("native framework %s from %s for %s: %w", library.Name, library.owner, target, err)
		}
	}
	return nil
}

func addPackageFramework(ctx context.Context, p *project, library nativeLibrary, sys *appleOS, simulator bool, app, minimum string, declared []string) error {
	source, err := library.source(ctx)
	if err != nil {
		return err
	}
	if err := verifyNativeChecksum(source, library.SHA256); err != nil {
		return err
	}
	if err := checkFramework(source, sys, simulator, library.Name, minimum, declared); err != nil {
		return err
	}
	fw := filepath.Join(app, "Frameworks", library.Name)
	if _, err := os.Lstat(fw); err == nil {
		return errors.New("the app holds a framework of that name already")
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	name := strings.TrimSuffix(library.Name, ".framework")
	binary := filepath.Join(fw, name)
	if err := copyNativeLibrary(source, binary, library.SHA256); err != nil {
		return err
	}
	if fat, err := macho.OpenFat(binary); err == nil {
		multiple := len(fat.Arches) > 1
		fat.Close()
		if multiple {
			if err := run(ctx, "lipo", "-thin", "arm64", binary, "-output", binary); err != nil {
				return err
			}
		}
	}
	plist := encodePlist(map[string]any{
		"CFBundleExecutable":         name,
		"CFBundleIdentifier":         p.ID + "." + strings.ToLower(name),
		"CFBundleName":               name,
		"CFBundlePackageType":        "FMWK",
		"CFBundleShortVersionString": "1.0",
		"CFBundleVersion":            "1",
		"CFBundleSupportedPlatforms": []any{sys.platformName(simulator)},
		"MinimumOSVersion":           minimum,
	})
	if err := os.WriteFile(filepath.Join(fw, "Info.plist"), plist, 0o644); err != nil {
		return err
	}
	if !simulator {
		return nil
	}
	return run(ctx, "codesign", "--force", "--sign", "-", fw)
}

// checkFramework refuses a framework's binary that sys's devices or simulator could not load in the app: anything but an arm64 Mach-O dynamic library built for that platform, as its LC_BUILD_VERSION says; one that needs a newer OS than minimum, the app's; one whose install name is not @rpath/<Name>.framework/<Name>; and one that loads a framework through @rpath that is not among declared, the frameworks packaged for the same target. A universal file is checked by its arm64 slice, which is the one the bundle keeps.
func checkFramework(path string, sys *appleOS, simulator bool, name, minimum string, declared []string) error {
	var f *macho.File
	if fat, err := macho.OpenFat(path); err == nil {
		defer fat.Close()
		for _, arch := range fat.Arches {
			if arch.Cpu == macho.CpuArm64 {
				f = arch.File
				break
			}
		}
	} else if thin, err := macho.Open(path); err == nil {
		defer thin.Close()
		f = thin
	}
	if f == nil || f.Cpu != macho.CpuArm64 || f.Magic != macho.Magic64 || f.Type != macho.TypeDylib {
		return errors.New("not an arm64 Mach-O dynamic library")
	}
	var platform, minos uint32
	var install string
	var loads []string
	for _, load := range f.Loads {
		raw := load.Raw()
		if len(raw) < 8 {
			continue
		}
		switch f.ByteOrder.Uint32(raw) {
		case machoBuildVersion:
			if len(raw) >= 16 {
				platform, minos = f.ByteOrder.Uint32(raw[8:]), f.ByteOrder.Uint32(raw[12:])
			}
		case machoIDDylib:
			install = dylibName(f.ByteOrder, raw)
		case machoLoadDylib, machoReexportDylib, machoLoadUpwardDylib, machoLazyLoadDylib:
			// A weak load may be missing at run time, so only these need their framework.
			loads = append(loads, dylibName(f.ByteOrder, raw))
		}
	}
	target := sys.target
	if simulator {
		target += "_simulator"
	}
	if want := frameworkPlatforms[target]; platform == 0 {
		return errors.New("names no platform: it has no LC_BUILD_VERSION")
	} else if platform != want {
		return fmt.Errorf("built for %s, not %s", machoPlatform(platform), machoPlatform(want))
	}
	needs := machoVersion(minos)
	app, _ := appleVersion(minimum)
	if slices.Compare(needs[:], app[:]) > 0 {
		version := fmt.Sprintf("%d.%d", needs[0], needs[1])
		if needs[2] != 0 {
			version += fmt.Sprintf(".%d", needs[2])
		}
		return fmt.Errorf("needs %s %s, newer than the app's minimum %s", sys.name, version, minimum)
	}
	base := strings.TrimSuffix(name, ".framework")
	if want := "@rpath/" + name + "/" + base; install != want {
		return fmt.Errorf("its install name is %q, not %s", install, want)
	}
	for _, load := range loads {
		rest, rpath := strings.CutPrefix(load, "@rpath/")
		dir, file, _ := strings.Cut(rest, "/")
		peer, framework := strings.CutSuffix(dir, ".framework")
		if !rpath || !framework {
			continue
		}
		if file != peer || !slices.Contains(declared, dir) {
			return fmt.Errorf("loads %s, which no framework packaged for %s provides", load, target)
		}
	}
	return nil
}

// dylibName is the path a dylib_command names, as its lc_str offset gives it: past the command's 24 bytes.
func dylibName(order binary.ByteOrder, raw []byte) string {
	if len(raw) < 24 {
		return ""
	}
	offset := order.Uint32(raw[8:])
	if offset < 24 || uint64(offset) >= uint64(len(raw)) {
		return ""
	}
	name, _, _ := strings.Cut(string(raw[offset:]), "\x00")
	return name
}

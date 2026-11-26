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
	machoCodeSignature   = 0x1d
	machoLazyLoadDylib   = 0x20
	machoBuildVersion    = 0x32
	machoRpath           = 0x8000001c
	machoReexportDylib   = 0x8000001f
	machoLoadUpwardDylib = 0x80000023
)

// machoSubtypeMask leaves a Mach-O CPU subtype without its capability bits, so that 0 is CPU_SUBTYPE_ARM64_ALL, plain arm64, and 2 arm64e.
const machoSubtypeMask = 0x00ffffff

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
	ids, err := bundleIDs(app)
	if err != nil {
		return err
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
		// Apple tells bundles apart by their identifiers alone, in any case, so a framework may not take one that NSGo gave a bundle of its own, such as NSGoStoreKit.framework's to StoreKit.framework, or that another framework has.
		id := p.ID + "." + strings.ToLower(strings.TrimSuffix(library.Name, ".framework"))
		if bundle, taken := ids[strings.ToLower(id)]; taken {
			return fmt.Errorf("native framework %s from %s for %s: its bundle ID %s is %s's", library.Name, library.owner, target, id, bundle)
		}
		ids[strings.ToLower(id)] = filepath.Join("Frameworks", library.Name)
		if err := addPackageFramework(ctx, p, library, sys, simulator, app, minimum, declared); err != nil {
			return fmt.Errorf("native framework %s from %s for %s: %w", library.Name, library.owner, target, err)
		}
	}
	return nil
}

// bundleIDs maps the identifiers of the bundles that app holds in its Frameworks, PlugIns, Extensions and AppClips, in lower case, to each bundle's path in app.
func bundleIDs(app string) (map[string]string, error) {
	ids := map[string]string{}
	for _, dir := range []string{"Frameworks", "PlugIns", "Extensions", "AppClips"} {
		plists, err := filepath.Glob(filepath.Join(app, dir, "*", "Info.plist"))
		if err != nil {
			return nil, err
		}
		for _, path := range plists {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			v, err := decodePlist(data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
			if plist, ok := v.(map[string]any); ok {
				if id, ok := plist["CFBundleIdentifier"].(string); ok {
					bundle, _ := filepath.Rel(app, filepath.Dir(path))
					ids[strings.ToLower(id)] = bundle
				}
			}
		}
	}
	return ids, nil
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

// checkFramework refuses a framework's binary that sys's devices or simulator could not load in the app: anything but a plain arm64 Mach-O dynamic library, not arm64e, built for that platform, as its LC_BUILD_VERSION says; one that needs a newer OS than minimum, the app's; one whose install name is not @rpath/<Name>.framework/<Name>; one that loads a framework through @rpath that is not among declared, the frameworks packaged for the same target, or that loads one without a run path to the app's Frameworks, which the app's executable has none of; and one that loads through @rpath what is no framework without a run path of the system's, as nothing in the bundle provides it. A universal file is checked by its arm64 slice, which is the one the bundle keeps (lipo -thin arm64).
func checkFramework(path string, sys *appleOS, simulator bool, name, minimum string, declared []string) error {
	var f *macho.File
	if fat, err := macho.OpenFat(path); err == nil {
		defer fat.Close()
		for _, arch := range fat.Arches {
			if arch.Cpu == macho.CpuArm64 && arch.SubCpu&machoSubtypeMask == 0 {
				f = arch.File
				break
			}
		}
	} else if thin, err := macho.Open(path); err == nil {
		defer thin.Close()
		f = thin
	}
	if f == nil || f.Cpu != macho.CpuArm64 || f.SubCpu&machoSubtypeMask != 0 || f.Magic != macho.Magic64 || f.Type != macho.TypeDylib {
		return errors.New("not an arm64 Mach-O dynamic library; arm64e is not arm64")
	}
	var platform, minos uint32
	var install string
	var loads, rpaths []string
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
			install = loadString(f.ByteOrder, raw, 24)
		case machoLoadDylib, machoReexportDylib, machoLoadUpwardDylib, machoLazyLoadDylib:
			// A weak load may be missing at run time, so only these need their framework.
			loads = append(loads, loadString(f.ByteOrder, raw, 24))
		case machoRpath:
			rpaths = append(rpaths, loadString(f.ByteOrder, raw, 12))
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
	// A peer is found in Frameworks through the framework's own run path: @loader_path is the framework's directory, and @executable_path the app's.
	frameworks := slices.ContainsFunc(rpaths, func(r string) bool {
		r = strings.TrimSuffix(r, "/")
		return r == "@loader_path/.." || r == "@executable_path/Frameworks"
	})
	// What is no framework only the system provides, as Swift's libraries for an OS older than iOS 12.2 are through /usr/lib/swift.
	system := slices.ContainsFunc(rpaths, func(r string) bool { return strings.HasPrefix(r, "/") })
	for _, load := range loads {
		rest, rpath := strings.CutPrefix(load, "@rpath/")
		if !rpath {
			continue
		}
		dir, file, _ := strings.Cut(rest, "/")
		peer, framework := strings.CutSuffix(dir, ".framework")
		if !framework {
			if !system {
				return fmt.Errorf("loads %s, which nothing in the app provides: the bundle holds only frameworks", load)
			}
			continue
		}
		switch {
		case file != peer || !slices.Contains(declared, dir):
			return fmt.Errorf("loads %s, which no framework packaged for %s provides", load, target)
		case !frameworks:
			return fmt.Errorf("loads %s with no run path to the app's Frameworks, such as @loader_path/..", load)
		}
	}
	return nil
}

// loadString is the string a load command names by the lc_str offset at its eighth byte, past the command's fixed part of size bytes: 24 for a dylib_command, 12 for an rpath_command.
func loadString(order binary.ByteOrder, raw []byte, size uint32) string {
	if uint32(len(raw)) < size {
		return ""
	}
	offset := order.Uint32(raw[8:])
	if offset < size || uint64(offset) >= uint64(len(raw)) {
		return ""
	}
	name, _, _ := strings.Cut(string(raw[offset:]), "\x00")
	return name
}

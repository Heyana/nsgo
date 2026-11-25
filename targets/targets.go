// Package targets names the build targets nsgo accepts and expands the groups they form, so a tool that writes or checks an app's build.targets shares the tool's own table.
package targets

import (
	"fmt"
	"slices"
	"strings"
)

// MiniProgram names every mini program target at once.
const MiniProgram = "miniprogram"

// names are the targets nsgo build accepts, each one package, in build order.
var names = []string{
	"macos_arm64", "macos_x64", "macos_universal",
	"windows_arm64", "windows_x64",
	"linux_arm64", "linux_x64",
	"ios", "ios_simulator",
	"tvos", "tvos_simulator",
	"watchos", "watchos_simulator",
	"android_arm64", "android_x64",
	"harmonyos",
	"web",
	"miniprogram_wechat", "miniprogram_qq", "miniprogram_alipay", "miniprogram_douyin",
	"tui_macos_arm64", "tui_macos_x64", "tui_windows_arm64", "tui_windows_x64", "tui_linux_arm64", "tui_linux_x64",
}

// Names returns every target nsgo build accepts, in build order.
func Names() []string { return slices.Clone(names) }

// GoAndXcode returns the packages a Mac with Go and Xcode can build, which is what all stands for. android_x64 is omitted: Go links android/amd64 only with cgo, so it needs an NDK; macos_universal, which joins the two macOS targets' builds; the harmonyos preview, and the mini programs, which only some apps fit.
func GoAndXcode() []string {
	var out []string
	for _, target := range names {
		if target != "android_x64" && target != "macos_universal" && target != "harmonyos" && !strings.HasPrefix(target, "miniprogram_") {
			out = append(out, target)
		}
	}
	return out
}

// Expand replaces all with every target that needs only Go and Xcode, a bare OS name with its architectures, and MiniProgram with every mini program, checks every name and drops duplicates, so each package is built once. android_x64 stays out of all and android because it needs an NDK, macos_universal out of all and macos because it builds both macOS architectures again to join them, and the mini programs because they take only Go apps that fit their packages.
func Expand(requested []string) ([]string, error) {
	var out []string
	for _, name := range requested {
		expanded := []string{name}
		switch {
		case name == "all":
			expanded = GoAndXcode()
		case name == "tui":
			expanded = withPrefix("tui_")
		case name == MiniProgram:
			expanded = withPrefix("miniprogram_")
		case slices.Contains([]string{"macos", "windows", "linux", "android"}, name):
			// The bare OS, as nsgo test -target spells it (#589).
			expanded = nil
			for _, target := range names {
				if goos, _, _ := strings.Cut(target, "_"); goos == name && target != "android_x64" && target != "macos_universal" {
					expanded = append(expanded, target)
				}
			}
		case !slices.Contains(names, name):
			return nil, fmt.Errorf("unknown target %q; use all, miniprogram or one of %s", name, strings.Join(names, ", "))
		}
		for _, target := range expanded {
			if !slices.Contains(out, target) {
				out = append(out, target)
			}
		}
	}
	return out, nil
}

// withPrefix returns every target whose name starts with prefix.
func withPrefix(prefix string) []string {
	var out []string
	for _, target := range names {
		if strings.HasPrefix(target, prefix) {
			out = append(out, target)
		}
	}
	return out
}

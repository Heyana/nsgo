package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
)

// Delegation keeps an nsgo installed on PATH from building a project with another version than the one its go.mod pins, as Gradle's wrapper does: in a module that declares nsgo as a tool at another version, nsgo runs the module's own, go tool nsgo, with the same arguments before doing anything else, so the module's nsgo resolves built-in commands and extensions alike.

// nsgoTool is the tool line a module that pins nsgo declares.
const nsgoTool = nsgoModule + "/cmd/nsgo"

// delegate runs the module's nsgo with args, and returns its exit status, when the working directory's module declares nsgoTool at a version other than this nsgo's. NSGO_DELEGATED=1, which it sets for the module's nsgo, stops a second delegation, and NSGO_NO_DELEGATE=1 turns delegation off.
func delegate(ctx context.Context, args []string) (int, bool) {
	if os.Getenv("NSGO_DELEGATED") == "1" || os.Getenv("NSGO_NO_DELEGATE") == "1" {
		return 0, false
	}
	pinned, ok := moduleNSGoVersion(ctx)
	if !ok || pinned == toolVersion() {
		return 0, false
	}
	cmd := goToolCommand(nsgoTool, args)
	cmd.Env = append(os.Environ(), "NSGO_DELEGATED=1", "NSGO_DELEGATED_FROM="+displayVersion(toolVersion()))
	code, err := runChild(cmd)
	if err != nil {
		report(fmt.Errorf("running the module's nsgo %s: %w", displayVersion(pinned), err))
		return 1, true
	}
	return code, true
}

// moduleNSGoVersion is the version of nsgo the working directory's module runs as its tool, as toolVersion reports it for that build: a replacement's version, empty for a replacement by a directory, else the required version. It is false outside a module, for a module without the tool, in NSGo's own module and when the go.mod cannot be read, which leaves the command to this nsgo.
func moduleNSGoVersion(ctx context.Context) (string, bool) {
	mod, err := moduleFile(ctx, ".")
	if mod == nil || err != nil {
		return "", false
	}
	declared := false
	for _, t := range mod.Tool {
		declared = declared || t.Path == nsgoTool
	}
	if !declared {
		return "", false
	}
	if mod.Module.Path == nsgoModule {
		// NSGo's own checkout, where whoever runs nsgo works on it and picks the build they mean.
		return "", false
	}
	var required string
	for _, r := range mod.Require {
		if r.Path == nsgoModule {
			required = r.Version
		}
	}
	for _, r := range mod.Replace {
		if r.Old.Path == nsgoModule && (r.Old.Version == "" || r.Old.Version == required) {
			return r.New.Version, true
		}
	}
	return required, true
}

// displayVersion is version as nsgo shows it, devel for a build that records none.
func displayVersion(version string) string {
	return cmp.Or(version, "devel")
}

// printVersion prints which nsgo runs: its version, and why when it is not the one called, the module's run in its place, or when NSGO_NO_DELEGATE kept another from running.
func printVersion(ctx context.Context) {
	version := displayVersion(toolVersion())
	switch {
	case os.Getenv("NSGO_DELEGATED") == "1":
		version += fmt.Sprintf(" (the module's; called as %s)", cmp.Or(os.Getenv("NSGO_DELEGATED_FROM"), "devel"))
	case os.Getenv("NSGO_NO_DELEGATE") == "1":
		if pinned, ok := moduleNSGoVersion(ctx); ok && pinned != toolVersion() {
			version += fmt.Sprintf(" (NSGO_NO_DELEGATE=1; the module's is %s)", displayVersion(pinned))
		}
	}
	fmt.Println("nsgo " + version)
}

package main

import (
	"runtime"
	"slices"
	"testing"
)

func TestTerminalTargetsAndHostSelection(t *testing.T) {
	names, err := expandTargets([]string{"tui", "tui_windows_x64"})
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 6 {
		t.Fatalf("terminal packages: %v", names)
	}
	for _, target := range names {
		env, err := vetEnv(t.Context(), target)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(env, "CGO_ENABLED=0") || !slices.Equal(targetTags(target), []string{"nsgotui"}) {
			t.Fatalf("%s: %v %v", target, env, targetTags(target))
		}
		if runnable(target) != (target == tuiTarget(runtime.GOOS, runtime.GOARCH)) {
			t.Errorf("wrong host selection for %s", target)
		}
	}
	if got, err := checkTestTarget(tuiTarget(runtime.GOOS, runtime.GOARCH), false); err != nil || got != "tui" {
		t.Fatalf("host terminal tests: %q %v", got, err)
	}
	for _, bad := range []string{"tui_plan9_x64", "tui_windows_riscv64", "tui_linux"} {
		if _, err := tuiEnv(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	linux := &machine{goos: "linux", goarch: "amd64", linuxLibs: []string{"GTK missing"}}
	if v := linux.run("tui_linux_x64"); !v.ok {
		t.Fatalf("terminal must not require GTK: %+v", v)
	}
}

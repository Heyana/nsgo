package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestWatchHostCompile compiles watch/Host.swift for both watchOS device and
// watchOS simulator SDKs and verifies the exported C bridge symbols and
// permission/usage-description guards stay intact.
func TestWatchHostCompile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs Xcode")
	}
	if err := exec.Command("xcrun", "--find", "swiftc").Run(); err != nil {
		t.Skip("needs Xcode's swiftc")
	}
	for _, sim := range []bool{true, false} {
		sdk := watchOS.sdkName(sim)
		t.Run(sdk, func(t *testing.T) {
			if err := exec.Command("xcrun", "--sdk", sdk, "--show-sdk-path").Run(); err != nil {
				t.Skipf("needs the %s SDK", sdk)
			}
			obj, err := compileWatchHost(t.Context(), sim)
			if err != nil {
				t.Fatal(err)
			}
			defer os.RemoveAll(filepath.Dir(obj))

			out, err := exec.Command("nm", "-gU", obj).Output()
			if err != nil {
				t.Fatalf("nm %s: %v", obj, err)
			}
			syms := string(out)
			for _, sym := range []string{
				"_nsgo_watch_connect",
				"_nsgo_watch_main",
				"_nsgo_watch_apply",
				"_nsgo_watch_snapshot",
				"_nsgo_watch_tick",
			} {
				if !strings.Contains(syms, sym) {
					t.Errorf("%s host object exports no %s", sdk, sym)
				}
			}
		})
	}
}

// TestWatchHostUsageDescriptionGuards checks that Host.swift verifies the
// required Info.plist usage description keys before prompting for watchOS
// permissions, preventing TCC termination on missing keys.
func TestWatchHostUsageDescriptionGuards(t *testing.T) {
	src := string(watchHost)
	for _, key := range []string{
		"NSMicrophoneUsageDescription",
		"NSCalendarsFullAccessUsageDescription",
		"NSCalendarsUsageDescription",
		"NSRemindersFullAccessUsageDescription",
		"NSRemindersUsageDescription",
		"NSBluetoothAlwaysUsageDescription",
		"CBCentralManagerOptionShowPowerAlertKey",
	} {
		if !strings.Contains(src, key) {
			t.Errorf("watch/Host.swift is missing %s", key)
		}
	}
}

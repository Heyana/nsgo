package main

import (
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestStoreKitCompile compiles StoreKit's library for this Mac and for the iOS and tvOS simulators, exporting the two functions internal/platform/apple calls, which the Swift source needs to keep doing.
func TestStoreKitCompile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs Xcode")
	}
	if err := exec.Command("xcrun", "--find", "swiftc").Run(); err != nil {
		t.Skip("needs Xcode's swiftc")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir()) // os.UserCacheDir reads it on macOS
	for _, c := range []struct{ sdk, triple string }{
		{"macosx", macStoreKitTriple(runtime.GOARCH)},
		{iOS.sdkName(true), iOS.triple(true)},
		{tvOS.sdkName(true), tvOS.triple(true)},
	} {
		lib, err := compileStoreKit(t.Context(), c.sdk, c.triple, "@rpath/"+storeKitDylib)
		if err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command("nm", "-gU", lib).Output()
		for _, sym := range []string{"_nsgo_storekit_start", "_nsgo_storekit_call"} {
			if err != nil || !strings.Contains(string(out), sym) {
				t.Errorf("the %s library exports no %s: %v", c.sdk, sym, err)
			}
		}
	}
}

// Only an app that sets apple.purchases gets the library; an App Clip never does.
func TestStoreKitOnlyWhereAsked(t *testing.T) {
	app := t.TempDir()
	p := &project{Config: projectcfg.Config{ID: "dev.example.app"}}
	if err := addMacStoreKit(t.Context(), p, app, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}
	if err := addIOSStoreKit(t.Context(), p, iOS, app, true); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(app); len(entries) != 0 {
		t.Errorf("an app without purchases got %v", entries)
	}
	if _, err := os.Stat(filepath.Join(app, "Contents", "Frameworks", storeKitDylib)); err == nil {
		t.Error("an app without purchases got the library")
	}
	p.Apple.Purchases = true
	p.AppClip = &appClip{Name: "Clip", ID: "dev.example.app.Clip", Main: "./clip"}
	if p.clipProject().Apple.Purchases {
		t.Error("the App Clip sells in-app purchases")
	}
}

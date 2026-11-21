package main

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/harmony"
)

func TestTestBundleID(t *testing.T) {
	short := testBundleID("dev.example.app")
	if short != "dev.example.app.nsgotest" {
		t.Fatalf("short test bundle = %q", short)
	}
	long := testBundleID(strings.Repeat("a", 56))
	if long == strings.Repeat("a", 56)+".nsgotest" {
		t.Fatal("long test bundle kept the app identity")
	}
	if _, err := harmony.BundleName(long); err != nil {
		t.Fatalf("fallback bundle = %q: %v", long, err)
	}
}

func TestTestProjectDropsHarmonyOSReleaseProfile(t *testing.T) {
	p := &project{ID: "dev.example.app"}
	signing := &harmony.Signing{Profile: "app.p7b"}
	p.Harmonyos.Signing = signing
	if testProject(p).Harmonyos.Signing != nil || p.Harmonyos.Signing != signing {
		t.Fatal("the release profile must stay with the original bundle ID")
	}
}

// iOS installs an App Clip only under its app's ID, so the test build's clip moves under the test's ID and leaves the app's own alone (#898).
func TestTestProjectMovesTheAppClip(t *testing.T) {
	p := &project{ID: "dev.example.app", AppClip: &appClip{ID: "dev.example.app.Clip"}}
	test := testProject(p)
	if test.ID != "dev.example.app.nsgotest" || test.AppClip.ID != "dev.example.app.nsgotest.Clip" {
		t.Fatalf("test project = %q with clip %q", test.ID, test.AppClip.ID)
	}
	if p.AppClip.ID != "dev.example.app.Clip" {
		t.Fatalf("the app's clip became %q", p.AppClip.ID)
	}
}

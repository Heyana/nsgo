package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestFindAdbInTheSDK checks the fallback that finds adb without it being
// on PATH, so nsgo dev works after installing the SDK in its usual place.
func TestFindAdbInTheSDK(t *testing.T) {
	home := t.TempDir()
	name := "adb"
	if runtime.GOOS == "windows" {
		name = "adb.exe"
	}
	want := filepath.Join(home, "Android", "platform-tools", name)
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PATH", "")
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")

	got, err := findAdb()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("findAdb = %q, want %q", got, want)
	}
}

// TestFindAdbWithoutTheSDK checks the error that names what to install.
func TestFindAdbWithoutTheSDK(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("PATH", "")
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")

	if _, err := findAdb(); err == nil {
		t.Fatal("findAdb found adb in an empty machine")
	}
}

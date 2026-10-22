package main

import (
	"runtime"
	"strings"
	"testing"
)

func TestNativeSessionFollowsTheHost(t *testing.T) {
	p := &project{}
	p.Dev.Target = "native"
	s, err := sessionFor(p, devOptions{})
	switch runtime.GOOS {
	case "darwin", "windows", "linux":
		if err != nil {
			t.Fatal(err)
		}
		if s.build == nil || s.start == nil {
			t.Fatal("the native session is missing a step")
		}
	default:
		if err == nil || !strings.Contains(err.Error(), "web") {
			t.Fatalf("native on %s: %v", runtime.GOOS, err)
		}
	}
}

func TestAndroidSessionInstallsOverADB(t *testing.T) {
	p := &project{}
	p.Dev.Target = "android"
	s, err := sessionFor(p, devOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s.build == nil || s.start == nil {
		t.Fatal("the Android session is missing a step")
	}
}

func TestOpenCommandShowsURLs(t *testing.T) {
	name, args := openCommand("https://nsh.dev")
	if got := append([]string{name}, args...); len(got) == 0 || got[len(got)-1] != "https://nsh.dev" {
		t.Fatalf("open command = %q, want the target last", got)
	}
	switch runtime.GOOS {
	case "darwin":
		if name != "open" {
			t.Fatalf("macOS opens with %q", name)
		}
	case "windows":
		if name != "rundll32" || len(args) != 2 || args[0] != "url.dll,FileProtocolHandler" {
			t.Fatalf("Windows opens with %q %q", name, args)
		}
	case "linux":
		if name != "xdg-open" || len(args) != 1 {
			t.Fatalf("Linux opens with %q %q", name, args)
		}
	}
}

package main

import (
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInstallScriptsEscapeTagPrefix(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("install.sh runs on macOS and Linux")
	}
	dir := t.TempDir()
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "Fixture", ID: "dev.nsgo.install1411", Version: "1.2.3"}}
	p.Build.Output = filepath.Join(dir, "dist")
	p.Releases.GitHub = "fixture/example"
	prefix := "releases/#v"
	p.Releases.TagPrefix = &prefix
	if err := p.writeInstallScripts(t.Context(), []string{"macos_arm64", "linux_x64", "windows_x64"}); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	// Use an owned package-manager route on every host; no system package
	// manager is called even if download failure handling regresses.
	for name, script := range map[string]string{
		"uname":  "#!/bin/sh\ncase \"$1\" in -s) echo Linux;; -m) echo x86_64;; *) exit 1;; esac\n",
		"pacman": "#!/bin/sh\nexit 97\n",
		"sudo":   "#!/bin/sh\nexit 97\n",
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Record the downloader's actual arguments and stop before installation.
	curl := "#!/bin/sh\nfor arg do\n case \"$arg\" in https://*) printf '%s\\n' \"$arg\" >> \"$NSGO_TEST_CURL_LOG\";; esac\ndone\nexit 22\n"
	if err := os.WriteFile(filepath.Join(bin, "curl"), []byte(curl), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "requests.log")
	cmd := exec.Command("sh", filepath.Join(p.releaseDir(), "install.sh"))
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "NSGO_TEST_CURL_LOG="+log,
		"FIXTURE_VERSION=", "FIXTURE_DOWNLOAD_URL=")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "could not download") {
		t.Fatalf("the stopped download returned %v: %s", err, out)
	}
	requests, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range strings.Fields(string(requests)) {
		u, err := url.Parse(raw)
		if err != nil || u.Fragment != "" || !strings.HasPrefix(u.EscapedPath(), "/fixture/example/releases/download/releases%2F%23v1.2.3/") {
			t.Errorf("download URL lost part of its release tag: %q (%v)", raw, err)
		}
	}
	ps, err := os.ReadFile(filepath.Join(p.releaseDir(), "install.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ps), "'releases%2F%23v'") {
		t.Error("PowerShell download URL does not escape the release tag prefix")
	}
}

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestLinuxPackagesNative installs only inside disposable containers, never in the host's package database. Enable with NSGO_TEST_LINUX_PACKAGES=1; Docker pulls the official distribution images and the containers install their own runtime dependencies.
func TestLinuxPackagesNative(t *testing.T) {
	if os.Getenv("NSGO_TEST_LINUX_PACKAGES") != "1" {
		t.Skip("set NSGO_TEST_LINUX_PACKAGES=1 for disposable Debian, Ubuntu and Arch installs")
	}
	docker, err := exec.LookPath("docker")
	if err != nil {
		t.Fatal("native package acceptance needs Docker:", err)
	}
	dir := t.TempDir()
	stage := filepath.Join(dir, "stage")
	writeFiles(t, stage, map[string]string{
		"PackageCheck":              "#!/bin/sh\nprintf 'nsgo package fixture\\n'\n",
		"assets/space #é\\name.txt": "package asset\n",
	})
	if err := os.Symlink("space #é\\name.txt", filepath.Join(stage, "assets", "alias")); err != nil {
		t.Fatal(err)
	}
	p := &project{Dir: dir, Name: "PackageCheck", ID: "dev.nsgo.test.package1410", Version: "1.0.0", URLSchemes: []string{"nsgo1410"}}
	p.Linux.Maintainer = "NSGo <packages@example.invalid>"
	deb, arch, err := buildLinuxPackages(t.Context(), p, stage, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, image, file, script string }{
		{"debian", "debian:trixie-slim", deb, debInstallCheck},
		{"ubuntu", "ubuntu:26.04", deb, debInstallCheck},
		{"arch", "archlinux:latest", arch, archInstallCheck},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The read-only mount contains only this test's packages and staged files. --rm also removes the isolated package database if a check fails.
			name := "nsgo1410-" + tc.name + "-" + strconv.Itoa(os.Getpid())
			t.Cleanup(func() { _ = exec.Command(docker, "rm", "-f", name).Run() })
			cmd := exec.CommandContext(t.Context(), docker, "run", "--rm", "--name", name, "--label", "dev.nsgo.test=linux-packages1410", "--mount", "type=bind,src="+dir+",dst=/packages,readonly", tc.image, "sh", "-ec", tc.script, "sh", "/packages/stage/"+filepath.Base(tc.file))
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("isolated %s install: %v\n%s", tc.name, err, out)
			}
			for _, marker := range []string{"nsgo package fixture", "NSGO_PACKAGE_INSTALLED", "NSGO_PACKAGE_REMOVED"} {
				if !bytes.Contains(out, []byte(marker)) {
					t.Fatalf("%s acceptance lacks %s:\n%s", tc.name, marker, out)
				}
			}
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "NSGO_PACKAGE_") || strings.Contains(line, "total files") || strings.Contains(line, "altered files") {
					t.Log(line)
				}
			}
		})
	}
}

const linuxInstalledChecks = `
test "$(/usr/bin/packagecheck)" = 'nsgo package fixture'
cat /opt/packagecheck/assets/alias | grep -qx 'package asset'
test "$(readlink /usr/bin/packagecheck)" = /opt/packagecheck/PackageCheck
test -f /usr/share/applications/dev.nsgo.test.package1410.desktop
printf 'nsgo package fixture\nNSGO_PACKAGE_INSTALLED\n'
`

const debInstallCheck = `
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y --no-install-recommends libgtk-4-1 desktop-file-utils shared-mime-info >/dev/null
dpkg -i "$1"
test "$(dpkg-query -W -f='${Status}' packagecheck)" = 'install ok installed'
test -z "$(dpkg --verify packagecheck)"
` + linuxInstalledChecks + `
dpkg --purge packagecheck
! dpkg-query -W -f='${Status}' packagecheck 2>/dev/null | grep -qx 'install ok installed'
test ! -e /usr/bin/packagecheck
test ! -e /opt/packagecheck/PackageCheck
test ! -e /usr/share/applications/dev.nsgo.test.package1410.desktop
printf 'NSGO_PACKAGE_REMOVED\n'
`

const archInstallCheck = `
pacman -Syu --noconfirm --needed gtk4 desktop-file-utils shared-mime-info >/dev/null
pacman -U --noconfirm "$1"
pacman -Q packagecheck
pacman -Qk packagecheck
test -f /var/lib/pacman/local/packagecheck-1.0.0-1/mtree
pacman -Qkk packagecheck
` + linuxInstalledChecks + `
# The database's manifest must detect a size change, not only missing paths.
printf 'changed asset\n' >> '/opt/packagecheck/assets/space #é\name.txt'
if pacman -Qkk packagecheck > /tmp/nsgo1410-altered.log 2>&1; then
 cat /tmp/nsgo1410-altered.log
 exit 1
fi
grep -q 'Size mismatch' /tmp/nsgo1410-altered.log
pacman -R --noconfirm packagecheck
! pacman -Q packagecheck >/dev/null 2>&1
test ! -e /usr/bin/packagecheck
test ! -e /opt/packagecheck/PackageCheck
test ! -e /usr/share/applications/dev.nsgo.test.package1410.desktop
printf 'NSGO_PACKAGE_REMOVED\n'
`

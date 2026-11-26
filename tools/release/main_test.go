package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The stand-in for cmd/nsgo prints its version as nsgo version does, so the test runs the whole build in seconds instead of compiling the real tool six times.
const stubMain = `package main

import (
	"fmt"
	"os"
	"runtime/debug"
)

func main() {
	info, _ := debug.ReadBuildInfo()
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Println("nsgo " + info.Main.Version)
	}
}
`

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// repository makes a Git repository of the nsgo module whose commit carries tag.
func repository(t *testing.T, tag string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module "+module+"\n\ngo 1.24\n")
	write(t, filepath.Join(root, "cmd", "nsgo", "main.go"), stubMain)
	write(t, filepath.Join(root, "LICENSE"), "license\n")
	write(t, filepath.Join(root, "README.md"), "readme\n")
	git(t, root, "init", "-q")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "release")
	git(t, root, "tag", "-a", tag, "-m", tag)
	return root
}

func TestBuildsEveryArchiveAndChecksums(t *testing.T) {
	root := repository(t, "v1.2.3-rc.1")
	out := t.TempDir()
	if err := run(root, []string{"-out", out, "1.2.3-rc.1"}); err != nil {
		t.Fatal(err)
	}
	sums, err := os.ReadFile(filepath.Join(out, checksums))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"nsgo_1.2.3-rc.1_darwin_amd64.tar.gz",
		"nsgo_1.2.3-rc.1_darwin_arm64.tar.gz",
		"nsgo_1.2.3-rc.1_linux_amd64.tar.gz",
		"nsgo_1.2.3-rc.1_linux_arm64.tar.gz",
		"nsgo_1.2.3-rc.1_windows_amd64.zip",
		"nsgo_1.2.3-rc.1_windows_arm64.zip",
	}
	lines := strings.Split(strings.TrimSuffix(string(sums), "\n"), "\n")
	if len(lines) != len(want) {
		t.Fatalf("%s has %d lines, want %d:\n%s", checksums, len(lines), len(want), sums)
	}
	for _, name := range want {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		if line := hex.EncodeToString(sum[:]) + "  " + name; !strings.Contains(string(sums), line+"\n") {
			t.Errorf("%s lacks %q", checksums, line)
		}
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(want)+1 {
		t.Errorf("%s holds %d files, want the archives and %s", out, len(entries), checksums)
	}

	// The same commit builds the same archives.
	again := t.TempDir()
	if err := run(root, []string{"-out", again, "1.2.3-rc.1"}); err != nil {
		t.Fatal(err)
	}
	if second, err := os.ReadFile(filepath.Join(again, checksums)); err != nil || string(second) != string(sums) {
		t.Errorf("a second build gave other checksums:\n%s\nwant\n%s", second, sums)
	}

	// A changed archive no longer matches its checksum.
	tampered := filepath.Join(out, want[0])
	if err := os.WriteFile(tampered, []byte("not an archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verify(out, "1.2.3-rc.1"); err == nil || !strings.Contains(err.Error(), "does not match its checksum") {
		t.Errorf("verify of a changed archive = %v, want a checksum mismatch", err)
	}
}

func TestRefusesAnotherVersionThanTheTag(t *testing.T) {
	root := repository(t, "v1.2.3")
	err := run(root, []string{"-out", t.TempDir(), "1.2.4"})
	if err == nil || !strings.Contains(err.Error(), `has the version "v1.2.3", want v1.2.4`) {
		t.Errorf("run = %v, want a version mismatch", err)
	}
}

func TestRefusesAModifiedCheckout(t *testing.T) {
	root := repository(t, "v1.2.3")
	write(t, filepath.Join(root, "README.md"), "changed\n")
	err := run(root, []string{"-out", t.TempDir(), "1.2.3"})
	if err == nil || !strings.Contains(err.Error(), "+dirty") {
		t.Errorf("run = %v, want a dirty version", err)
	}
}

func TestRefusesAnOutputInsideTheRepository(t *testing.T) {
	root := repository(t, "v1.2.3")
	err := run(root, []string{"-out", filepath.Join(root, "dist"), "1.2.3"})
	if err == nil || !strings.Contains(err.Error(), "inside the repository") {
		t.Errorf("run = %v, want the output refused", err)
	}
}

func TestRefusesAMalformedVersion(t *testing.T) {
	for _, v := range []string{"v1.2.3", "1.2", "1.2.3-", ""} {
		if err := run(t.TempDir(), []string{"-out", t.TempDir(), v}); err == nil {
			t.Errorf("run with %q succeeded", v)
		}
	}
}

func TestReadChecksumsRefusesMalformedLines(t *testing.T) {
	sum := strings.Repeat("ab", sha256.Size)
	for _, data := range []string{
		sum + " one-space.tar.gz\n",
		strings.ToUpper(sum) + "  upper.tar.gz\n",
		sum[:10] + "  short.tar.gz\n",
		sum + "  twice.tar.gz\n" + sum + "  twice.tar.gz\n",
	} {
		path := filepath.Join(t.TempDir(), checksums)
		write(t, path, data)
		if _, err := readChecksums(path); err == nil {
			t.Errorf("readChecksums accepted %q", data)
		}
	}
}

func TestTargetNames(t *testing.T) {
	if got := (target{"windows", "arm64"}).binary(); got != "nsgo.exe" {
		t.Errorf("windows binary = %q", got)
	}
	if got := (target{runtime.GOOS, runtime.GOARCH}).archive("0.4.0"); !strings.HasPrefix(got, "nsgo_0.4.0_"+runtime.GOOS+"_"+runtime.GOARCH+".") {
		t.Errorf("archive = %q", got)
	}
}

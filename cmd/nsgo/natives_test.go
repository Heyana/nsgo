package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func ghosttyProject(t *testing.T, linked bool) *project {
	t.Helper()
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join(cwd, "../.."))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	main := "package main\nfunc main(){}\n"
	if linked {
		main = "package main\nimport _ \"github.com/nsgo-dev/nsgo/base/terminal\"\nfunc main(){}\n"
	}
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module example.com/ghosttyfixture\ngo 1.27.3\nrequire github.com/nsgo-dev/nsgo v0.0.0\nreplace github.com/nsgo-dev/nsgo => " + filepath.ToSlash(root) + "\n",
		"main.go": main,
	})
	return &project{Dir: dir, Name: "GhosttyFixture", ID: "dev.nsgo.test.ghostty1454", Version: "1.0.0", packageLicenses: true}
}

func TestDesktopTerminalLibrariesFollowLinkedPackage(t *testing.T) {
	for _, system := range []string{"linux", "windows"} {
		for _, arch := range []string{"amd64", "arm64"} {
			t.Run(system+"/"+arch, func(t *testing.T) {
				p := ghosttyProject(t, true)
				out := t.TempDir()
				if err := addDesktopLibraries(t.Context(), p, out, system, arch); err != nil {
					t.Fatal(err)
				}
				name := "libghostty-vt.so"
				if system == "windows" {
					name = "ghostty-vt.dll"
				}
				got, err := os.ReadFile(filepath.Join(out, name))
				if err != nil {
					t.Fatal(err)
				}
				want, err := os.ReadFile(filepath.Join("../../base/terminal/internal/library/lib", system+"-"+arch, name))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Error("packaged library differs from its selected target build")
				}
				if err := validateNativeBinary(filepath.Join(out, name), system+"_"+goArchName(arch)); err != nil {
					t.Fatal(err)
				}
				p = ghosttyProject(t, false)
				empty := t.TempDir()
				if err := addDesktopLibraries(t.Context(), p, empty, system, arch); err != nil {
					t.Fatal(err)
				}
				entries, err := os.ReadDir(empty)
				if err != nil || len(entries) != 0 {
					t.Fatalf("an app without the terminal ships native files: %v %v", entries, err)
				}
			})
		}
	}
}

// TestTerminalDesktopPackageLoads builds and runs the host's real package with an empty cache. LibraryPath plus the live emulator prove the sidecar was selected.
func TestTerminalDesktopPackageLoads(t *testing.T) {
	if os.Getenv("NSGO_TERMINAL_PACKAGE_NATIVE") != "1" {
		t.Skip("set NSGO_TERMINAL_PACKAGE_NATIVE=1 on a Linux or Windows host")
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("requires a Linux or Windows host")
	}
	p := ghosttyProject(t, true)
	writeFiles(t, p.Dir, map[string]string{"main.go": `package main
import (
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"
 "github.com/nsgo-dev/nsgo/base/terminal"
)
type connection struct { done chan struct{} }
func (c *connection) Read(p []byte) (int,error) { <-c.done; return 0,io.EOF }
func (c *connection) Write(p []byte) (int,error) { return len(p),nil }
func (c *connection) Close() error { close(c.done); return nil }
func main() {
 path,err:=terminal.LibraryPath(); if err!=nil { panic(err) }
 exe,err:=os.Executable(); if err!=nil { panic(err) }
 if filepath.Dir(path)!=filepath.Dir(exe) { panic("library came from outside the package: "+path) }
 term,err:=terminal.New(terminal.Options{Conn:&connection{make(chan struct{})}}); if err!=nil { panic(err) }
 term.Feed([]byte("\x1b[31mPACKAGED-TERMINAL-1454\x1b[0m"))
 if !strings.Contains(term.Text(),"PACKAGED-TERMINAL-1454") { panic("emulator did not consume output") }
 term.Close(); <-term.Done()
 if err:=os.WriteFile(os.Getenv("NSGO_TERMINAL_WITNESS"),[]byte(fmt.Sprintf("library=%s\ntext=PACKAGED-TERMINAL-1454\n",path)),0600); err!=nil { panic(err) }
}
`})
	out := t.TempDir()
	var err error
	executable := filepath.Join(out, p.Name)
	if runtime.GOOS == "windows" {
		err = buildWindows(t.Context(), p, out, runtime.GOARCH)
		executable += ".exe"
	} else {
		err = buildLinux(t.Context(), p, out, runtime.GOARCH)
	}
	if err != nil {
		t.Fatal(err)
	}
	cache, witness := t.TempDir(), filepath.Join(t.TempDir(), "witness.txt")
	cmd := exec.CommandContext(t.Context(), executable)
	cmd.Env = append(os.Environ(), "NSGO_GHOSTTY_VT=", "XDG_CACHE_HOME="+cache, "LOCALAPPDATA="+cache, "NSGO_TERMINAL_WITNESS="+witness)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("package: %v\n%s", err, output)
	}
	data, err := os.ReadFile(witness)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(data))
	if _, err := os.Stat(filepath.Join(cache, "nsgo", "libghostty-vt")); !os.IsNotExist(err) {
		t.Fatalf("package extracted a fallback library: %v", err)
	}
}

func TestTerminalCompanionXCFramework(t *testing.T) {
	if os.Getenv("NSGO_TERMINAL_PACKAGE_NATIVE") != "1" || runtime.GOOS != "darwin" {
		t.Skip("set NSGO_TERMINAL_PACKAGE_NATIVE=1 on a Mac with Xcode")
	}
	out := t.TempDir()
	if err := addIOSEmbeddedLibraries(t.Context(), ghosttyProject(t, true), out, t.TempDir(), "15.0"); err != nil {
		t.Fatal(err)
	}
	framework := filepath.Join(out, "GhosttyVT.xcframework")
	if entries, err := os.ReadDir(framework); err == nil {
		for _, entry := range entries {
			t.Log(entry.Name())
		}
	}
	for _, platform := range []string{"ios-arm64", "ios-arm64_x86_64-simulator", "ios-arm64_x86_64-maccatalyst"} {
		fw := filepath.Join(framework, platform, ghosttyFramework)
		binary := filepath.Join(fw, "GhosttyVT")
		if info, err := os.Stat(binary); err != nil || info.Size() == 0 {
			t.Fatalf("missing %s: %v", platform, err)
		}
		if output, err := exec.CommandContext(t.Context(), "codesign", "--verify", "--strict", fw).CombinedOutput(); err != nil {
			t.Fatalf("signature %s: %v %s", platform, err, output)
		}
		if _, err := os.Stat(filepath.Join(fw, "Headers", "NSGo.h")); !os.IsNotExist(err) {
			t.Fatalf("runtime framework exposes app headers: %v", err)
		}
		output, err := exec.CommandContext(t.Context(), "lipo", "-archs", binary).CombinedOutput()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %s", platform, output)
	}
	if err := addIOSEmbeddedLibraries(t.Context(), ghosttyProject(t, false), out, t.TempDir(), "15.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(framework); !os.IsNotExist(err) {
		t.Fatalf("stale terminal companion remains for a build without terminal: %v", err)
	}
}

func TestTerminalTVOSCompanionXCFramework(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("requires macOS with Xcode")
	}
	out := t.TempDir()
	if err := addAppleEmbeddedLibraries(t.Context(), ghosttyProject(t, true), tvOS, out, t.TempDir(), "17.0"); err != nil {
		t.Fatal(err)
	}
	framework := filepath.Join(out, "GhosttyVT.xcframework")
	for _, slice := range []struct {
		dir      string
		platform string
		archs    string
	}{
		{"tvos-arm64", "AppleTVOS", "arm64"},
		{"tvos-arm64_x86_64-simulator", "AppleTVSimulator", "x86_64 arm64"},
	} {
		fw := filepath.Join(framework, slice.dir, ghosttyFramework)
		binary := filepath.Join(fw, "GhosttyVT")
		if info, err := os.Stat(binary); err != nil || info.Size() == 0 {
			t.Fatalf("missing %s: %v", slice.dir, err)
		}
		if out, err := exec.CommandContext(t.Context(), "codesign", "--verify", "--strict", fw).CombinedOutput(); err != nil {
			t.Fatalf("signature %s: %v %s", slice.dir, err, out)
		}
		if _, err := os.Stat(filepath.Join(fw, "Headers", "NSGo.h")); !os.IsNotExist(err) {
			t.Fatalf("runtime framework exposes app headers: %v", err)
		}
		gotArchs, err := output(t.Context(), "lipo", "-archs", binary)
		if err != nil {
			t.Fatal(err)
		}
		if gotArchs != slice.archs {
			t.Errorf("%s archs = %q, want %q", slice.dir, gotArchs, slice.archs)
		}
		plistBytes, err := os.ReadFile(filepath.Join(fw, "Info.plist"))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodePlist(plistBytes)
		if err != nil {
			t.Fatal(err)
		}
		plist := decoded.(map[string]any)
		if plist["MinimumOSVersion"] != "17.0" {
			t.Errorf("%s MinimumOSVersion = %v, want 17.0", slice.dir, plist["MinimumOSVersion"])
		}
		platforms, _ := plist["CFBundleSupportedPlatforms"].([]any)
		if len(platforms) != 1 || platforms[0] != slice.platform {
			t.Errorf("%s CFBundleSupportedPlatforms = %v, want [%s]", slice.dir, platforms, slice.platform)
		}
	}
	if err := addAppleEmbeddedLibraries(t.Context(), ghosttyProject(t, false), tvOS, out, t.TempDir(), "17.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(framework); !os.IsNotExist(err) {
		t.Fatalf("stale tvOS terminal companion remains for a build without terminal: %v", err)
	}
}

func TestTerminalTVOSAppFramework(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("requires macOS with Xcode")
	}
	for _, simulator := range []bool{false, true} {
		name := "device"
		wantPlatform := "AppleTVOS"
		if simulator {
			name = "simulator"
			wantPlatform = "AppleTVSimulator"
		}
		t.Run(name, func(t *testing.T) {
			p := ghosttyProject(t, true)
			app := filepath.Join(t.TempDir(), p.Name+".app")
			if err := os.MkdirAll(app, 0o755); err != nil {
				t.Fatal(err)
			}
			env := []string{"GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1"}
			if err := addIOSFrameworks(t.Context(), p, tvOS, app, env, simulator); err != nil {
				t.Fatal(err)
			}
			fw := filepath.Join(app, "Frameworks", ghosttyFramework)
			binary := filepath.Join(fw, "GhosttyVT")
			if err := checkFramework(binary, tvOS, simulator, ghosttyFramework, "17.0", []string{ghosttyFramework}); err != nil {
				t.Fatalf("tvOS GhosttyVT.framework check failed: %v", err)
			}
			plistBytes, err := os.ReadFile(filepath.Join(fw, "Info.plist"))
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := decodePlist(plistBytes)
			if err != nil {
				t.Fatal(err)
			}
			plist := decoded.(map[string]any)
			if plist["MinimumOSVersion"] != "17.0" {
				t.Errorf("MinimumOSVersion = %v, want 17.0", plist["MinimumOSVersion"])
			}
			platforms, _ := plist["CFBundleSupportedPlatforms"].([]any)
			if len(platforms) != 1 || platforms[0] != wantPlatform {
				t.Errorf("CFBundleSupportedPlatforms = %v, want [%s]", platforms, wantPlatform)
			}
		})
	}
}

func TestTerminalAAR(t *testing.T) {
	if os.Getenv("NSGO_TERMINAL_PACKAGE_NATIVE") != "1" {
		t.Skip("set NSGO_TERMINAL_PACKAGE_NATIVE=1 with the Android prebuild available")
	}
	for _, linked := range []bool{true, false} {
		p := ghosttyProject(t, linked)
		out := t.TempDir()
		if err := buildAndroidAAR(t.Context(), p, out, "arm64"); err != nil {
			t.Fatal(err)
		}
		archive, err := zip.OpenReader(filepath.Join(out, p.Name+".aar"))
		if err != nil {
			t.Fatal(err)
		}
		defer archive.Close()
		found := false
		for _, file := range archive.File {
			if file.Name != "jni/arm64-v8a/libghostty-vt.so" {
				continue
			}
			found = true
			reader, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(reader)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile("../../base/terminal/internal/library/lib/android-arm64/libghostty-vt.so")
			if err != nil || !bytes.Equal(data, want) {
				t.Fatalf("AAR library differs from the selected prebuild: %v", err)
			}
		}
		if found != linked {
			t.Fatalf("terminal linked=%v, AAR library present=%v", linked, found)
		}
	}
}

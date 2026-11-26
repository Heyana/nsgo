package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/licenseclassifier/v2/assets"
	"github.com/nsgo-dev/nsgo/cmd/nsgo/authenticode"
)

func nativeELF(arm bool) []byte {
	data := make([]byte, 64)
	copy(data, "\x7fELF\x02\x01\x01")
	binary.LittleEndian.PutUint16(data[16:], uint16(elf.ET_DYN))
	machine := elf.EM_X86_64
	if arm {
		machine = elf.EM_AARCH64
	}
	binary.LittleEndian.PutUint16(data[18:], uint16(machine))
	binary.LittleEndian.PutUint32(data[20:], 1)
	binary.LittleEndian.PutUint16(data[52:], 64)
	binary.LittleEndian.PutUint16(data[54:], 56)
	binary.LittleEndian.PutUint16(data[58:], 64)
	return data
}

func nativeMachO(arm bool) []byte {
	data := make([]byte, 32)
	binary.LittleEndian.PutUint32(data, macho.Magic64)
	cpu, subtype := macho.CpuAmd64, uint32(3)
	if arm {
		cpu, subtype = macho.CpuArm64, 0
	}
	binary.LittleEndian.PutUint32(data[4:], uint32(cpu))
	binary.LittleEndian.PutUint32(data[8:], subtype)
	binary.LittleEndian.PutUint32(data[12:], uint32(macho.TypeDylib))
	return data
}

func nativeDigest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func nativeFixture(t *testing.T, target, name string, data []byte) *project {
	t.Helper()
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	dir := t.TempDir()
	app := filepath.Join(dir, "app")
	license, err := assets.ReadLicenseFile("License/MIT/pristine.txt")
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{
		"app/go.mod":                "module example.com/app\ngo 1.27.3\nrequire example.com/native v0.0.0\nreplace example.com/native => ../dep\n",
		"app/main.go":               "package main\nimport _ \"example.com/native\"\nfunc main() {}\n",
		"dep/go.mod":                "module example.com/native\ngo 1.27.3\n",
		"dep/native.go":             "package native\n",
		"dep/LICENSE":               string(license),
		"dep/legal/native.txt":      string(license),
		"dep/lib/" + name:           string(data),
		"dep/" + nativeManifestName: fmt.Sprintf("version: 1\nlibraries:\n  %s:\n    - name: %s\n      path: lib/%s\n      sha256: %s\n      licenses: [legal/native.txt]\n", target, name, name, nativeDigest(data)),
	})
	return &project{Dir: app, Name: "Fixture", ID: "dev.example.fixture", Version: "1.0.0", packageLicenses: true}
}

func TestPackageNativesFollowBuildTagsAndLicenses(t *testing.T) {
	p := nativeFixture(t, "linux_x64", "libfixture.so.1", nativeELF(false))
	main := filepath.Join(p.Dir, "main.go")
	os.WriteFile(main, []byte("package main\nfunc main() {}\n"), 0o644)
	os.WriteFile(filepath.Join(p.Dir, "native.go"), []byte("//go:build nativeextra\n\npackage main\nimport _ \"example.com/native\"\n"), 0o644)
	env := []string{"GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0"}
	if _, err := p.licenseNoticesFile(t.Context(), "linux_x64", env, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if len(p.nativeLibraries["linux_x64"]) != 0 {
		t.Fatal("excluded package shipped native libraries")
	}
	if _, err := p.licenseNoticesFile(t.Context(), "linux_x64", env, []string{"nativeextra"}, nil, ""); err != nil {
		t.Fatal(err)
	}
	if len(p.nativeLibraries["linux_x64"]) != 1 || !strings.Contains(p.thirdPartyNotices, "native library libfixture.so.1") || !strings.Contains(p.thirdPartyNotices, "legal/native.txt") || strings.Contains(p.thirdPartyNotices, filepath.Dir(p.Dir)) {
		t.Fatalf("selected native declarations/notices: %+v", p.nativeLibraries)
	}
	license, err := assets.ReadLicenseFile("License/MPL-2.0/pristine.txt")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(filepath.Dir(p.Dir), "dep/legal/native.txt"), license, 0o644)
	p.Licenses.Copyleft = "error"
	if _, err := p.licenseNoticesFile(t.Context(), "linux_x64", env, []string{"nativeextra"}, nil, ""); err == nil || !strings.Contains(err.Error(), "copyleft") {
		t.Fatalf("native license bypassed policy: %v", err)
	}
	p.Licenses.Allow = []string{"example.com/native"}
	if _, err := p.licenseNoticesFile(t.Context(), "linux_x64", env, []string{"nativeextra"}, nil, ""); err != nil {
		t.Fatal(err)
	}
}

func TestPackageNativesLinuxInstallerInputs(t *testing.T) {
	data := nativeELF(false)
	p := nativeFixture(t, "linux_x64", "libfixture.so.1", data)
	out := t.TempDir()
	if err := buildLinux(t.Context(), p, out, "amd64"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(out, "libfixture.so.1"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("packaged library: %v", err)
	}
	notices, err := os.ReadFile(filepath.Join(out, noticesName))
	if err != nil || !bytes.Contains(notices, []byte("native library libfixture.so.1")) {
		t.Fatalf("packaged notices: %v", err)
	}
	files, err := linuxPackageFiles(p, out)
	if err != nil {
		t.Fatal(err)
	}
	library, license := false, false
	for _, file := range files {
		if file.name == "opt/fixture/libfixture.so.1" {
			library = bytes.Equal(file.data, data) && file.mode == 0o755
		}
		if file.name == "opt/fixture/"+noticesName {
			license = bytes.Equal(file.data, notices)
		}
	}
	if !library || !license {
		t.Fatal("Linux installer inputs lost the library or its license")
	}
	if _, err := debPackage(p, files, "amd64"); err != nil {
		t.Fatal(err)
	}
}

func TestPackageNativesCacheAndCorruption(t *testing.T) {
	data := nativeELF(false)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.Write(data) }))
	defer server.Close()
	library := nativeLibrary{Name: "fixture.so", URL: server.URL, SHA256: nativeDigest(data)}
	cache := t.TempDir()
	var group sync.WaitGroup
	for range 6 {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := library.fetch(t.Context(), cache); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	if requests.Load() != 1 {
		t.Fatalf("downloaded %d times", requests.Load())
	}
	path := filepath.Join(cache, library.SHA256)
	os.WriteFile(path, []byte("corrupt cache"), 0o600)
	if _, err := library.fetch(t.Context(), cache); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatal("corrupt cached library was reused")
	}
	server.Close()
	if _, err := library.fetch(t.Context(), cache); err != nil {
		t.Fatalf("valid cache needed the network: %v", err)
	}
}

func TestPackageNativesRejectBadDownloads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("wrong")) }))
	defer server.Close()
	cache := t.TempDir()
	library := nativeLibrary{Name: "fixture.so", URL: server.URL, SHA256: strings.Repeat("0", 64)}
	if _, err := library.fetch(t.Context(), cache); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("bad download: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cache, library.SHA256)); !os.IsNotExist(err) {
		t.Fatal("bad download entered the cache")
	}
	if files, _ := filepath.Glob(filepath.Join(cache, ".native-*")); len(files) != 0 {
		t.Fatal("failed download left partial files")
	}
}

func TestPackageNativesRejectInvalidManifests(t *testing.T) {
	good := "version: 1\nlibraries:\n  linux_x64:\n    - name: libfixture.so\n      path: lib.so\n      sha256: " + strings.Repeat("a", 64) + "\n      licenses: [LICENSE]\n"
	cases := map[string]string{
		"unknown field":  good + "unexpected: true\n",
		"two documents":  good + "---\nversion: 1\n",
		"unknown target": strings.ReplaceAll(good, "linux_x64", "linux_mips"),
		"path escape":    strings.ReplaceAll(good, "path: lib.so", "path: ../lib.so"),
		"license escape": strings.ReplaceAll(good, "[LICENSE]", "[../LICENSE]"),
		"no checksum":    strings.ReplaceAll(good, strings.Repeat("a", 64), "bad"),
		"two sources":    strings.ReplaceAll(good, "path: lib.so", "path: lib.so\n      url: https://example.com/lib.so"),
		"wrong name":     strings.ReplaceAll(good, "name: libfixture.so", "name: ../libfixture.so"),
		"no license":     strings.ReplaceAll(good, "[LICENSE]", "[]"),
	}
	for name, manifest := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, nativeManifestName), []byte(manifest), 0o644)
			if _, err := readNativeManifest(dir); err == nil {
				t.Fatal("accepted invalid manifest")
			}
		})
	}
}

func TestPackageNativesArchitectureAndCollisions(t *testing.T) {
	for _, test := range []struct {
		target string
		data   []byte
		good   bool
	}{
		{"linux_x64", nativeELF(false), true}, {"linux_arm64", nativeELF(false), false},
		{"macos_arm64", nativeMachO(true), true}, {"macos_x64", nativeMachO(true), false},
		{"windows_x64", nativeELF(false), false},
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, "fixture")
		os.WriteFile(path, test.data, 0o644)
		if err := validateNativeBinary(path, test.target); (err == nil) != test.good {
			t.Fatalf("%s: %v", test.target, err)
		}
	}
	p := nativeFixture(t, "linux_x64", "libfixture.so", nativeELF(false))
	if _, err := p.licenseNoticesFile(t.Context(), "linux_x64", []string{"GOOS=linux", "GOARCH=amd64"}, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	os.WriteFile(filepath.Join(out, "libfixture.so"), []byte("resource"), 0o644)
	if err := p.addNativeLibraries(t.Context(), "linux_x64", out); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Fatalf("resource collision: %v", err)
	}
}

func TestPackageNativesMissingUniversalSlice(t *testing.T) {
	arm, intel := t.TempDir(), t.TempDir()
	if err := matchMacLibraries(arm, intel); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, arm, map[string]string{"Contents/Frameworks/libfixture.dylib": "arm"})
	if err := matchMacLibraries(arm, intel); err == nil {
		t.Fatal("accepted missing Intel library")
	}
	writeFiles(t, intel, map[string]string{"Contents/Frameworks/libfixture.dylib": "intel"})
	if err := matchMacLibraries(arm, intel); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, intel, map[string]string{"Contents/Frameworks/extra.dylib": "intel"})
	if err := matchMacLibraries(arm, intel); err == nil {
		t.Fatal("dropped Intel-only library")
	}
}

func TestPackageNativesSignBeforeMacBundle(t *testing.T) {
	p := nativeFixture(t, "macos_arm64", "libfixture.dylib", nativeMachO(true))
	app := filepath.Join(t.TempDir(), "Fixture.app")
	if _, err := p.licenseNoticesFile(t.Context(), "macos_arm64", []string{"GOOS=darwin", "GOARCH=arm64"}, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
	if err := p.addNativeLibraries(t.Context(), "macos_arm64", filepath.Join(app, "Contents/Frameworks")); err != nil {
		t.Fatal(err)
	}
	p.Sign.Identity = "fixture identity"
	var order []string
	ctx := withProcessRunner(t, func(_ context.Context, command processCommand) ([]byte, error) {
		if command.name != "codesign" {
			t.Fatalf("unexpected %s", command.name)
		}
		order = append(order, filepath.Base(command.args[len(command.args)-1]))
		return nil, nil
	})
	if err := signMacApp(ctx, p, app); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "libfixture.dylib,Fixture.app" {
		t.Fatalf("signing order %v", order)
	}
}

func TestPackageNativesWindowsSigningAndInstallerInputs(t *testing.T) {
	image := buildTestPE(t)
	header := int(binary.LittleEndian.Uint32(image[0x3c:])) + 4
	flags := binary.LittleEndian.Uint16(image[header+18:])
	binary.LittleEndian.PutUint16(image[header+18:], flags|0x2000)
	p := nativeFixture(t, "windows_x64", "fixture.dll", image)
	p.Release = true
	installers := false
	p.Build.Installers = &installers
	key := testWindowsKey(t)
	certificate := filepath.Join(t.TempDir(), "certificate.p12")
	writeTestCertificate(t, certificate, key)
	p.Sign.Certificate, p.Sign.TimestampURL = certificate, "none"
	out := t.TempDir()
	if err := buildTarget(t.Context(), p, "windows_x64", out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Fixture.exe", "fixture.dll"} {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil || !authenticode.Signed(data) {
			t.Fatalf("%s was not signed before packaging: %v", name, err)
		}
	}
	p.Sign.Certificate = ""
	d, err := windowsInstallerPlan(t.Context(), p, out, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(d.Files, "fixture.dll") || !slices.Contains(d.Files, noticesName) {
		t.Fatalf("Windows installer inputs: %v", d.Files)
	}
}

func TestPackageNativesOverlayAndDevDiscovery(t *testing.T) {
	p := nativeFixture(t, "linux_x64", "libfixture.so", nativeELF(false))
	writeFiles(t, p.Dir, map[string]string{"main.go": "package main\nfunc main() {}\n"})
	path := filepath.Join(t.TempDir(), "native.go")
	os.WriteFile(path, []byte("package main\nimport _ \"example.com/native\"\n"), 0o644)
	p.packageLicenses = false
	if _, err := p.buildOverlay(t.Context(), "linux_x64", []string{"GOOS=linux", "GOARCH=amd64"}, nil, nil, map[string]string{"zz_native.go": path}); err != nil {
		t.Fatal(err)
	}
	if len(p.nativeLibraries["linux_x64"]) != 1 {
		t.Fatal("dev/overlay selection lost its native library")
	}
	if p.thirdPartyNotices != "" {
		t.Fatal("development added release notices")
	}
}

func TestPackageNativesVendoredAssets(t *testing.T) {
	p := nativeFixture(t, "linux_x64", "libfixture.so", nativeELF(false))
	dep := filepath.Join(filepath.Dir(p.Dir), "dep")
	writeFiles(t, dep, map[string]string{"native.go": "package native\nimport \"embed\"\n//go:embed nsgo-natives.yaml legal/native.txt lib/*\nvar _ embed.FS\n"})
	command := exec.CommandContext(t.Context(), "go", "mod", "vendor")
	command.Dir = p.Dir
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("vendor: %v\n%s", err, out)
	}
	if err := os.Rename(dep, dep+"-unused"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.licenseNoticesFile(t.Context(), "linux_x64", []string{"GOOS=linux", "GOARCH=amd64"}, nil, []string{"-mod=vendor"}, ""); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := p.addNativeLibraries(t.Context(), "linux_x64", out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "libfixture.so")); err != nil {
		t.Fatal(err)
	}
}

func TestPackageNativesRequireExplicitArchitecture(t *testing.T) {
	p := nativeFixture(t, "linux_x64", "libfixture.so", nativeELF(false))
	if _, err := p.licenseNoticesFile(t.Context(), "linux_arm64", []string{"GOOS=linux", "GOARCH=arm64"}, nil, nil, ""); err == nil || !strings.Contains(err.Error(), "no linux_arm64 entry") {
		t.Fatalf("missing target: %v", err)
	}
	dep := filepath.Join(filepath.Dir(p.Dir), "dep", nativeManifestName)
	data, _ := os.ReadFile(dep)
	os.WriteFile(dep, append(data, []byte("  linux_arm64: []\n")...), 0o644)
	if _, err := p.licenseNoticesFile(t.Context(), "linux_arm64", []string{"GOOS=linux", "GOARCH=arm64"}, nil, nil, ""); err != nil {
		t.Fatal(err)
	}
}

func TestPackageNativesRejectEscapingSymlinks(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	writeFiles(t, outside, map[string]string{"private.txt": "outside"})
	if err := os.Symlink(filepath.Join(outside, "private.txt"), filepath.Join(dir, "LICENSE")); err != nil {
		t.Skip(err)
	}
	if _, err := packageNativePath(dir, "LICENSE"); err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("escaped license: %v", err)
	}
}

// Opt in on macOS to exercise real Go binaries, lipo, codesign and the DMG. The native inputs belong to NSGo; no external download or release is involved.
func TestPackageNativesMacUniversalE2E(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("NSGO_NATIVE_LIBRARY_E2E") != "1" {
		t.Skip("set NSGO_NATIVE_LIBRARY_E2E=1 on macOS")
	}
	for _, key := range []string{"NSGO_NOTARY_KEY", "NSGO_NOTARY_KEY_ID", "NSGO_NOTARY_ISSUER"} {
		t.Setenv(key, "")
	}
	arm, err := os.ReadFile(filepath.Join("..", "..", "base", "terminal", "internal", "library", "lib", "darwin-arm64", "libghostty-vt.dylib"))
	if err != nil {
		t.Fatal(err)
	}
	intel, err := os.ReadFile(filepath.Join("..", "..", "base", "terminal", "internal", "library", "lib", "darwin-amd64", "libghostty-vt.dylib"))
	if err != nil {
		t.Fatal(err)
	}
	p := nativeFixture(t, "macos_arm64", "libfixture.dylib", arm)
	dep := filepath.Join(filepath.Dir(p.Dir), "dep")
	writeFiles(t, dep, map[string]string{
		"lib/intel.dylib":  string(intel),
		nativeManifestName: fmt.Sprintf("version: 1\nlibraries:\n  macos_arm64:\n    - name: libfixture.dylib\n      path: lib/libfixture.dylib\n      sha256: %s\n      licenses: [legal/native.txt]\n  macos_x64:\n    - name: libfixture.dylib\n      path: lib/intel.dylib\n      sha256: %s\n      licenses: [legal/native.txt]\n", nativeDigest(arm), nativeDigest(intel)),
	})
	p.Release = true
	p.Build.Output = filepath.Join(t.TempDir(), "build")
	out := t.TempDir()
	if err := buildMacUniversal(t.Context(), p, out); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(out, "Fixture.app")
	library := filepath.Join(app, "Contents", "Frameworks", "libfixture.dylib")
	fat, err := macho.OpenFat(library)
	if err != nil {
		t.Fatalf("universal library: %v", err)
	}
	defer fat.Close()
	if len(fat.Arches) != 2 {
		t.Fatal("library lost an architecture")
	}
	command := exec.CommandContext(t.Context(), "codesign", "--verify", "--deep", "--strict", app)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("bundle verification: %v\n%s", err, output)
	}
	images, err := filepath.Glob(filepath.Join(p.releaseDir(), "*.dmg"))
	if err != nil || len(images) != 1 {
		t.Fatalf("installer missing: %v,%v", images, err)
	}
	mount := t.TempDir()
	command = exec.CommandContext(t.Context(), "hdiutil", "attach", "-readonly", "-nobrowse", "-mountpoint", mount, images[0])
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("mount installer: %v\n%s", err, output)
	}
	defer func() {
		if output, err := exec.Command("hdiutil", "detach", "-force", mount).CombinedOutput(); err != nil {
			t.Errorf("detach installer: %v\n%s", err, output)
		}
	}()
	if _, err := os.Stat(filepath.Join(mount, "Fixture.app", "Contents", "Frameworks", "libfixture.dylib")); err != nil {
		t.Fatal(err)
	}
	if notices, err := os.ReadFile(filepath.Join(mount, "Fixture.app", "Contents", "Resources", noticesName)); err != nil || !bytes.Contains(notices, []byte("native library libfixture.dylib")) {
		t.Fatalf("installer notices: %v", err)
	}
	// A universal publisher asset can also serve a single-architecture package.
	universal, err := os.ReadFile(library)
	if err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dep, map[string]string{"lib/universal.dylib": string(universal)})
	native := nativeLibrary{Name: "libthin.dylib", Path: "lib/universal.dylib", SHA256: nativeDigest(universal), packageDir: dep, owner: "example.com/native"}
	p.nativeLibraries = map[string][]nativeLibrary{"macos_arm64": {native, native}}
	thinDir := t.TempDir()
	if err := p.addNativeLibraries(t.Context(), "macos_arm64", thinDir); err != nil {
		t.Fatal(err)
	}
	thin, err := macho.Open(filepath.Join(thinDir, "libthin.dylib"))
	if err != nil {
		t.Fatalf("thin publisher asset: %v", err)
	}
	defer thin.Close()
	if thin.Cpu != macho.CpuArm64 {
		t.Fatal("thinned the wrong architecture")
	}

}

func TestPackageNativesCanceledCacheWait(t *testing.T) {
	cache := t.TempDir()
	library := nativeLibrary{Name: "fixture.so", URL: "http://127.0.0.1:1", SHA256: strings.Repeat("0", 64)}
	lock, err := os.OpenFile(filepath.Join(cache, library.SHA256+".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if held, err := tryLockFile(lock); err != nil || !held {
		t.Fatalf("lock: %t,%v", held, err)
	}
	defer unlockFile(lock)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := library.fetch(ctx, cache); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("canceled lock wait: %v", err)
	}
}

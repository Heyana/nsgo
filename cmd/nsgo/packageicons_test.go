package main

import (
	"archive/zip"
	"bytes"
	"debug/pe"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageIconsAreIndependent(t *testing.T) {
	dir := t.TempDir()
	palette := map[string]color.NRGBA{"shared": {R: 255, A: 255}, "windows": {B: 255, A: 255}, "linux": {G: 255, A: 255}, "android": {R: 255, G: 128, A: 255}}
	for name, tint := range palette {
		img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
		draw.Draw(img, img.Bounds(), image.NewUniform(tint), image.Point{}, draw.Src)
		var data bytes.Buffer
		if err := png.Encode(&data, img); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".png"), data.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFiles(t, dir, map[string]string{"nsgo.yaml": "name: Icons\nid: dev.nsgo.icons\nicon:\n  png: shared.png\n  windows: windows.png\n  linux: linux.png\n  android:\n    png: android.png\nandroid:\n  uses_cleartext_traffic: true\n"})
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	check := func(name string, img image.Image, err error) {
		t.Helper()
		if err != nil || img == nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA); got != palette[name] {
			t.Fatalf("%s pixel = %v", name, got)
		}
	}
	img, err := p.iconImage()
	check("shared", img, err)
	img, err = p.androidIconImage()
	check("android", img, err)
	if adaptive, _, err := p.androidAdaptiveIcon(); err != nil || adaptive != nil {
		t.Fatalf("bitmap-only Android icon created adaptive layers: %v", err)
	}
	// Read the actual Windows COFF icon pixels the linker consumes.
	syso, cleanup, err := writeWindowsResources(t.Context(), p, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	coff, err := pe.NewFile(bytes.NewReader(syso))
	if err != nil {
		t.Fatal(err)
	}
	defer coff.Close()
	section := coff.Section(".rsrc")
	data, err := section.Data()
	if err != nil {
		t.Fatal(err)
	}
	offset := binary.LittleEndian.Uint32(data[section.Relocs[0].VirtualAddress:])
	img, err = png.Decode(bytes.NewReader(data[offset:]))
	check("windows", img, err)
	// Linux's unpacked app and package icon both use the Linux art.
	out := t.TempDir()
	if err := writeLinuxIcon(p, out); err != nil {
		t.Fatal(err)
	}
	img, err = readSquareIcon(filepath.Join(out, p.ID+".png"))
	check("linux", img, err)
	entries, err := linuxPackageFiles(p, out)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.name == "usr/share/icons/hicolor/256x256/apps/"+p.ID+".png" {
			img, err = png.Decode(bytes.NewReader(entry.data))
			check("linux", img, err)
			found = true
		}
	}
	if !found {
		t.Fatal("Linux package has no icon")
	}
	// Windows setup's default icon follows the Windows override as well.
	ico, err := p.iconFile("", ".ico")
	if err != nil {
		t.Fatal(err)
	}
	offset = binary.LittleEndian.Uint32(ico[18:22])
	img, err = png.Decode(bytes.NewReader(ico[offset:]))
	check("windows", img, err)
	// Build the real standalone Android APK and inspect its bitmap resource.
	writeFiles(t, dir, map[string]string{"go.mod": "module example.com/packageicons\n\ngo 1.27.3\n", "main.go": "package main\nfunc main(){}\n"})
	apkPath, err := buildAndroidAPK(t.Context(), p, t.TempDir(), "arm64")
	if err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(apkPath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	policy, err := archive.Open("res/xml/network_security_config.xml")
	if err != nil {
		t.Fatalf("YAML cleartext opt-in did not reach APK: %v", err)
	}
	policy.Close()
	bitmap, err := archive.Open("res/mipmap-xxxhdpi-v4/ic_launcher.png")
	if err != nil {
		t.Fatal(err)
	}
	img, err = png.Decode(bitmap)
	bitmap.Close()
	check("android", img, err)
	storeDir := t.TempDir()
	writeFiles(t, storeDir, map[string]string{p.Name + ".exe": "fixture"})
	p.Windows.Store.Identity = "Dev.PackageIcons"
	p.Windows.Store.Publisher = "CN=Example"
	msixPath, err := buildMSIX(t.Context(), p, storeDir, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	store, err := zip.OpenReader(msixPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	logo, err := store.Open("Assets/StoreLogo.png")
	if err != nil {
		t.Fatal(err)
	}
	img, err = png.Decode(logo)
	logo.Close()
	check("windows", img, err)
	p.Dev.Icon.Windows = filepath.Join(dir, "android.png")
	p.useDevIcons()
	img, err = p.windowsIconImage()
	check("android", img, err)
	img, err = p.linuxIconImage()
	check("linux", img, err)
}

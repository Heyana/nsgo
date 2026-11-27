package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testIcon(size int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), 0x80, 0xff})
		}
	}
	return img
}

func TestScaledPNG(t *testing.T) {
	for _, size := range []int{16, 256, 1024} {
		img, err := png.Decode(bytes.NewReader(scaledPNG(testIcon(512), size)))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
			t.Errorf("scaled to %d: got %v", size, b)
		}
	}
}

func TestICNS(t *testing.T) {
	data := icns(testIcon(64))
	if string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:])) != len(data) {
		t.Fatalf("bad header % x", data[:8])
	}
	var kinds []string
	for at := 8; at < len(data); {
		size := int(binary.BigEndian.Uint32(data[at+4:]))
		if !bytes.HasPrefix(data[at+8:], []byte("\x89PNG")) {
			t.Errorf("%s is not a PNG", data[at:at+4])
		}
		kinds = append(kinds, string(data[at:at+4]))
		at += size
	}
	if len(kinds) != len(icnsTypes) {
		t.Errorf("got entries %v", kinds)
	}
}

func TestIconResource(t *testing.T) {
	images := [][]byte{scaledPNG(testIcon(64), 16), scaledPNG(testIcon(64), 32)}
	for _, arch := range []string{"amd64", "arm64"} {
		syso, err := windowsResource(arch, images, []int{16, 32}, nil)
		if err != nil {
			t.Fatal(err)
		}
		f, err := pe.NewFile(bytes.NewReader(syso))
		if err != nil {
			t.Fatal(err)
		}
		sec := f.Section(".rsrc")
		if sec == nil {
			t.Fatalf("%s: no .rsrc section", arch)
		}
		// Two icons and the group, each with a data entry to relocate.
		if len(sec.Relocs) != 3 {
			t.Errorf("%s: %d relocations, want 3", arch, len(sec.Relocs))
		}
		data, err := sec.Data()
		if err != nil {
			t.Fatal(err)
		}
		// The first data entry points at the first image.
		at := binary.LittleEndian.Uint32(data[sec.Relocs[0].VirtualAddress:])
		if !bytes.HasPrefix(data[at:], images[0]) {
			t.Errorf("%s: the first entry does not point at the first image", arch)
		}
	}
	if _, err := windowsResource("riscv64", images, []int{16, 32}, nil); err == nil {
		t.Error("riscv64 made a resource")
	}
}

func TestTVBrandAssets(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles an asset catalog")
	}
	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("needs Xcode's actool")
	}
	path := filepath.Join(t.TempDir(), "Assets.xcassets")
	if err := writeTVBrandAssets(filepath.Join(path, "AppIcon.brandassets"), testIcon(256), false); err != nil {
		t.Fatal(err)
	}
	front, err := os.ReadFile(filepath.Join(path, "AppIcon.brandassets", "App Icon.imagestack", "Front.imagestacklayer", "Content.imageset", "front@2x.png"))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(front))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 480 {
		t.Errorf("the icon's front layer at 2x is %v, want 800x480", b)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Error("the front layer covers its corner; the back layer would not show")
	}
	plist, err := compileIcon(t.Context(), path, t.TempDir(), "appletvsimulator", "17.0", []string{"tv"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plist, "CFBundleIcons") {
		t.Errorf("actool named no icon: %q", plist)
	}
}

// wideIcon is a 5:3 icon, the shape an Apple TV's brand assets fill.
func wideIcon(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), 0x80, 0xff})
		}
	}
	return img
}

func TestTVBrandAssetsWide(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "AppIcon.brandassets")
	if err := writeTVBrandAssets(dir, wideIcon(1280, 768), true); err != nil {
		t.Fatal(err)
	}
	front, err := os.ReadFile(filepath.Join(dir, "App Icon.imagestack", "Front.imagestacklayer", "Content.imageset", "front@2x.png"))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(front))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 480 {
		t.Errorf("the wide icon's front layer at 2x is %v, want 800x480", b)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a == 0 {
		t.Error("the wide icon's front layer leaves its corner to the back layer")
	}
	// A 5:3 image fills the card; the top shelf, wider than it, keeps
	// its color at the sides.
	shelf, err := os.ReadFile(filepath.Join(dir, "Top Shelf Image.imageset", "shelf@1x.png"))
	if err != nil {
		t.Fatal(err)
	}
	img, err = png.Decode(bytes.NewReader(shelf))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 1920 || b.Dy() != 720 {
		t.Fatalf("the top shelf image is %v, want 1920x720", b)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0xffff {
		t.Error("the top shelf image's edge is transparent, want the icon's color")
	}
}

func TestCompileBrandAssets(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles an asset catalog")
	}
	if _, err := exec.LookPath("xcrun"); err != nil {
		t.Skip("needs Xcode's actool")
	}
	dir := filepath.Join(t.TempDir(), "MyTV.brandassets")
	if err := writeTVBrandAssets(dir, wideIcon(1280, 768), true); err != nil {
		t.Fatal(err)
	}
	plist, err := compileBrandAssets(t.Context(), dir, t.TempDir(), "appletvsimulator", "17.0", []string{"tv"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plist, "CFBundleIcons") {
		t.Errorf("actool named no icon: %q", plist)
	}
}

func TestReadIconShapes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "TV.png")
	var buf bytes.Buffer
	if err := png.Encode(&buf, wideIcon(400, 240)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := readIcon(path)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 400 || b.Dy() != 240 {
		t.Errorf("readIcon: got %v, want 400x240", b)
	}
	if _, err := readSquareIcon(path); err == nil {
		t.Error("readSquareIcon took a wide icon")
	}
}

func TestAndroidAdaptiveIcon(t *testing.T) {
	dir := t.TempDir()
	write := func(name string) string {
		path := filepath.Join(dir, name)
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, testIcon(64)); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		return path
	}
	p := &project{Config: projectcfg.Config{Icon: icons{Android: &androidIcon{Foreground: write("foreground.png"), Background: "#123456", Monochrome: write("mono.png")}}}}
	icon, legacy, err := p.androidAdaptiveIcon()
	if err != nil {
		t.Fatal(err)
	}
	// The legacy icon is the 72 dp of the 108 a launcher shows: the foreground over the background, 288 of 432 pixels.
	if b := legacy.Bounds(); b.Dx() != 288 || b.Dy() != 288 {
		t.Errorf("legacy icon size = %v", b)
	}
	if len(icon.Foreground) == 0 || len(icon.Background) == 0 || len(icon.Monochrome) == 0 {
		t.Fatalf("adaptive icon layers = %+v", icon)
	}
	img, err := png.Decode(bytes.NewReader(icon.Background))
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Dx(); got != 432 || img.Bounds().Dy() != 432 {
		t.Fatalf("background size = %v", img.Bounds())
	}
}

func TestEdgeColor(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.Set(x, y, color.NRGBA{0x20, 0x40, 0x80, 0xff})
		}
	}
	img.Set(4, 4, color.NRGBA{0xff, 0, 0, 0xff})
	if r, g, b, a := edgeColor(img).RGBA(); r>>8 != 0x20 || g>>8 != 0x40 || b>>8 != 0x80 || a != 0xffff {
		t.Errorf("edge color = %x %x %x %x, want the border's 20 40 80, opaque", r>>8, g>>8, b>>8, a)
	}
}

// The desktop entry beside a Linux executable, which the AppImage and the Flatpak install, names its windows' class by the app's ID, the program name the Linux backend gives them, so docks match a running window that is not pinned to the entry.
func TestLinuxDesktopEntryNamesTheWindowClass(t *testing.T) {
	dir := t.TempDir()
	icon := filepath.Join(dir, "icon.png")
	var b bytes.Buffer
	if err := png.Encode(&b, testIcon(16)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(icon, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	p := &project{Config: projectcfg.Config{Name: "App", ID: "dev.example.app", Icon: icons{PNG: icon}}}
	if err := writeLinuxIcon(p, dir); err != nil {
		t.Fatal(err)
	}
	entry, err := os.ReadFile(filepath.Join(dir, "dev.example.app.desktop"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(entry), "\nIcon=dev.example.app\n") || !strings.Contains(string(entry), "\nStartupWMClass=dev.example.app\n") {
		t.Errorf("desktop entry:\n%s", entry)
	}
}

// linux.categories reach the desktop entry beside the executable, which the AppImage and the Flatpak install, and the one the Debian and Arch packages install, as the freedesktop list desktop-file-validate accepts; without them the entries have no Categories key.
func TestLinuxDesktopEntryCategories(t *testing.T) {
	entries := func(p *project) []string {
		t.Helper()
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{p.Name: "binary"})
		p.Dir = dir
		if err := writeLinuxIcon(p, dir); err != nil {
			t.Fatal(err)
		}
		beside, err := os.ReadFile(filepath.Join(dir, p.ID+".desktop"))
		if err != nil {
			t.Fatal(err)
		}
		files, err := linuxPackageFiles(p, dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if f.name == "usr/share/applications/"+p.ID+".desktop" {
				return []string{string(beside), string(f.data)}
			}
		}
		t.Fatal("the packages install no desktop entry")
		return nil
	}
	p := &project{Config: projectcfg.Config{Name: "App", ID: "dev.example.app", Version: "1.0.0"}}
	for _, entry := range entries(p) {
		if strings.Contains(entry, "Categories=") {
			t.Errorf("desktop entry without linux.categories:\n%s", entry)
		}
	}
	p.Linux.Categories = []string{"Office", "Dictionary"}
	validate, _ := exec.LookPath("desktop-file-validate")
	for _, entry := range entries(p) {
		if !strings.Contains(entry, "\nCategories=Office;Dictionary;\n") {
			t.Errorf("desktop entry:\n%s", entry)
		}
		if validate == "" {
			continue
		}
		path := filepath.Join(t.TempDir(), p.ID+".desktop")
		if err := os.WriteFile(path, []byte(entry), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.CommandContext(t.Context(), validate, path).CombinedOutput(); err != nil || len(out) > 0 {
			t.Errorf("desktop-file-validate: %v\n%s\n%s", err, out, entry)
		}
	}
}

func TestAndroidIconLayerColors(t *testing.T) {
	for value, want := range map[string]color.NRGBA{"#123456": {0x12, 0x34, 0x56, 0xff}, "#12345678": {0x12, 0x34, 0x56, 0x78}} {
		img, err := androidIconLayer(value)
		if err != nil {
			t.Fatal(err)
		}
		if got := img.(*image.NRGBA).NRGBAAt(0, 0); got != want {
			t.Errorf("%s = %v, want %v", value, got, want)
		}
	}
	for _, value := range []string{"#12345", "#1234567", "#12345g", "#"} {
		if _, err := androidIconLayer(value); err == nil {
			t.Errorf("%s is a color", value)
		}
	}
}

package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
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
		syso, err := iconResource(images, []int{16, 32}, arch)
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
	if _, err := iconResource(images, []int{16, 32}, "riscv64"); err == nil {
		t.Error("riscv64 made a resource")
	}
}

func TestDevBadge(t *testing.T) {
	dir := t.TempDir()
	assets := filepath.Join(dir, "assets")
	if err := os.MkdirAll(filepath.Join(assets, "AppIcon.icon", "Assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	var icon bytes.Buffer
	png.Encode(&icon, testIcon(1024))
	files := map[string][]byte{
		"nsgo.yaml":                     []byte("name: Test\n"),
		"assets/Icon.png":               icon.Bytes(),
		"assets/AppIcon.icon/icon.json": []byte(`{"groups":[{"name":"art","layers":[]}],"supported-platforms":{"squares":"shared"}}`),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.useDevIcons(); err != nil {
		t.Fatal(err)
	}

	// The .icon gains the badge's group on top and its layers' files.
	data, err := os.ReadFile(filepath.Join(p.Icon.Apple, "icon.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Groups []struct {
			Name   string `json:"name"`
			Layers []struct {
				Image string `json:"image-name"`
			} `json:"layers"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Groups) != 2 || doc.Groups[0].Name != "nsgo-dev" || doc.Groups[1].Name != "art" {
		t.Fatalf("badged groups: %+v", doc.Groups)
	}
	for _, layer := range doc.Groups[0].Layers {
		if _, err := os.Stat(filepath.Join(p.Icon.Apple, "Assets", layer.Image)); err != nil {
			t.Error(err)
		}
	}
	if filepath.Base(p.Icon.Apple) != "AppIcon.icon" {
		t.Errorf("the badged .icon is %s; actool names the icon after it", p.Icon.Apple)
	}

	// The PNG keeps its pixels outside the badge, tints them under the
	// plate and is white under the label.
	img, err := readIcon(p.Icon.PNG)
	if err != nil {
		t.Fatal(err)
	}
	rgb := func(c color.Color) [3]uint32 {
		r, g, b, _ := c.RGBA()
		return [3]uint32{r >> 8, g >> 8, b >> 8}
	}
	src := testIcon(1024)
	if got, want := rgb(img.At(100, 900)), rgb(src.At(100, 900)); got != want {
		t.Errorf("outside the badge: got %v, want %v", got, want)
	}
	if got := rgb(img.At(500, 202)); got == rgb(src.At(500, 202)) {
		t.Error("the plate left its pixel unchanged")
	}
	if got := rgb(img.At(616, 215)); got != [3]uint32{0xff, 0xff, 0xff} {
		t.Errorf("under the label: got %v, want white", got)
	}
}

func TestDevBadgeICNS(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "App.icns")
	if err := os.WriteFile(src, icns(testIcon(1024)), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := badgeApple(src, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(data[:4]) != "icns" || int(binary.BigEndian.Uint32(data[4:])) != len(data) {
		t.Fatalf("bad header % x", data[:8])
	}
}

func TestDevBadgeFullIcon(t *testing.T) {
	// Four visible groups are actool's limit, so the badge joins the top
	// one, placed through its position. A hidden group does not count.
	src := filepath.Join(t.TempDir(), "App.icon")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := `{"groups":[
		{"name":"a","layers":[{"name":"art"}],"position":{"scale":0.5,"translation-in-points":[10,20]}},
		{"name":"b"},{"name":"c","hidden":true},{"name":"d"},{"name":"e"}]}`
	if err := os.WriteFile(filepath.Join(src, "icon.json"), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "App.icon")
	if err := badgeIconComposer(src, out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "icon.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Groups []struct {
			Layers []struct {
				Name     string       `json:"name"`
				Position iconPosition `json:"position"`
			} `json:"layers"`
		} `json:"groups"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Groups) != 5 {
		t.Fatalf("got %d groups, want the icon's 5", len(got.Groups))
	}
	layers := got.Groups[0].Layers
	if len(layers) != 3 || layers[0].Name != "label" || layers[1].Name != "plate" || layers[2].Name != "art" {
		t.Fatalf("top group's layers: %+v", layers)
	}
	// 0.74 of the badge group's scale over the group's 0.5, and the
	// badge group's offset of 0.74 × (248, -419) less the group's (10, 20).
	want := iconPosition{Scale: 1.48, Translation: [2]float32{(0.74*248 - 10) / 0.5, (0.74*-419 - 20) / 0.5}}
	p := layers[1].Position
	near := func(a, b float32) bool { return a-b < 0.01 && b-a < 0.01 }
	if !near(p.Scale, want.Scale) || !near(p.Translation[0], want.Translation[0]) || !near(p.Translation[1], want.Translation[1]) {
		t.Errorf("plate position: got %+v, want %+v", p, want)
	}
}

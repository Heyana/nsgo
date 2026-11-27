package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestMSIXIndexesLocalizedNamesAndAllVisualScales(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"locales/en.yaml": "appName: Fixture App\nhello: Hello\n", "locales/nl.yaml": "hello: Hallo\n",
		"locales/zh-Hans.yaml": "appName: 测试应用 🧪\n", "Fixture.exe": "owned executable fixture"})
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "Fixture", ID: "dev.nsgo.msixfixture", Version: "1.0.0"}}
	p.I18n.Dir, p.I18n.Source = filepath.Join(dir, "locales"), "en"
	p.Windows.Store.Identity, p.Windows.Store.Publisher = "NSGoFixture", "CN=NSGo Fixture"
	file, err := buildMSIX(t.Context(), p, dir, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	read := map[string][]byte{}
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		read[f.Name], err = io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(read["resources.pri"]) == 0 || !bytes.HasPrefix(read["resources.pri"], []byte("mrm_pri2")) {
		t.Fatal("MSIX has no native resource index")
	}
	if !bytes.Contains(read["AppxBlockMap.xml"], []byte(`Name="resources.pri"`)) {
		t.Fatal("PRI is not integrity covered")
	}
	var manifest struct {
		Display   string `xml:"Properties>DisplayName"`
		Languages []struct {
			Language string `xml:"Language,attr"`
		} `xml:"Resources>Resource"`
	}
	if err := xml.Unmarshal(read["AppxManifest.xml"], &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Display != msixAppDisplayName || len(manifest.Languages) != 3 || manifest.Languages[0].Language != "en" || manifest.Languages[1].Language != "nl" || manifest.Languages[2].Language != "zh-hans" {
		t.Fatalf("localized manifest %+v", manifest)
	}
	for _, logo := range msixLogos {
		for _, scale := range msixLogoScales {
			name := strings.TrimSuffix(logo.name, ".png") + ".scale-" + strconv.Itoa(scale) + ".png"
			bitmap, err := png.Decode(bytes.NewReader(read["Assets/"+name]))
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			size := (logo.size*scale + 99) / 100
			if bitmap.Bounds().Dx() != size || bitmap.Bounds().Dy() != size {
				t.Errorf("%s dimensions=%v want %d", name, bitmap.Bounds(), size)
			}
		}
	}
	// Native verification reads exactly the built package, not a hand-made PRI.
	if out := os.Getenv("NSGO_MSIX_FIXTURE_OUT"); out != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMSIXResourceFallbackAndReservedPayload(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	p := &project{Config: projectcfg.Config{Name: "Plain app", ID: "dev.nsgo.plain"}}
	files, err := msixResourceFiles(p, img)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 19 {
		t.Fatalf("%d resources, want 3 base + 15 scaled + PRI", len(files))
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"Plain app.exe": "fixture", "resources.pri": "app-owned conflict"})
	p.Dir = dir
	if _, err := buildMSIX(t.Context(), p, dir, "amd64"); err == nil || !strings.Contains(err.Error(), "reserves") {
		t.Fatalf("colliding PRI was silently replaced: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "resources.pri"))
	if err != nil || string(data) != "app-owned conflict" {
		t.Fatal("build changed the app's conflicting resource")
	}
}

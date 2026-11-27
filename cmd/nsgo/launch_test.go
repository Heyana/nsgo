package main

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLaunchScreenConfig(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "nsgo.yaml")
	for _, tc := range []struct {
		yaml string
		err  string
	}{
		{"", ""},
		{`launch_screen: {background: "#14A7CD", image: art/logo.png, dark: {background: "#000000"}}`, ""},
		{`launch_screen: {background: "#14A7CD80"}`, "launch_screen.background"},
		{`launch_screen: {image: logo.png, dark: {background: "#FF00FF"}}`, "launch_screen.dark"},
		{`launch_screen: {background: "#FF00FF", dark: {background: xyz}}`, "launch_screen.dark.background"},
		{`launch_screen: {}`, "needs a background or image"},
	} {
		if err := os.WriteFile(config, []byte("name: App\n"+tc.yaml+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := load(dir)
		if tc.err != "" {
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Errorf("load %s = %v, want %s", tc.yaml, err, tc.err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if tc.yaml == "" {
			if p.LaunchScreen != nil {
				t.Fatal("an app without launch_screen configured a screen")
			}
			continue
		}
		s := p.LaunchScreen
		if s.Color() != 0xff14a7cd || s.Dark.Color() != 0xff000000 || s.Image != filepath.Join(dir, "art", "logo.png") || s.Dark.Image != "" {
			t.Errorf("launch screen = %+v", s)
		}
		var plist bytes.Buffer
		if err := iosPlist.Execute(&plist, p); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"UILaunchScreen", "UIColorName", "NSGoLaunchBackground", "UIImageName", "NSGoLaunchImage"} {
			if !strings.Contains(plist.String(), key) {
				t.Errorf("iOS Info.plist is missing %s", key)
			}
		}
		plist.Reset()
		if err := tvosPlist.Execute(&plist, p); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(plist.String(), "<key>UILaunchScreen</key><dict/>") || strings.Contains(plist.String(), "NSGoLaunch") {
			t.Error("the phone launch configuration reached tvOS")
		}
	}
}

func TestLaunchScreenLeavesAppleInfoKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: App\napple:\n  info:\n    UILaunchScreen: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	var plist bytes.Buffer
	if err := iosPlist.Execute(&plist, p); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(plist.String(), "<key>UILaunchScreen</key>"); n != 1 {
		t.Errorf("iOS Info.plist has %d UILaunchScreen keys, want apple.info's only", n)
	}
}

func TestLaunchImagePreservesAspectRatio(t *testing.T) {
	source := image.NewNRGBA(image.Rect(0, 0, 4, 2))
	red := color.NRGBA{R: 255, A: 255}
	draw.Draw(source, source.Bounds(), image.NewUniform(red), image.Point{}, draw.Src)
	got, err := png.Decode(bytes.NewReader(launchPNG(source, 12, 8)))
	if err != nil {
		t.Fatal(err)
	}
	for _, pt := range []image.Point{{2, 4}, {9, 7}} {
		if pixel := color.NRGBAModel.Convert(got.At(pt.X, pt.Y)); pixel != red {
			t.Errorf("pixel at %v = %v, want %v", pt, pixel, red)
		}
	}
	for _, pt := range []image.Point{{1, 4}, {2, 3}, {10, 7}, {9, 8}} {
		if _, _, _, a := got.At(pt.X, pt.Y).RGBA(); a != 0 {
			t.Errorf("pixel at %v is opaque", pt)
		}
	}
}

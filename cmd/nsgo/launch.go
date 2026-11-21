package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
)

const launchImageSize = 128 // points or dp

type launchScreen struct {
	launchAppearance `yaml:",inline"`
	Dark             launchAppearance `yaml:"dark"`
}

type launchAppearance struct {
	Background string `yaml:"background"`
	Image      string `yaml:"image"`
	color      uint32 // opaque ARGB, zero when unset
}

func (p *project) validateLaunchScreen() error {
	s := p.LaunchScreen
	if s == nil {
		return nil
	}
	if s.Background == "" && s.Image == "" {
		return fmt.Errorf("launch_screen needs a background or image")
	}
	if (s.Dark.Background != "" && s.Background == "") || (s.Dark.Image != "" && s.Image == "") {
		return fmt.Errorf("launch_screen.dark overrides a background or image set in launch_screen")
	}
	if _, ok := p.Apple.Info["UILaunchScreen"]; ok {
		return fmt.Errorf("launch_screen and apple.info.UILaunchScreen both set the iOS launch screen")
	}
	if err := p.resolveLaunchAppearance("launch_screen", &s.launchAppearance); err != nil {
		return err
	}
	return p.resolveLaunchAppearance("launch_screen.dark", &s.Dark)
}

func (p *project) resolveLaunchAppearance(key string, a *launchAppearance) error {
	if a.Background != "" {
		rgb, err := hex.DecodeString(strings.TrimPrefix(a.Background, "#"))
		if err != nil || !strings.HasPrefix(a.Background, "#") || len(rgb) != 3 {
			return fmt.Errorf("%s.background: %q is not a #RRGGBB color", key, a.Background)
		}
		a.color = 0xff000000 | uint32(rgb[0])<<16 | uint32(rgb[1])<<8 | uint32(rgb[2])
	}
	if a.Image != "" {
		a.Image = p.resolve(a.Image)
	}
	return nil
}

func (p *project) androidLaunchScreen() (*apk.LaunchScreen, error) {
	s := p.LaunchScreen
	if s == nil {
		return nil, nil
	}
	light, err := s.launchAppearance.android()
	if err != nil {
		return nil, err
	}
	dark, err := s.Dark.android()
	if err != nil {
		return nil, err
	}
	return &apk.LaunchScreen{Light: light, Dark: dark}, nil
}

func (a launchAppearance) android() (apk.LaunchAppearance, error) {
	v := apk.LaunchAppearance{Background: a.color}
	if a.Image == "" {
		return v, nil
	}
	img, err := readLaunchImage(a.Image)
	if err != nil {
		return v, err
	}
	// The 288dp splash icon has a circular safe area 192dp wide.
	v.Image = launchPNG(img, 288*4, launchImageSize*4)
	return v, nil
}

func readLaunchImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("launch screen image: %w", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("launch screen image %s: %w", rel(path), err)
	}
	return img, nil
}

// launchPNG fits the image inside a centered square while preserving its
// proportions. On Android, the extra transparent border sits under the mask.
func launchPNG(img image.Image, canvas, limit int) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	side := max(w, h)
	w, h = max(1, w*limit/side), max(1, h*limit/side)
	at := image.Pt((canvas-w)/2, (canvas-h)/2)
	dst := image.NewNRGBA(image.Rect(0, 0, canvas, canvas))
	draw.Draw(dst, image.Rectangle{Min: at, Max: at.Add(image.Pt(w, h))}, scaledTo(img, w, h), image.Point{}, draw.Src)
	var out bytes.Buffer
	png.Encode(&out, dst)
	return out.Bytes()
}

type launchAssetAppearance struct {
	Appearance string `json:"appearance"`
	Value      string `json:"value"`
}

type launchImageAsset struct {
	Idiom       string                  `json:"idiom"`
	Filename    string                  `json:"filename"`
	Scale       string                  `json:"scale"`
	Appearances []launchAssetAppearance `json:"appearances,omitempty"`
}

type launchColorAsset struct {
	Idiom       string                  `json:"idiom"`
	Color       launchAssetColor        `json:"color"`
	Appearances []launchAssetAppearance `json:"appearances,omitempty"`
}

type launchAssetColor struct {
	ColorSpace string            `json:"color-space"`
	Components map[string]string `json:"components"`
}

func (s *launchScreen) writeIOSCatalog(catalog string) error {
	var colors []launchColorAsset
	var images []launchImageAsset
	files := map[string][]byte{}
	for i, a := range []launchAppearance{s.launchAppearance, s.Dark} {
		label := "Light"
		var appearance []launchAssetAppearance
		if i == 1 {
			label = "Dark"
			appearance = []launchAssetAppearance{{Appearance: "luminosity", Value: "dark"}}
		}
		if a.Background != "" {
			color := launchAssetColor{ColorSpace: "srgb", Components: map[string]string{
				"red":   fmt.Sprintf("0x%02X", byte(a.color>>16)),
				"green": fmt.Sprintf("0x%02X", byte(a.color>>8)),
				"blue":  fmt.Sprintf("0x%02X", byte(a.color)),
				"alpha": "1.000",
			}}
			colors = append(colors, launchColorAsset{Idiom: "universal", Color: color, Appearances: appearance})
		}
		if a.Image == "" {
			continue
		}
		img, err := readLaunchImage(a.Image)
		if err != nil {
			return err
		}
		for scale := 1; scale <= 3; scale++ {
			ratio := fmt.Sprintf("%dx", scale)
			name := label + "@" + ratio + ".png"
			size := launchImageSize * scale
			files[name] = launchPNG(img, size, size)
			images = append(images, launchImageAsset{Idiom: "universal", Filename: name, Scale: ratio, Appearances: appearance})
		}
	}
	if len(colors) > 0 {
		if err := writeLaunchAsset(filepath.Join(catalog, "NSGoLaunchBackground.colorset"), "colors", colors, map[string][]byte{}); err != nil {
			return err
		}
	}
	if len(images) > 0 {
		if err := writeLaunchAsset(filepath.Join(catalog, "NSGoLaunchImage.imageset"), "images", images, files); err != nil {
			return err
		}
	}
	return nil
}

func writeLaunchAsset(dir, key string, entries any, files map[string][]byte) error {
	data, err := json.Marshal(map[string]any{key: entries, "info": map[string]any{"author": "nsgo", "version": 1}})
	if err != nil {
		return err
	}
	files["Contents.json"] = data
	return writeDir(dir, files)
}

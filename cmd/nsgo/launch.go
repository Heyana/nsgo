package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
)

const launchImageSize = 128 // points or dp

func (p *project) androidLaunchScreen() (*apk.LaunchScreen, error) {
	s := p.LaunchScreen
	if s == nil {
		return nil, nil
	}
	light, err := androidLaunchAppearance(s.LaunchAppearance)
	if err != nil {
		return nil, err
	}
	dark, err := androidLaunchAppearance(s.Dark)
	if err != nil {
		return nil, err
	}
	return &apk.LaunchScreen{Light: light, Dark: dark}, nil
}

func androidLaunchAppearance(a launchAppearance) (apk.LaunchAppearance, error) {
	v := apk.LaunchAppearance{Background: a.Color()}
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

// launchPNG fits the image inside a centered square while preserving its proportions. On Android, the extra transparent border sits under the mask.
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

func writeIOSLaunchCatalog(s *launchScreen, catalog string) error {
	var colors []launchColorAsset
	var images []launchImageAsset
	files := map[string][]byte{}
	for i, a := range []launchAppearance{s.LaunchAppearance, s.Dark} {
		label := "Light"
		var appearance []launchAssetAppearance
		if i == 1 {
			label = "Dark"
			appearance = []launchAssetAppearance{{Appearance: "luminosity", Value: "dark"}}
		}
		if a.Background != "" {
			c := a.Color()
			color := launchAssetColor{ColorSpace: "srgb", Components: map[string]string{
				"red":   fmt.Sprintf("0x%02X", byte(c>>16)),
				"green": fmt.Sprintf("0x%02X", byte(c>>8)),
				"blue":  fmt.Sprintf("0x%02X", byte(c)),
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

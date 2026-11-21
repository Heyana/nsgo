package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/pe"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"go/build"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
)

// assetsDir is the directory beside nsgo.yaml for project files that
// packages do not carry, such as the icons they are made from.
const assetsDir = "assets"

// resolveIcons makes nsgo.yaml's icon paths absolute, or finds the
// defaults in assets: AppIcon.icon, AppIcon.icns and Icon.png, and the
// Apple TV's and Apple Watch's own, AppIconTV.* and AppIconWatch.*; for
// dev.icon the same names ending in -dev. A path nsgo.yaml names must
// exist; a missing default only leaves the app without that icon.
func (p *project) resolveIcons() error {
	if err := p.resolveIconSet(&p.Icon, "icon", ""); err != nil {
		return err
	}
	return p.resolveIconSet(&p.Dev.Icon, "dev.icon", "-dev")
}

func (p *project) resolveIconSet(set *icons, key, suffix string) error {
	var err error
	if set.Apple, err = p.iconPath(key, set.Apple, "AppIcon"+suffix+".icon", "AppIcon"+suffix+".icns"); err != nil {
		return err
	}
	if ext := filepath.Ext(set.Apple); set.Apple != "" && ext != ".icon" && ext != ".icns" {
		return fmt.Errorf("%s.apple: %s is neither an .icon nor an .icns", key, rel(set.Apple))
	}
	if set.PNG, err = p.iconPath(key, set.PNG, "Icon"+suffix+".png"); err != nil {
		return err
	}
	if set.TVOS, err = p.iconPath(key, set.TVOS, "AppIconTV"+suffix+".brandassets", "AppIconTV"+suffix+".png"); err != nil {
		return err
	}
	switch ext := filepath.Ext(set.TVOS); ext {
	case "", ".png":
	case ".brandassets":
		if info, err := os.Stat(set.TVOS); err != nil || !info.IsDir() {
			return fmt.Errorf("%s.tvos: %s is not a .brandassets folder", key, rel(set.TVOS))
		}
	default:
		return fmt.Errorf("%s.tvos: %s is neither a .brandassets nor a .png", key, rel(set.TVOS))
	}
	if set.WatchOS, err = p.iconPath(key, set.WatchOS, "AppIconWatch"+suffix+".icon", "AppIconWatch"+suffix+".png"); err != nil {
		return err
	}
	if ext := filepath.Ext(set.WatchOS); set.WatchOS != "" && ext != ".icon" && ext != ".png" {
		return fmt.Errorf("%s.watchos: %s is neither an .icon nor a .png", key, rel(set.WatchOS))
	}
	if set.Android != nil {
		if set.Android.Foreground == "" || set.Android.Background == "" {
			return fmt.Errorf("%s.android needs foreground and background", key)
		}
		if strings.HasPrefix(set.Android.Background, "#") {
			if _, err := androidIconLayer(set.Android.Background); err != nil {
				return err
			}
		} else {
			resolved := p.resolve(set.Android.Background)
			if _, err := os.Stat(resolved); err != nil {
				return fmt.Errorf("%s.android.background: %w", key, err)
			}
			set.Android.Background = resolved
		}
		for name, path := range map[string]*string{"foreground": &set.Android.Foreground, "monochrome": &set.Android.Monochrome} {
			if *path == "" {
				continue
			}
			resolved := p.resolve(*path)
			if _, err := os.Stat(resolved); err != nil {
				return fmt.Errorf("%s.android.%s: %w", key, name, err)
			}
			*path = resolved
		}
	}
	return nil
}

func (p *project) iconPath(key, path string, defaults ...string) (string, error) {
	if path != "" {
		path = p.resolve(path)
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("%s: %w", key, err)
		}
		return path, nil
	}
	for _, name := range defaults {
		path := filepath.Join(p.Dir, assetsDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", nil
}

// useDevIcons points icon at the files dev.icon has, for nsgo dev's
// builds.
func (p *project) useDevIcons() {
	if p.Dev.Icon.Apple != "" {
		p.Icon.Apple = p.Dev.Icon.Apple
	}
	if p.Dev.Icon.PNG != "" {
		p.Icon.PNG = p.Dev.Icon.PNG
	}
	if p.Dev.Icon.TVOS != "" {
		p.Icon.TVOS = p.Dev.Icon.TVOS
	}
	if p.Dev.Icon.WatchOS != "" {
		p.Icon.WatchOS = p.Dev.Icon.WatchOS
	}
	if p.Dev.Icon.Android != nil {
		p.Icon.Android = p.Dev.Icon.Android
	}
}

// iconImage decodes the project's PNG icon, nil when it has none.
func (p *project) iconImage() (image.Image, error) {
	return readOptionalIcon(p.Icon.PNG)
}

// adaptiveLayerSize is an adaptive icon layer's side in pixels: 108 dp at xxxhdpi, where the table puts the layers.
const adaptiveLayerSize = 432

// androidAdaptiveIcon decodes icon.android's layers, nil when it has none, and returns with them the legacy icon they make, the foreground over the background within the 72 dp a launcher shows of them, for an app without icon.png.
func (p *project) androidAdaptiveIcon() (*apk.AdaptiveIcon, image.Image, error) {
	if p.Icon.Android == nil {
		return nil, nil, nil
	}
	a := p.Icon.Android
	if a.Foreground == "" || a.Background == "" {
		return nil, nil, errors.New("icon.android needs foreground and background")
	}
	foreground, err := readSquareIcon(a.Foreground)
	if err != nil {
		return nil, nil, err
	}
	background, err := androidIconLayer(a.Background)
	if err != nil {
		return nil, nil, err
	}
	fg, bg := scaled(foreground, adaptiveLayerSize), scaled(background, adaptiveLayerSize)
	adaptive := &apk.AdaptiveIcon{Foreground: scaledPNG(fg, adaptiveLayerSize), Background: scaledPNG(bg, adaptiveLayerSize)}
	if a.Monochrome != "" {
		mono, err := readSquareIcon(a.Monochrome)
		if err != nil {
			return nil, nil, err
		}
		adaptive.Monochrome = scaledPNG(mono, adaptiveLayerSize)
	}
	inset := adaptiveLayerSize / 6 // 18 of 108 dp
	legacy := image.NewNRGBA(image.Rect(0, 0, adaptiveLayerSize-2*inset, adaptiveLayerSize-2*inset))
	draw.Draw(legacy, legacy.Bounds(), bg, image.Pt(inset, inset), draw.Src)
	draw.Draw(legacy, legacy.Bounds(), fg, image.Pt(inset, inset), draw.Over)
	return adaptive, legacy, nil
}

// androidIconLayer decodes an adaptive icon layer: a square PNG, or a #RRGGBB or #RRGGBBAA color as a one-pixel image.
func androidIconLayer(value string) (image.Image, error) {
	if !strings.HasPrefix(value, "#") {
		return readSquareIcon(value)
	}
	rgba, err := hex.DecodeString(value[1:])
	if err != nil || len(rgba) != 3 && len(rgba) != 4 {
		return nil, fmt.Errorf("icon.android.background: %q is not a #RRGGBB or #RRGGBBAA color", value)
	}
	c := color.NRGBA{R: rgba[0], G: rgba[1], B: rgba[2], A: 255}
	if len(rgba) == 4 {
		c.A = rgba[3]
	}
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, c)
	return img, nil
}

// readOptionalIcon decodes path as a square PNG icon, nil when it is
// empty.
func readOptionalIcon(path string) (image.Image, error) {
	if path == "" {
		return nil, nil
	}
	return readSquareIcon(path)
}

// readSquareIcon decodes a PNG icon and checks that it is square.
func readSquareIcon(path string) (image.Image, error) {
	img, err := readIcon(path)
	if err != nil {
		return nil, err
	}
	if b := img.Bounds(); b.Dx() != b.Dy() {
		return nil, fmt.Errorf("icon %s is %dx%d; use a square image", rel(path), b.Dx(), b.Dy())
	}
	return img, nil
}

// readIcon decodes a PNG icon of any shape, such as the wide art an Apple
// TV's icon fills with.
func readIcon(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("icon %s: %w", rel(path), err)
	}
	return img, nil
}

// scaledPNG encodes img scaled to size×size pixels.
func scaledPNG(img image.Image, size int) []byte {
	var out bytes.Buffer
	png.Encode(&out, scaled(img, size))
	return out.Bytes()
}

// scaled is img at size×size pixels, averaging the source pixels each one
// covers.
func scaled(img image.Image, size int) *image.NRGBA {
	return scaledTo(img, size, size)
}

// scaledTo is img at w×h pixels, averaging the source pixels each one
// covers.
func scaledTo(img image.Image, w, h int) *image.NRGBA {
	b := img.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		y0 := b.Min.Y + y*b.Dy()/h
		y1 := max(b.Min.Y+(y+1)*b.Dy()/h, y0+1)
		for x := range w {
			x0 := b.Min.X + x*b.Dx()/w
			x1 := max(b.Min.X+(x+1)*b.Dx()/w, x0+1)
			// Average premultiplied colors, so transparent pixels add
			// no color to the edges.
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := img.At(sx, sy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			dst.Set(x, y, color.RGBA64{uint16(r / n), uint16(g / n), uint16(bl / n), uint16(a / n)})
		}
	}
	return dst
}

// writeMacIcon puts the app's icon into a macOS bundle's Resources and
// returns the Info.plist entries naming it. An Icon Composer .icon
// compiles with actool, an .icns is copied, and a PNG becomes an .icns.
func writeMacIcon(ctx context.Context, p *project, resources string) (string, error) {
	switch filepath.Ext(p.Icon.Apple) {
	case ".icon":
		return compileIcon(ctx, p.Icon.Apple, resources, "macosx", "11.0", nil)
	case ".icns":
		data, err := os.ReadFile(p.Icon.Apple)
		if err != nil {
			return "", err
		}
		return iconFilePlist, os.WriteFile(filepath.Join(resources, "AppIcon.icns"), data, 0o644)
	}
	img, err := p.iconImage()
	if img == nil || err != nil {
		return "", err
	}
	return iconFilePlist, os.WriteFile(filepath.Join(resources, "AppIcon.icns"), icns(img), 0o644)
}

const iconFilePlist = "\t<key>CFBundleIconFile</key><string>AppIcon</string>\n"

// icnsTypes are the PNG entries of an .icns by their pixel size; each
// size above 16 also serves the half size at 2x.
var icnsTypes = []struct {
	kind string
	size int
}{
	{"icp4", 16}, {"icp5", 32}, {"ic11", 32}, {"ic12", 64}, {"ic07", 128},
	{"ic08", 256}, {"ic13", 256}, {"ic09", 512}, {"ic14", 512}, {"ic10", 1024},
}

// icns writes img as an .icns of PNG entries, so no Apple tool is needed.
func icns(img image.Image) []byte {
	pngs := map[int][]byte{}
	var body bytes.Buffer
	for _, t := range icnsTypes {
		if pngs[t.size] == nil {
			pngs[t.size] = scaledPNG(img, t.size)
		}
		body.WriteString(t.kind)
		binary.Write(&body, binary.BigEndian, uint32(8+len(pngs[t.size])))
		body.Write(pngs[t.size])
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes()
}

// writeIOSIcon compiles the app's icon into an iOS or tvOS bundle and
// returns the Info.plist entries naming it: on iOS the Icon Composer
// .icon when there is one, else the PNG through an asset catalog; on tvOS
// Icon.TVOS, the project's brand assets or a wide image, else the PNG as
// the layered icon and top shelf images the Apple TV asks for.
func writeIOSIcon(ctx context.Context, p *project, sys *appleOS, app string, simulator bool) (string, error) {
	sdk := sys.sdkName(simulator)
	if sys == tvOS {
		return writeTVOSIcon(ctx, p, app, sdk)
	}
	if filepath.Ext(p.Icon.Apple) == ".icon" {
		return compileIcon(ctx, p.Icon.Apple, app, sdk, sys.minimum, sys.idioms)
	}
	img, err := p.iconImage()
	if img == nil || err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "nsgo-icon-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	catalog := filepath.Join(tmp, "Assets.xcassets")
	if err := writeDir(filepath.Join(catalog, "AppIcon.appiconset"), map[string][]byte{
		"Icon.png":      scaledPNG(img, 1024),
		"Contents.json": []byte(`{"images":[{"filename":"Icon.png","idiom":"universal","platform":"ios","size":"1024x1024"}],"info":{"author":"nsgo","version":1}}`),
	}); err != nil {
		return "", err
	}
	return compileIcon(ctx, catalog, app, sdk, sys.minimum, sys.idioms)
}

// writeTVOSIcon compiles the app's icon into an Apple TV bundle: the
// project's brand assets as they are, a wide image as the icon's front
// layer and top shelf art, or the PNG at three quarters of their height
// over its edges' color.
func writeTVOSIcon(ctx context.Context, p *project, app, sdk string) (string, error) {
	icon := p.Icon.TVOS
	if icon == "" {
		icon = p.Icon.PNG
	}
	switch filepath.Ext(icon) {
	case "":
		return "", nil
	case ".brandassets":
		return compileBrandAssets(ctx, icon, app, sdk, tvOS.minimum, tvOS.idioms)
	}
	img, err := readIcon(icon)
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "nsgo-icon-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	catalog := filepath.Join(tmp, "Assets.xcassets")
	// An image of the project's own is wide art that fills the card; the
	// PNG it falls back to keeps its square shape over its color.
	wide := p.Icon.TVOS != ""
	if err := writeTVBrandAssets(filepath.Join(catalog, "AppIcon.brandassets"), img, wide); err != nil {
		return "", err
	}
	return compileIcon(ctx, catalog, app, sdk, tvOS.minimum, tvOS.idioms)
}

// compileBrandAssets compiles Xcode's brand assets collection, the
// layered icon and top shelf images an Apple TV asks for, into out:
// actool takes one inside an asset catalog, so a temporary catalog names
// it AppIcon.
func compileBrandAssets(ctx context.Context, dir, out, platform, minimum string, idioms []string) (string, error) {
	tmp, err := os.MkdirTemp("", "nsgo-icon-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	catalog := filepath.Join(tmp, "Assets.xcassets")
	if err := copyDir(dir, filepath.Join(catalog, "AppIcon.brandassets")); err != nil {
		return "", err
	}
	return compileIcon(ctx, catalog, out, platform, minimum, idioms)
}

// copyDir copies src's files into dst, making it.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dst, strings.TrimPrefix(path, src))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// writeDir writes files into dir, making it.
func writeDir(dir string, files map[string][]byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// catalogInfo is the info every asset catalog's Contents.json ends with.
const catalogInfo = `"info":{"author":"nsgo","version":1}`

// writeTVBrandAssets writes an Apple TV's icons from an image: the app
// icon and its App Store form as two layers the focus tilts apart, the
// icon over its own edges' color, and the top shelf images, the icon
// centered on that color. A wide image, the project's own for tvOS,
// fills its frames instead of leaving the color around it.
func writeTVBrandAssets(dir string, icon image.Image, wide bool) error {
	back := edgeColor(icon)
	art := tvImage
	if wide {
		art = tvFit
	}
	stack := func(name, idiom string, w, h int, scales ...int) error {
		layer := func(layer string, draw func(w, h int) []byte) error {
			files := map[string][]byte{}
			var images []string
			for _, scale := range scales {
				file := fmt.Sprintf("%s@%dx.png", strings.ToLower(layer), scale)
				files[file] = draw(w*scale, h*scale)
				images = append(images, fmt.Sprintf(`{"filename":%q,"idiom":%q,"scale":"%dx"}`, file, idiom, scale))
			}
			files["Contents.json"] = []byte(`{"images":[` + strings.Join(images, ",") + `],` + catalogInfo + `}`)
			d := filepath.Join(dir, name+".imagestack", layer+".imagestacklayer")
			if err := writeDir(filepath.Join(d, "Content.imageset"), files); err != nil {
				return err
			}
			return writeDir(d, map[string][]byte{"Contents.json": []byte(`{` + catalogInfo + `}`)})
		}
		if err := layer("Front", func(w, h int) []byte { return art(icon, nil, w, h) }); err != nil {
			return err
		}
		if err := layer("Back", func(w, h int) []byte { return tvImage(nil, back, w, h) }); err != nil {
			return err
		}
		return writeDir(filepath.Join(dir, name+".imagestack"), map[string][]byte{
			"Contents.json": []byte(`{"layers":[{"filename":"Front.imagestacklayer"},{"filename":"Back.imagestacklayer"}],` + catalogInfo + `}`),
		})
	}
	if err := stack("App Icon", "tv", 400, 240, 1, 2); err != nil {
		return err
	}
	if err := stack("App Icon - App Store", "tv-marketing", 1280, 768, 1); err != nil {
		return err
	}
	shelf := func(name string, w, h int) error {
		return writeDir(filepath.Join(dir, name+".imageset"), map[string][]byte{
			"shelf@1x.png": art(icon, back, w, h),
			"shelf@2x.png": art(icon, back, w*2, h*2),
			"Contents.json": []byte(`{"images":[{"filename":"shelf@1x.png","idiom":"tv","scale":"1x"},` +
				`{"filename":"shelf@2x.png","idiom":"tv","scale":"2x"}],` + catalogInfo + `}`),
		})
	}
	if err := shelf("Top Shelf Image", 1920, 720); err != nil {
		return err
	}
	if err := shelf("Top Shelf Image Wide", 2320, 720); err != nil {
		return err
	}
	return writeDir(dir, map[string][]byte{"Contents.json": []byte(`{"assets":[` +
		`{"filename":"App Icon - App Store.imagestack","idiom":"tv","role":"primary-app-icon","size":"1280x768"},` +
		`{"filename":"App Icon.imagestack","idiom":"tv","role":"primary-app-icon","size":"400x240"},` +
		`{"filename":"Top Shelf Image Wide.imageset","idiom":"tv","role":"top-shelf-image-wide","size":"2320x720"},` +
		`{"filename":"Top Shelf Image.imageset","idiom":"tv","role":"top-shelf-image","size":"1920x720"}],` + catalogInfo + `}`)})
}

// tvImage draws a w×h PNG: back filling it, if any, and icon over it,
// if any, centered at three quarters of its height.
func tvImage(icon image.Image, back color.Color, w, h int) []byte {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if back != nil {
		draw.Draw(dst, dst.Bounds(), image.NewUniform(back), image.Point{}, draw.Src)
	}
	if icon != nil {
		size := h * 3 / 4
		at := image.Pt((w-size)/2, (h-size)/2)
		draw.Draw(dst, image.Rectangle{Min: at, Max: at.Add(image.Pt(size, size))}, scaled(icon, size), image.Point{}, draw.Over)
	}
	var out bytes.Buffer
	png.Encode(&out, dst)
	return out.Bytes()
}

// tvFit draws a w×h PNG: back filling it, if any, and icon over it, if
// any, scaled as large as it fits and centered, so wide art fills the
// frame.
func tvFit(icon image.Image, back color.Color, w, h int) []byte {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	if back != nil {
		draw.Draw(dst, dst.Bounds(), image.NewUniform(back), image.Point{}, draw.Src)
	}
	if icon != nil {
		b := icon.Bounds()
		iw, ih := w, h
		if b.Dx()*h > b.Dy()*w {
			// Wider than the frame: its width sets the size.
			ih = max(1, b.Dy()*w/b.Dx())
		} else {
			iw = max(1, b.Dx()*h/b.Dy())
		}
		at := image.Pt((w-iw)/2, (h-ih)/2)
		draw.Draw(dst, image.Rectangle{Min: at, Max: at.Add(image.Pt(iw, ih))}, scaledTo(icon, iw, ih), image.Point{}, draw.Over)
	}
	var out bytes.Buffer
	png.Encode(&out, dst)
	return out.Bytes()
}

// edgeColor is the opaque average of img's edge pixels, the color a
// full-bleed icon's background has.
func edgeColor(img image.Image) color.Color {
	b := img.Bounds()
	var r, g, bl, n uint64
	add := func(x, y int) {
		cr, cg, cb, ca := img.At(x, y).RGBA()
		if ca == 0 {
			return
		}
		// Unpremultiply, so the edge's antialiasing does not darken it.
		r, g, bl, n = r+uint64(cr)*0xffff/uint64(ca), g+uint64(cg)*0xffff/uint64(ca), bl+uint64(cb)*0xffff/uint64(ca), n+1
	}
	for x := b.Min.X; x < b.Max.X; x++ {
		add(x, b.Min.Y)
		add(x, b.Max.Y-1)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		add(b.Min.X, y)
		add(b.Max.X-1, y)
	}
	if n == 0 {
		return color.Black
	}
	return color.RGBA64{uint16(r / n), uint16(g / n), uint16(bl / n), 0xffff}
}

// compileIcon runs actool on an Icon Composer .icon or an asset catalog
// holding AppIcon, writing Assets.car and the platform's icon files into
// out, and returns the Info.plist entries actool says name them. actool
// takes seconds, so the user's cache keeps its output by actool's
// version, the arguments and the input's files, and builds after the
// first copy it.
func compileIcon(ctx context.Context, input, out, platform, minimum string, idioms []string) (string, error) {
	name := "AppIcon"
	if filepath.Ext(input) == ".icon" {
		name = strings.TrimSuffix(filepath.Base(input), ".icon")
	}
	args := []string{"--platform", platform, "--minimum-deployment-target", minimum, "--app-icon", name}
	for _, idiom := range idioms {
		args = append(args, "--target-device", idiom)
	}
	version, err := output(ctx, "xcrun", "actool", "--version")
	if err != nil {
		return "", fmt.Errorf("the icon needs Xcode's actool: %w", err)
	}
	sum := sha256.New()
	fmt.Fprintf(sum, "%s\n%q\n", version, args)
	err = filepath.WalkDir(input, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		fmt.Fprintf(sum, "%q %d\n", strings.TrimPrefix(path, input), len(data))
		sum.Write(data)
		return err
	})
	if err != nil {
		return "", err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "nsgo", "icons", hex.EncodeToString(sum.Sum(nil)[:16]))
	if _, err := os.Stat(dir); err != nil {
		if err := runActool(ctx, input, dir, args); err != nil {
			return "", err
		}
	}
	data, err := os.ReadFile(filepath.Join(dir, "Info.plist"))
	if err != nil {
		return "", err
	}
	files := filepath.Join(dir, "files")
	return plistEntries(string(data)), filepath.WalkDir(files, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(out, strings.TrimPrefix(path, files))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// runActool compiles input into dir: the icon files in files, and the
// partial Info.plist naming them.
func runActool(ctx context.Context, input, dir string, args []string) error {
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), "tmp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	partial := filepath.Join(tmp, "Info.plist")
	args = append([]string{"actool", input, "--compile", filepath.Join(tmp, "files"),
		"--output-partial-info-plist", partial}, args...)
	if err := os.MkdirAll(filepath.Join(tmp, "files"), 0o755); err != nil {
		return err
	}
	if err := run(ctx, "xcrun", args...); err != nil {
		return fmt.Errorf("the icon needs Xcode's actool: %w", err)
	}
	data, err := os.ReadFile(partial)
	if err != nil {
		return err
	}
	// actool reports problems in its output and still succeeds; an icon
	// it could not compile leaves the plist empty.
	if plistEntries(string(data)) == "" {
		return fmt.Errorf("actool compiled no icon from %s", rel(input))
	}
	// Another build may have filled dir meanwhile, with the same files.
	if err := os.Rename(tmp, dir); err != nil {
		if _, statErr := os.Stat(dir); statErr != nil {
			return err
		}
	}
	return nil
}

// plistEntries returns the keys and values of a plist's top dictionary.
func plistEntries(plist string) string {
	start := strings.Index(plist, "<dict>")
	end := strings.LastIndex(plist, "</dict>")
	if start < 0 || end < start {
		return ""
	}
	return strings.TrimSpace(plist[start+len("<dict>") : end])
}

// writeLinuxIcon puts the icon beside a Linux executable as <id>.png,
// with a desktop entry that names it.
func writeLinuxIcon(p *project, dir string) error {
	img, err := p.iconImage()
	if err != nil {
		return err
	}
	if img != nil {
		if err := os.WriteFile(filepath.Join(dir, p.ID+".png"), scaledPNG(img, 512), 0o644); err != nil {
			return err
		}
	}
	types, definitions, err := p.linuxFileTypes()
	if err != nil {
		return err
	}
	if len(definitions) > 0 {
		if err := os.WriteFile(filepath.Join(dir, p.ID+".mime.xml"), definitions, 0o644); err != nil {
			return err
		}
	}
	n, err := p.appNames()
	if err != nil {
		return err
	}
	entry := desktopData{Name: p.Name, Exec: p.Name, ID: p.ID, MIMETypes: types, URLSchemes: p.URLSchemes, Categories: p.linuxCategories()}
	if n != nil {
		entry.Name = n.names[n.source]
		for _, lang := range n.languages()[1:] {
			entry.Names = append(entry.Names, [2]string{desktopLocale(lang), n.names[lang]})
		}
	}
	f, err := os.Create(filepath.Join(dir, p.ID+".desktop"))
	if err != nil {
		return err
	}
	defer f.Close()
	return desktopEntry.Execute(f, entry)
}

// desktopData is what a desktop entry says: the app's name, in its other languages by locale too, its executable, and its ID, which names its icon and is the class of its windows that docks match them by, as the Linux backend gives them the ID as the program name. It registers the media types of the files the app opens, and its URL schemes as x-scheme-handler types, whose links the launcher passes as %U, with files as file URLs.
type desktopData struct {
	Name, Exec, ID string
	Names          [][2]string
	MIMETypes      []string
	URLSchemes     []string
	Categories     []string
}

// desktopString escapes a Desktop Entry string, before Exec's separate argument parsing. Keep field codes out of the executable's name.
func desktopString(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(s)
}

func desktopExec(s string) string {
	prefix := ""
	// GIO checks argv[0] exists before expanding %%. Keep a literal '%' in a command argument, so discovery and launch see the same path.
	if strings.Contains(s, "%") {
		prefix = "env -- "
	}
	s = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`, `%`, `%%`).Replace(s)
	return prefix + desktopString(`"`+s+`"`)
}

var desktopEntry = template.Must(template.New("desktop").Funcs(template.FuncMap{
	"s": desktopString, "exec": desktopExec,
}).Parse(`[Desktop Entry]
Type=Application
Name={{s .Name}}
{{- range .Names}}
Name[{{index . 0}}]={{s (index . 1)}}
{{- end}}
Exec={{exec .Exec}}{{if .URLSchemes}} %U{{else if .MIMETypes}} %F{{end}}
Icon={{.ID}}
Terminal=false
{{- if or .MIMETypes .URLSchemes}}
MimeType={{range .MIMETypes}}{{.}};{{end}}{{range .URLSchemes}}x-scheme-handler/{{.}};{{end}}
{{- end}}
StartupWMClass={{.ID}}
{{- if .Categories}}
Categories={{range .Categories}}{{.}};{{end}}
{{- end}}
`))

// writeWindowsResources writes the app's resources, its icon and its version, as an object into the app's package directory, so go build links it into the executable, and returns the object and the function that removes it again. Go only links .syso files from the package's own directory, and its linker takes resources from one object alone: when an object of the app's own holds resources, the executable takes those instead, and the object returned is nil.
func writeWindowsResources(ctx context.Context, p *project, goarch string) ([]byte, func(), error) {
	own, err := windowsAppResources(p.Dir, goarch)
	if err != nil || own != "" {
		if own != "" {
			warnIn(ctx, "%s holds the executable's resources, so they leave out the icon and version nsgo.yaml gives", own)
		}
		return nil, func() {}, err
	}
	img, err := p.iconImage()
	if err != nil {
		return nil, func() {}, err
	}
	var images [][]byte
	if img != nil {
		for _, size := range windowsIconSizes {
			images = append(images, scaledPNG(img, size))
		}
	}
	version, err := p.windowsVersion()
	if err != nil {
		return nil, func() {}, err
	}
	syso, err := windowsResource(goarch, images, windowsIconSizes, version)
	if err != nil {
		return nil, func() {}, err
	}
	path := filepath.Join(p.Dir, windowsResourceObject(goarch))
	if err := os.WriteFile(path, syso, 0o644); err != nil {
		return nil, func() {}, err
	}
	return syso, func() { os.Remove(path) }, nil
}

// windowsResourceObject is the name of the object writeWindowsResources writes.
func windowsResourceObject(goarch string) string { return "nsgo_icon_windows_" + goarch + ".syso" }

// windowsAppResources returns the name of an object of the app's own in dir that go build links for Windows on goarch and that holds resources, or "".
func windowsAppResources(dir, goarch string) (string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.syso"))
	if err != nil {
		return "", err
	}
	ctx := build.Default
	ctx.GOOS, ctx.GOARCH, ctx.CgoEnabled = "windows", goarch, false
	for _, file := range files {
		name := filepath.Base(file)
		if name == windowsResourceObject(goarch) {
			continue
		}
		if ok, err := ctx.MatchFile(dir, name); err != nil || !ok {
			continue
		}
		f, err := pe.Open(file)
		if err != nil {
			continue
		}
		rsrc := f.Section(".rsrc") != nil
		f.Close()
		if rsrc {
			return name, nil
		}
	}
	return "", nil
}

// windowsIconSizes are the sizes Explorer, the taskbar and title bars
// pick from at common scales.
var windowsIconSizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

//go:build darwin && !ios

package macos

import (
	"container/list"
	"context"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/imagedecode"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
)

const (
	menuIconPoints = 16
	maxMenuIcons   = 64
)

type menuIconEntry struct {
	key   string
	image objc.ID // retained NSImage, or 0 on decode failure
}

// menuIconCache keeps decoded menu NSImages on the main thread, bounded by maxMenuIcons, releasing evicted images so menu rebuilds never leak.
var menuIconCache = struct {
	order   *list.List
	entries map[string]*list.Element
}{
	order:   list.New(),
	entries: map[string]*list.Element{},
}

func menuIconGet(key string) (objc.ID, bool) {
	if el, ok := menuIconCache.entries[key]; ok {
		menuIconCache.order.MoveToFront(el)
		return el.Value.(*menuIconEntry).image, true
	}
	return 0, false
}

func menuIconPut(key string, img objc.ID) {
	if el, ok := menuIconCache.entries[key]; ok {
		ent := el.Value.(*menuIconEntry)
		if ent.image != 0 && ent.image != img {
			ent.image.Send(selRelease)
		}
		ent.image = img
		menuIconCache.order.MoveToFront(el)
		return
	}
	el := menuIconCache.order.PushFront(&menuIconEntry{key: key, image: img})
	menuIconCache.entries[key] = el
	for menuIconCache.order.Len() > maxMenuIcons {
		back := menuIconCache.order.Back()
		ent := back.Value.(*menuIconEntry)
		menuIconCache.order.Remove(back)
		delete(menuIconCache.entries, ent.key)
		if ent.image != 0 {
			ent.image.Send(selRelease)
		}
	}
}

func clearMenuIconCache() {
	for el := menuIconCache.order.Front(); el != nil; el = el.Next() {
		if img := el.Value.(*menuIconEntry).image; img != 0 {
			img.Send(selRelease)
		}
	}
	menuIconCache.order.Init()
	clear(menuIconCache.entries)
}

// applyMenuIcon attaches icon to an NSMenuItem when icon is non-empty and decodes cleanly.
func applyMenuIcon(it objc.ID, icon driver.MenuIcon) {
	if img := menuIconNSImage(icon); img != 0 {
		it.Send(sel("setImage:"), img)
		// AppKit 27’s automatic policy can hide an assigned image.
		if objc.Send[bool](it, sel("respondsToSelector:"), sel("setPreferredImageVisibility:")) {
			it.Send(sel("setPreferredImageVisibility:"), 1) // NSMenuItemImageVisibilityVisible
		}
	}
}

// menuIconNSImage returns a cached 16×16pt NSImage for icon, or 0 when empty or invalid. The cache retains ownership; callers attach it with setImage:, which takes its own retain.
func menuIconNSImage(icon driver.MenuIcon) objc.ID {
	if icon.Empty() {
		return 0
	}
	key := icon.Key()
	if img, ok := menuIconGet(key); ok {
		return img
	}
	img := buildMenuNSImage(icon)
	menuIconPut(key, img)
	return img
}

func buildMenuNSImage(icon driver.MenuIcon) objc.ID {
	nsSize := apple.CGSize{Width: menuIconPoints, Height: menuIconPoints}
	nsImg := objc.ID(apple.Class("NSImage")).Send(selAlloc).Send(sel("initWithSize:"), nsSize)
	if nsImg == 0 {
		return 0
	}
	added := false
	addRep := func(pix []byte, w, h int) {
		if w <= 0 || h <= 0 || len(pix) < w*h*4 {
			return
		}
		cg := apple.RGBAImage(pix, w, h)
		if cg == 0 {
			return
		}
		defer apple.Release(cg)
		rep := objc.ID(apple.Class("NSBitmapImageRep")).Send(selAlloc).Send(sel("initWithCGImage:"), objc.ID(cg))
		if rep == 0 {
			return
		}
		defer rep.Send(selRelease)
		rep.Send(sel("setSize:"), nsSize)
		nsImg.Send(sel("addRepresentation:"), rep)
		added = true
	}

	if icon.SVG != nil {
		if img1x, err := imagedecode.RasterizeSVG(icon.SVG, menuIconPoints, menuIconPoints, driver.RGB(0x000000), imagedecode.RGBA); err == nil && len(img1x.Frames) > 0 {
			addRep(img1x.Frames[0].Pix, img1x.Width, img1x.Height)
		}
		if img2x, err := imagedecode.RasterizeSVG(icon.SVG, menuIconPoints*2, menuIconPoints*2, driver.RGB(0x000000), imagedecode.RGBA); err == nil && len(img2x.Frames) > 0 {
			addRep(img2x.Frames[0].Pix, img2x.Width, img2x.Height)
		}
	} else if dec := decodeMacMenuIcon(icon); dec != nil && len(dec.Frames) > 0 && dec.Width > 0 && dec.Height > 0 {
		pix, w, h := dec.Frames[0].Pix, dec.Width, dec.Height
		if w != h || w > menuIconPoints*2 {
			target := menuIconPoints * 2
			if max(w, h) <= menuIconPoints {
				target = menuIconPoints
			}
			pix = imagedecode.FitPremultiplied(pix, w, h, target, target)
			w, h = target, target
		}
		addRep(pix, w, h)
	}

	if !added {
		nsImg.Send(selRelease)
		return 0
	}
	nsImg.Send(sel("setTemplate:"), icon.IsTemplate())
	return nsImg
}

func decodeMacMenuIcon(icon driver.MenuIcon) *imagedecode.Image {
	opts := imagedecode.Options{Layout: imagedecode.RGBA, Cover: menuIconPoints * 2}
	if len(icon.Data) > 0 {
		img, err := imagedecode.Decode(icon.Data, opts)
		if err != nil {
			return nil
		}
		return img
	}
	src := icon.Source
	if src.Empty() {
		return nil
	}
	if src.Pixels != nil && !src.Pixels.Rect.Empty() {
		return imagedecode.FromPixels(src.Pixels, imagedecode.RGBA)
	}
	if len(src.Data) > 0 {
		img, err := imagedecode.Decode(src.Data, opts)
		if err != nil {
			return nil
		}
		return img
	}
	if isRemoteURI(src.URI) {
		return nil
	}
	data, err := imageload.Load(context.Background(), src, nil)
	if err != nil || len(data) == 0 {
		return nil
	}
	img, err := imagedecode.Decode(data, opts)
	if err != nil {
		return nil
	}
	return img
}

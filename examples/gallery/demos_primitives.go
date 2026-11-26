//go:build !gallery_group || gallery_primitives

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("basics", 0, "Primitives", m.GroupPrimitives, []demo{
		{"Text", m.DemoText, m.DemoTextDetail, lucide.Type, textDemo},
		{"Images", m.DemoImages, m.DemoImagesDetail, lucide.Image, imagesDemo},
		{"Scroll", m.DemoScroll, m.DemoScrollDetail, lucide.ScrollText, scrollingDemo},
		{"Lists", m.DemoLists, m.DemoListsDetail, lucide.List, listsDemo},
		{"Canvas", m.DemoCanvas, m.DemoCanvasDetail, lucide.PenTool, canvasDemo},
		{"Shader", m.DemoShader, m.DemoShaderDetail, lucide.Sparkles, shaderDemo},
		{"WebView", m.DemoWebView, m.DemoWebViewDetail, lucide.Globe, webViewDemo},
		{"Video", m.DemoVideo, m.DemoVideoDetail, lucide.Film, videoDemo},
		{"Overlay", m.DemoOverlay, m.DemoOverlayDetail, lucide.Layers, cardsDemo(overlayCard)},
	})
}

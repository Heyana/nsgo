//go:build !gallery_group || gallery_apps

package main

import (
	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

func init() {
	type m = locales.Messages
	registerGroup("components", 0, "Apps", m.GroupApps, []demo{
		{"Stocks", m.DemoStocks, m.DemoStocksDetail, lucide.ChartCandlestick, stocksDemo},
		{"AI Chat", m.DemoAIChat, m.DemoAIChatDetail, lucide.MessageCircle, chatDemo},
		{"Photos", m.DemoPhotos, m.DemoPhotosDetail, lucide.Images, photosDemo},
	})
}
